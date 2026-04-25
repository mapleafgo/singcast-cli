# singcast CLI Design

## Overview

Add a standalone CLI binary (`singcast`) that can independently run as a proxy core from the terminal, similar to mihomo/clash. The CLI reads mihomo YAML configs (or sing-box JSON), translates them internally, and starts the sing-box proxy service with clash_api enabled.

The existing FFI shared library build (`ffi/`) remains unchanged.

## CLI Framework

Uses [urfave/cli/v3](https://github.com/urfave/cli) for subcommand routing, flag parsing, help generation, and shell completion.

## Commands

### `singcast run`

Start the proxy service.

```
singcast run -c config.yaml [-d] [--api 127.0.0.1:9090] [--home ~/.singcast]
```

Flags:
- `-c, --config` — config file path (required). Auto-detects YAML vs JSON format.
- `-d, --daemon` — daemon mode. Log to `{homeDir}/singcast.log`, PID written to `{homeDir}/singcast.pid`.
- `--api` — override `external-controller` address from config. Only takes effect when explicitly set.
- `--home` — home directory. Defaults to `~/.singcast/`.

Workflow:
1. Parse flags
2. Read config file
3. If YAML: call `translator.Translate()` to produce sing-box JSON
4. If JSON: pass through as-is
5. Write translated config to `{homeDir}/config.json`
6. `core.Init(homeDir)` → `core.Service.Setup(configPath)` → `core.Service.Start()`
7. Foreground: block on SIGINT/SIGTERM → `core.Service.Stop()`
8. Daemon: redirect stdout/stderr to log file, write PID file, exit parent

### `singcast convert`

Translate config without starting the service.

```
singcast convert -c config.yaml [-o output.json]
```

Flags:
- `-c, --config` — input config file path (required)
- `-o, --output` — output file path. Defaults to stdout.

Exit codes: 0 = success, 1 = failure.

### `singcast check`

Validate a config file (translate + basic sanity check).

```
singcast check -c config.yaml
```

Flags:
- `-c, --config` — config file path (required)

Outputs warnings and errors to stderr. Exit codes: 0 = valid, 1 = errors found.

### `singcast version`

Print version info.

```
singcast version
```

## Project Structure

```
cmd/singcast/
  main.go       — CLI entry point, urfave/cli app setup
  run.go        — run command action
  convert.go    — convert command action
  check.go      — check command action
```

All files in `package main`. No new packages needed.

### CLI App Setup (main.go)

```go
func main() {
    version := "..." // injected via -ldflags

    cmd := &cli.Command{
        Name:    "singcast",
        Version: version,
        Usage:   "A lightweight proxy core powered by sing-box",
        Commands: []*cli.Command{
            runCommand(),
            convertCommand(),
            checkCommand(),
            versionCommand(version),
        },
    }

    if err := cmd.Run(context.Background(), os.Args); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

Each command function (e.g. `runCommand()`) returns a `*cli.Command` with its flags, action, and help text. urfave/cli/v3 uses `*cli.Command` as the top-level app entry (no separate `cli.App` type).

## Translator Changes

### New file: `translator/experimental.go`

Translates mihomo `external-controller`, `secret`, `external-ui` into sing-box `experimental` section.

```
external-controller → experimental.clash_api.external_controller
secret              → experimental.clash_api.secret
external-ui         → experimental.clash_api.external_ui
(auto)              → experimental.cache_file.enabled: true (when clash_api is enabled)
```

### Modified: `translator/types_singbox.go`

Add `Experimental` field to the config output struct.

### Modified: `translator/assemble.go`

Call `translateExperimental()` during assembly.

## Build

Replace `Makefile` with `Taskfile.yml`. Task groups:

| Group | Tasks | Description |
|---|---|---|
| **CLI** | `cli`, `cli-linux-amd64/arm64`, `cli-darwin-amd64/arm64`, `cli-windows-amd64/arm64`, `cli-all` | Standalone CLI binary |
| **FFI** | `ffi-linux-amd64/arm64`, `ffi-darwin-amd64/arm64`, `ffi-windows-amd64/arm64`, `ffi-android-amd64/arm64`, `ffi-ios-arm64`, `ffi-all` | Shared library (.so/.dylib/.dll) |
| **Mobile** | `mobile-android-arm64/amd64`, `mobile-ios-arm64`, `mobile-all` | gomobile SDK packages |
| **Release** | `release` | CLI + FFI + mobile 全部构建 |
| **Dev** | `test`, `clean` | Development utilities |

Usage: `task cli-linux-amd64`, `task ffi-all`, `task release`

## Dependencies

One new third-party dependency:
- `github.com/urfave/cli/v3` — CLI framework (subcommand routing, flags, help, shell completion)

Standard library:
- `os/signal` — signal handling
- `os` — daemon mode (PID file, log redirect)

## Existing Code Impact

| Component | Change |
|---|---|
| `main.go` | No change (FFI shell) |
| `ffi/` | No change |
| `core/` | No change |
| `translator/` | Add experimental.go, minor changes to types_singbox.go + assemble.go |
| `Makefile` | Replaced by `Taskfile.yml` |

## homeDir Convention

Default `~/.singcast/`. Override via `--home` flag on `run` command. Contains:
- `config.json` — translated sing-box config
- `temp/` — sing-box temp directory
- `singcast.log` — daemon mode log
- `singcast.pid` — daemon mode PID file

## urfave/cli/v3 Notes

- v3 uses `cli.Command` as the root (no `cli.App`). `cmd.Run(ctx, os.Args)` is the entry point.
- Flags are defined per-command via `[]cli.Flag`.
- The `version` subcommand is a custom `cli.Command` (not the built-in `--version` flag), so it shows extended build info.
- Shell completion can be enabled later via `cmd.EnableShellCompletion = true` with no structural changes.
