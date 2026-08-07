# mihomo ↔ sing-box 配置映射表

> 版本基准：**mihomo v1.19.24** (2026-04-20) ↔ **sing-box v1.13.12** (2026-05-08)
>
> 本文档用于指导 singcast 配置翻译器的开发。翻译方向：mihomo YAML → sing-box JSON。

---

## A. 全局配置 → Inbound + Route + Log + Experimental

| mihomo YAML | sing-box JSON | 翻译逻辑 |
|---|---|---|
| `mixed-port: 7890` | `inbounds[].{type:"mixed", listen:"127.0.0.1", listen_port:7890}` | 端口映射为 mixed inbound |
| `port: 7890` | `inbounds[].{type:"http", listen:"127.0.0.1", listen_port:7890}` | HTTP 代理端口 |
| `socks-port: 7891` | `inbounds[].{type:"socks", listen:"127.0.0.1", listen_port:7891}` | SOCKS 端口 |
| `redir-port: 7893` | `inbounds[].{type:"redirect", listen_port:7893}` | Linux 透明代理 |
| `tproxy-port: 7894` | `inbounds[].{type:"tproxy", listen_port:7894}` | Linux TProxy |
| `allow-lan: true` | inbound `listen` 改为 `"0.0.0.0"` | false 时为 `127.0.0.1` |
| `bind-address: "*"` | inbound `listen` | 映射绑定地址 |
| `mode: rule` | `experimental.clash_api.default_mode: "Rule"` | rule/global/direct 映射 |
| `log-level: info` | `log.level: "info"` | silent/error/warning/info/debug 直接映射 |
| `ipv6: true` | `dns.strategy` + inbound 层面 | 需多处设置 |
| `external-controller: addr` | `experimental.clash_api.external_controller` | 直接映射 |
| `secret: "xxx"` | `experimental.clash_api.secret` | 直接映射 |
| `profile.store-selected: true` | `experimental.cache_file.enabled: true` | 拆分映射 |
| `profile.store-fake-ip: true` | `experimental.cache_file.store_fakeip: true` | 直接映射 |
| `unified-delay: false` | (无对应) | 忽略 |
| `tcp-concurrent: false` | (无对应) | 忽略 |
| `keep-alive-interval: 30` | dial fields `tcp_keep_alive_interval: "30s"` | 需加单位 |
| `hosts: {domain: ip}` | `dns.servers[].{type:"hosts", predefined:{domain:ip}}` | mihomo hosts 值支持字符串或 IP 数组，均映射到 sing-box hosts DNS server（见节 N.4） |
| `find-process-mode: strict` | `route.find_process: true/false` | always/strict→true, off→false |
| `geodata-mode: true` | (sing-box 使用 rule_set binary) | 影响规则格式选择 |
| `sniffer.*` | route rules `action: "sniff"` + `action: "hijack-dns"` | **独立实现**：忽略 mihomo sniffer 配置，无条件启用嗅探和 DNS 劫持（见节 T.1） |

**注意**：mihomo 把代理端口放在顶层，sing-box 放在 `inbounds` 数组中。翻译时根据 mihomo 配置的端口生成对应的 inbound 列表。

---

## B. 代理协议映射

### B.1 通用字段映射（所有协议共享）

| mihomo | sing-box | 说明 |
|---|---|---|
| `name` | `tag` | 代理名称 → 标签 |
| `server` | `server` | 服务器地址 |
| `port` | `server_port` | **字段名不同** |
| `udp: true` | `network: "tcp"` (默认即 tcp+udp) | sing-box 默认双栈 |
| `ip-version` | `domain_resolver.strategy` | IPv4/IPv6 策略 |
| `interface-name` | `bind_interface` | 绑定接口 |
| `routing-mark` | `routing_mark` | 路由标记 |
| `tfo: true` | `tcp_fast_open: true` | TCP Fast Open |
| `mptcp: true` | `tcp_multi_path: true` | Multipath TCP |
| `dialer-proxy` | `detour` | 通过指定代理连接 |

### B.2 TLS 字段映射（VMess/VLESS/Trojan 共享）

| mihomo | sing-box | 说明 |
|---|---|---|
| `tls: true` | `tls.enabled: true` | 嵌套结构 |
| `sni` / `servername` | `tls.server_name` | mihomo 两个字段名，统一映射 |
| `skip-cert-verify: true` | `tls.insecure: true` | **字段名不同** |
| `alpn: [h2, http/1.1]` | `tls.alpn: ["h2", "http/1.1"]` | 直接映射 |
| `fingerprint` | `tls.certificate_public_key_sha256` | 证书指纹 |
| `client-fingerprint: chrome` | `tls.utls: {enabled:true, fingerprint:"chrome"}` | 嵌套结构 |
| `reality-opts.public-key` | `tls.reality.public_key` | 嵌套结构 |
| `reality-opts.short-id` | `tls.reality.short_id` | 嵌套结构 |
| `certificate` | `tls.client_certificate` | mTLS 证书 |
| `private-key` | `tls.client_key` | mTLS 私钥 |

### B.3 传输层映射

| mihomo `network` | sing-box `transport` | 说明 |
|---|---|---|
| `tcp` (默认) | 不设置 transport | 默认 TCP |
| `ws` | `{type:"ws", path, headers}` | WebSocket |
| `ws-opts.path` | `transport.path` | 路径 |
| `ws-opts.headers` | `transport.headers` | 请求头 |
| `ws-opts.max-early-data` | `transport.max_early_data` | Early Data |
| `ws-opts.v2ray-http-upgrade` | `transport: {type:"httpupgrade"}` | HTTP Upgrade 是独立类型 |
| `http` | `{type:"http", method, path, headers}` | HTTP/2 |
| `h2` | `{type:"http", host, path}` | H2 用 http transport |
| `grpc` | `{type:"grpc", service_name}` | gRPC |
| `grpc-opts.grpc-service-name` | `transport.service_name` | 服务名 |

### B.4 多路复用映射

| mihomo `smux` | sing-box `multiplex` | 说明 |
|---|---|---|
| `smux.enabled` | `multiplex.enabled` | 直接映射 |
| `smux.protocol: h2mux` | `multiplex.protocol: "h2mux"` | smux/yamux/h2mux |
| `smux.max-connections` | `multiplex.max_connections` | 直接映射 |
| `smux.max-streams` | `multiplex.max_streams` | 直接映射 |
| `smux.padding` | `multiplex.padding` | 直接映射 |
| `smux.brutal-opts` | `multiplex.brutal` | Brutal 配置 |

---

### B.5 VLESS（核心用例）

| mihomo 字段 | sing-box 字段 | 翻译说明 |
|---|---|---|
| `type: vless` | `"type": "vless"` | 直接 |
| `uuid` | `uuid` | 直接 |
| `flow: xtls-rprx-vision` | `flow: "xtls-rprx-vision"` | 直接 |
| `packet-encoding: xudp` | `packet_encoding: "xudp"` | 直接 |
| `encryption` | (无对应) | VLESS 加密扩展，sing-box 不支持 |
| + 通用字段 + TLS + 传输层 | | |

### B.6 VMess

| mihomo 字段 | sing-box 字段 | 翻译说明 |
|---|---|---|
| `type: vmess` | `"type": "vmess"` | 直接 |
| `uuid` | `uuid` | 直接 |
| `alterId: 0` | `alter_id: 0` | 直接 |
| `cipher: auto` | `security: "auto"` | **字段名不同** |
| `global-padding` | `global_padding` | 直接 |
| `authenticated-length` | `authenticated_length` | 直接 |
| `packet-encoding` | `packet_encoding` | 直接 |
| + 通用字段 + TLS + 传输层 | | |

### B.7 Trojan

| mihomo 字段 | sing-box 字段 | 翻译说明 |
|---|---|---|
| `type: trojan` | `"type": "trojan"` | 直接 |
| `password` | `password` | 直接 |
| `ss-opts.enabled` | (无对应) | trojan-go Shadowsocks 不支持 |
| + 通用字段 + TLS + 传输层 | | |

### B.8 Shadowsocks

| mihomo 字段 | sing-box 字段 | 翻译说明 |
|---|---|---|
| `type: ss` | `"type": "shadowsocks"` | **类型名不同** |
| `cipher` | `method` | **字段名不同** |
| `password` | `password` | 直接 |
| `plugin: obfs-local` | `plugin: "obfs-local"` | 直接 |
| `plugin-opts` (object) | `plugin_opts` (string) | **需拼装为 SIP003 格式字符串** |
| `udp-over-tcp` | `udp_over_tcp` | 直接 |

plugin-opts 拼装示例：
```
mihomo:  plugin: obfs-local, plugin-opts: {mode: tls, host: bing.com}
sing-box: plugin: "obfs-local", plugin_opts: "obfs=tls;host=bing.com"
```

**cipher 方法对应**：
| mihomo | sing-box | 说明 |
|---|---|---|
| aes-128-gcm | aes-128-gcm | 直接 |
| aes-256-gcm | aes-256-gcm | 直接 |
| chacha20-ietf-poly1305 | chacha20-ietf-poly1305 | 直接 |
| 2022-blake3-aes-128-gcm | 2022-blake3-aes-128-gcm | 直接 |
| 2022-blake3-aes-256-gcm | 2022-blake3-aes-256-gcm | 直接 |
| 2022-blake3-chacha20-poly1305 | 2022-blake3-chacha20-poly1305 | 直接 |
| xchacha20-ietf-poly1305 | xchacha20-ietf-poly1305 | 直接 |
| rc4-md5 | (不支持) | 跳过 |
| lea-128-gcm 等 LEA 系列 | (不支持) | 跳过 |

