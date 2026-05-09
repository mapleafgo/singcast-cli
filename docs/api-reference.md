# API Reference

cff-core exposes a dual-layer FFI interface: **Desktop** (c-shared C ABI) and **Mobile** (gomobile). Both layers wrap the same core runtime.

## Overview

| Capability | Desktop | Mobile |
|------------|---------|--------|
| Service lifecycle | CoreInit / Start / Stop / Destroy | Init / Start / Stop / Destroy |
| Config management | Check / ReloadTUN / Override | Check / ReloadTUN / Override |
| Proxy control | Select / Test / Mode | Select / Test / Mode |
| Real-time events | CoreCallback | EventHandler |
| Connection tracking | Query / Close | Query / Close |
| Traffic & resource stats | Query | Query |
| Platform IO (TUN, WiFi, DNS) | Limited | Full (TUN fd, SocketProtector, WiFi) |
| Memory management | SetMemoryLimit / Query | SetMemoryLimit / Query |

---

## Desktop FFI (C ABI)

Exported as C-compatible symbols via `c-shared` build mode. All functions return JSON strings unless noted otherwise.

### Lifecycle

| Function | Signature | Description |
|----------|-----------|-------------|
| `CoreInit` | `char* CoreInit(const char* optionsJSON)` | Initialize runtime. JSON: `{"home_dir":"/path","log_max_lines":500,"debug":false}` |
| `CoreStartWithContent` | `char* CoreStartWithContent(const char* content, const char* ruleSetProxy)` | Start with Clash YAML or sing-box JSON content |
| `CoreStop` | `char* CoreStop()` | Stop the service |
| `CoreDestroy` | `void CoreDestroy()` | Release all resources (terminal) |

### Config

| Function | Signature | Description |
|----------|-----------|-------------|
| `CoreCheckConfig` | `char* CoreCheckConfig(const char* content)` | Validate Clash YAML or sing-box JSON |
| `CoreReloadTUN` | `char* CoreReloadTUN()` | Restart TUN interface with same config |
| `CoreSetOverridePackages` | `char* CoreSetOverridePackages(const char* overrideJSON)` | Update VPN split-tunneling package list |

### Logging

| Function | Signature | Description |
|----------|-----------|-------------|
| `CoreSetLogLevel` | `void CoreSetLogLevel(int level)` | Set min log level (2=Error .. 6=Trace) |
| `CoreSetError` | `void CoreSetError(const char* message)` | Push error to event stream |
| `CoreWriteMessage` | `void CoreWriteMessage(int level, const char* message)` | Inject log into core stream |

### Pause / Wake / Network

| Function | Signature | Description |
|----------|-----------|-------------|
| `CorePause` | `void CorePause()` | Suspend network activity |
| `CoreWake` | `void CoreWake()` | Resume network activity |
| `CoreResetNetwork` | `void CoreResetNetwork()` | Reset connections + DNS cache + force outbound reconnect |

### Proxy Control

| Function | Signature | Description |
|----------|-----------|-------------|
| `CoreSelectProxy` | `char* CoreSelectProxy(const char* group, const char* tag)` | Select node in proxy group |
| `CoreTestDelay` | `char* CoreTestDelay(const char* name, int timeoutMs)` | URL test, returns `{"delay":ms}` or `{"delay":-1}` |
| `CoreSetMode` | `char* CoreSetMode(const char* mode)` | Set routing mode: `rule` / `global` / `direct` |
| `CoreSetGroupExpand` | `char* CoreSetGroupExpand(const char* group, int expand)` | Persist UI expand state |

### Queries

| Function | Signature | Description |
|----------|-----------|-------------|
| `CoreQueryProxies` | `char* CoreQueryProxies()` | Proxy groups and nodes |
| `CoreQueryTraffic` | `char* CoreQueryTraffic()` | Traffic stats + started_at |
| `CoreQueryLogs` | `char* CoreQueryLogs(int clear)` | Recent logs. `clear=1` to empty buffer after query |
| `CoreQueryConnections` | `char* CoreQueryConnections()` | Active connections |
| `CoreQueryTunOptions` | `char* CoreQueryTunOptions()` | TUN configuration |
| `CoreQueryMemoryStats` | `char* CoreQueryMemoryStats()` | Go runtime memory |

### Connection Management

| Function | Signature | Description |
|----------|-----------|-------------|
| `CoreCloseConnection` | `char* CoreCloseConnection(const char* id)` | Close connection by ID |
| `CoreCloseAllConnections` | `char* CoreCloseAllConnections()` | Close all connections |

### Platform

