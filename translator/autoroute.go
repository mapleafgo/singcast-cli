package translator

import "strings"

// translateRules generates geo-based auto-routing rules based on the detected country.
func translateRules(_ *RawConfig, t *translation) {
	proxyTag := firstGroupTag(t)
	if proxyTag == "" {
		proxyTag = "DIRECT"
	}

	var cc string
	if t.opts != nil && t.opts.Country != "" {
		cc = strings.ToLower(strings.TrimSpace(t.opts.Country))
	}
	if len(cc) != 2 {
		cc = strings.ToLower(DetectCountry(""))
	}

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

// generateCNRoutes builds CN routing rules following sing-box best practices.
// DNS pollution is handled at the DNS layer (direct DNS for Chinese domains),
// so route rules use simple geosite/geoip matching without logical guards.
func generateCNRoutes(proxyTag string, t *translation) {
	// overseas-ai → proxy
	ensureCustomRuleSetDef("overseas-ai", "https://raw.githubusercontent.com/viewer12/OverseasAI.list/main/rule/Singbox/OverseasAI/OverseasAI.srs", t)
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

	directTag := findFirstDirectDNSTag(t)
	fakeipTag := findFakeIPTag(t)

	if directTag == "" {
		t.warn("no direct DNS server found; DNS rules not generated")
		return
	}

	var rules []map[string]any

	// Direct mode → domestic DNS
	rules = append(rules, map[string]any{
		"clash_mode": "Direct",
		"server":     directTag,
	})

	// ECH query-server-name → domestic DNS (no detour).
	// sing-box fetches ECH config via DNS HTTPS query to this domain.
	// Without a direct DNS rule, the query falls through to the final DNS
	// server which may detour through a proxy — creating a circular
	// dependency (proxy → ECH → DNS → proxy) that causes 3-minute timeouts.
	if len(t.echQueryServers) > 0 {
		rules = append(rules, map[string]any{
			"domain": t.echQueryServers,
			"server": directTag,
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

	// FakeIP for A/AAAA queries
	if fakeipTag != "" {
		rules = append(rules, map[string]any{
			"query_type": []string{"A", "AAAA"},
			"server":     fakeipTag,
		})
	}

	// Prepend our rules before any existing rules (e.g., fakeip-filter, hosts)
	t.config.DNS.Rules = append(rules, t.config.DNS.Rules...)
}