### B.9 Hysteria2

| mihomo 字段 | sing-box 字段 | 翻译说明 |
|---|---|---|
| `type: hysteria2` | `"type": "hysteria2"` | 直接 |
| `password` | `password` | 直接 |
| `up: "30 Mbps"` | `up_mbps: 30` | **需解析单位，转为纯数字 Mbps** |
| `down: "200 Mbps"` | `down_mbps: 200` | 同上 |
| `obfs: salamander` + `obfs-password` | `obfs: {type:"salamander", password:"..."}` | 嵌套结构 |
| `ports: 443-8443` | `server_ports: ["443:8443"]` | 端口跳跃 |
| `hop-interval: 30` | `hop_interval: "30s"` | **需加时间单位** |
| `sni` | `tls.server_name` | 同 TLS 映射 |
| `skip-cert-verify` | `tls.insecure` | 同 TLS 映射 |
| `alpn` | `tls.alpn` | 同 TLS 映射 |

### B.10 WireGuard

| mihomo 字段 | sing-box 字段 | 翻译说明 |
|---|---|---|
| `type: wireguard` | `type: "wireguard"` (endpoint) | **sing-box 1.11+ 迁移为 endpoint** |
| `server` | `peers[].address` | 移到 peers 中 |
| `port` | `peers[].port` | 移到 peers 中 |
| `ip: 172.16.0.2` | `address: ["172.16.0.2/32"]` | **需加 CIDR 后缀** |
| `ipv6: fd01:...` | address 追加 `"fd01:.../128"` | **需加 CIDR 后缀** |
| `private-key` | `private_key` | 直接 |
| `public-key` | `peers[].public_key` | 移到 peers 中 |
| `pre-shared-key` | `peers[].pre_shared_key` | 移到 peers 中 |
| `reserved` | `peers[].reserved` | 移到 peers 中 |
| `persistent-keepalive` | `peers[].persistent_keepalive_interval` | 移到 peers 中 |
| `mtu` | `mtu` | 直接 |
| `dns` | (通过 DNS 规则处理) | 单独映射 |

### B.11 TUIC

sing-box 仅支持 TUIC v5（uuid + password 认证）。mihomo 的 TUIC v4（token 认证）不支持，翻译时跳过并输出警告。

| mihomo 字段 | sing-box 字段 | 翻译说明 |
|---|---|---|
| `type: tuic` | `"type": "tuic"` | 直接 |
| `uuid` | `uuid` | 直接（v5） |
| `password` | `password` | 直接（v5） |
| `token` | (不支持) | TUIC v4 认证，跳过并输出警告 |
| `udp-relay-mode: native` | `udp_relay_mode: "native"` | 直接 |
| `udp-relay-mode: quic` | `udp_relay_mode: "quic"` | 直接 |
| `congestion-controller: bbr` | `congestion_control: "bbr"` | **字段名不同** |
| `reduce-rtt: true` | `zero_rtt_handshake: true` | **字段名不同** |
| `sni` | `tls.server_name` | 同 TLS 映射 |
| `skip-cert-verify` | `tls.insecure` | 同 TLS 映射 |
| `alpn` | `tls.alpn` | 同 TLS 映射 |
| + 通用字段 + TLS | | |

> TUIC 必须启用 TLS，翻译器需自动设置 `tls.enabled: true`。

### B.12 HTTP Proxy

| mihomo 字段 | sing-box 字段 | 翻译说明 |
|---|---|---|
| `type: http` | `"type": "http"` | 直接 |
| `username` | `username` | 直接 |
| `password` | `password` | 直接 |
| `headers` | `headers` | 直接（键值对对象） |
| `tls: true` | `tls.enabled: true` | 同 TLS 映射 |
| `sni` | `tls.server_name` | 同 TLS 映射 |
| `skip-cert-verify` | `tls.insecure` | 同 TLS 映射 |
| `alpn` | `tls.alpn` | 同 TLS 映射 |
| `client-fingerprint` | `tls.utls` | 同 TLS 映射 |
| + 通用字段 + TLS | | |

### B.13 SOCKS5

| mihomo 字段 | sing-box 字段 | 翻译说明 |
|---|---|---|
| `type: socks5` | `"type": "socks"` | **类型名不同** |
| `username` | `username` | 直接 |
| `password` | `password` | 直接 |
| `udp: true` | `network: ["tcp", "udp"]` | sing-box 通过 network 控制协议 |
| `tls: true` | `tls.enabled: true` | 同 TLS 映射 |
| `sni` | `tls.server_name` | 同 TLS 映射 |
| `skip-cert-verify` | `tls.insecure` | 同 TLS 映射 |
| + 通用字段 + TLS | | |

> sing-box SOCKS outbound 支持 `version` 字段（4/4a/5），默认 5。mihomo 的 `socks5` 对应 `version: 5`。

### B.14 不支持/低优先级协议

| mihomo 类型 | sing-box 支持 | 处理策略 |
|---|---|---|
| SSR (shadowsocksR) | 不支持 | 跳过，输出警告 |
| Snell | 不支持 | 跳过，输出警告 |
| SSH | 不支持 | 跳过 |
| Hysteria v1 | 不支持 | 跳过 |
| Masque | 不支持 | 跳过 |

---

## C. 代理组映射

### C.1 Select → Selector

| mihomo | sing-box | 翻译说明 |
|---|---|---|
| `name` | `tag` | 直接 |
| `type: select` | `"type": "selector"` | **类型名不同** |
| `proxies: [...]` | `outbounds: [...]` | **字段名不同** |
| (无) | `default: "第一个代理"` | 用第一个代理作为默认 |

**丢失字段（当前忽略，不展开/不过滤）**：`use`、`filter`、`exclude-filter`、`include-all`、`include-all-proxies`、`include-all-providers` — 当前实现只按 `proxies` 列表和已知 outbound tag 过滤；provider 展开方案见 S 节（参考设计，未实现）。

### C.2 URL-Test → URLTest

| mihomo | sing-box | 翻译说明 |
|---|---|---|
| `name` | `tag` | 直接 |
| `type: url-test` | `"type": "urltest"` | **类型名不同** |
| `proxies: [...]` | `outbounds: [...]` | **字段名不同** |
| `url` | `url` | 直接 |
| `interval: 300` | `interval: "5m"` | **秒 → Go Duration 字符串** |
| `tolerance: 50` | `tolerance: 50` | 直接 (单位都是 ms) |
| `lazy: true` | `idle_timeout: "30m"` | 概念不同，近似映射 |
| `timeout: 5000` | (无直接对应) | 忽略 |

### C.3 Fallback → URLTest（近似替代）

sing-box 没有 fallback 类型。用 `urltest` 近似替代。
- 区别：fallback 按顺序选第一个可用节点，urltest 选延迟最低的
- 行为差异可接受

### C.4 Load-Balance → Selector（降级）

sing-box 没有原生的 load-balance 类型。翻译时降级为 `selector`，记录警告。用户可手动选择节点，但无法实现负载均衡语义。

### C.5 Relay

mihomo relay 已弃用（建议用 dialer-proxy）。sing-box 无对应。跳过。

### C.6 内置组

| mihomo | sing-box | 说明 |
|---|---|---|
| `DIRECT` | `{type: "direct", tag: "DIRECT"}` | 必须显式定义 |
| `REJECT` | route rule `action: "reject"` | sing-box 1.13.0 移除了 `block` outbound，改为 route rule action |

---

## D. 规则映射

> **当前实现注意**：singcast 翻译器**不翻译 mihomo 的 `rules` 和 `rule-providers`**，这些配置会被整体忽略并输出 warning，路由规则由 `autoroute.go` 自动地理分流生成（见 T.3）。以下 D 节仅作为 sing-box 规则格式的映射参考，适用于直接提供 sing-box JSON 或后续扩展翻译能力。

> **大小写约定**：sing-box 规则中的 `outbound` 字段值必须与目标 outbound 的 `tag` 精确匹配。内置组使用大写 `"DIRECT"` / `"REJECT"`（见 C.6），代理组名称保留 mihomo 原始大小写。以下示例统一使用大写。

### D.1 域名规则

| mihomo 规则 | sing-box route rule | 说明 |
|---|---|---|
| `DOMAIN,example.com,PROXY` | `{domain:["example.com"], outbound:"PROXY"}` | 完整域名 |
| `DOMAIN-SUFFIX,google.com,DIRECT` | `{domain_suffix:["google.com"], outbound:"DIRECT"}` | 域名后缀 |
| `DOMAIN-KEYWORD,ads,REJECT` | `{domain_keyword:["ads"], action:"reject"}` | 关键字 |
| `DOMAIN-REGEX,^abc.*,PROXY` | `{domain_regex:["^abc.*"], outbound:"PROXY"}` | 正则 |
| `DOMAIN-WILDCARD,*.google.com,PROXY` | `{domain_regex:["^.*\\.google\\.com$"], outbound:"PROXY"}` | **需转为正则** |
| `GEOSITE,youtube,PROXY` | `{rule_set:["geosite-youtube"], outbound:"PROXY"}` | **需预配置 rule_set** |