| Function | Signature | Description |
|----------|-----------|-------------|
| `CoreNeedFindProcess` | `int CoreNeedFindProcess()` | Whether config requires process finding |
| `CoreFlushSystemDNS` | `void CoreFlushSystemDNS()` | Flush system DNS cache |

### Memory

| Function | Signature | Description |
|----------|-----------|-------------|
| `CoreSetMemoryLimit` | `char* CoreSetMemoryLimit(int64_t bytes)` | Go runtime soft memory limit (0=disable) |

### Utilities

| Function | Signature | Description |
|----------|-----------|-------------|
| `CoreGetVersion` | `char* CoreGetVersion()` | Version info (JSON) |
| `CoreSetCallback` | `void CoreSetCallback(void* cb)` | Set event callback |
| `CoreSetLocale` | `void CoreSetLocale(const char* localeID)` | Locale for error messages |
| `CoreFreeString` | `void CoreFreeString(char* s)` | Free C string memory |

### Return Format

- Success: `{"ok":true}` or query-specific JSON
- Error: `{"error":"message"}`
- All `char*` returns must be freed with `CoreFreeString`

---

## Mobile SDK (gomobile)

Built with `gomobile bind`, generates AAR (Android) and xcframework (iOS). Methods are on the `Singcast` struct.

### Lifecycle

| Method | Description |
|--------|-------------|
| `New() *Singcast` | Create instance |
| `Init(optionsJSON string) error` | Initialize runtime |
| `StartWithContent(content, ruleSetProxy string) error` | Start with config |
| `Stop() error` | Stop service |
| `Destroy()` | Release resources (terminal) |

### Platform IO

| Method | Description |
|--------|-------------|
| `SetTunFd(fd int32)` | Set TUN fd from VpnService / NetworkExtension |
| `SetSocketProtector(p SocketProtector)` | Register socket protector for VPN bypass |
| `SetInterfacesJSON(json string)` | Provide network interface data |
| `UpdateDefaultInterface(name string, index int64, expensive bool)` | Report default interface |
| `SetIncludeAllNetworks(v bool)` | iOS includeAllNetworks |
| `SetWIFIState(ssid, bssid string)` | Report WiFi SSID/BSSID |
| `QueryTunOptions() string` | Get TUN config as JSON |

### Config

| Method | Description |
|--------|-------------|
| `CheckConfig(content string) error` | Validate config |
| `ReloadTUN() error` | Restart TUN interface with same config |
| `SetOverridePackages(overrideJSON string) error` | Update VPN split-tunneling |

### Logging

| Method | Description |
|--------|-------------|
| `SetLogLevel(level int32)` | Set min log level (2–6) |
| `SetError(message string)` | Push error to event stream |
| `WriteMessage(level int32, message string)` | Inject log message |

### Pause / Wake / Network

| Method | Description |
|--------|-------------|
| `Pause()` | Suspend network |
| `Wake()` | Resume network |
| `ResetNetwork()` | Reset connections + DNS |

### Proxy Control

| Method | Description |
|--------|-------------|
| `SelectProxy(group, tag string) error` | Select proxy node |
| `TestDelay(name string, timeoutMs int32) int32` | URL test delay (ms), -1 on error/timeout |
| `SetMode(mode string) error` | Set routing mode |
| `SetGroupExpand(group string, expand bool) error` | UI expand state |

### Queries

| Method | Description |
|--------|-------------|
| `QueryProxies() string` | Proxy groups (JSON) |
| `QueryTraffic() string` | Traffic stats with `started_at` (JSON) |
| `QueryLogs(clear bool) string` | Recent logs. `clear=true` empties buffer |
| `QueryConnections() string` | Active connections (JSON) |

### Connection Management

| Method | Description |
|--------|-------------|
| `CloseConnection(id string) error` | Close by ID |
| `CloseAllConnections() error` | Close all |

### Platform Queries

| Method | Description |
|--------|-------------|
| `NeedWIFIState() bool` | WiFi monitoring required |
| `NeedFindProcess() bool` | Process finding required |
| `UpdateWIFIState()` | Trigger WiFi state update |
| `QueryMemoryStats() string` | Go runtime memory (JSON) |
| `FlushSystemDNS()` | Flush DNS cache |

### Memory

| Method | Description |
|--------|-------------|
| `SetMemoryLimit(bytes int64)` | Go runtime soft memory limit |

### Events

| Method | Description |
|--------|-------------|
| `SetOnEvent(handler EventHandler)` | Register event handler |
| `Version() string` | Version info (JSON) |
| `SetLocale(localeID string)` | Locale (static) |

---

## Event Callback

Both platforms receive events via callback. The callback signature:

- **Desktop**: `void callback(int eventType, const char* jsonPayload)`
- **Mobile**: `void OnEvent(int eventType, String jsonPayload)`

