package translator

import (
	"github.com/mapleafgo/singcast/translator/proxy"
)

// translateProviders handles rule-providers and proxy-providers.
// Since the frontend inlines provider content, proxy-providers are already handled.
// Rule-providers are translated to rule_set entries for any that weren't already
// created from GEOIP/GEOSITE rules.
func translateProviders(cfg *RawConfig, t *translation) {
	if cfg.RuleProvider == nil {
		return
	}

	for name, provider := range cfg.RuleProvider {
		tag := "rp-" + name

		// Skip if already defined (e.g., from GEOIP/GEOSITE auto-creation)
		if _, exists := t.ruleSetDefs[tag]; exists {
			continue
		}

		ruleSet := map[string]any{
			"tag":  tag,
			"type": "remote",
		}

		// Map URL
		if url, ok := provider["url"].(string); ok && url != "" {
			ruleSet["url"] = url
		}

		// Map behavior to format
		behavior, _ := provider["behavior"].(string)
		switch behavior {
		case "domain", "ipcidr":
			ruleSet["format"] = "binary"
		default:
			ruleSet["format"] = "source"
		}

		// Map interval (seconds -> duration)
		if interval, ok := toInt(provider["interval"]); ok && interval > 0 {
			ruleSet["update_interval"] = proxy.SecondsToDuration(interval)
		} else {
			ruleSet["update_interval"] = "1d"
		}

		// Map proxy (download detour)
		if p, ok := provider["proxy"].(string); ok && p != "" {
			ruleSet["download_detour"] = p
		}

		t.ruleSetDefs[tag] = ruleSet
	}
}