### D.2 IP 规则

| mihomo 规则 | sing-box route rule | 说明 |
|---|---|---|
| `IP-CIDR,10.0.0.0/8,DIRECT` | `{ip_cidr:["10.0.0.0/8"], outbound:"DIRECT"}` | 直接 |
| `IP-CIDR6,::1/128,DIRECT` | `{ip_cidr:["::1/128"], outbound:"DIRECT"}` | 直接 |
| `IP-SUFFIX,8.8.8.8/24,PROXY` | (无直接对应) | 需转为 ip_cidr 列表或忽略 |
| `IP-ASN,13335,DIRECT` | (无直接对应) | 需 rule_set 或忽略 |
| `GEOIP,CN,DIRECT` | `{rule_set:["geoip-cn"], outbound:"DIRECT"}` | **需预配置 rule_set** |
| `SRC-GEOIP,cn,DIRECT` | `{rule_set:["geoip-cn"], rule_set_ipcidr_match_source:true, outbound:"DIRECT"}` | 来源匹配 |
| `SRC-IP-CIDR,...` | `{source_ip_cidr:[...], outbound:"..."}` | 直接 |
| `IP-CIDR,...,no-resolve` | (sing-box 默认行为不同) | 通常可忽略 |

### D.3 端口/进程/网络规则

| mihomo 规则 | sing-box route rule | 说明 |
|---|---|---|
| `DST-PORT,80,DIRECT` | `{port:[80], outbound:"DIRECT"}` | 直接 |
| `DST-PORT,80-443,DIRECT` | `{port_range:["80:443"], outbound:"DIRECT"}` | 范围映射 |
| `SRC-PORT,7777,DIRECT` | `{source_port:[7777], outbound:"DIRECT"}` | 直接 |
| `IN-PORT,7890,PROXY` | `{inbound:["mixed-in"], outbound:"PROXY"}` | 通过 inbound tag 匹配 |
| `IN-TYPE,SOCKS/HTTP,PROXY` | `{inbound:["socks-in","http-in"], outbound:"PROXY"}` | 通过 inbound tag 匹配 |
| `PROCESS-NAME,curl,PROXY` | `{process_name:["curl"], outbound:"PROXY"}` | 直接 |
| `PROCESS-PATH,/usr/bin/wget` | `{process_path:["/usr/bin/wget"], outbound:"PROXY"}` | 直接 |
| `PROCESS-PATH-REGEX,.*wget` | `{process_path_regex:[".*wget"], outbound:"PROXY"}` | 直接 |
| `NETWORK,udp,DIRECT` | `{network:"udp", outbound:"DIRECT"}` | 直接 |
| `UID,1001,DIRECT` | `{user_id:[1001], outbound:"DIRECT"}` | Linux |

### D.4 逻辑规则

| mihomo | sing-box | 说明 |
|---|---|---|
| `AND,((A),(B))` | `{type:"logical", mode:"and", rules:[A,B], outbound:"..."}` | 直接 |
| `OR,((A),(B))` | `{type:"logical", mode:"or", rules:[A,B], outbound:"..."}` | 直接 |
| `NOT,((A))` | 单规则 + `invert:true`，或 logical and + invert | 反转匹配 |

### D.5 最终规则

| mihomo | sing-box | 说明 |
|---|---|---|
| `MATCH,auto` | `route.final: "auto"` | 移到 route 顶层 |

### D.6 规则集（参考映射，当前未启用）

> **注意**：当前实现**不翻译 rule-provider**（`warnUnsupportedRouting` 忽略并警告），自动注册的 rule_set 见 T.3/T.8。下表仅说明若实现映射时的字段对应关系。

| mihomo rule-provider | sing-box rule_set | 翻译说明 |
|---|---|---|
| `type: http` | `type: "remote"` | 直接 |
| `url: "https://..."` | `url: "https://..."` | 直接 |
| `path: ./rule.yaml` | (sing-box 自行管理缓存) | 忽略 path |
| `interval: 600` | `update_interval: "10m"` | **秒 → Go Duration** |
| `behavior: classical` | `format: "source"` | 行为映射 |
| `behavior: domain/ipcidr` | `format: "source"` 或 `"binary"` | 建议用 binary |
| `format: yaml/text` | `format: "source"` | 格式映射 |
| `proxy: DIRECT` | `download_detour: "DIRECT"` | **字段名不同** |

### D.7 mihomo 不支持但 sing-box 支持的规则字段

以下 sing-box 规则字段在 mihomo 中无对应，翻译时不涉及：
- `auth_user`、`client`、`clash_mode`
- `wifi_ssid`、`wifi_bssid`
- `network_is_expensive`、`network_is_constrained`
- `source_mac_address`
- `interface_address`

---

## E. DNS 映射

**这是差异最大的部分，结构完全不同。**

mihomo 使用 `nameserver` + `fallback` + `fallback-filter` + `nameserver-policy` 的分层模式。
sing-box 使用 `dns.servers` + `dns.rules` 的路由模式。

### E.1 基本配置

| mihomo | sing-box | 翻译说明 |
|---|---|---|
| `dns.enable: true` | (DNS 由 route 自动管理) | sing-box 总是启用 |
| `dns.listen: 0.0.0.0:1053` | (通过 inbound DNS 或 route 处理) | 结构不同 |
| `dns.ipv6: false` | `dns.strategy: "prefer_ipv4"` | 策略映射 |
| `dns.enhanced-mode: fake-ip` | 添加 `type:"fakeip"` DNS server | 概念不同 |
| `dns.fake-ip-range: 198.18.0.1/16` | fakeip server `inet4_range: "198.18.0.0/15"` | **CIDR 范围可能需调整** |
| `dns.fake-ip-filter: [...]` | DNS rules 中匹配后路由到非 fakeip 服务器 | 通过规则排除 |
| `dns.cache-algorithm: lru` | (sing-box 默认 LRU) | 默认一致 |
| `dns.disable-cache: false` | `dns.disable_cache: false` | 直接映射 |

### E.2 DNS 服务器

| mihomo | sing-box (1.12.0+ 新格式) | 翻译说明 |
|---|---|---|
| `nameserver: [https://doh.pub/dns-query]` | `{type:"https", tag:"doh-pub", server:"doh.pub", path:"/dns-query"}` | 解析 URL |
| `nameserver: [114.114.114.114]` | `{type:"udp", tag:"udp-114", server:"114.114.114.114"}` | 纯 IP 用 UDP |
| `nameserver: [tls://8.8.4.4]` | `{type:"tls", tag:"tls-8.8", server:"8.8.4.4"}` | 协议前缀映射 |
| `fallback: [tls://1.1.1.1]` | `{type:"tls", tag:"fallback-1", server:"1.1.1.1"}` | 同上 |
| `default-nameserver: [223.5.5.5]` | `{type:"udp", tag:"default-0", server:"223.5.5.5"}` | 用于解析其他 DNS 服务器域名 |
| `direct-nameserver: [system]` | `{type:"local", tag:"dns-local"}` | 本地解析 |
| `proxy-server-nameserver: [...]` | DNS rule 中匹配代理域名后路由到该服务器 | 通过规则路由 |

### E.3 DNS 策略路由（独立实现）

> **注意**：mihomo 的 `nameserver-policy` 是**解析但不翻译**的。翻译器在 `autoroute.go` 的 `generateDNSRules()` 中独立实现了等效功能——基于 geosite/geoip rule_set 自动生成 DNS 路由规则，而非翻译用户的 nameserver-policy 映射（见节 T.2）。

实际生成的 DNS 规则策略：
- `clash_mode: "Direct"` → 国内 DNS
- 私有域名、国内 geosite/geoip → 国内 DNS
- A/AAAA 查询 → FakeIP 服务器（如启用）
- 其他 → DNS final 服务器

```
sing-box dns.rules（自动生成）:
  [
    {clash_mode: "Direct", server: "dns-local"},
    {rule_set: ["geosite-private", "geosite-cn", "geoip-cn"], server: "dns-local"},
    {query_type: ["A", "AAAA"], server: "fakeip"},
  ]
```

---

## F. TUN 映射

翻译器：`translator/tun.go` → `translateTUN()`。所有可选字段仅在非零值时写入输出。

