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
		generateCountryRoutes(cc, proxyTag, t)
	}

	// DNS rule generated after translateDNS (Step 6) when t.config.DNS is available.
}

// generateCNRoutes builds CN routing rules for GFW bypass with DNS pollution guards.
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

	// geosite-cn domain resolving to non-CN IP → proxy (DNS pollution guard)
	ensureRuleSetDef("geosite-cn", "geosite", "cn", t)
	ensureRuleSetDef("geoip-cn", "geoip", "cn", t)
	t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
		"type": "logical",
		"mode": "and",
		"rules": []map[string]any{
			{"rule_set": []string{"geosite-cn"}},
			{"rule_set": []string{"geoip-cn"}, "invert": true},
		},
		"outbound": proxyTag,
	})

	// geosite-cn → direct
	t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
		"rule_set": []string{"geosite-cn"},
		"outbound": "DIRECT",
	})

	// Non-!cn domain resolving to CN IP → direct (inverted !cn guards DNS pollution)
	t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
		"type": "logical",
		"mode": "and",
		"rules": []map[string]any{
			{"rule_set": []string{"geosite-geolocation-!cn"}, "invert": true},
			{"rule_set": []string{"geoip-cn"}},
		},
		"outbound": "DIRECT",
	})

	// .cn domain suffix → direct (fallback)
	t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
		"domain_suffix": []string{".cn"},
		"outbound":      "DIRECT",
	})
}

// generateCountryRoutes builds non-CN routing rules: local traffic → direct, rest → proxy.
func generateCountryRoutes(cc string, proxyTag string, t *translation) {
	// geosite-{cc} domain resolving to non-{cc} IP → proxy (CDN mismatch guard)
	ensureRuleSetDef("geosite-"+cc, "geosite", cc, t)
	ensureRuleSetDef("geoip-"+cc, "geoip", cc, t)
	t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
		"type": "logical",
		"mode": "and",
		"rules": []map[string]any{
			{"rule_set": []string{"geosite-" + cc}},
			{"rule_set": []string{"geoip-" + cc}, "invert": true},
		},
		"outbound": proxyTag,
	})

	// geosite-{cc} → direct
	t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
		"rule_set": []string{"geosite-" + cc},
		"outbound": "DIRECT",
	})

	// geoip-{cc} → direct
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

// generateDNSRules creates DNS rules following sing-box best practices.
// Must be called after translateDNS so DNS servers are available.
//
// CN rules (in order):
//  1. outbound:"any" → remote DNS (resolve proxy server domains)
//  2. clash_mode:"Direct" → direct DNS
//  3. geosite-cn → direct DNS (domestic domains)
//  4. geoip-cn → direct DNS (domestic IPs)
//  5. query_type:A/AAAA → fakeip (if enabled)
//
// Non-CN rules:
//  1. clash_mode:"Direct" → direct DNS
//  2. geosite-{cc} → direct DNS
//  3. geoip-{cc} → direct DNS
//  4. query_type:A/AAAA → fakeip (if enabled)
func generateDNSRules(cc string, t *translation) {
	if t.config.DNS == nil || len(t.config.DNS.Servers) == 0 {
		return
	}

	directTag := findFirstDirectDNSTag(t)
	remoteTag := findFirstRemoteDNSTag(t)
	fakeipTag := findFakeIPTag(t)

	if directTag == "" {
		t.warn("no direct DNS server found; DNS rules not generated")
		return
	}

	var rules []map[string]any

	if cc == "cn" && remoteTag != "" {
		// Resolve proxy server domains via remote DNS
		rules = append(rules, map[string]any{
			"outbound": "any",
			"server":   remoteTag,
		})
	}

	// Direct mode → domestic DNS
	rules = append(rules, map[string]any{
		"clash_mode": "Direct",
		"server":     directTag,
	})

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
