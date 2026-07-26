package translator

import "fmt"

// validate 校验输出配置的内部引用完整性。
//
// 这一步的意义是把"翻译成功但 sing-box 启动失败"挡在翻译阶段：sing-box 对
// 悬空的 outbound / DNS server 引用是启动硬失败，用户只能看到内核起不来、
// 无从定位。因此凡是 sing-box 会硬失败的引用，这里也返回 error；
// 仅影响单条规则的引用问题降级为 warning，由调用方展示。
func validate(t *translation) error {
	tagSet, err := collectOutboundTags(t)
	if err != nil {
		return err
	}

	if err := validateGroupMembers(t, tagSet); err != nil {
		return err
	}
	validateRouteRules(t, tagSet)

	if t.config.Route.Final != "" && !tagSet[t.config.Route.Final] {
		return fmt.Errorf("route.final references non-existent outbound: %s", t.config.Route.Final)
	}

	return validateDNSReferences(t, tagSet)
}

// collectOutboundTags 收集全部 outbound tag 并检查重名。
// assemble 注入的内建 DIRECT 允许存在一次；用户自定义节点若也叫 DIRECT，
// 输出里就会有两个同名 outbound，sing-box 报 duplicate tag。
func collectOutboundTags(t *translation) (map[string]bool, error) {
	tagSet := map[string]bool{"DIRECT": true}
	directSeen := false
	for _, ob := range t.config.Outbounds {
		tag, _ := ob["tag"].(string)
		if tag == "" {
			continue
		}
		if tag == "DIRECT" {
			if directSeen {
				return nil, fmt.Errorf("duplicate outbound tag: DIRECT (user proxy conflicts with built-in DIRECT)")
			}
			directSeen = true
			continue
		}
		if tagSet[tag] {
			return nil, fmt.Errorf("duplicate outbound tag: %s", tag)
		}
		tagSet[tag] = true
	}
	return tagSet, nil
}

// validateGroupMembers 校验 selector/urltest 等分组的成员引用。
// 组成员悬空是 sing-box 启动硬失败，而 group.go 的清理只做单趟、
// 处理顺序不利时会漏掉，这里是最后一道闸。
func validateGroupMembers(t *translation, tagSet map[string]bool) error {
	for _, ob := range t.config.Outbounds {
		raw, has := ob["outbounds"]
		if !has {
			continue
		}
		tag, _ := ob["tag"].(string)
		members, ok := outboundMembers(raw)
		if !ok {
			// 不能静默跳过：类型变化会让这条校验默默失效，
			// 而它防的正是"翻译通过但内核起不来"。
			return fmt.Errorf("proxy-group %q has outbounds of unexpected type %T", tag, raw)
		}
		for _, m := range members {
			if !tagSet[m] {
				return fmt.Errorf("proxy-group %q references non-existent outbound: %s", tag, m)
			}
		}
	}
	return nil
}

// outboundMembers 取出分组成员列表。翻译器内部统一用 []string，
// 但同时接受 []any 以兼容将来可能的表示变化。
func outboundMembers(raw any) ([]string, bool) {
	switch v := raw.(type) {
	case []string:
		return v, true
	case []any:
		members := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			members = append(members, s)
		}
		return members, true
	default:
		return nil, false
	}
}

// validateRouteRules 校验路由规则的 outbound 引用。
// 悬空引用只影响该条规则，降级为 warning——直接报错会让一条无关规则
// 阻断整份配置。
func validateRouteRules(t *translation, tagSet map[string]bool) {
	for _, rule := range t.config.Route.Rules {
		outbound, _ := rule["outbound"].(string)
		if outbound != "" && !tagSet[outbound] {
			t.warn("rule references non-existent outbound: " + outbound)
		}
	}
}

// validateDNSReferences 校验 DNS 规则的 server 引用、DNS server 的 detour 引用
// 以及 dns.final。三者悬空同样是 sing-box 启动硬失败。
func validateDNSReferences(t *translation, tagSet map[string]bool) error {
	if t.config.DNS == nil {
		return nil
	}

	serverTags := make(map[string]bool, len(t.config.DNS.Servers))
	for _, srv := range t.config.DNS.Servers {
		if tag, _ := srv["tag"].(string); tag != "" {
			serverTags[tag] = true
		}
	}

	for _, srv := range t.config.DNS.Servers {
		detour, _ := srv["detour"].(string)
		if detour != "" && !tagSet[detour] {
			tag, _ := srv["tag"].(string)
			return fmt.Errorf("dns server %q references non-existent detour outbound: %s", tag, detour)
		}
	}

	for _, rule := range t.config.DNS.Rules {
		server, _ := rule["server"].(string)
		if server != "" && !serverTags[server] {
			return fmt.Errorf("dns rule references non-existent server: %s", server)
		}
	}

	if t.config.DNS.Final != "" && !serverTags[t.config.DNS.Final] {
		return fmt.Errorf("dns.final references non-existent server: %s", t.config.DNS.Final)
	}
	return nil
}
