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
