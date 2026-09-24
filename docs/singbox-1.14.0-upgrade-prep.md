# sing-box 1.14.0 升级准备

> 状态：准备/评估（尚未切换内核）
> 日期：2026-09-03
> 当前稳定版：`v1.13.21`（已在 `go.mod`）
> 目标：`v1.14.0`（已正式发布）

## 0. 结论摘要

- `sing-box v1.14.0` 已于 **2026-08-31** 正式发布，非预发布版。
- 本项目直接依赖 sing-box 的内部包（`box` / `adapter` / `option` / `experimental/clashapi` / `common/urltest` / `common/trafficcontrol` / `protocol/group` / `constant`），**1.14 对 clash / 流量统计 / 事件订阅接口做了重构**，是本次升级的主要成本。
- 影响面集中在 `core/` 的运行时查询与事件订阅；`translator/` 输出字段在 1.14 的 `CacheFileOptions` 中仍保留（`store_rdrc` / `store_fakeip` 等），字段层面不构成阻塞，但需用 1.14 校验一次真实配置。
- 配套依赖注意：`sing-box v1.14.0` 依赖 `sing v0.9.0-beta.4`、`sing-tun v0.9.0-beta.4`、`sing-quic v0.7.0-beta.4`（SagerNet 子库惯例：即使内核正式版，子库仍跟 beta 号线）。

## 1. 版本现状

| 模块 | 当前 (1.13) | 目标 (1.14) | 备注 |
|---|---|---|---|
| `github.com/sagernet/sing-box` | `v1.13.21` | `v1.14.0` | 正式版 |
| `github.com/sagernet/sing` | `v0.8.14` | `v0.9.0-beta.4` | 子库跟随 beta |
| `github.com/sagernet/sing-tun` | `v0.8.15` | `v0.9.0-beta.4` | 子库跟随 beta |
| `github.com/sagernet/sing-quic` | `v0.6.5` | `v0.7.0-beta.4` | 子库跟随 beta |
| `github.com/sagernet/quic-go` | `v0.61.0-sing-box-mod.5` | `v0.61.0-sing-box-mod.7` | |
| `github.com/sagernet/tailscale` | `v1.102.1-sing-box-1.14-mod.3` | `v1.102.1-sing-box-1.14-mod.4` | 当前已是 1.14-mod.3 |
| `go` 工具链 | `go 1.26.0` | 要求 `>= go 1.25.5` | 满足 |

`sing-box v1.14.0` 还新增了 `sing-openvpn`、`sing-openconnect`、`sing-cloudflared`、`sing-snell`、`sing-usbip`、`gliderssh`、`pkg/sftp`、`age` 等依赖（伴随 OpenVPN/OpenConnect/Snell/USB/IP 等能力），`go mod tidy` 会一并收敛。

## 2. 编译层面实测破坏点

在 `go.mod` 切换到 `v1.14.0` 后实测编译，错误集中在两类：

1. **import 路径不存在**：
   - `core/events.go:8` → `github.com/sagernet/sing-box/experimental/clashapi/trafficontrol`（1.14 已不存在）
2. **`go.sum` 缺新依赖条目**（`go mod tidy` 中断所致）：
   - `github.com/libp2p/go-nat`（sing-quic hysteria2/realm）
   - `github.com/sagernet/sing-snell` / `snellv4` / `snellv5` / `snellv6`（sing-box snell）
   - 这些一经 `go mod tidy` 补全即可，不属代码问题。

## 3. 核心迁移映射（core 层）

`box.Box` 在 1.14 仍暴露 `Router()` / `Outbound()` / `Inbound()` / `Network()` / `Endpoint()` / `LogFactory()`，因此 `service.go` / `watchdog.go` 通过 `rs.instance.Outbound()`、`rs.instance.Router().ResetNetwork()` 的路径基本不变。

变化集中在 clash/流量/事件订阅，映射如下：

| 旧 1.13 符号 | 1.14 对应 | 涉及文件 |
|---|---|---|
| `experimental/clashapi/trafficontrol` | `common/trafficcontrol` | `core/events.go` |
| `clashapi.Server.TrafficManager()` | 从 `boxCtx` 取 `service.PtrFromContext[trafficcontrol.Manager](ctx)` | `core/query.go` |
| `*trafficontrol.Manager.Snapshot()`（含 Upload/Download/Connections/Memory） | `(Total() (up,down) int64)` + `(ConnectionsLen() int)`；内存改用 `runtime.ReadMemStats` | `core/query.go` |
| `*trafficontrol.Manager.Connections()` | `(*trafficcontrol.Manager).Connections()` | `core/query.go` |
| `*trafficontrol.Manager.Connection(id)` | `(*trafficcontrol.Manager).Connection(id)` | `core/query.go` |
| `*trafficontrol.Manager.ResetStatistic()` | `(*trafficcontrol.Manager).Clear()`（清空已关闭统计，语义略不同） | `core/query.go` |
| `*trafficontrol.Manager.SetEventHook(sub)` | `SubscribeEvents()` / `UnSubscribeEvents()` | `core/events.go` |
| `clashapi.Server.HistoryStorage()` | `service.PtrFromContext[urltest.HistoryStorage](ctx)` | `core/query.go` / `core/events.go` |
| `adapter.URLTestHistoryStorage` 类型 | 改用 `*urltest.HistoryStorage`（1.14 已移除该 adapter 接口） | `core/query.go` |
| `urltest.HistoryStorage.SetHook(sub)` | `AddUpdateHook(sub)` | `core/events.go` |
| `clashapi.Server.SetModeUpdateHook(sub)` | `clashapi.Server.AddModeUpdateHook(sub)`（`adapter.ClashServer` 接口已含） | `core/events.go` |
| `urltest.URLTest(ctx, link, outbound)` | 仍存在（`common/urltest`） | `core/watchdog.go` / `core/query.go` |
| `protocol/group.URLTestOutbounds` | 仍存在（参数为 `*urltest.HistoryStorage`） | `core/query.go`（如需批量测速） |
| `adapter.CacheFile` | 仍存在（`LoadGroupExpand` / `StoreGroupExpand` / `StoreRDRC` / `StoreDNS` 均保留） | `core/query.go` |

