# API Reference

cff-core exposes a dual-layer FFI interface: **Desktop** (c-shared C ABI) and **Mobile** (gomobile). Both layers wrap the same core runtime.

## Overview

| Capability | Desktop | Mobile |
|------------|---------|--------|
| Service lifecycle | CoreInit / Start / Stop / Destroy | Init / Start / Stop / Destroy |
| State query | CoreQueryState | State |
| Config management | Check / ReloadTUN / Override | Check / ReloadTUN / Override |
| Proxy control | Select / Test / TestGroup / Mode | Select / Test / TestGroup / Mode |
| Query API | Query* functions | Query* methods |
| Connection tracking | Query / Close | Query / Close |
| Rules / DNS / Cache | QueryRules / QueryDNS / Flush* | QueryRules / QueryDNS / Flush* |
| Platform IO (TUN, WiFi, DNS) | Limited | Full (TUN fd, SocketProtector, WiFi) |
| Memory management | SetMemoryLimit / Query / TriggerGC | SetMemoryLimit / Query / TriggerGC |

---

## Desktop FFI (C ABI)

Exported as C-compatible symbols via `c-shared` build mode. All functions return JSON strings unless noted otherwise.

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
| `CoreReloadTUN` | `char* CoreReloadTUN()` | Restart TUN interface with same config |
| `CoreSetOverridePackages` | `char* CoreSetOverridePackages(const char* overrideJSON)` | Update VPN split-tunneling package list |

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
| `CoreQueryTraffic` | `char* CoreQueryTraffic()` | Traffic stats (JSON) |
| `CoreQueryLogs` | `char* CoreQueryLogs()` | Recent log entries from ring buffer |
| `CoreQueryConnections` | `char* CoreQueryConnections()` | Active connections |
| `CoreQueryMode` | `char* CoreQueryMode()` | Current mode and available modes |
| `CoreQueryMemoryStats` | `char* CoreQueryMemoryStats()` | Go runtime memory |
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

- Success: `{"ok":true}` or query-specific JSON
- Error: `{"error":"message"}`
- All `char*` returns must be freed with `CoreFreeString`

---

## Mobile SDK (gomobile)

Built with `gomobile bind`, generates AAR (Android) and xcframework (iOS). Methods are on the `Singcast` struct.

### Lifecycle

| Method | Description |
|--------|-------------|
| `Create() *Singcast` | Create instance |
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
| `ReloadTUN() error` | Restart TUN interface with same config |
| `SetOverridePackages(overrideJSON string) error` | Update VPN split-tunneling |

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
| `QueryTraffic() string` | Traffic stats (JSON) |
| `QueryLogs() string` | Recent log entries |
| `QueryConnections() string` | Active connections (JSON) |
| `QueryMode() string` | Current mode and available modes |
| `QueryMemoryStats() string` | Go runtime memory (JSON) |
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
    "items": [
      { "tag": "hk-node-01" },
      { "tag": "us-node-01" }
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

### Traffic

```json
{
  "up": 1048576,
  "down": 10485760,
  "connections": 42
}
```

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

### LogEntry

```json
[
  { "level": 4, "message": "service running" }
]
```

| Field | Type | Description |
|-------|------|-------------|
| `level` | int32 | 2=error, 3=warn, 4=info, 5=debug, 6=trace |
| `message` | string | Log content |
| `timestamp` | int64 | Unix timestamp (milliseconds) |

### MemoryStats

```json
{
  "sys": 33554432,
  "heap_alloc": 16777216,
  "heap_sys": 25165824,
  "stack_inuse": 1048576,
  "goroutines": 42,
  "limit": 0
}
```

| Field | Type | Description |
|-------|------|-------------|
| `sys` | int64 | Total OS memory (bytes) |
| `heap_alloc` | int64 | Allocated heap (bytes) |
| `heap_sys` | int64 | Heap from OS (bytes) |
| `stack_inuse` | int64 | Stack in use (bytes) |
| `goroutines` | int64 | Active goroutine count |
| `limit` | int64 | Soft memory limit (0=disabled) |

---

## Interfaces (Mobile Only)

### SocketProtector

```go
type SocketProtector interface {
    Protect(fd int32) bool
}
```

Called by the core to protect a socket fd from VPN routing. On Android, implement this to call `VpnService.protect(fd)`.
