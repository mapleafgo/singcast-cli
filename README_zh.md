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

构建标签：`with_clash_api,with_utls,with_quic,with_gvisor,with_v2ray_api`

## FFI 接口

所有函数返回 JSON 字符串，以 C 兼容符号导出。

| 函数 | 参数 | 说明 |
|------|------|------|
| `CoreInit` | `homeDir: string` | 初始化核心运行时 |
| `CoreStart` | `configPath: string, ruleSetProxy: string` | 使用配置文件启动。`ruleSetProxy` 为规则集下载代理 URL 前缀（空 = 直连） |
| `CoreStartWithContent` | `content: string, ruleSetProxy: string` | 使用内容启动。支持 Clash YAML 或 sing-box JSON |
| `CoreStop` | | 停止服务 |
| `CoreClose` | | 关闭并释放资源 |
| `CoreReloadConfig` | | 从上次使用的路径重载配置 |
| `CoreCheckConfig` | `content: string` | 校验 Clash YAML 或 sing-box JSON |
| `CoreQueryProxies` | | 查询代理组和节点（JSON） |
| `CoreQueryTraffic` | | 查询实时流量统计（JSON） |
| `CoreQueryLogs` | | 查询最近日志（JSON） |
| `CoreQueryConnections` | | 查询活跃连接（JSON） |
| `CoreSelectProxy` | `group: string, tag: string` | 选择代理组中的节点 |
| `CoreTestDelay` | `name: string` | 测试代理延迟。使用代理组配置中的 URL |
| `CoreSetMode` | `mode: string` | 设置路由模式：`rule` / `global` / `direct` |
| `CoreCloseConnection` | `id: string` | 按 ID 关闭连接 |
| `CoreCloseAllConnections` | | 关闭所有活跃连接 |
| `CoreSetCallback` | `cb: pointer` | 设置事件回调（C 函数指针 `void (*)(int eventType, const char* jsonPayload)`） |
| `CoreGetVersion` | | 获取版本信息（JSON） |

### 事件回调

回调函数接收 `(eventType, jsonPayload)` 两个参数。事件类型：

| 类型 | 值 | 触发时机 | 载荷格式 |
|------|----|----------|----------|
| `EventTraffic` | `0` | 实时流量更新 | [TrafficSnapshot](#trafficsnapshot) |
| `EventLogs` | `1` | 新日志或日志清空 | [`LogEntry[]`](#logentry) |
| `EventConnections` | `2` | 连接建立/关闭 | [ConnectionEvent](#connectionevent) |
| `EventProxyUpdate` | `3` | 代理组状态变化 | [`ProxyGroup[]`](#proxygroup) |
| `EventModeUpdate` | `4` | Clash 路由模式变化 | [ModeUpdate](#modeupdate) |

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

| 字段 | 类型 | 说明 |
|------|------|------|
| `up` | int64 | 上传速度（字节/秒） |
| `down` | int64 | 下载速度（字节/秒） |
| `up_total` | int64 | 累计上传字节 |
| `down_total` | int64 | 累计下载字节 |
| `memory` | int64 | 内存占用（字节） |
| `goroutines` | int32 | 协程数量 |
| `connections_in` | int32 | 入站连接数 |
| `connections_out` | int32 | 出站连接数 |

#### LogEntry

```json
[
  { "level": 2, "message": "[TCP] 192.168.1.1:12345 -> example.com:443" }
]
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `level` | int32 | 日志级别（0=panic, 1=fatal, 2=error, 3=warn, 4=info, 5=debug, 6=trace） |
| `message` | string | 日志内容 |

#### ConnectionEvent

```json
{
  "reset": false,
  "items": [
    { "event_type": 0, "id": "abc123" }
  ]
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `reset` | bool | 为 `true` 时表示用 `items` 替换所有已追踪连接 |
| `items` | array | 连接事件列表 |
| `items[].event_type` | int32 | `0` = 新建连接，`1` = 关闭连接 |
| `items[].id` | string | 连接 ID |

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

| 字段 | 类型 | 说明 |
|------|------|------|
| `tag` | string | 代理组名称 |
| `type` | string | 组类型（Selector、URLTest 等） |
| `selectable` | bool | 是否支持手动选择节点 |
| `selected` | string | 当前选中的代理节点 tag |
| `items` | array | 组内代理节点列表 |
| `items[].tag` | string | 节点 tag |
| `items[].type` | string | 协议类型（vless、vmess、trojan 等） |
| `items[].delay` | int32 | 最近一次 URL 测试延迟（毫秒，0 = 未测试） |

#### ModeUpdate

初始化模式（来自 `InitializeClashMode`）：

```json
{
  "modes": ["Rule", "Global", "Direct"],
  "current_mode": "Rule"
}
```

模式切换（来自 `UpdateClashMode`）：

```json
{
  "current_mode": "Global"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `modes` | string[] | 可用模式列表（仅在初始化回调中存在） |
| `current_mode` | string | 当前激活的模式 |

## 移动端 SDK

通过 `gomobile bind` 构建，生成 AAR（Android）和 xcframework（iOS）。

```bash
task mobile-android-arm64   # Android AAR
task mobile-ios-arm64       # iOS xcframework
task mobile-all             # 所有移动端目标
```

### API（gomobile）

| 方法 | 参数 | 说明 |
|------|------|------|
| `Init` | `homeDir: string` | 初始化核心运行时 |
| `SetTunFd` | `fd: int32` | 设置来自 VpnService/NetworkExtension 的 TUN fd |
| `CheckConfig` | `content: string` | 校验 Clash YAML 或 sing-box JSON |
| `StartWithContent` | `content: string, ruleSetProxy: string` | 使用内容启动。支持 Clash YAML 或 sing-box JSON |
| `Start` | `configPath: string, ruleSetProxy: string` | 使用配置文件启动 |
| `Stop` | | 停止服务 |
| `Close` | | 释放所有资源 |
| `ReloadConfig` | | 从上次使用的路径重载配置 |
| `CloseConnection` | `id: string` | 按 ID 关闭连接 |
| `CloseAllConnections` | | 关闭所有活跃连接 |
| `SelectProxy` | `group: string, tag: string` | 选择代理组中的节点 |
| `SetMode` | `mode: string` | 设置路由模式：`rule` / `global` / `direct` |
| `QueryProxies` | | 查询代理组（JSON） |
| `QueryTraffic` | | 查询流量统计（JSON） |
| `QueryLogs` | | 查询最近日志（JSON） |
| `QueryConnections` | | 查询活跃连接（JSON） |
| `TestDelay` | `name: string` | 测试代理延迟。使用代理组配置中的 URL |
| `SetOnEvent` | `handler: EventHandler` | 设置事件处理器。需实现 `EventHandler` 接口：`void OnEvent(int eventType, String jsonPayload)` |
| `Version` | | 获取版本信息（JSON） |

### 移动端 TUN 集成

**Android（Kotlin）：**
```kotlin
val singcast = Singcast()
singcast.init(homeDir)

// 用户点击连接时 — 启动 VpnService 并传入 TUN fd
val fd = vpnService.Builder()
    .addAddress("172.18.0.1", 30)
    .establish().fileDescriptor
singcast.setTunFd(fd.toInt())
singcast.startWithContent(yamlContent, "")
```

**iOS（Swift）：**
```swift
let singcast = Singcast()
singcast.init(homeDir)

// 用户点击连接时 — 从 NEPacketTunnelProvider 提取 fd
let fd = tunnelFileDescriptor  // 来自 NetworkExtension
singcast.setTunFd(fd)
singcast.startWithContent(yamlContent, "")
```

## 许可证

MIT