| mihomo tun | sing-box inbound tun | 翻译说明 | 默认值 |
|---|---|---|---|
| `enable: true` | 添加 `type:"tun"` inbound | `false` 时不生成 TUN inbound | `false` |
| `stack: system/gvisor/mixed` | `stack` | 空值时默认 `"mixed"` | `"mixed"` |
| `device: utun0` | `interface_name` | **字段名不同**，空值时不写入 | — |
| `auto-route: true` | `auto_route` | 无条件写入（含 `false`） | `false` |
| `strict-route: true` | `strict_route` | 无条件写入（含 `false`） | `false` |
| `auto-detect-interface` | `route.auto_detect_interface` | **route 顶层字段**。`general.go` 默认设 `true`，仅当 mihomo 显式 `false` 时覆盖为 `false` | `true` |
| `dns-hijack: [any:53]` | `assemble()` 默认规则 | 结构体中有字段，但翻译器不处理——由 `assemble()` 统一注入 `{"protocol":"dns","action":"hijack-dns"}` | 自动 |
| `inet4-address` | `address[]` | 空值时默认 `"172.18.0.1/30"` | `"172.18.0.1/30"` |
| `inet6-address` | `address[]` | 仅当全局 `ipv6: true` 时追加；空值默认 `"fdfe:dcba:9876::1/126"` | `"fdfe:dcba:9876::1/126"` |
| `mtu: 9000` | `mtu` | `0` 时不写入（sing-box 使用自身默认值） | — |
| `auto-redirect: true` | `auto_redirect` | 仅 `true` 时写入，仅 Linux 有效 | — |
| `udp-timeout: 300` | `udp_timeout: "5m"` | **秒 → Go Duration 字符串**，`0` 时不写入 | — |
| `route-address: [...]` | `route_address` | 空切片不写入 | — |
| `route-exclude-address: [...]` | `route_exclude_address` | 空切片不写入 | — |
| `iproute2-table-index` | `iproute2_table_index` | Linux，`0` 时不写入 | — |
| `iproute2-rule-index` | `iproute2_rule_index` | Linux，`0` 时不写入 | — |
| `include-uid: [...]` | `include_uid` | Linux，空切片不写入 | — |
| `include-uid-range: [...]` | `include_uid_range` | Linux，空切片不写入 | — |
| `exclude-uid: [...]` | `exclude_uid` | Linux，空切片不写入 | — |
| `exclude-uid-range: [...]` | `exclude_uid_range` | Linux，空切片不写入 | — |
| `include-android-user: [...]` | `include_android_user` | Android，空切片不写入 | — |
| `include-package: [...]` | `include_package` | Android，空切片不写入 | — |
| `exclude-package: [...]` | `exclude_package` | Android，空切片不写入 | — |
| `gso: true` | (1.11.0 后移除) | 忽略 | — |

---

## G. sing-box 配置骨架（翻译输出模板）

```json
{
  "log": {
    "level": "info",
    "timestamp": true
  },
  "dns": {
    "servers": [
      {"type": "udp", "tag": "def-0", "server": "223.5.5.5"},
      {"type": "https", "tag": "ns-0", "server": "dns.google", "detour": "PROXY"}
    ],
    "rules": [],
    "final": "ns-0",
    "strategy": "prefer_ipv4"
  },
  "inbounds": [
    {
      "type": "mixed",
      "tag": "mixed-in",
      "listen": "127.0.0.1",
      "listen_port": 7890
    }
  ],
  "outbounds": [
    {"type": "direct", "tag": "DIRECT"},
    {"type": "selector", "tag": "...", "outbounds": []},
    {"type": "vless", "tag": "...", ...}
  ],
  "route": {
    "rules": [
      {"action": "sniff"},
      {"protocol": "dns", "action": "hijack-dns"},
      {"clash_mode": "Direct", "outbound": "DIRECT", "action": "route"},
      {"clash_mode": "Global", "outbound": "PROXY", "action": "route"}
    ],
    "rule_set": [
      {"type": "remote", "tag": "geosite-cn", "format": "binary", "url": "...", "download_detour": "DIRECT", "update_interval": "1d"}
    ],
    "final": "PROXY",
    "auto_detect_interface": true,
    "default_domain_resolver": "def-0",
    "find_process": true
  },
  "experimental": {
    "clash_api": {
      "external_controller": "127.0.0.1:9090",
      "default_mode": "Rule"
    },
    "cache_file": {
      "enabled": true,
      "store_fakeip": true,
      "store_dns": true
    }
  }
}
```

---

## H. 翻译器代码架构

```
translator/
├── detect.go          自动检测 YAML/JSON 格式
├── translator.go      主翻译入口，协调各子模块
├── types.go           mihomo/sing-box 数据结构定义
├── general.go         全局配置 → inbound/route/log/experimental
├── proxy/
│   ├── proxy.go       代理协议分发
│   ├── vless.go       VLESS 翻译
│   ├── vmess.go       VMess 翻译
│   ├── trojan.go      Trojan 翻译
│   ├── shadowsocks.go Shadowsocks 翻译
│   ├── hysteria2.go   Hysteria2 翻译
│   ├── wireguard.go   WireGuard 翻译
│   ├── tuic.go        TUIC 翻译
│   ├── http.go        HTTP Proxy 翻译
│   ├── socks.go       SOCKS5 翻译
│   └── tls.go         TLS/REALITY/uTLS 公共翻译
├── group.go           代理组 → selector/urltest
├── rule.go            规则翻译（autoroute 自动地理分流，忽略用户 rules）
├── dns.go             DNS 翻译
└── tun.go             TUN 翻译
```

输入: mihomo YAML 文件路径
输出: sing-box JSON 字节流 + warnings 列表（不支持的字段/协议）

---

## I. 已知限制与不兼容项

1. **proxy-provider / rule-provider 动态加载**：mihomo 支持运行时动态加载代理集和规则集，sing-box 的 rule_set 需要预先声明。当前实现**不支持 provider 拉取/展开**：`proxy-providers`、`rule-providers` 被整体忽略并输出 warning，用户须改用内联 `proxies`，规则由自动地理分流替代（见 T.3）。

2. **sub-rules**：mihomo 支持子规则嵌套，sing-box 的 logical rule 可部分替代但语义不完全一致。

3. **fallback 代理组**：sing-box 无 fallback 类型，用 urltest 近似替代。

4. **load-balance 代理组**：sing-box 无对应，降级为 selector。

5. **DNS fallback-filter**：mihomo 基于 geoip/geosite 判断何时使用 fallback DNS，sing-box 需要显式 DNS 规则实现相同效果。

6. **SSR/Snell/SSH/Hysteria v1**：sing-box 不支持这些协议，翻译时跳过并输出警告。TUIC v4（token 认证）同样不支持，但 TUIC v5（uuid+password）完全支持（见 B.11）。

7. **WireGuard 结构**：sing-box 1.11+ 将 WireGuard 从 outbound 迁移为 endpoint，结构完全不同。

8. **单位转换**：mihomo 用纯数字（秒/Mbps），sing-box 用 Go Duration 字符串（"5m"/"30s"）。需统一转换。

9. **plugin_opts 格式**：mihomo Shadowsocks 的 plugin-opts 是对象，sing-box 的 plugin_opts 是 SIP003 字符串。需拼装转换。

10. **WireGuard IP 地址**：mihomo 用 `ip: 172.16.0.2`，sing-box 用 CIDR `address: ["172.16.0.2/32"]`。需自动加后缀。

---

## J. 补全：通用 Dial Fields 完整映射

以下字段适用于 sing-box 所有 outbound 和 inbound 的 dial/listen 配置。

### J.1 出站 Dial Fields（所有 outbound 共享）

| mihomo 字段 | sing-box 字段 | 翻译说明 |
|---|---|---|
| `dialer-proxy: "ss1"` | `detour: "ss1"` | **字段名不同** |
| `interface-name: eth0` | `bind_interface: "eth0"` | **字段名不同** |
| `routing-mark: 1234` | `routing_mark: 1234` | 直接 |
| `ip-version: ipv4` | `domain_resolver: {strategy: "ipv4_only"}` | 结构不同 |
| `ip-version: ipv6` | `domain_resolver: {strategy: "ipv6_only"}` | |
| `ip-version: dual` | `domain_resolver: {strategy: "prefer_ipv4"}` | |
| `ip-version: ipv4-prefer` | `domain_resolver: {strategy: "prefer_ipv4"}` | |
| `ip-version: ipv6-prefer` | `domain_resolver: {strategy: "prefer_ipv6"}` | |
| `tfo: true` | `tcp_fast_open: true` | **字段名不同** |
| `mptcp: true` | `tcp_multi_path: true` | **字段名不同** |
| (无) | `connect_timeout: "5s"` | mihomo 无此字段，使用默认值 |
| (无) | `reuse_addr: false` | mihomo 无此字段 |
| (无) | `udp_fragment: false` | mihomo 无此字段 |
| (无) | `disable_tcp_keep_alive: false` | mihomo 无此字段 |
| (无) | `tcp_keep_alive: "1m"` | mihomo 无此字段 |
| (无) | `tcp_keep_alive_interval: "30s"` | 对应 mihomo `keep-alive-interval` |
| (无) | `bind_address_no_port: false` | Linux 专用 |
| (无) | `netns: ""` | Linux 网络命名空间 |
| (无) | `domain_resolver: "dns-tag"` | 域名解析器标签或对象 |
| (无) | `network_strategy: "default"` | 1.11.0+ 网络策略 |
| (无) | `network_type: ["wifi"]` | 1.11.0+ 网络类型 |
| (无) | `fallback_network_type: ["cellular"]` | 1.11.0+ 回退网络 |
| (无) | `fallback_delay: "300ms"` | 1.11.0+ 回退延迟 |

### J.2 入站 Listen Fields（所有 inbound 共享）

