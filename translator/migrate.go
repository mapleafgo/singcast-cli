package translator

import (
	"encoding/json"
	"fmt"
	"strings"
)

// UpgradeLegacyDNSRules 把已转换配置（sing-box JSON）里的 legacy DNS address filter
// 形态升级为 sing-box 1.14 的 evaluate + match_response 形态。
//
// 背景：订阅在旧版本转换时会把 GeoIP rule-set 直接塞进 DNS 规则（按响应地址判定），
// 这在 sing-box 1.14 属于 Legacy Address Filter Fields，启动即报错、1.16 将移除。
// 已保存的 profile 是"已转换 JSON"，转换入口会原样透传，因此必须在透传路径上也做一次
// 升级，否则升级内核后旧订阅档无法启动。
//
// 返回升级后的 JSON、是否发生改动；JSON 非法或无需升级时原样返回。
func UpgradeLegacyDNSRules(jsonStr string) (string, bool, error) {
	var root map[string]any
	// 解析失败说明输入不是 JSON（交给 YAML 分支处理），原样返回即可，不算错误；
	// 这里用正向判断，避免把"输入不是 JSON"误当成需要上报的解析错误。
	if json.Unmarshal([]byte(jsonStr), &root) == nil {
		return upgradeLegacyDNSRulesInRoot(root, jsonStr)
	}
	return jsonStr, false, nil
}

// upgradeLegacyDNSRulesInRoot 在已解析的 JSON 根对象上执行规则升级。
func upgradeLegacyDNSRulesInRoot(root map[string]any, original string) (string, bool, error) {
	dns, _ := root["dns"].(map[string]any)
	if dns == nil {
		return original, false, nil
	}
	rawRules, _ := dns["rules"].([]any)
	if len(rawRules) == 0 {
		return original, false, nil
	}

	ipOnlyRuleSets := collectIPOnlyDNSRuleSets(root)
	if len(ipOnlyRuleSets) == 0 {
		return original, false, nil
	}

	changed := false
	upgraded := make([]any, 0, len(rawRules))
	for _, raw := range rawRules {
		rule, ok := raw.(map[string]any)
		if !ok {
			upgraded = append(upgraded, raw)
			continue
		}
		if !dnsRuleNeedsResponseMatch(rule, ipOnlyRuleSets) {
			upgraded = append(upgraded, rule)
			continue
		}
		server, _ := rule["server"].(string)
		if server == "" {
			// 没有显式 server 就无法确定 evaluate 的目标，保持原样交给 sing-box 报错。
			upgraded = append(upgraded, rule)
			continue
		}
		if !dnsRuleHasPrecedingEvaluate(upgraded, server) {
			upgraded = append(upgraded, map[string]any{
				"action":     "evaluate",
				"server":     server,
				"query_type": dnsAddressQueryTypes,
			})
		}
		rule["match_response"] = true
		if _, has := rule["query_type"]; !has {
			rule["query_type"] = dnsAddressQueryTypes
		}
		upgraded = append(upgraded, rule)
		changed = true
	}
	if !changed {
		return original, false, nil
	}

	dns["rules"] = upgraded
	out, err := json.Marshal(root)
	if err != nil {
		return "", false, fmt.Errorf("marshal upgraded config: %w", err)
	}
	return string(out), true, nil
}

// collectIPOnlyDNSRuleSets 从 route.rule_set 定义中挑出"只含 ip_cidr"的 rule-set tag。
// 官方 GeoIP rule-set 由 SagerNet/sing-geoip 提供，tag 约定为 geoip-{cc}；
// 同时兼容同为纯 IP 语义的本地/内联定义。
func collectIPOnlyDNSRuleSets(root map[string]any) map[string]bool {
	route, _ := root["route"].(map[string]any)
	if route == nil {
		return nil
	}
	defs, _ := route["rule_set"].([]any)
	if len(defs) == 0 {
		return nil
	}

	tags := make(map[string]bool)
	for _, raw := range defs {
		def, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		tag, _ := def["tag"].(string)
		if tag == "" {
			continue
		}
		if isIPOnlyRuleSetDef(def) {
			tags[tag] = true
		}
	}
	return tags
}

func isIPOnlyRuleSetDef(def map[string]any) bool {
	if url, _ := def["url"].(string); strings.Contains(url, "sing-geoip/rule-set/") {
		return true
	}
	if tag, _ := def["tag"].(string); strings.HasPrefix(tag, "geoip-") {
		return true
	}
	return false
}

// dnsRuleNeedsResponseMatch 判断该 DNS 规则是否引用了纯 IP rule-set 且尚未启用响应匹配。
func dnsRuleNeedsResponseMatch(rule map[string]any, ipOnly map[string]bool) bool {
	// match_response 可能是 true 或字符串 tag；只有真正启用的才跳过升级。
	switch v := rule["match_response"].(type) {
	case bool:
		if v {
			return false
		}
	case string:
		if v != "" {
			return false
		}
	}
	// logical 规则不支持在自身设置 match_response，交由子规则单独处理。
	if ruleType, _ := rule["type"].(string); ruleType == "logical" {
		return false
	}
	for _, tag := range ruleSetTagsOf(rule) {
		if ipOnly[tag] {
			return true
		}
	}
	return false
}

func ruleSetTagsOf(rule map[string]any) []string {
	switch v := rule["rule_set"].(type) {
	case string:
		return []string{v}
	case []any:
		tags := make([]string, 0, len(v))
		for _, item := range v {
			if tag, ok := item.(string); ok {
				tags = append(tags, tag)
			}
		}
		return tags
	}
	return nil
}

// dnsRuleHasPrecedingEvaluate 判断前一条规则是否已是为同一 server 准备的 evaluate，
// 避免对连续的同类规则重复插入。
func dnsRuleHasPrecedingEvaluate(rules []any, server string) bool {
	if len(rules) == 0 {
		return false
	}
	prev, ok := rules[len(rules)-1].(map[string]any)
	if !ok {
		return false
	}
	action, _ := prev["action"].(string)
	prevServer, _ := prev["server"].(string)
	return action == "evaluate" && prevServer == server
}
