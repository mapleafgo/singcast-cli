package translator

import "strings"

func assemble(t *translation) {
	// Inject direct outbound (block and dns outbounds removed in sing-box 1.13.0,
	// replaced by route rule actions: "reject" and "hijack-dns")
	t.config.Outbounds = append([]map[string]any{
		{"type": "direct", "tag": "DIRECT"},
	}, t.config.Outbounds...)

	// Convert REJECT outbound references to reject rule actions
	convertRejectActions(t)

	// Add action:"route" to all rules with an outbound field (sing-box v1.13 requires action)
	addRouteAction(t)

	// Inject default route rules at the beginning
	defaultRules := []map[string]any{
		{"action": "sniff"},
		{"protocol": "dns", "action": "hijack-dns"},
	}
	t.config.Route.Rules = append(defaultRules, t.config.Route.Rules...)

	// Set default route.final if not set
	if t.config.Route.Final == "" {
		// Use the first group tag, or DIRECT if no groups
		if tag := firstGroupTag(t); tag != "" {
			t.config.Route.Final = tag
		} else {
			t.config.Route.Final = "DIRECT"
		}
	}

	// Add rule_set definitions from accumulated definitions, with download_detour
	detour := firstGroupTag(t)
	for _, def := range t.ruleSetDefs {
		if typ, _ := def["type"].(string); typ == "remote" {
			if _, has := def["download_detour"]; !has && detour != "" {
				def["download_detour"] = detour
			}
		}
		t.config.Route.RuleSet = append(t.config.Route.RuleSet, def)
	}

	// Remove route rules that reference rule_set tags with no definition
	// (e.g., classical-format rule-providers that were skipped)
	removeOrphanRuleSetRules(t)
}

// convertRejectActions replaces REJECT outbound references with reject rule actions.
// sing-box 1.13.0 removed the "block" outbound type; use action:"reject" instead.
func convertRejectActions(t *translation) {
	for _, rule := range t.config.Route.Rules {
		if outbound, _ := rule["outbound"].(string); outbound == "REJECT" {
			delete(rule, "outbound")
			rule["action"] = "reject"
		}
	}
}

// addRouteAction adds action:"route" to all rules that have an outbound but no action.
// sing-box v1.13 requires an explicit action field on every rule.
func addRouteAction(t *translation) {
	for _, rule := range t.config.Route.Rules {
		if _, hasAction := rule["action"]; !hasAction {
			if _, hasOutbound := rule["outbound"]; hasOutbound {
				rule["action"] = "route"
			}
		}
	}
}

// removeOrphanRuleSetRules removes route/DNS rules that reference rule_set tags
// without a corresponding definition (e.g., skipped mihomo rule-providers).
// Also recurses into logical rule sub-rules.
func removeOrphanRuleSetRules(t *translation) {
	definedTags := make(map[string]bool)
	for _, def := range t.ruleSetDefs {
		if tag, ok := def["tag"].(string); ok {
			definedTags[tag] = true
		}
	}

	t.config.Route.Rules = filterOrphanRules(t.config.Route.Rules, definedTags, t)
	if t.config.DNS != nil {
		t.config.DNS.Rules = filterOrphanRules(t.config.DNS.Rules, definedTags, t)
	}
}

// filterOrphanRules filters rules referencing undefined rule_set tags.
// Recurses into logical rule "rules" sub-arrays.
func filterOrphanRules(rules []map[string]any, definedTags map[string]bool, t *translation) []map[string]any {
	var filtered []map[string]any
	for _, rule := range rules {
		// Recurse into logical rule sub-rules first
		if subRules, _ := rule["rules"].([]map[string]any); len(subRules) > 0 {
			rule["rules"] = filterOrphanSubRules(subRules, definedTags, t)
		}

		rsTags, _ := rule["rule_set"].([]string)
		if len(rsTags) == 0 {
			filtered = append(filtered, rule)
			continue
		}
		var valid []string
		for _, tag := range rsTags {
			if definedTags[tag] {
				valid = append(valid, tag)
			}
		}
		if len(valid) > 0 {
			rule["rule_set"] = valid
			filtered = append(filtered, rule)
		} else {
			t.warn("dropped rule referencing undefined rule_set: " + strings.Join(rsTags, ", "))
		}
	}
	return filtered
}

// filterOrphanSubRules removes sub-rules within logical rules that reference undefined rule_set tags.
func filterOrphanSubRules(subRules []map[string]any, definedTags map[string]bool, t *translation) []map[string]any {
	var filtered []map[string]any
	for _, sub := range subRules {
		rsTags, _ := sub["rule_set"].([]string)
		if len(rsTags) == 0 {
			filtered = append(filtered, sub)
			continue
		}
		var valid []string
		for _, tag := range rsTags {
			if definedTags[tag] {
				valid = append(valid, tag)
			}
		}
		if len(valid) > 0 {
			sub["rule_set"] = valid
			filtered = append(filtered, sub)
		} else {
			t.warn("dropped logical sub-rule referencing undefined rule_set: " + strings.Join(rsTags, ", "))
		}
	}
	return filtered
}
