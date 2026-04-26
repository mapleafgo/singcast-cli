# singcast

**English** | [中文](README_zh.md)

A lightweight proxy core based on [sing-box](https://github.com/SagerNet/sing-box), with automatic Clash Meta (Mihomo) YAML configuration translation. Provides both CLI and FFI interfaces for integration with any application.

## Features

- **Config Translation** — Automatically converts Mihomo YAML to sing-box JSON
- **Multi-Protocol** — VLESS, VMess, Shadowsocks, Trojan, Hysteria2, TUIC, WireGuard, SOCKS5, HTTP, AnyTLS
- **Auto Routing** — GeoIP/GeoSite-based routing with country detection
- **Multi-Platform** — Linux, macOS, Windows, Android, iOS
- **FFI** — C-compatible shared library for mobile/desktop integration
- **Daemon Mode** — Background process with PID file and signal-based reload

## CLI

```bash
# Start proxy service
singcast run -c config.yaml

# Run as daemon
singcast run -c config.yaml -d

# With rule-set download proxy
singcast run -c config.yaml -p https://gh-proxy.org

# Translate YAML to sing-box JSON
singcast convert -c config.yaml -o output.json

# Validate config
singcast check -c config.yaml

# Show version
singcast version
```

### `run` Flags

| Flag | Alias | Description |
|------|-------|-------------|
| `--config` | `-c` | Config file path (YAML or JSON) |
| `--daemon` | `-d` | Run as daemon (fork to background) |
| `--api` | | Override external-controller address |
| `--home` | | Home directory (default `~/.singcast`) |
| `--rule-set-proxy` | `-p` | URL prefix for rule-set downloads |

Environment variable `SINGCAST_RULE_SET_PROXY` can also set the rule-set proxy.

### Signals

- `SIGHUP` — Reload config (Unix only)
- `SIGINT` / `SIGTERM` — Graceful shutdown

## Build

[Task](https://taskfile.dev) is required.

```bash
# CLI for current platform
task cli

# CLI for all platforms
task cli-all

# FFI shared library (desktop)
task ffi-darwin-arm64
task ffi-linux-amd64
task ffi-windows-amd64

# FFI shared library (mobile)
task ffi-android-arm64
task ffi-ios-arm64

# All targets
task all
```

Build tags: `with_clash_api,with_utls,with_quic,with_gvisor,with_v2ray_api`

## FFI Interface

All functions return JSON strings. Exported as C-compatible symbols.

| Function | Params | Description |
|----------|--------|-------------|
| `CoreInit` | `homeDir: string` | Initialize the core runtime |
| `CoreStart` | `configPath: string, ruleSetProxy: string` | Start with config file. `ruleSetProxy` is URL prefix for rule-set downloads (empty = direct) |
| `CoreStartWithContent` | `content: string, ruleSetProxy: string` | Start with content. Accepts Clash YAML or sing-box JSON |
| `CoreStop` | | Stop the service |
| `CoreClose` | | Shutdown and release resources |
| `CoreReloadConfig` | | Reload config from last used path |
| `CoreCheckConfig` | `content: string` | Validate Clash YAML or sing-box JSON |
| `CoreQueryProxies` | | Query proxy groups and nodes (JSON) |
| `CoreQueryTraffic` | | Query real-time traffic stats (JSON) |
| `CoreQueryLogs` | | Query recent log entries (JSON) |
| `CoreQueryConnections` | | Query active connections (JSON) |
| `CoreSelectProxy` | `group: string, tag: string` | Select a proxy node in a group |
| `CoreTestDelay` | `name: string` | Test proxy delay. Uses URL from group config |
| `CoreSetMode` | `mode: string` | Set routing mode: `rule` / `global` / `direct` |
| `CoreCloseConnection` | `id: string` | Close a connection by ID |
| `CoreCloseAllConnections` | | Close all active connections |
| `CoreSetCallback` | `cb: pointer` | Set event callback (C function pointer `void (*)(int eventType, const char* jsonPayload)`) |
| `CoreGetVersion` | | Get version info (JSON) |

### Event Callback

The callback receives `(eventType, jsonPayload)`. Event types:

| Type | Value | Trigger | Payload |
|------|-------|---------|---------|
| `EventTraffic` | `0` | Real-time traffic update | [TrafficSnapshot](#trafficsnapshot) |
| `EventLogs` | `1` | New log entries or buffer cleared | [`LogEntry[]`](#logentry) |
| `EventConnections` | `2` | Connection open/close | [ConnectionEvent](#connectionevent) |
| `EventProxyUpdate` | `3` | Proxy group state changed | [`ProxyGroup[]`](#proxygroup) |
| `EventModeUpdate` | `4` | Clash routing mode changed | [ModeUpdate](#modeupdate) |

#### TrafficSnapshot

```json
{
  "up": 1024,
  "down": 4096,
  "up_total": 1048576,
  "down_total": 4194304,
  "memory": 33554432,
  "goroutines": 42,
  "connections_in": 10,
  "connections_out": 15
}
```

| Field | Type | Description |
|-------|------|-------------|
| `up` | int64 | Upload speed (bytes/s) |
| `down` | int64 | Download speed (bytes/s) |
| `up_total` | int64 | Total uploaded bytes |
| `down_total` | int64 | Total downloaded bytes |
| `memory` | int64 | Memory usage (bytes) |
| `goroutines` | int32 | Number of goroutines |
| `connections_in` | int32 | Inbound connections |
| `connections_out` | int32 | Outbound connections |

#### LogEntry

```json
[
  { "level": 2, "message": "[TCP] 192.168.1.1:12345 -> example.com:443" }
]
```

| Field | Type | Description |
|-------|------|-------------|
| `level` | int32 | Log level (0=panic, 1=fatal, 2=error, 3=warn, 4=info, 5=debug, 6=trace) |
| `message` | string | Log message |

#### ConnectionEvent

```json
{
  "reset": false,
  "items": [
    { "event_type": 0, "id": "abc123" }
  ]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `reset` | bool | If `true`, replace all tracked connections with `items` |
| `items` | array | List of connection events |
| `items[].event_type` | int32 | `0` = new connection, `1` = closed |
| `items[].id` | string | Connection ID |

#### ProxyGroup

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
| `selectable` | bool | Whether the group supports manual selection |
| `selected` | string | Currently selected proxy tag |
| `items` | array | Proxy nodes in this group |
| `items[].tag` | string | Proxy node tag |
| `items[].type` | string | Protocol type (vless, vmess, trojan, etc.) |
| `items[].delay` | int32 | Latest URL test delay in ms (0 = not tested) |

#### ModeUpdate

Initial mode (from `InitializeClashMode`):

```json
{
  "modes": ["Rule", "Global", "Direct"],
  "current_mode": "Rule"
}
```

Mode changed (from `UpdateClashMode`):

```json
{
  "current_mode": "Global"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `modes` | string[] | Available modes (only present on initial callback) |
| `current_mode` | string | Current active mode |

## Mobile SDK

Built with `gomobile bind` — generates AAR (Android) and xcframework (iOS).

```bash
task mobile-android-arm64   # Android AAR
task mobile-ios-arm64       # iOS xcframework
task mobile-all             # All mobile targets
```

### API (gomobile)

| Method | Params | Description |
|--------|--------|-------------|
| `Init` | `homeDir: string` | Initialize the core runtime |
| `SetTunFd` | `fd: int32` | Set TUN fd from VpnService/NetworkExtension |
| `CheckConfig` | `content: string` | Validate Clash YAML or sing-box JSON |
| `StartWithContent` | `content: string, ruleSetProxy: string` | Start with content. Accepts Clash YAML or sing-box JSON |
| `Start` | `configPath: string, ruleSetProxy: string` | Start with config file |
| `Stop` | | Stop the service |
| `Close` | | Release all resources |
| `ReloadConfig` | | Reload config from last used path |
| `CloseConnection` | `id: string` | Close a connection by ID |
| `CloseAllConnections` | | Close all active connections |
| `SelectProxy` | `group: string, tag: string` | Select a proxy node in a group |
| `SetMode` | `mode: string` | Set routing mode: `rule` / `global` / `direct` |
| `QueryProxies` | | Query proxy groups (JSON) |
| `QueryTraffic` | | Query traffic stats (JSON) |
| `QueryLogs` | | Query recent logs (JSON) |
| `QueryConnections` | | Query active connections (JSON) |
| `TestDelay` | `name: string` | Test proxy delay. Uses URL from group config |
| `SetOnEvent` | `handler: EventHandler` | Set event handler. Implement the `EventHandler` interface: `void OnEvent(int eventType, String jsonPayload)` |
| `Version` | | Get version info (JSON) |

### Mobile TUN Integration

**Android (Kotlin):**
```kotlin
val singcast = Singcast()
singcast.init(homeDir)

// When user connects — start VpnService and pass the TUN fd
val fd = vpnService.Builder()
    .addAddress("172.18.0.1", 30)
    .establish().fileDescriptor
singcast.setTunFd(fd.toInt())
singcast.startWithContent(yamlContent, "")
```

**iOS (Swift):**
```swift
let singcast = Singcast()
singcast.init(homeDir)

// When user connects — extract fd from NEPacketTunnelProvider
let fd = tunnelFileDescriptor  // from NetworkExtension
singcast.setTunFd(fd)
singcast.startWithContent(yamlContent, "")
```

## License

MIT
