package translator

import "strings"

const RawGitHubPrefix = "https://raw.githubusercontent.com/"

// ProxyURL prepends the proxy prefix for raw.githubusercontent.com URLs.
func ProxyURL(rawURL, proxy string) string {
	if prefix := strings.TrimRight(proxy, "/"); prefix != "" {
		if strings.HasPrefix(rawURL, RawGitHubPrefix) {
			return prefix + "/" + rawURL
		}
	}
	return rawURL
}

// proxyURL wraps ProxyURL with Options for internal use.
func proxyURL(rawURL string, opts *Options) string {
	if opts != nil {
		return ProxyURL(rawURL, opts.RuleSetURLPrefix)
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
		"download_detour": "DIRECT",
		"update_interval": "1d",
	}
}

// ensureRuleSetDef creates a rule_set definition for GEOIP/GEOSITE if absent.
func ensureRuleSetDef(tag string, geoType string, name string, t *translation) {
	var base string
	if geoType == "geoip" {
		base = RawGitHubPrefix + "SagerNet/sing-geoip/rule-set/geoip-" + strings.ToLower(name) + ".srs"
	} else {
		base = RawGitHubPrefix + "SagerNet/sing-geosite/rule-set/geosite-" + strings.ToLower(name) + ".srs"
	}
	registerRuleSet(tag, base, t)
}
