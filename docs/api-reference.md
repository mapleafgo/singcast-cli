# API Reference

cff-core exposes a dual-layer FFI interface: **Desktop** (c-shared C ABI) and **Mobile** (gomobile). Both layers wrap the same core runtime.

## Overview

| Capability | Desktop | Mobile |
|------------|---------|--------|
| Service lifecycle | CoreInit / Start / Stop / Destroy | Init / Start / Stop / Destroy |
| State query | CoreQueryState | State |
| Config management | CheckConfig | CheckConfig |
| Proxy control | Select / Test / TestGroup / Mode | Select / Test / TestGroup / Mode |
| Query API | Query* functions | Query* methods |
| Connection tracking | Query / Close | Query / Close |
| Rules / DNS / Cache | QueryRules / QueryDNS / Flush* | QueryRules / QueryDNS / Flush* |
| Event callbacks | CoreSetEventCallback | SetOnEvent |
| Platform IO (TUN, WiFi, DNS) | — | Full (TUN fd, SocketProtector, WiFi) |
| Memory management | SetMemoryLimit / TriggerGC | SetMemoryLimit / TriggerGC |

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
| `CoreQueryState` | `int CoreQueryState()` | Service state: 0=Created, 1=Initialized, 2=Running, 3=Destroyed |

### Config

| Function | Signature | Description |
|----------|-----------|-------------|
| `CoreCheckConfig` | `char* CoreCheckConfig(const char* content)` | Validate Clash YAML or sing-box JSON |

### Logging

| Function | Signature | Description |
|----------|-----------|-------------|
| `CoreSetLogLevel` | `void CoreSetLogLevel(int level)` | Set min log level (2=Error .. 6=Trace) |

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
| `CoreQueryStats` | `char* CoreQueryStats()` | Traffic + memory snapshot: up/down/connections/memory |
| `CoreQueryConnections` | `char* CoreQueryConnections()` | Active connections |
| `CoreQueryMode` | `char* CoreQueryMode()` | Current mode and available modes |
| `CoreQueryRules` | `char* CoreQueryRules()` | Routing rule list |
| `CoreQueryDNS` | `char* CoreQueryDNS(const char* name, int qType)` | DNS query result |

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
| `State() int32` | Service state: 0=Created, 1=Initialized, 2=Running, 3=Destroyed |

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

### Logging

| Method | Description |
|--------|-------------|
| `SetLogLevel(level int32)` | Set min log level (2–6) |

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
| `QueryStats() string` | Traffic + memory snapshot: up/down/connections/memory |
| `QueryConnections() string` | Active connections (JSON) |
| `QueryMode() string` | Current mode and available modes |
| `QueryRules() string` | Routing rule list (JSON) |
| `QueryDNS(name string, qType uint16) string` | DNS query result (JSON) |

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

eventType values: 0=Log, 1=URLTest, 2=ModeUpdate, 3=ConnEvent. See the Desktop Event Callback section for payload details.

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

### DNS Query Result

```json
{
  "Status": 0,
  "Question": [{ "Name": "example.com.", "Qtype": 1, "Qclass": 1 }],
  "Answer": [
    { "name": "example.com.", "type": 1, "TTL": 300, "data": "93.184.216.34" }
  ],
  "Server": "internal"
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
