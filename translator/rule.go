package translator

import "strings"

const rawGitHubPrefix = "https://raw.githubusercontent.com/"

// proxyURL prepends the proxy prefix for raw.githubusercontent.com URLs.
func proxyURL(rawURL string, opts *Options) string {
	if opts != nil {
		if prefix := strings.TrimRight(opts.RuleSetURLPrefix, "/"); prefix != "" {
			if strings.HasPrefix(rawURL, rawGitHubPrefix) {
				return prefix + "/" + rawURL
			}
		}
	}
	return rawURL
}

// registerRuleSet adds a rule_set definition if absent.
func registerRuleSet(tag string, rawURL string, t *translation) {
	if _, exists := t.ruleSetDefs[tag]; exists {
		return
	}
	t.ruleSetDefs[tag] = map[string]any{
		"type":            "remote",
		"tag":             tag,
		"format":          "binary",
		"url":             proxyURL(rawURL, t.opts),
		"update_interval": "1d",
	}
}

// ensureRuleSetDef creates a rule_set definition for GEOIP/GEOSITE if absent.
func ensureRuleSetDef(tag string, geoType string, name string, t *translation) {
	var base string
	if geoType == "geoip" {
		base = rawGitHubPrefix + "SagerNet/sing-geoip/rule-set/geoip-" + strings.ToLower(name) + ".srs"
	} else {
		base = rawGitHubPrefix + "SagerNet/sing-geosite/rule-set/geosite-" + strings.ToLower(name) + ".srs"
	}
	registerRuleSet(tag, base, t)
}

// ensureCustomRuleSetDef creates a rule_set definition with a custom URL if absent.
func ensureCustomRuleSetDef(tag string, rawURL string, t *translation) {
	registerRuleSet(tag, rawURL, t)
}
