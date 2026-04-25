package translator

import (
	"github.com/mapleafgo/singcast/translator/proxy"
)

// translateGroups translates mihomo proxy-groups to sing-box outbounds.
// proxyOutbounds are the already-translated proxy outbounds (not used directly,
// but the proxy tags are available via t.proxyTags for filtering).
func translateGroups(cfg *RawConfig, t *translation) []map[string]any {
	var groups []map[string]any

	// Pre-populate group tags so cross-group references work regardless of order
	for _, g := range cfg.ProxyGroup {
		if name, _ := g["name"].(string); name != "" {
			t.groupTags[name] = true
			t.groupTagOrder = append(t.groupTagOrder, name)
		}
	}

	for _, g := range cfg.ProxyGroup {
		name, _ := g["name"].(string)
		if name == "" {
			t.warn("proxy-group missing name, skipping")
			continue
		}

		groupType, _ := g["type"].(string)

		// Collect and filter proxies for this group
		filtered := filterGroupProxies(g, t)
		if len(filtered) == 0 {
			t.warn("proxy-group \"" + name + "\" has no valid proxies after filtering, skipping")
			continue
		}

		var outbound map[string]any

		switch groupType {
		case "select":
			outbound = translateSelectGroup(name, filtered)
		case "url-test":
			outbound = translateURLTestGroup(name, filtered, g)
		case "fallback":
			outbound = translateFallbackGroup(name, filtered, g)
		case "load-balance":
			outbound = translateLoadBalanceGroup(name, filtered, t)
		case "relay":
			t.warn("proxy-group \"" + name + "\": relay is not supported in sing-box, skipping")
			continue
		default:
			t.warn("proxy-group \"" + name + "\": unknown type \"" + groupType + "\", skipping")
			continue
		}

		if outbound != nil {
			groups = append(groups, outbound)
		}
	}

	return groups
}

// filterGroupProxies collects proxies from the group's "proxies" list,
// filters them against t.proxyTags and t.groupTags, and returns valid ones.
func filterGroupProxies(g map[string]any, t *translation) []string {
	rawProxies, _ := g["proxies"].([]any)
	var filtered []string

	for _, p := range rawProxies {
		name, ok := p.(string)
		if !ok {
			continue
		}
		// Accept if it's a known proxy tag, a known group tag, or DIRECT.
		// REJECT is not accepted: sing-box 1.13.0 removed the block outbound.
		if t.proxyTags[name] || t.groupTags[name] || name == "DIRECT" {
			filtered = append(filtered, name)
		}
	}

	return filtered
}

func translateSelectGroup(name string, proxies []string) map[string]any {
	return map[string]any{
		"type":                        "selector",
		"tag":                         name,
		"outbounds":                   proxies,
		"default":                     proxies[0],
		"interrupt_exist_connections": true,
	}
}

func translateURLTestGroup(name string, proxies []string, g map[string]any) map[string]any {
	url, interval := groupURLDefaults(g)

	result := map[string]any{
		"type":                        "urltest",
		"tag":                         name,
		"outbounds":                   proxies,
		"url":                         url,
		"interval":                    proxy.SecondsToDuration(interval),
		"interrupt_exist_connections": true,
	}

	if tol, ok := toInt(g["tolerance"]); ok && tol > 0 {
		result["tolerance"] = tol
	}

	return result
}

func translateFallbackGroup(name string, proxies []string, g map[string]any) map[string]any {
	url, interval := groupURLDefaults(g)

	return map[string]any{
		"type":                        "urltest",
		"tag":                         name,
		"outbounds":                   proxies,
		"url":                         url,
		"interval":                    proxy.SecondsToDuration(interval),
		"tolerance":                   180000, // 3 minutes in ms
		"interrupt_exist_connections": true,
	}
}

func groupURLDefaults(g map[string]any) (string, int) {
	url := "http://www.gstatic.com/generate_204"
	if u, ok := g["url"].(string); ok && u != "" {
		url = u
	}
	interval := 180
	if iv, ok := toInt(g["interval"]); ok && iv > 0 {
		interval = iv
	}
	return url, interval
}

func translateLoadBalanceGroup(name string, proxies []string, t *translation) map[string]any {
	t.warn("proxy-group \"" + name + "\": load-balance has no sing-box equivalent, degraded to selector")

	return map[string]any{
		"type":                        "selector",
		"tag":                         name,
		"outbounds":                   proxies,
		"default":                     proxies[0],
		"interrupt_exist_connections": true,
	}
}

// toInt converts a value to int, handling both int and float64 (from YAML/JSON).
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}
