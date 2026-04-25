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

构建标签：`with_clash_api,with_utls,with_quic,with_v2ray_api`

## FFI 接口

所有函数返回 JSON 字符串，以 C 兼容符号导出。

| 函数 | 说明 |
|------|------|
| `CoreInit(homeDir)` | 初始化核心运行时 |
| `CoreStart(configPath, ruleSetProxy)` | 启动代理服务 |
| `CoreStop()` | 停止服务 |
| `CoreClose()` | 关闭并释放资源 |
| `CoreReloadConfig()` | 从上次路径重载配置 |
| `CoreCheckConfig(jsonContent)` | 校验 sing-box JSON 配置 |
| `CoreTranslateConfig(yaml, ruleSetProxy)` | 将 YAML 翻译为 sing-box JSON |
| `CoreQueryProxies()` | 查询代理组和节点 |
| `CoreQueryTraffic()` | 查询实时流量统计 |
| `CoreQueryLogs()` | 查询最近日志 |
| `CoreQueryConnections()` | 查询活跃连接 |
| `CoreSelectProxy(group, tag)` | 选择代理组中的节点 |
| `CoreTestDelay(name, url)` | 测试代理延迟 |
| `CoreSetMode(mode)` | 设置路由模式（rule/global/direct） |
| `CoreCloseConnection(id)` | 按 ID 关闭连接 |
| `CoreCloseAllConnections()` | 关闭所有活跃连接 |
| `CoreSetCallback(cb)` | 设置事件回调 |
| `CoreGetVersion()` | 获取版本信息 |

## 许可证

MIT
