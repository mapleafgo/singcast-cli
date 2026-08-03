# API Reference

Singcast exposes three integration interfaces: **Desktop** (c-shared C ABI), **Mobile** (gomobile), and **IPC** (JSON-RPC 2.0 over Unix socket / Windows named pipe). All layers wrap the same core runtime powered by [sing-box](https://github.com/SagerNet/sing-box), with built-in Clash Meta (Mihomo) YAML to sing-box JSON translation.

## Overview

| Capability | Desktop | Mobile | IPC |
|------------|---------|--------|-----|
| Service lifecycle | CoreInit / Start / Stop / Destroy | Init / Start / Stop / Destroy | core.startWithContent / core.stop |
| State query | CoreQueryState | State | core.queryState |
| Config management | CoreCheckConfig | CheckConfig | core.checkConfig |
| Subscription convert | — | Convert | core.convert |
| Proxy control | Select / Test / TestGroup / Mode | Select / Test / TestGroup / Mode | core.selectProxy / core.testDelay / core.testGroupDelay / core.setMode |
| Query API | Query* functions | Query* methods | core.query* |
| Connection tracking | Query / Close | Query / Close | core.queryConnections / core.closeConnection |
| Rules / Cache | QueryRules / Flush* | QueryRules / Flush* | core.queryRules / core.flush* |
| Event callbacks | CoreSetEventCallback | SetOnEvent | JSON-RPC notifications (push) |
| Platform IO (TUN, WiFi, DNS) | — | Full (TUN fd, SocketProtector, WiFi) | — |
| Memory management | SetMemoryLimit / TriggerGC | SetMemoryLimit / TriggerGC | core.triggerGC |
| Service management | — | — | service.install / service.uninstall (Windows) |

---

## Desktop FFI (C ABI)

Exported as C-compatible symbols via `c-shared` build mode. The desktop FFI directly wraps `core.Service`.

### Lifecycle

| Function | Signature | Description |
|----------|-----------|-------------|
| `CoreInit` | `char* CoreInit(const char* optionsJSON)` | Initialize runtime. JSON: `{"home_dir":"/path","debug":false}` |
| `CoreStartWithContent` | `char* CoreStartWithContent(const char* content, const char* ruleSetProxy)` | Start with Clash YAML or sing-box JSON content |
| `CoreStop` | `char* CoreStop()` | Stop the service |
| `CoreDestroy` | `void CoreDestroy()` | Release all resources (terminal) |
| `CoreQueryState` | `char* CoreQueryState()` | Service state string: `created`, `initialized`, `starting`, `running`, `destroyed` |

### Config

| Function | Signature | Description |
|----------|-----------|-------------|
| `CoreCheckConfig` | `char* CoreCheckConfig(const char* content)` | Validate Clash YAML or sing-box JSON |

### Logging

| Function | Signature | Description |
|----------|-----------|-------------|
| `CoreSetLogLevel` | `void CoreSetLogLevel(int level)` | Set min log level (2=Error, 3=Warn, 4=Info, 5=Debug, 6=Trace) |

### Network

| Function | Signature | Description |
|----------|-----------|-------------|
| `CoreResetNetwork` | `void CoreResetNetwork()` | Reset connections + DNS cache + force outbound reconnect |

### Proxy Control

| Function | Signature | Description |
|----------|-----------|-------------|
| `CoreSelectProxy` | `char* CoreSelectProxy(const char* group, const char* tag)` | Select node in proxy group |
| `CoreTestDelay` | `int CoreTestDelay(const char* name, int timeoutMs)` | URL test, returns delay in ms or -1 on error |
| `CoreTestGroupDelay` | `char* CoreTestGroupDelay(const char* group, int timeoutMs)` | Test all nodes in group, returns `{"tag":ms}` |
| `CoreSetMode` | `char* CoreSetMode(const char* mode)` | Set routing mode: `rule` / `global` / `direct` |
| `CoreSetGroupExpand` | `char* CoreSetGroupExpand(const char* group, int expand)` | Persist UI expand state |

### Queries

| Function | Signature | Description |
|----------|-----------|-------------|
| `CoreQueryProxies` | `char* CoreQueryProxies()` | Proxy groups and nodes |
| `CoreQueryStats` | `char* CoreQueryStats()` | Traffic + memory snapshot |
| `CoreQueryConnections` | `char* CoreQueryConnections()` | Active connections |
| `CoreQueryMode` | `char* CoreQueryMode()` | Current mode and available modes |
| `CoreQueryRules` | `char* CoreQueryRules()` | Routing rule list |

### Connection Management

| Function | Signature | Description |
|----------|-----------|-------------|
| `CoreCloseConnection` | `char* CoreCloseConnection(const char* id)` | Close connection by ID |
| `CoreCloseAllConnections` | `char* CoreCloseAllConnections()` | Close all connections |

### Cache / GC

| Function | Signature | Description |
|----------|-----------|-------------|
| `CoreFlushFakeIP` | `char* CoreFlushFakeIP()` | Clear FakeIP address cache |
| `CoreFlushDNSCache` | `char* CoreFlushDNSCache()` | Clear internal DNS query cache |
| `CoreFlushSystemDNS` | `void CoreFlushSystemDNS()` | Flush system DNS cache |
| `CoreTriggerGC` | `void CoreTriggerGC()` | Force garbage collection |

### Memory

| Function | Signature | Description |
|----------|-----------|-------------|
| `CoreSetMemoryLimit` | `void CoreSetMemoryLimit(int64_t bytes)` | Go runtime soft memory limit (0=disable) |

### Version

| Function | Signature | Description |
|----------|-----------|-------------|
| `CoreGetVersion` | `char* CoreGetVersion()` | Version info (JSON) |
| `CoreFreeString` | `void CoreFreeString(char* s)` | Free C string memory |

### Return Format

- Success: `null` (no need to free) or query-specific JSON string
- Error: error message string — must be freed with `CoreFreeString`
- All non-null `char*` returns must be freed with `CoreFreeString`

### Event Callback

A single unified callback delivers all events from the core. Invoked from background goroutines — consumers must dispatch to the UI thread if needed.

| Function | Signature |
|----------|-----------|
| `CoreSetEventCallback` | `void CoreSetEventCallback(void* cb)` |

Callback signature: `void (*)(int eventType, const char* json)`

#### Event Types

| eventType | Constant | json payload |
|-----------|----------|-------------|
| 0 | Log | `{"level":4,"message":"...","timestamp":123456}` |
| 1 | URLTest | `""` (empty) |
| 2 | ModeUpdate | Mode string, e.g. `"rule"` |
| 3 | ConnEvent | Connection JSON with `event` field (0=New, 1=Update, 2=Closed) |
| 4 | StateChange | State string, e.g. `"running"` |

Must be set before `CoreStartWithContent`. Persists across restarts.

#### Memory Ownership

The `const char* json` parameter uses **ownership transfer** — the native side allocates and does NOT free it. The callback recipient must copy the string, then call `CoreFreeString`.

This is required because Flutter's `NativeCallable.listener` is asynchronous (see [dart-lang/sdk#54554](https://github.com/dart-lang/sdk/issues/54554)) — it schedules execution on the Dart event loop and returns immediately, so the native side cannot safely free the string after the callback returns.

```dart
void onEvent(int eventType, Pointer<Utf8> json) {
  final str = json.toDartString(); // copy first
  CoreFreeString(json);            // then free the C string
  switch (eventType) {
    case 0: // Log — str is {"level":4,"message":"...","timestamp":...}
    case 1: // URLTest updated
    case 2: // ModeUpdate — str is "rule", "global", or "direct"
    case 3: // ConnEvent — str is connection JSON with "event" field
    case 4: // StateChange — str is "initialized", "running", etc.
  }
}
```

---

## Mobile SDK (gomobile)

Built with `gomobile bind` from the `mobile/` package, generates AAR (Android) and xcframework (iOS). `Create()` automatically enables mobile mode (`SetMobile(true)`).

### Lifecycle

| Method | Description |
|--------|-------------|
| `Create() *Singcast` | Create instance (mobile mode enabled) |
| `Init(optionsJSON string) error` | Initialize runtime |
| `StartWithContent(content, ruleSetProxy string) error` | Start with config |
| `Stop() error` | Stop service |
| `Destroy()` | Release resources (terminal) |
| `State() string` | Service state: `"created"`, `"initialized"`, `"starting"`, `"running"`, `"destroyed"` |

### Platform IO

| Method | Description |
|--------|-------------|
| `SetTunFd(fd int32)` | Set TUN fd from VpnService / NetworkExtension |
| `SetSocketProtector(p SocketProtector)` | Register socket protector for VPN bypass |
| `SetInterfacesJSON(json string)` | Provide network interface data |
| `UpdateDefaultInterface(name string, index int64, expensive bool)` | Report default interface |
| `SetIncludeAllNetworks(v bool)` | iOS includeAllNetworks |
| `SetWIFIState(ssid, bssid string)` | Report WiFi SSID/BSSID |

### Config

| Method | Description |
|--------|-------------|
| `CheckConfig(content string) error` | Validate config |
| `Convert(content string) (string, error)` | Convert Clash YAML / URI list / base64 subscription to sing-box JSON |

### Logging

| Method | Description |
|--------|-------------|
| `SetLogLevel(level int32)` | Set min log level (2=Error, 3=Warn, 4=Info, 5=Debug, 6=Trace) |

### Network

| Method | Description |
|--------|-------------|
| `ResetNetwork()` | Reset connections + DNS |

### Proxy Control

| Method | Description |
|--------|-------------|
| `SelectProxy(group, tag string) error` | Select proxy node |
| `TestDelay(name string, timeoutMs int32) int32` | URL test delay (ms), -1 on error/timeout |
| `TestGroupDelay(groupTag string, timeoutMs int32) string` | Test all nodes in group, JSON `{"tag":ms}` |
| `SetMode(mode string) error` | Set routing mode |
| `SetGroupExpand(group string, expand bool) error` | UI expand state |

### Queries

| Method | Description |
|--------|-------------|
| `QueryProxies() string` | Proxy groups (JSON) |
| `QueryStats() string` | Traffic + memory snapshot |
| `QueryConnections() string` | Active connections (JSON) |
| `QueryMode() string` | Current mode and available modes |
| `QueryRules() string` | Routing rule list (JSON) |

### Connection Management

| Method | Description |
|--------|-------------|
| `CloseConnection(id string) error` | Close by ID |
| `CloseAllConnections() error` | Close all |

### Cache / GC

| Method | Description |
|--------|-------------|
| `FlushFakeIP() error` | Clear FakeIP cache |
| `FlushDNSCache() error` | Clear internal DNS cache |
| `FlushSystemDNS()` | Flush system DNS cache |
| `TriggerGC()` | Force garbage collection |

### Memory

| Method | Description |
|--------|-------------|
| `SetMemoryLimit(bytes int64)` | Go runtime soft memory limit |

### Version

| Method | Description |
|--------|-------------|
| `Version() string` | Version info (JSON) |

### Event Listener

A single unified listener delivers all events. Called from background threads. Register before `StartWithContent`.

| Method | Interface |
|--------|-----------|
| `SetOnEvent(l)` | `EventListener.OnEvent(eventType int32, json string)` |

eventType values: 0=Log, 1=URLTest, 2=ModeUpdate, 3=ConnEvent, 4=StateChange. See the Desktop Event Callback section for payload details.

---

## IPC (JSON-RPC 2.0)

The IPC interface enables GUI applications to communicate with a long-running singcast service process over a persistent connection. The protocol is [JSON-RPC 2.0](https://www.jsonrpc.org/specification), transported over line-delimited JSON on a Unix domain socket (POSIX) or Windows named pipe.

### Architecture

```
┌─────────┐   JSON-RPC 2.0   ┌──────────────────┐
│   GUI    │ ◄──────────────► │  singcast ipc     │
│  (Flutter) │  Unix socket /   │  ┌────────────┐  │
│           │  Named pipe      │  │ core.Service│  │
└─────────┘                   │  └────────────┘  │
                              └──────────────────┘
```

The GUI connects as a client; the service is the server. Each connection is exclusively owned — a new connection replaces the previous one. The service sends JSON-RPC **notifications** (no `id` field) for events and traffic updates; the GUI sends **requests** (with `id` field) and receives **responses**.

### Transport

| Platform | Address |
|----------|---------|
| Linux / macOS | Unix socket: `<home_dir>/command.sock` (mode `0600`) |
| Windows | Named pipe: `\\.\pipe\singcast` (SDDL: SYSTEM + Admins + Interactive) |

All messages are UTF-8 JSON terminated by `\n`. Each JSON object must fit on a single line.

### Protocol

#### Request (GUI → Service)

```json
{"jsonrpc":"2.0","method":"core.queryState","id":1}
{"jsonrpc":"2.0","method":"core.startWithContent","params":{"content":"...","rule_set_proxy":""},"id":2}
```

#### Response (Service → GUI)

```json
{"jsonrpc":"2.0","result":"running","id":1}
{"jsonrpc":"2.0","error":{"code":-32601,"message":"method not found: foo"},"id":2}
```

#### Notification (Service → GUI, no `id`)

```json
{"jsonrpc":"2.0","method":"event.stateUpdate","params":{"state":"running"}}
{"jsonrpc":"2.0","method":"event.trafficUpdate","params":{"up":1024,"down":4096,"connections":5,"memory":33554432}}
```

### CLI

```bash
# Start IPC service (foreground)
singcast ipc --home /path/to/home

# On Windows, can also run as a service (see service.install)
```

On Windows, `singcast ipc` auto-detects whether it's running under the Windows Service Manager. If so, it registers as a Windows service; otherwise it runs in foreground.

### Core Methods (GUI → Service)

#### Lifecycle

| Method | Params | Result | Description |
|--------|--------|--------|-------------|
| `core.startWithContent` | `{"content": string, "rule_set_proxy?": string}` | `null` | Start with Clash YAML or sing-box JSON |
| `core.stop` | — | `null` | Stop the service |

#### Queries

| Method | Params | Result | Description |
|--------|--------|--------|-------------|
| `core.queryState` | — | `string` | State: `"created"`, `"initialized"`, `"starting"`, `"running"`, `"destroyed"` |
| `core.queryStats` | — | [Stats](#stats) | Traffic + memory snapshot |
| `core.queryProxies` | — | [ProxyGroup](#proxygroup) | Proxy groups and nodes |
| `core.queryConnections` | — | [Connection](#connection) | Active connections |
| `core.queryMode` | — | [Mode](#mode) | Current mode and available modes |
| `core.queryRules` | — | [Rule](#rule) | Routing rule list |

#### Proxy Control

| Method | Params | Result | Description |
|--------|--------|--------|-------------|
| `core.selectProxy` | `{"group_tag": string, "outbound_tag": string}` | `null` | Select node in proxy group |
| `core.testDelay` | `{"tag": string, "timeout_ms": int32}` | `{"delay": int32}` | URL test, delay in ms or -1 |
| `core.testGroupDelay` | `{"group_tag": string, "timeout_ms": int32}` | `{"tag": ms, ...}` | Test all nodes in group |
| `core.setMode` | `{"mode": string}` | `null` | Set routing mode: `"rule"` / `"global"` / `"direct"` |
| `core.setGroupExpand` | `{"group_tag": string, "expand": bool}` | `null` | Persist UI expand state |

#### Connection Management

| Method | Params | Result | Description |
|--------|--------|--------|-------------|
| `core.closeConnection` | `{"id": string}` | `null` | Close connection by ID |
| `core.closeAllConnections` | — | `null` | Close all connections |

#### Cache / GC

| Method | Params | Result | Description |
|--------|--------|--------|-------------|
| `core.flushFakeIP` | — | `null` | Clear FakeIP cache |
| `core.flushDNSCache` | — | `null` | Clear internal DNS cache |
| `core.flushSystemDNS` | — | `null` | Flush system DNS cache |
| `core.resetNetwork` | — | `null` | Reset connections + DNS + force reconnect |
| `core.triggerGC` | — | `null` | Force garbage collection |

#### Config & Logging

| Method | Params | Result | Description |
|--------|--------|--------|-------------|
| `core.checkConfig` | `{"content": string}` | `{"error": string}` | Validate config, empty error on success |
| `core.convert` | `{"content": string}` | `{"json": string}` | Convert Clash YAML / URI list / base64 subscription to sing-box JSON |
| `core.setLogLevel` | `{"level": int32}` | `null` | Set min log level (2=Error, 3=Warn, 4=Info, 5=Debug, 6=Trace) |
| `core.getVersion` | — | JSON string | Version info |

### Service Methods (Windows Only)

| Method | Params | Result | Description |
|--------|--------|--------|-------------|
| `service.install` | — | `null` | Register singcast as Windows service (auto-start) |
| `service.uninstall` | — | `null` | Remove the Windows service |

On non-Windows platforms, these methods return error: `"service management is only supported on Windows"`.

### Notifications (Service → GUI)

All notifications have no `id` field. The GUI must handle them asynchronously.

| Notification | Params | Description |
|-------------|--------|-------------|
| `event.log` | `{"level": int, "message": string, "timestamp": int}` | Core log message |
| `event.urlTest` | `null` | URL test completed, re-query proxies |
| `event.modeUpdate` | `{"mode": string}` | Routing mode changed |
| `event.connEvent` | Connection JSON with `"event"` field (0=New, 1=Update, 2=Closed) | Connection event |
| `event.stateUpdate` | `{"state": string}` | Service state changed |
| `event.trafficUpdate` | [Stats](#stats) | Traffic stats, pushed every second while running |

### Error Codes

| Code | Meaning |
|------|---------|
| `-32600` | Invalid request |
| `-32601` | Method not found |
| `-32602` | Invalid params |
| `1` | Application error (see `message` for details) |

### Connection Lifecycle

1. Service starts with `singcast ipc --home <dir>` and listens on the transport address
2. GUI connects to the socket/pipe
3. Service begins pushing `event.trafficUpdate` every second (while kernel is running)
4. GUI sends JSON-RPC requests, service responds
5. Service pushes notifications for state changes, logs, connection events, etc.
6. On disconnect:
   - If kernel is still running → service stays alive (TUN protection), waits for reconnection
   - If kernel is stopped → service shuts down

### Example Session

```
# GUI connects to command.sock

→ {"jsonrpc":"2.0","method":"core.queryState","id":1}
← {"jsonrpc":"2.0","result":"initialized","id":1}

→ {"jsonrpc":"2.0","method":"core.startWithContent","params":{"content":"proxies: ..."},"id":2}
← {"jsonrpc":"2.0","result":null,"id":2}

← {"jsonrpc":"2.0","method":"event.stateUpdate","params":{"state":"running"}}
← {"jsonrpc":"2.0","method":"event.trafficUpdate","params":{"up":0,"down":0,"connections":0,"memory":33554432}}
← {"jsonrpc":"2.0","method":"event.trafficUpdate","params":{"up":1024,"down":4096,"connections":3,"memory":33554432}}

→ {"jsonrpc":"2.0","method":"core.queryProxies","id":3}
← {"jsonrpc":"2.0","result":[{"tag":"PROXY","type":"Selector",...}],"id":3}

→ {"jsonrpc":"2.0","method":"core.stop","id":4}
← {"jsonrpc":"2.0","result":null,"id":4}
← {"jsonrpc":"2.0","method":"event.stateUpdate","params":{"state":"initialized"}}
```

---

## Mobile VPN Flow

### Initialization Sequence

```
Create()                          // mobile mode auto-enabled
Init({"home_dir":"..."})
SetTunFd(fd)                      // from VpnService.establish()
SetSocketProtector(protector)     // wrap VpnService.protect()
SetInterfacesJSON(json)
UpdateDefaultInterface(...)
StartWithContent(config)          // starts sing-box with TUN + socket protection
```

### Hot Reload (without Stop)

```
SetTunFd(newFd)                   // inject new TUN fd
StartWithContent(newConfig)       // old instance auto-closed, new one starts with new fd
```

### Socket Protection

On Android VPN, all traffic routes through the TUN interface. Without `SetSocketProtector`, proxy outbound sockets would also enter TUN, causing a routing loop. The protector calls `VpnService.protect(fd)` to bind proxy sockets to the physical network interface, bypassing TUN.

---

## Data Structures

### ProxyGroup

```json
[
  {
    "tag": "PROXY",
    "type": "Selector",
    "selectable": true,
    "selected": "hk-node-01",
    "expand": true,
    "items": [
      { "tag": "hk-node-01", "type": "VLESS", "delay": 120 },
      { "tag": "us-node-01", "type": "VMess", "delay": 350 },
      { "tag": "jp-node-01", "type": "Trojan" }
    ]
  }
]
```

| Field | Type | Description |
|-------|------|-------------|
| `tag` | string | Group name |
| `type` | string | Group type (Selector, URLTest, etc.) |
| `selectable` | bool | Supports manual node selection |
| `selected` | string | Currently selected node tag |
| `expand` | bool | UI expand state (absent if never set) |
| `items` | array | Nodes in this group |
| `items[].tag` | string | Node tag |
| `items[].type` | string | Protocol type (VLESS, VMess, Trojan, etc.) |
| `items[].delay` | int32 | Last URL test delay in ms (absent if never tested) |

### GroupDelay

```json
{
  "hk-node-01": 120,
  "us-node-01": -1
}
```

| Field | Type | Description |
|-------|------|-------------|
| key | string | Outbound tag |
| value | int32 | Delay in ms, -1 on error/timeout |

### Connection

```json
[
  {
    "event": 0,
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "network": "tcp",
    "source": "192.168.1.2:54321",
    "destination": "93.184.216.34:443",
    "domain": "example.com",
    "outbound": "PROXY",
    "rule": "domain(example.com) -> direct",
    "upload": 1024,
    "download": 4096,
    "start": "2026-05-10T12:00:00Z"
  }
]
```

| Field | Type | Description |
|-------|------|-------------|
| `event` | int32 | Connection event type (ConnEvent only): 0=New, 1=Update, 2=Closed |
| `id` | string | Connection UUID |
| `network` | string | Protocol: `tcp` or `udp` |
| `source` | string | Source address |
| `destination` | string | Destination address |
| `domain` | string | Domain name (absent if not resolved) |
| `outbound` | string | Outbound tag used |
| `rule` | string | Matched rule (absent if no rule) |
| `upload` | int64 | Uploaded bytes |
| `download` | int64 | Downloaded bytes |
| `start` | string | Start time (RFC3339) |

### Mode

```json
{
  "modes": ["Rule", "Global", "Direct"],
  "current_mode": "Rule"
}
```

### Stats

```json
{
  "up": 1048576,
  "down": 10485760,
  "connections": 42,
  "memory": 33554432
}
```

| Field | Type | Description |
|-------|------|-------------|
| `up` | int64 | Cumulative upload (bytes) |
| `down` | int64 | Cumulative download (bytes) |
| `connections` | int | Active connection count |
| `memory` | uint64 | Resident memory (bytes) |

### Rule

```json
{
  "rules": [
    { "type": "default", "payload": "...", "proxy": "direct" }
  ]
}
```

---

## Interfaces (Mobile Only)

### SocketProtector

```go
type SocketProtector interface {
    Protect(fd int32) bool
}
```

Called by the core to protect a socket fd from VPN routing. On Android, implement this to call `VpnService.protect(fd)`.

### EventListener

```go
type EventListener interface {
    OnEvent(eventType int32, json string)
}
```

Unified callback for all core events.
