package translator

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RawGitHubPrefix 是官方 rule-set 与社区 rule-set 共用的 GitHub 原始文件前缀；
// ApplyRuleSetProxy 只改写该前缀，避免代理参数影响任意 URL。
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

// registerRuleSet adds a rule_set definition if absent.
func registerRuleSet(tag string, rawURL string, t *translation) {
	if _, exists := t.ruleSetDefs[tag]; exists {
		return
	}
	t.ruleSetDefs[tag] = map[string]any{
		"type":            "remote",
		"tag":             tag,
		"format":          "binary",
		"url":             rawURL,
		"download_detour": "DIRECT",
		"update_interval": "1d",
	}
}

// ensureRuleSetDef creates a rule_set definition for GEOIP/GEOSITE if absent.
// 注意：sing-geoip rule-set 按 ISO 国家代码提供文件（geoip-{cc}.srs），
// 而 sing-geosite rule-set 按分类提供（cn、geolocation-!cn 等），不存在按任意
// 国家代码的文件，调用方不得为任意 cc 拼接 geosite-{cc}。
func ensureRuleSetDef(tag string, geoType string, name string, t *translation) {
	var base string
	if geoType == "geoip" {
		base = RawGitHubPrefix + "SagerNet/sing-geoip/rule-set/geoip-" + strings.ToLower(name) + ".srs"
	} else {
		base = RawGitHubPrefix + "SagerNet/sing-geosite/rule-set/geosite-" + strings.ToLower(name) + ".srs"
	}
	registerRuleSet(tag, base, t)
}

// ApplyRuleSetProxy 对翻译后的 sing-box JSON 中 route.rule_set[].url
// 加上代理前缀，仅影响 raw.githubusercontent.com 链接。
// 这是启动时运行时参数，不属于翻译逻辑。
// 输入不是合法 JSON 时返回错误，避免静默丢失前缀改写。
func ApplyRuleSetProxy(jsonStr string, proxy string) (string, error) {
	if proxy == "" {
		return jsonStr, nil
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &root); err != nil {
		return "", fmt.Errorf("parse json: %w", err)
	}
	route, _ := root["route"].(map[string]any)
	if route == nil {
		return jsonStr, nil
	}
	ruleSets, _ := route["rule_set"].([]any)
	if ruleSets == nil {
		return jsonStr, nil
	}
	for _, rs := range ruleSets {
		def, _ := rs.(map[string]any)
		if def == nil {
			continue
		}
		if u, _ := def["url"].(string); u != "" {
			def["url"] = ProxyURL(u, proxy)
		}
	}
	out, err := json.Marshal(root)
	if err != nil {
		return "", fmt.Errorf("marshal json: %w", err)
	}
	return string(out), nil
}
