package translator

import "strings"

// ensureRuleSetDef creates a rule_set definition for GEOIP/GEOSITE if not already present.
func ensureRuleSetDef(tag string, geoType string, name string, t *translation) {
	if _, exists := t.ruleSetDefs[tag]; exists {
		return
	}

	var base string
	if geoType == "geoip" {
		base = "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-" + strings.ToLower(name) + ".srs"
	} else {
		base = "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-" + strings.ToLower(name) + ".srs"
	}

	url := base
	if t.opts != nil {
		if prefix := strings.TrimRight(t.opts.RuleSetURLPrefix, "/"); prefix != "" {
			url = prefix + "/" + base
		}
	}

	t.ruleSetDefs[tag] = map[string]any{
		"type":            "remote",
		"tag":             tag,
		"format":          "binary",
		"url":             url,
		"update_interval": "1d",
	}
}