| mihomo 对应 | sing-box 字段 | 说明 |
|---|---|---|
| `allow-lan` + `bind-address` | `listen` | 绑定地址 |
| 端口字段 | `listen_port` | 监听端口 |
| `authentication: ["user:pass"]` | `users: [{username:"user", password:"pass"}]` | **格式不同** |
| (无) | `tcp_fast_open: false` | |
| (无) | `tcp_multi_path: false` | |
| (无) | `udp_fragment: false` | |
| (无) | `udp_timeout: "5m"` | UDP 空闲超时 |
| (无) | `domain_resolver: ""` | 入站域名解析 |
| (无) | `detour: ""` | 转发到可注入入站 |
| `skip-auth-prefixes` | (无对应) | sing-box 通过 users 管理 |
| `lan-allowed-ips` | (无对应) | 通过 route rules 实现 |
| `lan-disallowed-ips` | (无对应) | 通过 route rules 实现 |

---

## K. 补全：TLS 完整字段映射

### K.1 TLS 基础字段

| mihomo | sing-box | 翻译说明 |
|---|---|---|
| `tls: true` | `tls.enabled: true` | 嵌套到 tls 对象 |
| `sni: xxx` (trojan) | `tls.server_name: "xxx"` | 统一 |
| `servername: xxx` (vmess/vless) | `tls.server_name: "xxx"` | 统一 |
| `skip-cert-verify: true` | `tls.insecure: true` | **字段名不同** |
| `alpn: [h2, http/1.1]` | `tls.alpn: ["h2", "http/1.1"]` | 直接 |
| `fingerprint: xxxx` (SHA256) | `tls.certificate_public_key_sha256: ["xxxx"]` | **格式不同，是数组** |
| `certificate: xxxx` (mTLS) | `tls.client_certificate: ["xxxx"]` | **格式不同，是数组** |
| `private-key: xxx` (mTLS) | `tls.client_key: ["xxx"]` | **格式不同，是数组** |
| (无) | `tls.min_version: "1.2"` | sing-box 独有 |
| (无) | `tls.max_version: "1.3"` | sing-box 独有 |
| (无) | `tls.cipher_suites: [...]` | sing-box 独有 |
| (无) | `tls.curve_preferences: [...]` | sing-box 独有 |
| (无) | `tls.certificate: "PEM"` | CA 证书 |
| (无) | `tls.certificate_path: "/path"` | CA 证书路径 |
| (无) | `tls.disable_sni: false` | 不发送 SNI |
| (无) | `tls.fragment: false` | TLS 片段（反审查） |
| (无) | `tls.fragment_fallback_delay: "100ms"` | TLS 片段回退 |
| (无) | `tls.record_fragment: false` | TLS 记录片段 |

### K.2 uTLS 指纹映射

| mihomo `client-fingerprint` | sing-box `tls.utls.fingerprint` | 说明 |
|---|---|---|
| `chrome` | `"chrome"` | 直接 |
| `firefox` | `"firefox"` | 直接 |
| `safari` | `"safari"` | 直接 |
| `ios` | `"ios"` | 直接 |
| `android` | `"android"` | 直接 |
| `edge` | `"edge"` | 直接 |
| `360` | `"360"` | 直接 |
| `qq` | `"qq"` | 直接 |
| `random` | `"random"` | 直接 |
| (无) | `"randomized"` | sing-box 独有 |

### K.3 REALITY 完整字段

| mihomo `reality-opts` | sing-box `tls.reality` | 翻译说明 |
|---|---|---|
| `public-key: xxxx` | `public_key: "xxxx"` | 直接 |
| `short-id: xxxx` | `short_id: "xxxx"` | 直接 |
| `support-x25519mlkem768: true` | (无对应) | sing-box 不支持 |

### K.4 ECH (Encrypted Client Hello) 完整字段

| mihomo `ech-opts` | sing-box `tls.ech` | 翻译说明 |
|---|---|---|
| `enable: true` | `ech.enabled: true` | 直接 |
| `config: base64_encoded` | `ech.config: ["base64_encoded"]` | **格式不同，是数组** |
| (无) | `ech.config_path: "/path"` | sing-box 可从文件读取 |
| `query-server-name: xxx.com` | `ech.query_server_name: "xxx.com"` | 直接 |

### K.5 全局客户端指纹

| mihomo | sing-box | 说明 |
|---|---|---|
| `global-client-fingerprint: chrome` | (无全局设置) | 需在每个 outbound 的 `tls.utls` 中单独设置 |

---

## L. 补全：Multiplex Brutal-Opts 完整映射

### L.1 Multiplex 基础字段

| mihomo `smux` | sing-box `multiplex` | 翻译说明 |
|---|---|---|
| `enabled: true` | `enabled: true` | 直接 |
| `protocol: h2mux` | `protocol: "h2mux"` | smux/yamux/h2mux 直接映射 |
| `max-connections: 4` | `max_connections: 4` | 直接 |
| `min-streams: 4` | `min_streams: 4` | 直接 |
| `max-streams: 0` | `max_streams: 0` | 直接 |
| `statistic: false` | (无对应) | 忽略 |
| `only-tcp: false` | (无对应) | 忽略 |
| `padding: true` | `padding: true` | 直接 |

### L.2 Brutal-Opts 子字段

| mihomo `smux.brutal-opts` | sing-box `multiplex.brutal` | 翻译说明 |
|---|---|---|
| `enabled: true` | `brutal.enabled: true` | 嵌套 |
| `up: 50` (Mbps) | `brutal.up_mbps: 50` | **需解析单位** |
| `up: "50 Mbps"` (字符串) | `brutal.up_mbps: 50` | 解析字符串提取数字 |
| `down: 100` (Mbps) | `brutal.down_mbps: 100` | **需解析单位** |

带宽单位解析规则（mihomo）：
- 纯数字 → 默认 Mbps
- `"50 Mbps"` 或 `"50 mbps"` → 50 Mbps
- `"50"` → 50 Mbps
- sing-box 只接受纯数字 Mbps（`up_mbps`/`down_mbps`）

---

## M. 补全：Shadowsocks Cipher 方法完整对照

### M.1 通用方法

| mihomo cipher | sing-box method | 状态 |
|---|---|---|
| `aes-128-gcm` | `aes-128-gcm` | 直接映射 |
| `aes-192-gcm` | `aes-192-gcm` | 直接映射 |
| `aes-256-gcm` | `aes-256-gcm` | 直接映射 |
| `chacha20-ietf-poly1305` | `chacha20-ietf-poly1305` | 直接映射 |
| `xchacha20-ietf-poly1305` | `xchacha20-ietf-poly1305` | 直接映射 |
| `chacha20-ietf` | (不支持) | 跳过 |
| `chacha20` | (不支持) | 跳过 |
| `xchacha20` | (不支持) | 跳过 |
| `aes-128-cfb` | (不支持) | 跳过 |
| `aes-192-cfb` | (不支持) | 跳过 |
| `aes-256-cfb` | (不支持) | 跳过 |
| `aes-128-ctr` | (不支持) | 跳过 |
| `aes-192-ctr` | (不支持) | 跳过 |
| `aes-256-ctr` | (不支持) | 跳过 |
| `rc4-md5` | (不支持) | 跳过 |
| `none` | (不支持) | 跳过 |

### M.2 2022 Blake3 方法

| mihomo cipher | sing-box method | 状态 |
|---|---|---|
| `2022-blake3-aes-128-gcm` | `2022-blake3-aes-128-gcm` | 直接映射 |
| `2022-blake3-aes-256-gcm` | `2022-blake3-aes-256-gcm` | 直接映射 |
| `2022-blake3-chacha20-poly1305` | `2022-blake3-chacha20-poly1305` | 直接映射 |

### M.3 mihomo 独有方法（sing-box 不支持）

| mihomo cipher | 状态 | 说明 |
|---|---|---|
| `aes-128-gcm-siv` | 跳过 | |
| `aes-256-gcm-siv` | 跳过 | |
| `aes-128-ccm` / `aes-192-ccm` / `aes-256-ccm` | 跳过 | |
| `lea-128-gcm` / `lea-192-gcm` / `lea-256-gcm` | 跳过 | LEA 韩国标准 |
| `rabbit128-poly1305` | 跳过 | |
| `aegis-128l` / `aegis-256` | 跳过 | |
| `aez-384` | 跳过 | |
| `deoxys-ii-256-128` | 跳过 | |
| `chacha8-ietf-poly1305` / `xchacha8-ietf-poly1305` | 跳过 | |

### M.4 Shadowsocks UDP-over-TCP

| mihomo | sing-box | 翻译说明 |
|---|---|---|
| `udp-over-tcp: true` | `udp_over_tcp: true` | 直接 |
| `udp-over-tcp-version: 2` | `udp_over_tcp: {version: 2}` | **简单→对象** |

---

## N. 补全：DNS 完整字段映射

### N.1 DNS 服务器类型映射

mihomo 使用 URL 字符串指定 DNS 类型，sing-box 使用类型化对象（1.12.0+）：

| mihomo URL 格式 | sing-box type | 说明 |
|---|---|---|
| `114.114.114.114` | `{type:"udp", server:"114.114.114.114"}` | 纯 IP → UDP |
| `https://doh.pub/dns-query` | `{type:"https", server:"doh.pub", path:"/dns-query"}` | HTTPS/DoH |
| `tls://8.8.4.4` | `{type:"tls", server:"8.8.4.4"}` | TLS/DoT |
| `quic://dns.adguard.com` | `{type:"quic", server:"dns.adguard.com"}` | QUIC/DoQ |
| `h3://dns.google/dns-query` | `{type:"h3", server:"dns.google", path:"/dns-query"}` | HTTP/3/DoH3 |
| `system` | `{type:"local"}` | 系统本地解析 |
| `dhcp://eth0` | `{type:"dhcp", interface:"eth0"}` | DHCP |
| `fakeip` | `{type:"fakeip", inet4_range:"198.18.0.0/15"}` | FakeIP |
| `rcode://success` | `{type:"rcode", rcode:"success"}` | 预设响应 |