### 3.1 关键机制说明

- 1.14 的 `trafficcontrol.Manager` 与 `urltest.HistoryStorage` 均由 `box.New` 装配进 service context：
  - `box.go` → `trafficManager := trafficcontrol.NewManager(outboundManager)`
  - `daemon/instance.go` → `service.PtrFromContext[trafficcontrol.Manager](ctx)` / `service.PtrFromContext[urltest.HistoryStorage](ctx)`
- 因此本项目可用 `service.PtrFromContext[trafficcontrol.Manager](rs.boxCtx)` 与 `service.PtrFromContext[urltest.HistoryStorage](rs.boxCtx)` 获取，无需依赖 `clashapi.Server` 方法。
- 内存统计：旧 `Snapshot().Memory` 不再由 `trafficcontrol.Manager` 提供；1.14 在 `experimental/clashapi/api_meta.go` 自行 `runtime.ReadMemStats`。本项目可将 `QueryStats` 的 `Memory` 改为 `runtime.ReadMemStats` 计算。

## 4. translator 兼容性

- `translator/experimental.go` 输出 `cache_file.store_rdrc` / `store_fakeip`：1.14 `option.CacheFileOptions` 仍保留 `StoreRDRC`（json `store_rdrc`）、`StoreFakeIP`（json `store_fakeip`），带 `schema:"omit"` 弃用标记但不会被 unmarshal 拒绝。
- 规则层：`translator/autoroute.go` 已用 `rule_set`（`ensureRuleSetDef`），`translator/assemble.go` 已用 route `action=sniff`，都是 1.14 推荐形态；无内联 `geosite` / `geoip` / inbound 旧 `sniff` / `domain_strategy` 残留。
- 配置路径：需在迁移后用 1.14 对一份真实配置做 CheckConfig/启动验证，确认无 `unknown field` / 校验失败（尤其 DNS 的 `default_domain_resolver`、`domain_resolver` 等）。

## 5. 迁移步骤（建议）

1. 备份当前 `go.mod` / `go.sum`（已提交且已推送，`git status` 干净）。
2. 升级依赖：
   ```bash
   go get github.com/sagernet/sing-box@v1.14.0 \
     github.com/sagernet/sing@v0.9.0-beta.4 \
     github.com/sagernet/sing-tun@v0.9.0-beta.4 \
     github.com/sagernet/sing-quic@v0.7.0-beta.4
   go mod tidy
   ```
3. 迁移 `core/` clash/流量/事件订阅接口（见第 3 节映射表）：
   - `core/events.go`：改 import 为 `common/trafficcontrol`；`HistoryStorage` / `TrafficManager` / `AddModeUpdateHook` 从 context 获取改用新版订阅 API。
   - `core/query.go`：`srv.HistoryStorage()` / `srv.TrafficManager()` 改从 `rs.boxCtx` 取；`Snapshot` 拆成 `Total()` + `ConnectionsLen()`；`Memory` 用 `runtime.ReadMemStats`；`ResetStatistic` 映射 `Clear()`。
4. 处理 `platform.go` / `platform_test.go` 对 `sing-tun v0.9.0-beta.4` 的 API 变化（`tun.Options` / `tun.New` / `tun.DefaultInterfaceMonitor` 等需以编译结果为准逐步适配）。
5. 用 1.14 校验 translator 输出（`go test ./translator/...` + `CheckConfig`），必要时修正字段。
6. 全量验证：
   ```bash
   go build -tags 'with_clash_api,with_utls,with_quic,with_gvisor' ./...
   task test
   golangci-lint run ./...
   ```

## 6. 风险与注意事项

- **子库仍为 beta**：`sing` / `sing-tun` / `sing-quic` 跟随 `v0.9.0-beta.4` / `v0.7.0-beta.4`，属 beta 版本；如需更保守，可等其转正后再升级。
- **行为面广**：`QueryStats`（内存字段来源）、`QueryConnections`、`CloseConnections`（`ResetStatistic` 语义）、连接事件订阅的推送频率在 1.14 可能与 1.13 有差异，需要移动端/事件回调的回归验证。
- **内存统计缺失**：1.14 `trafficcontrol.Manager` 不含 memory，需自行引入 `runtime.ReadMemStats`，数值口径与旧 `Snapshot.Memory` 可能不同。
- **不应留下半成品状态**：在 core 迁移完成并通过测试前，`go.mod` 应保持在 `v1.13.21`（本仓库可构建）而非 `v1.14.0`。
