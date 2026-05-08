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
		// CN geolocation → direct
		ensureRuleSetDef("geosite-geolocation-cn", "geosite", "geolocation-cn", t)
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"rule_set": []string{"geosite-geolocation-cn"},
			"outbound": "DIRECT",
		})

		// Domains NOT in geolocation-!cn that resolve to CN IP → direct.
		// Uses logical AND to prevent DNS-polluted foreign domains from going direct:
		// if a domain is in geolocation-!cn (e.g. google.com), it won't match this rule
		// even when DNS pollution resolves it to a CN IP.
		ensureRuleSetDef("geosite-geolocation-!cn", "geosite", "geolocation-!cn", t)
		ensureRuleSetDef("geoip-cn", "geoip", "cn", t)
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"type": "logical",
			"mode": "and",
			"rules": []map[string]any{
				{"rule_set": []string{"geosite-geolocation-!cn"}, "invert": true},
				{"rule_set": []string{"geoip-cn"}},
			},
			"outbound": "DIRECT",
		})
	} else {
		// Generic country: geoip-{cc} → direct
		ensureRuleSetDef("geoip-"+cc, "geoip", cc, t)
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"rule_set": []string{"geoip-" + cc},
			"outbound": "DIRECT",
		})

		// geosite-{cc} → direct
		ensureRuleSetDef("geosite-"+cc, "geosite", cc, t)
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"rule_set": []string{"geosite-" + cc},
			"outbound": "DIRECT",
		})
	}

	// Final route is set by assemble() → proxyTag (first group tag)
}
