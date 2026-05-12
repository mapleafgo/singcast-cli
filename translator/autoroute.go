package translator

import "strings"

// translateRules generates serenity-style auto-routing rules based on the detected country.
// The mihomo rules in cfg are NOT translated 1:1; instead, a fixed set of geo-based
// routing rules is generated for optimal behavior.
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

	// Private IP → DIRECT (use built-in ip_is_private field, no rule_set needed)
	t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
		"ip_is_private": true,
		"outbound":      "DIRECT",
	})

	generateGeoRoute(cc, proxyTag, t)
}

// generateGeoRoute creates country-specific route rules.
func generateGeoRoute(cc string, proxyTag string, t *translation) {
	if cc == "cn" {
		// overseas-ai → proxy (overseas AI services like ChatGPT, Claude, etc.)
		ensureCustomRuleSetDef("overseas-ai", "https://raw.githubusercontent.com/viewer12/OverseasAI.list/main/rule/Singbox/OverseasAI/OverseasAI.srs", t)
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"rule_set": []string{"overseas-ai"},
			"outbound": proxyTag,
		})

		// Non-CN geolocation → proxy (must come BEFORE cn rule so foreign domains
		// like gstatic.com are never accidentally caught by geolocation-cn).
		ensureRuleSetDef("geosite-geolocation-!cn", "geosite", "geolocation-!cn", t)
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"rule_set": []string{"geosite-geolocation-!cn"},
			"outbound": proxyTag,
		})

		// geolocation-cn domain resolving to non-CN IP → proxy.
		// Catches domains like gstatic.com that are in geolocation-cn but not in
		// geolocation-!cn, preventing DIRECT routing to blocked foreign IPs.
		ensureRuleSetDef("geosite-geolocation-cn", "geosite", "geolocation-cn", t)
		ensureRuleSetDef("geoip-cn", "geoip", "cn", t)
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"type": "logical",
			"mode": "and",
			"rules": []map[string]any{
				{"rule_set": []string{"geosite-geolocation-cn"}},
				{"rule_set": []string{"geoip-cn"}, "invert": true},
			},
			"outbound": proxyTag,
		})

		// CN geolocation → direct
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"rule_set": []string{"geosite-geolocation-cn"},
			"outbound": "DIRECT",
		})

		// Domains NOT in geolocation-!cn that resolve to CN IP → direct.
		// Uses logical AND to prevent DNS-polluted foreign domains from going direct:
		// if a domain is in geolocation-!cn (e.g. google.com), it won't match this rule
		// even when DNS pollution resolves it to a CN IP.
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"type": "logical",
			"mode": "and",
			"rules": []map[string]any{
				{"rule_set": []string{"geosite-geolocation-!cn"}, "invert": true},
				{"rule_set": []string{"geoip-cn"}},
			},
			"outbound": "DIRECT",
		})

		// .cn domain suffix → direct (fallback after all geo rules; catches .cn
		// domains not in any geosite rule set, e.g. pub-web.flutter-io.cn).
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"domain_suffix": []string{"." + cc},
			"outbound":      "DIRECT",
		})
	} else {
		// Non-CN geolocation → proxy (must come first so foreign domains
		// are never accidentally caught by country-specific rules).
		ensureRuleSetDef("geosite-geolocation-!cn", "geosite", "geolocation-!cn", t)
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"rule_set": []string{"geosite-geolocation-!cn"},
			"outbound": proxyTag,
		})

		// geosite-{cc} domain resolving to non-{cc} IP → proxy.
		// Prevents country-specific domains from going DIRECT when they
		// resolve to foreign IPs that may be unreachable (e.g. behind a firewall).
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

		// Domains NOT in geolocation-!cn that resolve to {cc} IP → direct.
		// Uses logical AND to prevent DNS-polluted foreign domains from going direct.
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"type": "logical",
			"mode": "and",
			"rules": []map[string]any{
				{"rule_set": []string{"geosite-geolocation-!cn"}, "invert": true},
				{"rule_set": []string{"geoip-" + cc}},
			},
			"outbound": "DIRECT",
		})

		// .{cc} domain suffix → direct (fallback after all geo rules).
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"domain_suffix": []string{"." + cc},
			"outbound":      "DIRECT",
		})
	}

	// Final route is set by assemble() → proxyTag (first group tag)
}
