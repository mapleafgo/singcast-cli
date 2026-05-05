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

### 生命周期

| 函数 | 参数 | 说明 |
|------|------|------|
| `CoreInit` | `optionsJSON: string` | 初始化核心运行时。JSON：`{"home_dir":"/path","log_max_lines":500,"debug":false}` |
| `CoreStartWithContent` | `content: string, ruleSetProxy: string` | 使用内容启动。支持 Clash YAML 或 sing-box JSON |
| `CoreStop` | | 停止服务 |
| `CoreDestroy` | | 关闭并释放资源 |

### 配置

| 函数 | 参数 | 说明 |
|------|------|------|
| `CoreCheckConfig` | `content: string` | 校验 Clash YAML 或 sing-box JSON |
| `CoreReloadConfig` | `content: string, ruleSetProxy: string` | 使用新配置内容重载 |
| `CoreReloadTUN` | | 仅重载 TUN 接口 |
| `CoreSetOverridePackages` | `overrideJSON: string` | 设置 VPN 分流应用包名 |

### 日志

| 函数 | 参数 | 说明 |
|------|------|------|
| `CoreSetLogLevel` | `level: int` | 设置最低日志级别（2=Error … 6=Trace） |
| `CoreSetError` | `message: string` | 向客户端推送错误消息 |
| `CoreWriteMessage` | `level: int, message: string` | 写入自定义日志消息 |

### 暂停 / 唤醒 / 网络

| 函数 | 参数 | 说明 |
|------|------|------|
| `CorePause` | | 暂停网络活动 |
| `CoreWake` | | 恢复网络活动 |
| `CoreResetNetwork` | | 重置所有连接和 DNS 缓存 |

### 代理控制

| 函数 | 参数 | 说明 |
|------|------|------|
| `CoreSelectProxy` | `group: string, tag: string` | 选择代理组中的节点 |
| `CoreTestDelay` | `name: string` | 测试代理延迟。使用代理组配置中的 URL |
| `CoreSetMode` | `mode: string` | 设置路由模式：`rule` / `global` / `direct` |
| `CoreSetGroupExpand` | `group: string, expand: int` | 设置代理组展开状态（0=收起, 1=展开） |

### 查询

| 函数 | 参数 | 说明 |
|------|------|------|
| `CoreQueryProxies` | | 查询代理组和节点（JSON） |
| `CoreQueryTraffic` | | 查询实时流量统计（JSON） |
| `CoreQueryLogs` | `clear: int` | 查询最近日志（JSON）。传入 `1` 可在查询后清空缓冲区 |
| `CoreQueryConnections` | | 查询活跃连接（JSON） |
| `CoreQueryTunOptions` | | 查询 TUN 配置（JSON） |
| `CoreQueryMemoryStats` | | 查询 Go 运行时内存统计（JSON） |

### 连接管理

| 函数 | 参数 | 说明 |
|------|------|------|
| `CoreCloseConnection` | `id: string` | 按 ID 关闭连接 |
| `CoreCloseAllConnections` | | 关闭所有活跃连接 |

### 平台

| 函数 | 参数 | 说明 |
|------|------|------|
| `CoreNeedFindProcess` | → `int` | 是否需要进程查找 |
| `CoreWriteMessage` | `level: int, message: string` | 写入自定义日志消息 |
| `CoreFlushSystemDNS` | | 刷新系统 DNS 缓存 |

### 内存

| 函数 | 参数 | 说明 |
|------|------|------|
| `CoreSetMemoryLimit` | `bytes: int64` | 设置 Go 运行时软内存限制（0=禁用） |

### 工具

| 函数 | 参数 | 说明 |
|------|------|------|
| `CoreGetVersion` | | 获取版本信息（JSON） |
| `CoreSetCallback` | `cb: pointer` | 设置事件回调（`void (*)(int eventType, const char* jsonPayload)`） |
| `CoreSetLocale` | `localeID: string` | 设置错误消息语言 |

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
  "connections_out": 15,
  "started_at": 1746000000
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
| `started_at` | int64 | 服务启动时间（unix 时间戳） |

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

#### 生命周期

| 方法 | 参数 | 说明 |
|------|------|------|
| `Init` | `optionsJSON: string` | 初始化核心运行时。JSON：`{"home_dir":"/path","log_max_lines":500}` |
| `StartWithContent` | `content: string, ruleSetProxy: string` | 使用内容启动。支持 Clash YAML 或 sing-box JSON |
| `Stop` | | 停止服务 |
| `Destroy` | | 释放所有资源。实例不可复用 |

#### 平台 IO

