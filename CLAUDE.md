# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

Singcast 是一个基于 [sing-box](https://github.com/SagerNet/sing-box) 的轻量级代理内核。同时接受 **Clash Meta (Mihomo) YAML** 和 **sing-box JSON** 两种配置格式——运行时自动将 Clash 配置翻译为 sing-box 格式，统一由 sing-box 引擎执行。Go module 路径为 `github.com/mapleafgo/singcast`。

## 构建与测试

构建系统使用 [Task](https://taskfile.dev)。所有构建和测试都需要 Go build tags: `with_clash_api,with_utls,with_quic,with_gvisor`（**不可省略**，sing-box 大量使用条件编译，缺少会编译失败）。

```bash
task cli                  # 构建 CLI → bin/singcast
task test                 # 运行测试（translator + core 包）
task clean                # 清理 bin/
```

运行单个测试或包：
```bash
go test -tags 'with_clash_api,with_utls,with_quic,with_gvisor' ./translator/...
go test -tags 'with_clash_api,with_utls,with_quic,with_gvisor' -run TestTranslateDNS ./translator/
```

## 架构

### 三个入口

1. **CLI** (`cmd/singcast/`) — `singcast run|convert|check|version`，使用 `urfave/cli/v3`。`run` 命令直接调用 `core.Service.StartWithContent`。

2. **桌面端 FFI** (`cmd/lib/`) — 通过 `cgo` 导出 C 共享库（`.so`/`.dylib`/`.dll`）。导出 `Core*` 系列函数包装 `core.Service`。内存所有权：返回的 `char*` 必须由调用方通过 `CoreFreeString` 释放。事件回调使用所有权转移模式（native 分配，消费者复制后释放）。

3. **移动端 SDK** (`mobile/`) — `gomobile bind` 目标，产出 AAR (Android) 和 xcframework (iOS)。`Singcast` 结构体包装 `core.Service`，增加移动端特有功能：TUN fd 注入、socket protector（VPN 绕过）、网络接口上报。

### 核心层 (`core/`)

`Service` 是核心运行时，状态机：`Created → Initialized → Starting → Running → Destroyed`。通过 `atomic` 状态 + CAS 循环实现线程安全。

- `StartWithContent(content, ruleSetProxy)` — 检测格式，必要时翻译 YAML，创建 sing-box 实例
- 移动端强制设置 `auto_detect_interface=true`，用于 VPN socket 保护
- 状态变更通过统一回调发出事件（`EventStateChange`）
- Clash API 服务提供流量跟踪、代理组、连接管理和模式切换

### 翻译器层 (`translator/`)

翻译器将 Mihomo YAML → sing-box JSON，在 `Convert()`/`ConvertWithMeta()` 统一入口中按 10 步流水线执行：

1. 格式检测 (`detect.go`) — 先尝试 JSON 解析，失败则为 YAML
2. 全局配置 → inbounds (`general.go`) — mixed/HTTP/SOCKS/redir/tproxy 端口、认证、sniffer
3. 代理节点 → outbounds (`proxy.go` + `translator/proxy/`) — 每种协议一个翻译器，位于 `proxy/` 子目录
4. 代理组 → outbounds (`group.go`) — Selector、URLTest、Fallback
5. 规则 → 路由规则 (`rule.go`) — GEOIP/GEOSITE 转为 rule_set 引用
6. DNS (`dns.go`) — nameservers、fallback、FakeIP、hosts
7. TUN (`tun.go`) — TUN 接口配置
8. 实验性功能 (`experimental.go`) — clash_api、cache
9. 组装 (`assemble.go`) — 注入内置规则、默认路由、将 REJECT 转为 route action、rule_set 定义
10. 校验 (`validation.go`) — 验证 outbound/route 引用是否存在

核心数据类型：`RawConfig`（Mihomo YAML 结构，`types_mihomo.go`）、`singboxConfig`/`singboxRoute`/`singboxDNS`（sing-box 输出，`types_singbox.go`）、`translation`（流水线状态累积器）。

### 平台层 (`core/platform.go`)

`PlatformIO` 提供移动端 I/O：socket protector 回调、TUN fd 存储、接口监控、WiFi 状态。桌面端为空操作。

## 关键约定

- sing-box 选项结构统一使用 `map[string]any`（非类型化结构体），保持灵活性
- 翻译器返回 `(jsonString, warnings, error)` — warnings 是非致命问题，调用方应展示给用户
- `RawConfig` 中的 YAML 字段名遵循 Mihomo 惯例（如 `mixed-port`、`socks-port`）
- 协议翻译器位于 `translator/proxy/`，每个协议一个文件
- 测试文件与源码同目录（如 `dns.go` + `dns_test.go`）
- build tags `with_clash_api,with_utls,with_quic,with_gvisor` 在构建和测试中都必须指定