### N.2 DNS 服务器附加参数（mihomo 用 `#` 附加）

mihomo DNS URL 可用 `#` 附加参数：
```
https://8.8.8.8/dns-query#proxy&ecs=1.1.1.1/24&ecs-override=true
```

| mihomo 参数 | sing-box 对应 | 翻译说明 |
|---|---|---|
| `#proxy` | `detour: "proxy-tag"` | 通过指定代理连接 |
| `#RULES` | (通过 DNS rules 实现) | 遵守路由规则 |
| `&h3` | `type: "h3"` | 强制 HTTP/3 |
| `&skip-cert-verify` | `tls.insecure: true` | 跳过证书验证 |
| `&ecs=<addr>` | `client_subnet: "addr"` | EDNS Client Subnet |
| `&ecs-override` | (无对应) | 忽略 |
| `&disable-ipv4` | `strategy: "ipv6_only"` | 丢弃 A 记录 |
| `&disable-ipv6` | `strategy: "ipv4_only"` | 丢弃 AAAA 记录 |
| `&disable-qtype-<int>` | (无对应) | 忽略 |

### N.3 DNS Legacy 格式字段（sing-box 1.12.0 前兼容）

翻译器应生成新格式，但了解 legacy 格式有助于兼容：

| sing-box legacy 字段 | 新格式 (1.12.0+) | 说明 |
|---|---|---|
| `address: "tls://1.1.1.1"` | `{type:"tls", server:"1.1.1.1"}` | URL → 类型化 |
| `address_resolver: "dns-tag"` | `domain_resolver: "dns-tag"` | 字段重命名 |
| `address_strategy: "prefer_ipv4"` | `domain_resolver: {strategy:"prefer_ipv4"}` | 嵌套结构 |
| `detour: "proxy-tag"` | dial fields `detour` | 位置变化 |
| `strategy: "ipv4_only"` | (移到 dns.rules 中) | 位置变化 |

### N.4 DNS 完整配置映射

| mihomo `dns` | sing-box `dns` | 翻译说明 |
|---|---|---|
| `enable: true` | (始终启用) | sing-box DNS 由 route 驱动 |
| `listen: 0.0.0.0:1053` | (无直接对应) | sing-box 通过 route 处理 DNS |
| `ipv6: false` | `strategy: "prefer_ipv4"` | 策略映射 |
| `prefer-h3: false` | 用 `type:"h3"` 替代 `type:"https"` | 需生成不同类型 |
| `use-hosts: true` | 控制 hosts DNS server 是否生成 | `false` 时不生成 hosts DNS server 和对应规则 |
| `use-system-hosts: true` | (无对应) | sing-box 不读取系统 hosts 文件 |
| `hosts: {domain: ip}` | `dns.servers[].{type:"hosts", predefined:{domain:ip}}` + DNS rules | mihomo hosts 值支持**字符串**（单个 IP）或**数组**（多个 IP，如 `[223.5.5.5, 2400:3200::1]`），均透传到 sing-box hosts DNS server 的 `predefined` 字段（sing-box 原生支持多 IP） |
| `respect-rules: false` | (通过 DNS rules 实现) | 需显式规则 |
| `enhanced-mode: fake-ip` | 添加 `type:"fakeip"` server | FakeIP 是服务器类型 |
| `enhanced-mode: redir-host` | 不添加 fakeip server | 标准 DNS 模式 |
| `fake-ip-range: 198.18.0.1/16` | fakeip server `inet4_range: "198.18.0.0/15"` | **CIDR 可能需调整** |
| `fake-ip-range6: fdfe:dcba:9876::1/64` | fakeip server `inet6_range: "fdfe:dcba:9876::/64"` | 同上 |
| `fake-ip-filter-mode: blacklist` | (sing-box 默认行为) | blacklist 模式 |
| `fake-ip-filter-mode: whitelist` | DNS rule 中反转匹配 | 需额外规则 |
| `fake-ip-filter: [...]` | DNS rules: domain 匹配后路由到非 fakeip | 通过规则排除 |
| `fake-ip-ttl: 1` | (无对应) | sing-box FakeIP 缓存由 cache_file 管理 |
| `default-nameserver: [...]` | 独立 DNS server，用于解析其他 server 域名 | 语义相同 |
| `nameserver: [...]` | `dns.servers` 中对应条目 | 主 DNS |
| `fallback: [...]` | `dns.servers` 中对应条目 + DNS rules | 需要 rules 路由 |
| `nameserver-policy: {...}` | `dns.rules` 中 domain 匹配 | 策略→规则 |
| `proxy-server-nameserver: [...]` | DNS rule: 匹配代理域名 → 路由到该 server | 规则路由 |
| `direct-nameserver: [...]` | DNS rule: 匹配直连域名 → 路由到该 server | 规则路由 |
| `direct-nameserver-follow-policy: false` | (无对应) | 忽略 |
| `cache-algorithm: lru` | (默认 LRU) | 一致 |
| `cache-algorithm: arc` | (不支持) | 使用默认 LRU |

### N.5 DNS FakeIP Filter 翻译策略

mihomo:
```yaml
dns:
  enhanced-mode: fake-ip
  fake-ip-filter:
    - '*.lan'
    - 'localhost.ptlogin2.qq.com'
  nameserver:
    - https://doh.pub/dns-query
```

sing-box 等价：
```json
{
  "dns": {
    "servers": [
      {"tag": "fakeip", "type": "fakeip", "inet4_range": "198.18.0.0/15"},
      {"tag": "doh-pub", "type": "https", "server": "doh.pub"}
    ],
    "rules": [
      {"domain_suffix": [".lan"], "server": "doh-pub"},
      {"domain": ["localhost.ptlogin2.qq.com"], "server": "doh-pub"}
    ],
    "final": "fakeip"
  }
}
```

---

## O. 补全：TUN 平台特有字段

以下字段已在 `translateTUN()` 中实现，与通用字段（节 F）共用同一翻译函数。

### O.1 Linux 特有

| mihomo tun | sing-box tun inbound | 翻译说明 | 状态 |
|---|---|---|---|
| `auto-redirect: true` | `auto_redirect: true` | 仅 `true` 时写入 | ✅ |
| (无) | `auto_redirect_input_mark` | mihomo 无对应，sing-box 特有 | — |
| (无) | `auto_redirect_output_mark` | 同上 | — |
| (无) | `auto_redirect_reset_mark` | 同上 | — |
| (无) | `auto_redirect_nfqueue` | 同上 | — |
| `iproute2-table-index` | `iproute2_table_index` | `> 0` 时写入 | ✅ |
| `iproute2-rule-index` | `iproute2_rule_index` | `> 0` 时写入 | ✅ |
| `include-uid` | `include_uid` | 空切片不写入 | ✅ |
| `include-uid-range` | `include_uid_range` | 空切片不写入 | ✅ |
| `exclude-uid` | `exclude_uid` | 空切片不写入 | ✅ |
| `exclude-uid-range` | `exclude_uid_range` | 空切片不写入 | ✅ |
| `gso` / `gso-max-size` | (1.11.0 后移除) | 忽略 | — |

### O.2 Android 特有

| mihomo tun | sing-box tun inbound | 翻译说明 | 状态 |
|---|---|---|---|
| `include-android-user` | `include_android_user` | 空切片不写入 | ✅ |
| `include-package` | `include_package` | 空切片不写入 | ✅ |
| `exclude-package` | `exclude_package` | 空切片不写入 | ✅ |

### O.3 macOS 特有

| mihomo tun | sing-box tun inbound | 翻译说明 | 状态 |
|---|---|---|---|
| `device: utun0` | `interface_name: "utun0"` | macOS 必须 `utun` 开头 | ✅ |

### O.4 TUN 嵌入平台选项 (sing-box)

sing-box tun inbound 支持 `platform` 对象（`TunPlatformOptions`），但**仅包含 `http_proxy` 子对象**，不包含 `auto_route` / `strict_route`。

`auto_route` 和 `strict_route` 是 TUN inbound 顶层字段，移动端通过 `libbox.TunOptions` 接口的 `GetAutoRoute()` / `GetStrictRoute()` 获取（来源是 `tun.Options`，非 platform）。

翻译器**不生成** `platform` 对象——mihomo 无 `http_proxy` 配置，无需映射。

---

## P. 补全：全局配置剩余字段

### P.1 认证相关

| mihomo | sing-box | 翻译说明 |
|---|---|---|
| `authentication: ["user1:pass1"]` | `inbound.users: [{username:"user1", password:"pass1"}]` | 格式转换 |
| `skip-auth-prefixes: ["127.0.0.1/8"]` | (无对应) | 忽略 |

### P.2 外部控制

