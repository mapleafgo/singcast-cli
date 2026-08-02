package translator

import "fmt"

// translateRules generates geo-based auto-routing rules based on the detected country.
// 注意：本函数用自动地理分流整体替代用户 rules，被替代的配置项由
// warnUnsupportedRouting 逐项告知用户。
func translateRules(cfg *RawConfig, t *translation) {
	warnUnsupportedRouting(cfg, t)

	proxyTag := firstGroupTag(t)
	if proxyTag == "" {
		proxyTag = "DIRECT"
	}

	cc := detectCC(t)

	// Private IP → DIRECT
	t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
		"ip_is_private": true,
		"outbound":      "DIRECT",
	})

	if cc == "cn" {
		generateCNRoutes(proxyTag, t)
	} else {
		generateCountryRoutes(cc, t)
	}

	// DNS rule generated after translateDNS (Step 6) when t.config.DNS is available.
}

// warnUnsupportedRouting 告知用户哪些配置项没有被翻译。
// 本翻译器用自动地理分流替代 mihomo 的显式规则，并且不实现 provider 拉取；
// 这些配置项被整体忽略——静默丢弃会让用户面对"规则不生效"却无从排查。
func warnUnsupportedRouting(cfg *RawConfig, t *translation) {
	if cfg == nil {
		return
	}
	if n := len(cfg.Rule); n > 0 {
		t.warn(fmt.Sprintf("%d 条 rules 未翻译：已由自动地理分流替代", n))
	}
	if n := len(cfg.ProxyProvider); n > 0 {
		t.warn(fmt.Sprintf("%d 个 proxy-providers 未翻译：不支持订阅拉取，请改用内联 proxies", n))
	}
	if n := len(cfg.RuleProvider); n > 0 {
		t.warn(fmt.Sprintf("%d 个 rule-providers 未翻译：已由自动地理分流替代", n))
	}
}

// generateCNRoutes builds CN routing rules following sing-box best practices.
// DNS pollution is handled at the DNS layer (direct DNS for Chinese domains),
// so route rules use simple geosite/geoip matching without logical guards.
func generateCNRoutes(proxyTag string, t *translation) {
	// overseas-ai → proxy
	registerRuleSet("overseas-ai", "https://raw.githubusercontent.com/viewer12/OverseasAI.list/main/rule/Singbox/OverseasAI/OverseasAI.srs", t)
	t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
		"rule_set": []string{"overseas-ai"},
		"outbound": proxyTag,
	})

	// Non-CN geolocation → proxy (must precede geosite-cn)
	ensureRuleSetDef("geosite-geolocation-!cn", "geosite", "geolocation-!cn", t)
	t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
		"rule_set": []string{"geosite-geolocation-!cn"},
		"outbound": proxyTag,
	})

	// geosite-cn → direct
	ensureRuleSetDef("geosite-cn", "geosite", "cn", t)
	t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
		"rule_set": []string{"geosite-cn"},
		"outbound": "DIRECT",
	})

	// geoip-cn → direct
	ensureRuleSetDef("geoip-cn", "geoip", "cn", t)
	t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
		"rule_set": []string{"geoip-cn"},
		"outbound": "DIRECT",
	})

	// .cn domain suffix → direct (fallback)
	t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
		"domain_suffix": []string{".cn"},
		"outbound":      "DIRECT",
	})
}

// generateCountryRoutes builds non-CN routing rules: local traffic → direct, rest → proxy.
func generateCountryRoutes(cc string, t *translation) {
	// geosite-{cc} → direct
	ensureRuleSetDef("geosite-"+cc, "geosite", cc, t)
	t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
		"rule_set": []string{"geosite-" + cc},
		"outbound": "DIRECT",
	})

	// geoip-{cc} → direct
	ensureRuleSetDef("geoip-"+cc, "geoip", cc, t)
	t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
		"rule_set": []string{"geoip-" + cc},
		"outbound": "DIRECT",
	})

	// .{cc} domain suffix → direct (fallback)
	t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
		"domain_suffix": []string{"." + cc},
		"outbound":      "DIRECT",
	})
}

