package translator

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

	// Add rule_set definitions from accumulated definitions
	for _, def := range t.ruleSetDefs {
		t.config.Route.RuleSet = append(t.config.Route.RuleSet, def)
	}
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