| mihomo | sing-box | 翻译说明 |
|---|---|---|
| `external-controller: 127.0.0.1:9090` | `experimental.clash_api.external_controller: "127.0.0.1:9090"` | 直接 |
| `external-controller-tls: addr` | (无对应) | sing-box Clash API 不支持 TLS |
| `external-controller-unix: path` | (无对应) | 忽略 |
| `external-controller-cors` | (无对应) | 忽略 |
| `external-ui: /path` | `experimental.clash_api.external_ui: "/path"` | 直接 |
| `external-ui-name: xd` | `experimental.clash_api.external_ui_download_url` (映射) | 名称→URL |
| `external-ui-url: "https://..."` | `experimental.clash_api.external_ui_download_url` | 直接 |
| `external-doh-server: /dns-query` | (无对应) | 忽略 |
| `secret: "xxx"` | `experimental.clash_api.secret: "xxx"` | 直接 |

### P.3 缓存/Profile

| mihomo | sing-box | 翻译说明 |
|---|---|---|
| `profile.store-selected: true` | `experimental.cache_file.enabled: true` | 直接 |
| `profile.store-fake-ip: true` | `experimental.cache_file.store_fakeip: true` | 直接 |
| (无) | `experimental.cache_file.path: "cache.db"` | sing-box 独有 |
| (无) | `experimental.cache_file.store_dns: true` | DNS 缓存持久化（1.14.0 前为 `store_rdrc`） |

### P.4 GEO 数据

| mihomo | sing-box | 翻译说明 |
|---|---|---|
| `geodata-mode: false` (mmdb) | `rule_set format: "binary"` | sing-box 使用 rule_set binary |
| `geodata-mode: true` (dat) | `rule_set format: "source"` | |
| `geodata-loader: memconservative` | (无对应) | sing-box 自行管理内存 |
| `geo-auto-update: false` | `rule_set update_interval` | sing-box 通过 rule_set 配置 |
| `geo-update-interval: 24` | rule_set `update_interval: "24h"` | 小时→Duration |
| `geox-url.geoip: "https://..."` | `rule_set url` | 直接 |
| `geox-url.geosite: "https://..."` | `rule_set url` | 直接 |
| `geox-url.mmdb: "https://..."` | (rule_set 替代) | 不再直接用 mmdb |
| `geox-url.asn: "https://..."` | (rule_set 替代) | 不再直接用 |
| `global-ua: "clash.meta"` | (无对应) | sing-box 无全局 UA |
| `etag-support: true` | (无对应) | sing-box rule_set 自行管理 |

### P.5 TCP/连接相关

| mihomo | sing-box | 翻译说明 |
|---|---|---|
| `unified-delay: false` | (无对应) | 忽略 |
| `tcp-concurrent: false` | (无对应) | sing-box 有类似行为但不可配置 |
| `keep-alive-interval: 30` | dial `tcp_keep_alive_interval: "30s"` | **秒→Duration** |
| `keep-alive-idle: 7200` | dial `tcp_keep_alive: "2h"` | **秒→Duration** |
| `disable-keep-alive: false` | dial `disable_tcp_keep_alive: false` | 直接 |

### P.6 出站接口

| mihomo | sing-box | 翻译说明 |
|---|---|---|
| `interface-name: "eth0"` | route `default_interface: "eth0"` 或 dial `bind_interface` | 全局→路由级 |
| `routing-mark: 0` | route `default_mark: 0` | 全局→路由级 |

### P.7 NTP

| mihomo `ntp` | sing-box `ntp` | 翻译说明 |
|---|---|---|
| `enable: true` | (添加 ntp 顶层配置) | sing-box 也支持 ntp |
| `server: time.apple.com` | `ntp.server: "time.apple.com"` | 直接 |
| `port: 123` | `ntp.server_port: 123` | **字段名不同** |
| `interval: 30` | `ntp.interval: "30m"` | **分钟→Duration** |
| `write-to-system: true` | (无对应) | sing-box 不支持写系统时间 |

### P.8 Tunnels

| mihomo `tunnels` | sing-box | 翻译说明 |
|---|---|---|
| `tcp/udp,127.0.0.1:6553,114.114.114.114:53,proxy` | (无直接对应) | sing-box 无 tunnel 配置，需通过 route rules + direct outbound override 实现 |

---

## Q. 补全：代理组通用字段完整映射

### Q.1 所有代理组共享字段

| mihomo proxy-group | sing-box outbound group | 翻译说明 |
|---|---|---|
| `name: "xxx"` | `tag: "xxx"` | 直接 |
| `proxies: [...]` | `outbounds: [...]` | **字段名不同** |
| `use: [provider1]` | (无对应) | **当前忽略，不展开**（S 节为参考设计） |
| `url: "http://..."` | `url: "http://..."` | urltest 直接映射 |
| `interval: 300` | `interval: "5m"` | **秒→Duration** |
| `timeout: 5000` | (无对应) | 忽略 |
| `lazy: true` | `idle_timeout: "30m"` | 近似映射 |
| `max-failed-times: 5` | (无对应) | 忽略 |
| `disable-udp: true` | (无对应) | 忽略 |
| `interface-name: eth0` | dial `bind_interface` | 移到 dial fields |
| `routing-mark: 11451` | dial `routing_mark` | 移到 dial fields |
| `filter: "(?i)港\|hk"` | (无对应) | **当前忽略**：仅按 `proxies` 已知 tag 过滤 |
| `exclude-filter: "美\|日"` | (无对应) | **当前忽略**：仅按 `proxies` 已知 tag 过滤 |
| `exclude-type: "Shadowsocks\|Http"` | (无对应) | **当前忽略** |
| `include-all: true` | (无对应) | **当前忽略** |
| `include-all-proxies: true` | (无对应) | **当前忽略** |
| `include-all-providers: true` | (无对应) | **当前忽略** |
| `expected-status: 204` | (无对应) | 忽略 |
| `hidden: true` | (无对应) | UI 层面，翻译时忽略 |
| `icon: xxx` | (无对应) | UI 层面，翻译时忽略 |

### Q.2 Load-Balance 策略映射

| mihomo strategy | sing-box 对应 | 状态 |
|---|---|---|
| `consistent-hashing` | 无对应 | 降级为 selector |
| `round-robin` | 无对应 | 降级为 selector |
| `sticky-sessions` | 无对应 | 降级为 selector |

---

## R. 补全：单位转换规则汇总

翻译器需要统一的单位转换逻辑：

### R.1 时间单位（mihomo 秒数 → sing-box Go Duration）

| mihomo 值 | sing-box 值 | 规则 |
|---|---|---|
| `30` (秒) | `"30s"` | 秒→Duration |
| `300` (秒) | `"5m"` | 秒→分 (可选优化) |
| `7200` (秒) | `"2h"` | 秒→小时 (可选优化) |
| `30` (分钟, NTP interval) | `"30m"` | 分钟→Duration |
| `24` (小时, geo-update-interval) | `"24h"` | 小时→Duration |

转换函数：`secondsToDuration(seconds int) string`
- < 60: `fmt.Sprintf("%ds", seconds)`
- < 3600: `fmt.Sprintf("%dm", seconds/60)` (可选显示秒)
- >= 3600: `fmt.Sprintf("%dh", seconds/3600)`

### R.2 带宽单位（mihomo → sing-box Mbps）

| mihomo 值 | sing-box 值 | 规则 |
|---|---|---|
| `50` (纯数字, 默认 Mbps) | `50` | 直接 |
| `"50 Mbps"` | `50` | 解析字符串 |
| `"50 mbps"` | `50` | 大小写不敏感 |
| `"30 Mbps"` (Hysteria2) | `30` (up_mbps/down_mbps) | 解析字符串 |
| `"500 Kbps"` | `1` | Kbps→Mbps 向上取整（不低于 1） |
| `"1 Gbps"` | `1000` | Gbps→Mbps |

转换函数：`parseBandwidth(s string) int` (返回 Mbps)

### R.3 端口范围

| mihomo | sing-box | 规则 |
|---|---|---|
| `ports: "443-8443"` | `server_ports: ["443:8443"]` | `-` → `:` |
| `hop-interval: 30` | `hop_interval: "30s"` | 秒→Duration |

---

## S. 参考设计：proxy-provider 展开策略（当前未实现）

> **注意**：以下策略是**参考设计**，当前翻译器**未实现 provider 下载/展开**。`proxy-providers` 会被整体忽略并输出 warning（`warnUnsupportedRouting`），`use`、`include-all*`、正则 `filter` 等字段不生效。

mihomo 的 proxy-provider 允许运行时动态加载代理列表，sing-box 无此概念。若未来实现展开，翻译策略：

### S.1 静态展开流程（若实现）

```
1. 读取 mihomo config
2. 对每个 proxy-group:
   a. 收集 proxies 列表中的代理名
   b. 对 use 字段中引用的每个 provider:
      - 读取 provider 配置（type: http 时需先下载）
      - 从 provider 内容中提取所有代理
      - 应用 filter / exclude-filter / exclude-type 过滤
      - 将过滤后的代理名加入 outbounds 列表
   c. 如果 include-all/include-all-proxies 为 true:
      - 收集所有 proxies 中的代理
   d. 将最终列表写入 sing-box selector/urltest 的 outbounds
3. provider 中的代理本身也需要翻译并加入 sing-box outbounds
```

### S.2 HTTP Provider 处理

