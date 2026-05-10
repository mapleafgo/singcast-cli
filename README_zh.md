# singcast

[English](README.md) | **中文**

基于 [sing-box](https://github.com/SagerNet/sing-box) 的轻量级代理核心，支持自动将 Clash Meta (Mihomo) YAML 配置翻译为 sing-box 格式。提供 CLI 和 FFI 接口，方便集成到任何应用。

## 功能

- **配置翻译** — 自动将 Mihomo YAML 转换为 sing-box JSON
- **多协议** — VLESS、VMess、Shadowsocks、Trojan、Hysteria2、TUIC、WireGuard、SOCKS5、HTTP、AnyTLS
- **自动路由** — 基于 GeoIP/GeoSite 的分流，自动检测所在国家
- **多平台** — Linux、macOS、Windows、Android、iOS
- **FFI** — C 兼容的共享库，便于移动端/桌面端集成
- **守护进程** — 后台运行，支持 PID 文件和信号重载

## 命令行

```bash
# 启动代理服务
singcast run -c config.yaml

# 后台运行
singcast run -c config.yaml -d

# 通过代理下载规则集
singcast run -c config.yaml -p https://gh-proxy.org

# 翻译 YAML 为 sing-box JSON
singcast convert -c config.yaml -o output.json

# 校验配置
singcast check -c config.yaml

# 查看版本
singcast version
```

### `run` 参数

| 参数 | 缩写 | 说明 |
|------|------|------|
| `--config` | `-c` | 配置文件路径（YAML 或 JSON） |
| `--daemon` | `-d` | 以守护进程方式运行 |
| `--api` | | 覆盖 external-controller 地址 |
| `--home` | | 主目录（默认 `~/.singcast`） |
| `--rule-set-proxy` | `-p` | 规则集下载代理 URL 前缀 |

也可通过环境变量 `SINGCAST_RULE_SET_PROXY` 设置规则集代理。

### 信号

- `SIGHUP` — 重载配置（仅 Unix）
- `SIGINT` / `SIGTERM` — 优雅关闭

## 构建

需要安装 [Task](https://taskfile.dev)。

```bash
# 构建当前平台 CLI
task cli

# 构建所有平台 CLI
task cli-all

# FFI 桌面端共享库
task ffi-darwin-arm64
task ffi-linux-amd64
task ffi-windows-amd64

# FFI 移动端共享库
task ffi-android-arm64
task ffi-ios-arm64

# 构建全部
task all
```

构建标签：`with_clash_api,with_utls,with_quic,with_gvisor`

## FFI 接口

提供桌面端 (c-shared) 和移动端 (gomobile) 双层 FFI 接口，方便集成到任何应用。

### 核心能力

- **服务生命周期** — 初始化、启动、停止、销毁，支持热重载
- **配置管理** — 校验、支持热重载
- **代理控制** — 节点选择、URL 延迟测试、路由模式切换
- **查询 API** — 代理组、流量、日志、连接、内存统计，返回 JSON
- **平台 IO** — TUN fd、Socket 保护、WiFi 状态（移动端）
- **资源监控** — 内存统计、协程数、OOM 保护

### 快速上手（桌面端）

```c
#include "cff_core.h"

CoreInit("{\"home_dir\":\"/tmp/singcast\"}");
CoreStartWithContent(yaml_content, "");

// 查询状态
char* proxies = CoreQueryProxies();
CoreFreeString(proxies);

CoreStop();
CoreDestroy();
```

### 快速上手（移动端）

**Android（Kotlin）：**
```kotlin
val singcast = Singcast()
singcast.init("""{"home_dir":"$homeDir"}""}")

val fd = vpnService.Builder()
    .addAddress("172.18.0.1", 30)
    .establish().fileDescriptor
singcast.setTunFd(fd.toInt())
singcast.startWithContent(yamlContent, "")
```

**iOS（Swift）：**
```swift
let singcast = Singcast()
singcast.init("{\"home_dir\":\"\(homeDir)\"}")

let fd = tunnelFileDescriptor  // 来自 NetworkExtension
singcast.setTunFd(fd)
singcast.startWithContent(yamlContent, "")
```

完整 API 参考：[docs/api-reference.md](docs/api-reference.md)

## 许可证

MIT