// generateDNSRules implements singcast's equivalent of mihomo's nameserver-policy.
// Instead of translating the user's nameserver-policy map (which is parsed but intentionally
// ignored), we generate geo-based DNS routing rules using sing-box rule_set.
//
// Must be called after translateDNS so DNS servers are available.
//
// Strategy: domestic geosite/geoip → direct DNS, everything else → fallback/fakeip.
// This differs from mihomo where users explicitly map patterns to servers.
func generateDNSRules(cc string, t *translation) {
	if t.config.DNS == nil || len(t.config.DNS.Servers) == 0 {
		return
	}

	// DNS 未启用时只输出 bootstrap DNS server 做 default_domain_resolver 兜底，
	// 不生成 geo-based DNS 路由规则——用户未启用 DNS，不应改变其 DNS 行为意图。
	if !t.dnsEnabled {
		return
	}

	directTag := findFirstDirectDNSTag(t)
	fakeipTag := findFakeIPTag(t)

	if directTag == "" {
		t.warn("no direct DNS server found; DNS rules not generated")
		// 兜底规则仍需落到规则表末尾，否则白名单模式下的默认 server 会整条丢失。
		t.config.DNS.Rules = append(t.config.DNS.Rules, t.dnsTerminalRules...)
		return
	}

	// modeRules 排在最前：clash_mode 是全局开关，优先级高于任何域名/地理匹配。
	modeRules := []map[string]any{{
		"clash_mode": "Direct",
		"server":     directTag,
	}}

	// autoRules 排在用户显式规则（hosts、fake-ip-filter）之后：那些是用户逐条
	// 指定的例外，必须先于自动分流生效。
	var rules []map[string]any

	// ECH query-server-name → direct DNS (no detour).
	// sing-box fetches ECH config via a DNS HTTPS (type 65) query to this domain.
	// Without a direct DNS rule the query falls through to the final DNS server,
	// which may detour through a proxy — creating a circular dependency
	// (proxy → ECH → DNS → proxy) that causes multi-minute timeouts.
	//
	// Prefer an encrypted server (DoH/DoQ/DoH3) over plain UDP: type 65 records are
	// more reliable over encrypted transports, and plain UDP is the most prone to the
	// intermittent interference that — because sing-box routes to a single server with
	// no failover — can stall every ECH outbound at once. See findFirstECHCapableDNSTag.
	if len(t.echQueryServers) > 0 {
		echTag := findFirstECHCapableDNSTag(t)
		if echTag == "" {
			echTag = directTag
		}
		rules = append(rules, map[string]any{
			"domain": t.echQueryServers,
			"server": echTag,
		})
	}

	// Domestic geosite + geoip → direct DNS
	geositeTag := "geosite-" + cc
	geoipTag := "geoip-" + cc
	ensureRuleSetDef(geositeTag, "geosite", cc, t)
	ensureRuleSetDef(geoipTag, "geoip", cc, t)
	rules = append(rules, map[string]any{
		"rule_set": []string{geositeTag, geoipTag},
		"server":   directTag,
	})

	// FakeIP 兜住其余 A/AAAA 查询。它匹配一切 A/AAAA，因此必须排在 hosts 与
	// fake-ip-filter 之后，否则那两者对 A/AAAA 查询将永远不生效。
	if fakeipTag != "" {
		rules = append(rules, map[string]any{
			"query_type": []string{"A", "AAAA"},
			"server":     fakeipTag,
		})
	}

	// 最终顺序：mode 开关 → 用户显式规则 → 自动分流 → 无条件兜底。
	ordered := make([]map[string]any, 0,
		len(modeRules)+len(t.config.DNS.Rules)+len(rules)+len(t.dnsTerminalRules))
	ordered = append(ordered, modeRules...)
	ordered = append(ordered, t.config.DNS.Rules...)
	ordered = append(ordered, rules...)
	ordered = append(ordered, t.dnsTerminalRules...)
	t.config.DNS.Rules = ordered
}