对于 `type: http` 的 provider：
- 若实现时：尝试下载 provider 内容
- 如果下载失败：输出 warning，该 group 的 outbounds 留空
- 下载成功后解析 YAML，提取代理列表

### S.3 provider 相关字段参考映射

| mihomo proxy-provider | sing-box | 参考策略（若实现） |
|---|---|---|
| `type: http` | (无对应) | 下载并展开 |
| `type: file` | (无对应) | 读取文件并展开 |
| `type: inline` | (无对应) | 直接解析 payload |
| `url` | (无对应) | 用于下载 |
| `path` | (无对应) | 用于缓存 |
| `interval` | (无对应) | 忽略 |
| `proxy` | (无对应) | 下载时的代理 |
| `health-check` | (无对应) | sing-box urltest 自行测试 |
| `override` | 应用到每个代理 | 覆盖代理字段 |
| `filter` / `exclude-filter` | 过滤 | 正则过滤代理名 |
| `exclude-type` | 过滤 | 按类型排除 |
| `payload` | 直接解析 | inline 内容 |
| `header` | 用于下载请求 | HTTP 头 |

---

## T. 独立实现功能（非翻译）

以下功能**不是从 mihomo 配置翻译而来**，而是 singcast 根据运行环境自动生成。这些功能的 mihomo 对应配置即使存在也会被忽略。

### T.1 嗅探器（Sniffer）

**mihomo 配置**：`sniffer.enable`、`sniffer.sniffing`、`sniffer.skip-dest`、`sniffer.force`、`sniffer.parse-pure-ip`、`sniffer.force-dns-mapping`

**singcast 处理**：完全忽略 mihomo 的 sniffer 配置，在 `assemble()` 中无条件注入以下 route rules：

```json
[
  {"action": "sniff"},
  {"protocol": "dns", "action": "hijack-dns"}
]
```

- `action: "sniff"` — 检测 HTTP Host、TLS SNI、QUIC SNI，提取真实域名用于路由匹配（geosite、domain 规则）
- `action: "hijack-dns"` — 将所有 DNS 查询劫持到 sing-box 的 DNS 管线，由 `generateDNSRules()` 生成的 DNS 规则根据 clash_mode 和 geosite/geoip 路由到正确的 DNS 服务器

**代码位置**：`translator/assemble.go`

### T.2 DNS 策略路由（nameserver-policy）

**mihomo 配置**：`nameserver-policy`（将域名模式映射到指定 DNS 服务器）

**singcast 处理**：`RawConfig.NameServerPolicy` 被解析但**不翻译**。翻译器在 `autoroute.go` 的 `generateDNSRules()` 中独立实现等效功能——基于检测到的国家代码自动生成 geo-based DNS 路由规则：

| 条件 | DNS 服务器 |
|------|-----------|
| `clash_mode: "Direct"` | 国内 DNS（local 类型） |
| 国内 geosite + geoip | 国内 DNS（local 类型） |
| A/AAAA 查询 | FakeIP 服务器（如启用） |
| 其他 | DNS final 服务器 |

这与 mihomo 的 nameserver-policy 不同：mihomo 由用户显式映射模式到服务器，singcast 基于 geosite/geoip 自动生成。

**代码位置**：`translator/autoroute.go`

### T.3 自动路由规则

**mihomo 配置**：mihomo 的 `rules` 由用户手动指定

**singcast 处理**：`autoroute.go` 的 `translateRules()` 基于检测到的国家代码自动生成路由规则，**替代用户定义的 rules**（用户的 `rules` 被整体忽略并输出 warning）：

**CN 用户**（`generateCNRoutes`）：

| 规则 | 出站 |
|------|------|
| 私有 IP | DIRECT |
| geosite-private（内网/本地域名） | DIRECT |
| overseas-ai rule_set | 代理组 |
| geosite-microsoft@cn | DIRECT |
| geosite-steam@cn | DIRECT |
| geosite-category-games@cn | DIRECT |
| geosite-onedrive | DIRECT |
| geosite-geolocation-!cn | 代理组 |
| geosite-cn | DIRECT |
| geoip-cn | DIRECT |
| .cn 域名后缀 | DIRECT |

**非 CN 用户**（`generateCountryRoutes`）：

| 规则 | 出站 |
|------|------|
| 私有 IP | DIRECT |
| geosite-private（内网/本地域名） | DIRECT |
| geoip-{cc} | DIRECT |
| .{cc} 域名后缀（通用 ccTLD 跳过） | DIRECT |

> **注意**：官方 `SagerNet/sing-geosite` 的 rule-set 按分类提供（如 `cn`、`geolocation-!cn`、品牌/服务），不提供按国家代码的 `geosite-{cc}` 文件（例如 `geosite-bd.srs` 不存在，见 issue #69）。因此非 CN 用户只使用官方完整覆盖的 `geoip-{cc}` 与国别顶级域名后缀做“本国直连”，不再拼接 `geosite-{cc}`。被当通用域名使用的 ccTLD（`io/tv/ai/me/cc/co/ly/to/ws/sh/gg/je/fm/am/la`）与 Freenom 免费域名（`tk/cf/ga/gq/ml`）不会生成后缀直连，避免误放行外国流量。

**代码位置**：`translator/autoroute.go`

### T.4 Clash 模式条件

**mihomo 配置**：`mode: rule/global/direct` 控制整体行为

**singcast 处理**：`assemble()` 中的 `addClashModeCondition()` 为所有有 outbound 的 route rule 自动添加 `clash_mode: "Rule"` 条件，并在路由最前面插入 Direct/Global 兜底规则：

```json
[
  {"clash_mode": "Direct", "outbound": "DIRECT", "action": "route"},
  {"clash_mode": "Global", "outbound": "PROXY", "action": "route"}
]
```

这确保 Rule 模式下用户的规则生效，切换到 Direct/Global 时立即接管所有流量。

**代码位置**：`translator/assemble.go`

### T.5 REJECT 处理

**mihomo 配置**：`REJECT` 作为特殊 outbound

**sing-box 变更**：sing-box 1.13.0 移除了 `block` outbound 类型

**singcast 处理**：`assemble()` 中的 `convertRejectActions()` 将所有 `outbound: "REJECT"` 替换为 `action: "reject"` route rule action，不再生成 `{type: "block", tag: "REJECT"}` outbound。

**代码位置**：`translator/assemble.go`

### T.6 DNS 服务器 detour 自动设置

**mihomo 行为**：DNS 服务器（nameserver/fallback）直连

**singcast 处理**：`translateDNS()` 中自动为 nameserver/fallback 服务器设置 `detour` 字段指向第一个代理组。原因：在 GFW 环境下，外国 DoH/DoT 服务器（Cloudflare、Google）必须通过代理才能访问，否则 DNS 解析失败。

```json
// nameserver/fallback DNS 服务器自动添加 detour
{"type": "https", "tag": "ns-0", "server": "dns.google", "detour": "PROXY"}
```

default-nameserver 和 proxy-server-nameserver 不设置 detour（它们使用国内 IP 直连 DNS）。

**代码位置**：`translator/dns.go`

### T.7 DNS 域名解析器（default_domain_resolver）

**mihomo 行为**：`default-nameserver` 隐式用于解析其他 DNS 服务器的域名

**singcast 处理**：sing-box 要求显式设置 `route.default_domain_resolver` 解决 DNS 鸡生蛋问题（代理服务器域名需要 DNS 解析，但 DNS 服务器本身可能也是域名）。翻译器自动从 default-nameserver 中优先选择 IP-based UDP 类型服务器作为 `default_domain_resolver`，同时为所有域名地址的 DNS 服务器设置 `domain_resolver` 字段避免循环依赖。

```
dns.servers[].server = "dns.google"  (域名)
dns.servers[].domain_resolver = "def-0"  (指向 IP-based UDP 服务器)

route.default_domain_resolver = "def-0"
```

**代码位置**：`translator/dns.go`

### T.8 rule_set 默认参数注入

**mihomo 行为**：rule-provider 各自配置 path、interval 等

**singcast 处理**：`rule.go` 的 `registerRuleSet()` 为所有自动生成的 rule_set 定义统一注入默认参数：

| 字段 | 值 | 说明 |
|------|-----|------|
| `download_detour` | `"DIRECT"` | 规则集下载走直连，避免通过不可达的代理形成环路 |
| `update_interval` | `"1d"` | 默认每天更新一次 |
| `format` | `"binary"` | 使用 sing-box binary 格式（更高效） |
| `type` | `"remote"` | 所有自动注册的 rule_set 都是远程类型 |

`download_detour: "DIRECT"` 是关键安全措施——如果 rule_set 下载经过代理，而代理服务器本身需要 rule_set 规则才能正确路由，就会形成循环依赖。用户在 GFW 环境下应使用 `--rule-set-proxy` 进行 URL 级别代理（如 gh-proxy.org 镜像），而非让下载走代理 outbound。

自动注册的 rule_set 全部来自 autoroute 生成清单（见 T.3）：官方 `SagerNet/sing-geoip`、`SagerNet/sing-geosite` 及 `overseas-ai` 第三方列表；`--rule-set-proxy` 只对 `raw.githubusercontent.com` 开头的 URL 做前缀拼接（`rule.go` 的 `ProxyURL`）。

**代码位置**：`translator/rule.go`
