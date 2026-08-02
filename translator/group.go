package translator

import (
	"math"
	"slices"
	"strconv"

	"github.com/mapleafgo/singcast/translator/proxy"
)

const (
	maxGroupInterval = 1800 // 30 minutes; 超过此值的 interval 会被截断
	// minGroupIdleTimeout 是 idle_timeout 的下限（用 max 取值，不是上限）：
	// sing-box 要求 idle_timeout 大于 interval，取 max(interval*2, 30min)
	// 既满足该约束，又与 sing-box 自身的 30 分钟默认值一致。
	minGroupIdleTimeout = 1800
	// maxTolerance 封顶到 uint16 上限：sing-box option.Group.Tolerance 是
	// uint16，超出会在解析阶段溢出报错。
	maxTolerance = math.MaxUint16
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
			filtered = filterHealthCheckProxies(filtered, t)
			if len(filtered) == 0 {
				t.warn("proxy-group \"" + name + "\" has no valid health-check proxies after filtering, skipping")
				continue
			}
			outbound = translateURLTestGroup(name, filtered, g, t)
		case "fallback":
			filtered = filterHealthCheckProxies(filtered, t)
			if len(filtered) == 0 {
				t.warn("proxy-group \"" + name + "\" has no valid health-check proxies after filtering, skipping")
				continue
			}
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

func filterHealthCheckProxies(proxies []string, t *translation) []string {
	if len(t.invalidHealthCheckProxyTags) == 0 {
		return proxies
	}

	filtered := make([]string, 0, len(proxies))
	for _, name := range proxies {
		if !t.invalidHealthCheckProxyTags[name] {
			filtered = append(filtered, name)
		}
	}
	return filtered
}

// cleanupGroupOutbounds removes references to skipped groups from every translated
// group's outbound list. If a group's outbound list becomes empty after cleanup,
// the group is dropped entirely (and removed from translated so groupTagOrder won't
// include it). Returns the filtered group list.
//
// 迭代至不动点：丢弃一个组会让引用它的组也可能变空，而单趟扫描中"先处理、
// 后失效"的组不会被回补清理——组 A 引用组 B、B 在 A 之后被丢弃时，
// A 就会留下悬空引用，sing-box 启动报 outbound not found。
func cleanupGroupOutbounds(groups []map[string]any, translated map[string]bool, t *translation) []map[string]any {
	for {
		result, dropped := cleanupGroupOutboundsOnce(groups, translated, t)
		groups = result
		if dropped == 0 {
			return groups
		}
	}
}

// cleanupGroupOutboundsOnce 执行一趟清理，返回过滤后的组列表和本趟丢弃的组数。
func cleanupGroupOutboundsOnce(groups []map[string]any, translated map[string]bool, t *translation) ([]map[string]any, int) {
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
	dropped := 0
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
			dropped++
			continue
		}

		ob["outbounds"] = clean

		// Fix default selection if it was removed
		if def, ok := ob["default"].(string); ok && !validTags[def] {
			ob["default"] = clean[0]
		}

		result = append(result, ob)
	}

	return result, dropped
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
		"idle_timeout":                proxy.SecondsToDuration(max(interval*2, minGroupIdleTimeout)),
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
		"idle_timeout":                proxy.SecondsToDuration(max(interval*2, minGroupIdleTimeout)),
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

// generateDefaultGroups 为没有 proxy-groups 的订阅（v2ray URI 列表）自动生成默认组。
//
// 仅一个节点时不生成 urltest 组，直接用 selector 包裹。
// selector 组名为 "PROXY"，urltest 组名为 "Auto"，放在 selector 顶部。
func generateDefaultGroups(t *translation) []map[string]any {
	// 收集全部有效代理 tag（保持插入顺序）
	allProxies := make([]string, 0, len(t.proxyTags))
	for tag := range t.proxyTags {
		allProxies = append(allProxies, tag)
	}
	if len(allProxies) == 0 {
		return nil
	}
	// proxyTags 是 map，顺序不稳定；但 URI 列表节点数通常很少，排序保证输出确定性
	slices.Sort(allProxies)

	// urltest 可用代理：过滤掉无效健康检查节点（stub / 缺 endpoint）
	healthCheckProxies := filterHealthCheckProxies(allProxies, t)

	var groups []map[string]any
	var selectorOutbounds []string

	if len(healthCheckProxies) >= 2 {
		auto := translateURLTestGroup("Auto", healthCheckProxies, map[string]any{}, t)
		groups = append(groups, auto)
		selectorOutbounds = append(selectorOutbounds, "Auto")
		t.groupTags["Auto"] = true
	}

	selectorOutbounds = append(selectorOutbounds, allProxies...)
	proxy := translateSelectGroup("PROXY", selectorOutbounds)
	groups = append(groups, proxy)
	t.groupTags["PROXY"] = true

	t.groupTagOrder = []string{"PROXY"}
	return groups
}
