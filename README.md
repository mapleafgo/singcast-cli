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

Build tags: `with_clash_api,with_utls,with_quic,with_v2ray_api`

## FFI Interface

All functions return JSON strings. Exported as C-compatible symbols.

| Function | Description |
|----------|-------------|
| `CoreInit(homeDir)` | Initialize the core runtime |
| `CoreStart(configPath, ruleSetProxy)` | Start the proxy service |
| `CoreStop()` | Stop the service |
| `CoreClose()` | Shutdown and release resources |
| `CoreReloadConfig()` | Reload config from last path |
| `CoreCheckConfig(jsonContent)` | Validate a sing-box JSON config |
| `CoreTranslateConfig(yaml, ruleSetProxy)` | Translate YAML to sing-box JSON |
| `CoreQueryProxies()` | Query proxy groups and nodes |
| `CoreQueryTraffic()` | Query real-time traffic stats |
| `CoreQueryLogs()` | Query recent log entries |
| `CoreQueryConnections()` | Query active connections |
| `CoreSelectProxy(group, tag)` | Select a proxy in a group |
| `CoreTestDelay(name, url)` | Test proxy delay |
| `CoreSetMode(mode)` | Set routing mode (rule/global/direct) |
| `CoreCloseConnection(id)` | Close a connection by ID |
| `CoreCloseAllConnections()` | Close all active connections |
| `CoreSetCallback(cb)` | Set event callback |
| `CoreGetVersion()` | Get version info |

## License

MIT
