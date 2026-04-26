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
| `CoreSetCallback` | `cb: pointer` | Set event callback (C function pointer) |
| `CoreGetVersion` | | Get version info (JSON) |

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
| `SetOnEvent` | `fn: func(eventType int, jsonPayload string)` | Set event callback |
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