| 方法 | 参数 | 说明 |
|------|------|------|
| `SetTunFd` | `fd: int32` | 设置来自 VpnService/NetworkExtension 的 TUN fd |
| `SetSocketProtector` | `p: SocketProtector` | 设置 socket 保护器用于 VPN 绕过（`bool Protect(int fd)`） |
| `SetInterfacesJSON` | `json: string` | 提供来自平台的网络接口数据 |
| `UpdateDefaultInterface` | `name: string, index: int64, expensive: bool` | 上报当前默认网络接口 |
| `SetIncludeAllNetworks` | `v: bool` | 设置 iOS includeAllNetworks |
| `SetWIFIState` | `ssid: string, bssid: string` | 上报当前 WiFi 状态 |
| `QueryTunOptions` | | 获取 TUN 配置（JSON） |

#### 配置

| 方法 | 参数 | 说明 |
|------|------|------|
| `CheckConfig` | `content: string` | 校验 Clash YAML 或 sing-box JSON |
| `ReloadConfig` | `content: string, ruleSetProxy: string` | 使用新配置内容重载 |
| `ReloadTUN` | | 仅重载 TUN 接口 |
| `SetOverridePackages` | `overrideJSON: string` | 设置 VPN 分流应用包名 |

#### 日志

| 方法 | 参数 | 说明 |
|------|------|------|
| `SetLogLevel` | `level: int32` | 设置最低日志级别（2=Error … 6=Trace） |
| `SetError` | `message: string` | 向客户端推送错误消息 |
| `WriteMessage` | `level: int32, message: string` | 写入自定义日志消息 |

#### 暂停 / 唤醒 / 网络

| 方法 | 参数 | 说明 |
|------|------|------|
| `Pause` | | 暂停网络活动 |
| `Wake` | | 恢复网络活动 |
| `ResetNetwork` | | 重置所有连接和 DNS 缓存 |

#### 代理控制

| 方法 | 参数 | 说明 |
|------|------|------|
| `SelectProxy` | `group: string, tag: string` | 选择代理组中的节点 |
| `TestDelay` | `name: string` | 测试代理延迟。使用代理组配置中的 URL |
| `SetMode` | `mode: string` | 设置路由模式：`rule` / `global` / `direct` |
| `SetGroupExpand` | `group: string, expand: bool` | 设置代理组展开状态 |

#### 查询

| 方法 | 参数 | 说明 |
|------|------|------|
| `QueryProxies` | | 查询代理组（JSON） |
| `QueryTraffic` | | 查询流量统计（JSON） |
| `QueryLogs` | `clear: bool` | 查询最近日志（JSON）。传入 `true` 可在查询后清空缓冲区 |
| `QueryConnections` | | 查询活跃连接（JSON） |

#### 连接管理

| 方法 | 参数 | 说明 |
|------|------|------|
| `CloseConnection` | `id: string` | 按 ID 关闭连接 |
| `CloseAllConnections` | | 关闭所有活跃连接 |

#### 平台查询

| 方法 | 参数 | 说明 |
|------|------|------|
| `NeedWIFIState` | → `bool` | 是否需要 WiFi 状态监听 |
| `NeedFindProcess` | → `bool` | 是否需要进程查找 |
| `UpdateWIFIState` | | 触发 WiFi 状态更新 |
| `QueryMemoryStats` | | 查询 Go 运行时内存统计（JSON） |
| `FlushSystemDNS` | | 刷新系统 DNS 缓存 |

#### 内存

| 方法 | 参数 | 说明 |
|------|------|------|
| `SetMemoryLimit` | `bytes: int64` | 设置 Go 运行时软内存限制（0=禁用） |

#### 事件

| 方法 | 参数 | 说明 |
|------|------|------|
| `SetOnEvent` | `handler: EventHandler` | 设置事件处理器（`void OnEvent(int eventType, String jsonPayload)`） |

#### 工具

| 方法 | 参数 | 说明 |
|------|------|------|
| `Version` | | 获取版本信息（JSON） |
| `SetLocale` | `localeID: string` | 设置错误消息语言（静态方法） |

### 移动端 TUN 集成

**Android（Kotlin）：**
```kotlin
val singcast = Singcast()
singcast.init("""{"home_dir":"$homeDir"}""")

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
singcast.init("{\"home_dir\":\"\(homeDir)\"}")

// 用户点击连接时 — 从 NEPacketTunnelProvider 提取 fd
let fd = tunnelFileDescriptor  // 来自 NetworkExtension
singcast.setTunFd(fd)
singcast.startWithContent(yamlContent, "")
```

## 许可证

MIT