### Event Types

| Type | Value | Trigger | Payload |
|------|-------|---------|---------|
| `EventTraffic` | 0 | Periodic (~1s) | [TrafficSnapshot](#trafficsnapshot) |
| `EventLogs` | 1 | New logs or buffer cleared | [`LogEntry[]`](#logentry) |
| `EventConnections` | 2 | Connection open/close/update | [ConnectionEvent](#connectionevent) |
| `EventProxyUpdate` | 3 | Group state changed | [`ProxyGroup[]`](#proxygroup) |
| `EventModeUpdate` | 4 | Routing mode changed | [ModeUpdate](#modeupdate) |
| `EventCoreLog` | 6 | Internal core log | `{"level":int,"message":string}` |
| `EventConnected` | 7 | Client connected | `{"connected":true}` |
| `EventDisconnected` | 8 | Client disconnected | `{"message":string}` |

---

## Data Structures

### TrafficSnapshot

Real-time traffic and resource statistics. Pushed via `EventTraffic` and queryable via `CoreQueryTraffic` / `QueryTraffic`.

```json
{
  "up": 1024,
  "down": 4096,
  "up_total": 1048576,
  "down_total": 4194304,
  "memory": 33554432,
  "goroutines": 42,
  "connections_in": 10,
  "connections_out": 15,
  "started_at": 1746000000
}
```

| Field | Type | Description |
|-------|------|-------------|
| `up` | int64 | Upload speed (bytes/s) |
| `down` | int64 | Download speed (bytes/s) |
| `up_total` | int64 | Cumulative uploaded bytes |
| `down_total` | int64 | Cumulative downloaded bytes |
| `memory` | int64 | Go runtime memory (bytes) |
| `goroutines` | int32 | Active goroutine count |
| `connections_in` | int32 | Inbound connections |
| `connections_out` | int32 | Outbound connections |
| `started_at` | int64 | Service start unix timestamp |

### LogEntry

```json
[
  { "level": 2, "message": "[TCP] 192.168.1.1:12345 -> example.com:443" }
]
```

| Field | Type | Description |
|-------|------|-------------|
| `level` | int32 | 0=panic, 1=fatal, 2=error, 3=warn, 4=info, 5=debug, 6=trace |
| `message` | string | Log content |

### ProxyGroup

```json
[
  {
    "tag": "PROXY",
    "type": "Selector",
    "selectable": true,
    "selected": "hk-node-01",
    "items": [
      { "tag": "hk-node-01", "type": "vless", "delay": 120 },
      { "tag": "us-node-01", "type": "vmess", "delay": 230 }
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
| `items` | array | Nodes in this group |
| `items[].tag` | string | Node tag |
| `items[].type` | string | Protocol type |
| `items[].delay` | int32 | Latest URL test delay (ms, 0=untested) |

### ConnectionEvent

```json
{
  "reset": false,
  "items": [
    {
      "event_type": 0,
      "id": "abc123",
      "destination": "example.com:443",
      "domain": "example.com",
      "network": "tcp",
      "outbound": "PROXY",
      "uplink": 1024,
      "downlink": 4096
    }
  ]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `reset` | bool | If true, replace all tracked connections |
| `items[].event_type` | int32 | 0=open, 1=close, 2=update |
| `items[].id` | string | Connection ID |
| `items[].inbound` | string | Inbound tag |
| `items[].network` | string | `tcp` or `udp` |
| `items[].source` | string | Source address |
| `items[].destination` | string | Destination address |
| `items[].domain` | string | Resolved domain |
| `items[].protocol` | string | Application protocol |
| `items[].outbound` | string | Outbound tag |
| `items[].chain` | string[] | Proxy chain |
| `items[].process_path` | string | Source process path (if enabled) |
| `items[].uplink` / `downlink` | int64 | Current bytes |
| `items[].uplink_total` / `downlink_total` | int64 | Cumulative bytes |

### ModeUpdate

Initial:

```json
{
  "modes": ["Rule", "Global", "Direct"],
  "current_mode": "Rule"
}
```

Changed:

```json
{
  "current_mode": "Global"
}
```

### TunOptionsSnapshot

```json
{
  "inet4_address": ["172.19.0.1/30"],
  "dns_server_address": "172.19.0.2",
  "mtu": 9000,
  "auto_route": true,
  "strict_route": false,
  "http_proxy_enabled": true,
  "http_proxy_server": "127.0.0.1",
  "http_proxy_server_port": 2080
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

### EventHandler

```go
type EventHandler interface {
    OnEvent(eventType int32, jsonPayload string)
}
```

Receives all runtime events. Register with `SetOnEvent` after `Init`.
