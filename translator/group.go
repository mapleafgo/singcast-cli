package translator

import (
	"math"
	"strconv"

	"github.com/mapleafgo/singcast/translator/proxy"
)

const (
	maxGroupInterval    = 1800 // 30 minutes; 超过此值的 interval 会被截断
	maxGroupIdleTimeout = 1800  // 30 minutes
	maxTolerance        = math.MaxUint16
	// fallbackTolerance 让 fallback 组模拟 Clash 行为：当前节点延迟比其他节点
	// 高超过 10 秒（通常意味着不可用）时才切换，避免频繁跳节点。
	fallbackTolerance = 10000
)

// translateGroups translates mihomo proxy-groups to sing-box outbounds.
// proxyOutbounds are the already-translated proxy outbounds (not used directly,
// but the proxy tags are available via t.proxyTags for filtering).
func translateGroups(cfg *RawConfig, t *translation) []map[string]any {
	var groups []map[string]any

	translated := make(map[string]bool)

	// Pre-populate groupTags so cross-group references work regardless of order.
	// groupTagOrder is intentionally NOT pre-populated: only groups that are actually
	// translated successfully are added later, so firstGroupTag() never returns a
	// tag for a group that was skipped (e.g. all proxies unsupported).
	for _, g := range cfg.ProxyGroup {
		if name, _ := g["name"].(string); name != "" {
			t.groupTags[name] = true
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
			outbound = translateURLTestGroup(name, filtered, g, t)
		case "fallback":
			outbound = translateFallbackGroup(name, filtered, g, t)
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
			translated[name] = true
		}
	}

	// Clean up dangling references to skipped groups in all group outbound lists.
	// A group might reference another group that was skipped (filterGroupProxies
	// accepted it because groupTags was pre-populated). Remove those references,
	// and drop any group that becomes empty as a result.
	groups = cleanupGroupOutbounds(groups, translated, t)

	// Build groupTagOrder from translated groups only (preserving config order)
	for _, g := range cfg.ProxyGroup {
		name, _ := g["name"].(string)
		if name != "" && translated[name] {
			t.groupTagOrder = append(t.groupTagOrder, name)
		} else if name != "" {
			// Remove skipped groups from groupTags so no downstream code
			// treats them as valid outbound references.
			delete(t.groupTags, name)
		}
	}

	if len(t.groupTagOrder) == 0 && len(cfg.ProxyGroup) > 0 {
		t.warn("no proxy groups were translated successfully; route.final will fall back to DIRECT")
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

// cleanupGroupOutbounds removes references to skipped groups from every translated
// group's outbound list. If a group's outbound list becomes empty after cleanup,
// the group is dropped entirely (and removed from translated so groupTagOrder won't
// include it). Returns the filtered group list.
func cleanupGroupOutbounds(groups []map[string]any, translated map[string]bool, t *translation) []map[string]any {
	// Valid outbound tags: real proxies + successfully translated groups + DIRECT
	validTags := make(map[string]bool, len(t.proxyTags)+len(translated)+1)
	for tag := range t.proxyTags {
		validTags[tag] = true
	}
	for tag := range translated {
		validTags[tag] = true
	}
	validTags["DIRECT"] = true

	var result []map[string]any
	for _, ob := range groups {
		tag, _ := ob["tag"].(string)
		oldList, _ := ob["outbounds"].([]string)

		var clean []string
		for _, name := range oldList {
			if validTags[name] {
				clean = append(clean, name)
			}
		}

		if len(clean) == 0 {
			t.warn("proxy-group \"" + tag + "\" has no valid outbounds after cleanup, dropping")
			delete(translated, tag)
			delete(t.groupTags, tag)
			continue
		}

		ob["outbounds"] = clean

		// Fix default selection if it was removed
		if def, ok := ob["default"].(string); ok && !validTags[def] {
			ob["default"] = clean[0]
		}

		result = append(result, ob)
	}

	return result
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

func translateURLTestGroup(name string, proxies []string, g map[string]any, t *translation) map[string]any {
	url, interval := groupURLDefaults(g, name, t)

	result := map[string]any{
		"type":                        "urltest",
		"tag":                         name,
		"outbounds":                   proxies,
		"url":                         url,
		"interval":                    proxy.SecondsToDuration(interval),
		"idle_timeout":                proxy.SecondsToDuration(max(interval*2, maxGroupIdleTimeout)),
		"interrupt_exist_connections": true,
	}

	if tol, ok := toInt(g["tolerance"]); ok && tol > 0 {
		result["tolerance"] = min(tol, maxTolerance)
	}

	return result
}

func translateFallbackGroup(name string, proxies []string, g map[string]any, t *translation) map[string]any {
	url, interval := groupURLDefaults(g, name, t)

	return map[string]any{
		"type":                        "urltest",
		"tag":                         name,
		"outbounds":                   proxies,
		"url":                         url,
		"interval":                    proxy.SecondsToDuration(interval),
		"idle_timeout":                proxy.SecondsToDuration(max(interval*2, maxGroupIdleTimeout)),
		"tolerance":                   fallbackTolerance,
		"interrupt_exist_connections": true,
	}
}

func groupURLDefaults(g map[string]any, name string, t *translation) (string, int) {
	url := "http://www.gstatic.com/generate_204"
	if u, ok := g["url"].(string); ok && u != "" {
		url = u
	}
	interval := 180
	if iv, ok := toInt(g["interval"]); ok && iv > 0 {
		if iv > maxGroupInterval {
			t.warn("proxy-group \"" + name + "\": interval " + strconv.Itoa(iv) +
				"s exceeds " + strconv.Itoa(maxGroupInterval) + "s, clamped to " + strconv.Itoa(maxGroupInterval) + "s")
			interval = maxGroupInterval
		} else {
			interval = iv
		}
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
