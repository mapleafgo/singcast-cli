package translator

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// translateRules translates mihomo rule strings to sing-box route rules.
// Appends results to t.config.Route.Rules.
func translateRules(cfg *RawConfig, t *translation) {
	for _, rule := range cfg.Rule {
		if rule == "" {
			continue
		}
		parseRule(rule, t)
	}
}

// parseRule parses a single mihomo rule string and dispatches to the appropriate handler.
func parseRule(rule string, t *translation) {
	parts := strings.Split(rule, ",")

	ruleType := parts[0]

	// MATCH is special: MATCH,target
	if ruleType == "MATCH" {
		if len(parts) >= 2 {
			target := parts[1]
			if !isValidOutbound(target, t) {
				t.warn("MATCH rule references non-existent outbound \"" + target + "\"")
				return
			}
			t.config.Route.Final = target
		}
		return
	}

	// Logical rules: AND/OR/NOT,((sub1),(sub2),...),target
	if ruleType == "AND" || ruleType == "OR" {
		parseLogicalRule(parts, t)
		return
	}
	if ruleType == "NOT" {
		parseNotRule(parts, t)
		return
	}

	// Standard rules: TYPE,PAYLOAD,TARGET[,no-resolve]
	if len(parts) < 3 {
		t.warn("invalid rule (too few parts): " + rule)
		return
	}

	payload := parts[1]
	target := parts[2]
	isSrc := false
	noResolve := false
	for i := 3; i < len(parts); i++ {
		switch parts[i] {
		case "src":
			isSrc = true
		case "no-resolve":
			noResolve = true
		}
	}

	if !isValidOutbound(target, t) {
		t.warn("rule references non-existent outbound \"" + target + "\": " + rule)
		return
	}

	switch ruleType {
	case "DOMAIN":
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"domain":   []string{payload},
			"outbound": target,
		})
	case "DOMAIN-SUFFIX":
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"domain_suffix": []string{payload},
			"outbound":      target,
		})
	case "DOMAIN-KEYWORD":
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"domain_keyword": []string{payload},
			"outbound":       target,
		})
	case "DOMAIN-REGEX":
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"domain_regex": []string{payload},
			"outbound":     target,
		})
	case "GEOSITE":
		tag := "geosite-" + payload
		ensureRuleSetDef(tag, "geosite", payload, t)
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"rule_set": []string{tag},
			"outbound": target,
		})
	case "GEOIP":
		tag := "geoip-" + payload
		ensureRuleSetDef(tag, "geoip", payload, t)
		rule := map[string]any{
			"rule_set": []string{tag},
			"outbound": target,
		}
		if isSrc {
			rule["rule_set_ip_cidr_match_source"] = true
		}
		t.config.Route.Rules = append(t.config.Route.Rules, rule)
	case "IP-CIDR", "IP-CIDR6":
		rule := map[string]any{
			"ip_cidr":  []string{payload},
			"outbound": target,
		}
		if noResolve {
			rule["ip_cidr_resolve_no"] = true
		}
		t.config.Route.Rules = append(t.config.Route.Rules, rule)
	case "SRC-IP-CIDR":
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"source_ip_cidr": []string{payload},
			"outbound":       target,
		})
	case "DST-PORT":
		if strings.Contains(payload, "-") {
			// Port range: "80-443" -> "80:443"
			portRange := strings.ReplaceAll(payload, "-", ":")
			t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
				"port_range": []string{portRange},
				"outbound":   target,
			})
		} else {
			portVal, err := strconv.Atoi(payload)
			if err != nil {
				t.warn("invalid DST-PORT value: " + payload)
				return
			}
			t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
				"port":     []int{portVal},
				"outbound": target,
			})
		}
	case "SRC-PORT":
		if strings.Contains(payload, "-") {
			portRange := strings.ReplaceAll(payload, "-", ":")
			t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
				"source_port_range": []string{portRange},
				"outbound":          target,
			})
		} else {
			portVal, err := strconv.Atoi(payload)
			if err != nil {
				t.warn("invalid SRC-PORT value: " + payload)
				return
			}
			t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
				"source_port": []int{portVal},
				"outbound":    target,
			})
		}
	case "PROCESS-NAME":
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"process_name": []string{payload},
			"outbound":     target,
		})
	case "PROCESS-PATH":
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"process_path": []string{payload},
			"outbound":     target,
		})
	case "NETWORK":
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"network":  []string{payload},
			"outbound": target,
		})
	case "RULE-SET":
		tag := "rp-" + payload
		rule := map[string]any{
			"rule_set": []string{tag},
			"outbound": target,
		}
		if isSrc {
			rule["rule_set_ip_cidr_match_source"] = true
		}
		t.config.Route.Rules = append(t.config.Route.Rules, rule)
	case "DOMAIN-WILDCARD":
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"domain_regex": []string{wildcardToRegex(payload)},
			"outbound":     target,
		})
	case "SRC-GEOIP":
		tag := "geoip-" + payload
		ensureRuleSetDef(tag, "geoip", payload, t)
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"rule_set":                     []string{tag},
			"rule_set_ip_cidr_match_source": true,
			"outbound":                     target,
		})
	case "PROCESS-PATH-REGEX":
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"process_path_regex": []string{payload},
			"outbound":           target,
		})
	case "UID":
		uid, err := strconv.Atoi(payload)
		if err != nil {
			t.warn("invalid UID value: " + payload)
			return
		}
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"user_id":  []int{uid},
			"outbound": target,
		})
	case "IN-TYPE", "IN-NAME":
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"inbound":  []string{payload},
			"outbound": target,
		})
	case "IN-USER":
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"auth_user": []string{payload},
			"outbound":  target,
		})
	case "PROCESS-PATH-WILDCARD":
		t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
			"process_path_regex": []string{wildcardToRegex(payload)},
			"outbound":           target,
		})
	case "IP-SUFFIX", "SRC-IP-SUFFIX", "IN-PORT",
		"IP-ASN", "SRC-IP-ASN", "DSCP",
		"PROCESS-NAME-REGEX", "PROCESS-NAME-WILDCARD",
		"SUB-RULE":
		t.warn("rule type " + ruleType + " has no sing-box equivalent, skipping: " + rule)
	default:
		t.warn("unsupported rule type: " + ruleType)
	}
}

// parseLogicalRule handles AND/OR logical rules.
// Format: AND/OR,((sub1),(sub2),...),target
func parseLogicalRule(parts []string, t *translation) {
	ruleType := parts[0]
	if len(parts) < 3 {
		t.warn("invalid logical rule: " + strings.Join(parts, ","))
		return
	}

	// The middle part(s) contain the sub-rules wrapped in (( ))
	// The target is the last part
	target := parts[len(parts)-1]

	if !isValidOutbound(target, t) {
		t.warn("logical rule references non-existent outbound \"" + target + "\"")
		return
	}

	// Rejoin everything between type and target as the payload
	payload := strings.Join(parts[1:len(parts)-1], ",")

	// Extract sub-rules from ((...))
	subRules := extractSubRules(payload, t)
	if len(subRules) == 0 {
		t.warn("logical rule has no valid sub-rules: " + strings.Join(parts, ","))
		return
	}

	mode := "and"
	if ruleType == "OR" {
		mode = "or"
	}

	t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
		"type":     "logical",
		"mode":     mode,
		"rules":    subRules,
		"outbound": target,
	})
}

// extractSubRules parses the ((A),(B),...) format into sing-box rule maps.
func extractSubRules(payload string, t *translation) []map[string]any {
	// Trim outer parentheses: ((A),(B)) -> (A),(B)
	s := strings.TrimSpace(payload)
	if strings.HasPrefix(s, "((") && strings.HasSuffix(s, "))") {
		s = s[1 : len(s)-1]
	}

	var rules []map[string]any
	// Split by "),(" to get individual sub-rules
	segments := splitSubRules(s)

	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		// Remove surrounding parentheses
		seg = strings.TrimPrefix(seg, "(")
		seg = strings.TrimSuffix(seg, ")")
		seg = strings.TrimSpace(seg)

		if seg == "" {
			continue
		}

		ruleMap := parseSingleSubRule(seg, t)
		if ruleMap != nil {
			rules = append(rules, ruleMap)
		}
	}

	return rules
}

// splitSubRules splits a string like "(DOMAIN,example.com,PROXY),(DOMAIN-SUFFIX,com,DIRECT)"
// into individual sub-rule strings.
func splitSubRules(s string) []string {
	var segments []string
	depth := 0
	current := strings.Builder{}

	for _, ch := range s {
		if ch == '(' {
			depth++
		}
		if ch == ')' {
			depth--
		}
		if ch == ',' && depth == 0 {
			segments = append(segments, current.String())
			current.Reset()
			continue
		}
		current.WriteRune(ch)
	}
	if current.Len() > 0 {
		segments = append(segments, current.String())
	}

	return segments
}

// parseSingleSubRule parses one sub-rule like "DOMAIN,example.com,PROXY" into a rule map.
// The outbound is NOT included since it's at the logical rule level.
func parseSingleSubRule(rule string, t *translation) map[string]any {
	parts := strings.Split(rule, ",")
	if len(parts) < 2 {
		t.warn("invalid sub-rule: " + rule)
		return nil
	}

	ruleType := parts[0]
	// For sub-rules within logical rules, the target is parts[2] but in sing-box
	// logical rules, the outbound is at the parent level. We ignore it here.
	payload := ""
	if len(parts) >= 2 {
		payload = parts[1]
	}

	switch ruleType {
	case "DOMAIN":
		return map[string]any{"domain": []string{payload}}
	case "DOMAIN-SUFFIX":
		return map[string]any{"domain_suffix": []string{payload}}
	case "DOMAIN-KEYWORD":
		return map[string]any{"domain_keyword": []string{payload}}
	case "DOMAIN-REGEX":
		return map[string]any{"domain_regex": []string{payload}}
	case "GEOSITE":
		tag := "geosite-" + payload
		ensureRuleSetDef(tag, "geosite", payload, t)
		return map[string]any{"rule_set": []string{tag}}
	case "GEOIP":
		tag := "geoip-" + payload
		ensureRuleSetDef(tag, "geoip", payload, t)
		return map[string]any{
			"rule_set": []string{tag},
		}
	case "IP-CIDR", "IP-CIDR6":
		return map[string]any{"ip_cidr": []string{payload}}
	case "SRC-IP-CIDR":
		return map[string]any{"source_ip_cidr": []string{payload}}
	case "DST-PORT":
		if strings.Contains(payload, "-") {
			portRange := strings.ReplaceAll(payload, "-", ":")
			return map[string]any{"port_range": []string{portRange}}
		}
		portVal, err := strconv.Atoi(payload)
		if err != nil {
			t.warn("invalid DST-PORT value in sub-rule: " + payload)
			return nil
		}
		return map[string]any{"port": []int{portVal}}
	case "SRC-PORT":
		portVal, err := strconv.Atoi(payload)
		if err != nil {
			t.warn("invalid SRC-PORT value in sub-rule: " + payload)
			return nil
		}
		return map[string]any{"source_port": []int{portVal}}
	case "PROCESS-NAME":
		return map[string]any{"process_name": []string{payload}}
	case "PROCESS-PATH":
		return map[string]any{"process_path": []string{payload}}
	case "NETWORK":
		return map[string]any{"network": []string{payload}}
	case "RULE-SET":
		tag := "rp-" + payload
		return map[string]any{"rule_set": []string{tag}}
	case "DOMAIN-WILDCARD":
		return map[string]any{"domain_regex": []string{wildcardToRegex(payload)}}
	case "SRC-GEOIP":
		tag := "geoip-" + payload
		ensureRuleSetDef(tag, "geoip", payload, t)
		return map[string]any{
			"rule_set":                      []string{tag},
			"rule_set_ip_cidr_match_source": true,
		}
	case "PROCESS-PATH-REGEX":
		return map[string]any{"process_path_regex": []string{payload}}
	case "UID":
		uid, err := strconv.Atoi(payload)
		if err != nil {
			t.warn("invalid UID value in sub-rule: " + payload)
			return nil
		}
		return map[string]any{"user_id": []int{uid}}
	case "IN-TYPE", "IN-NAME":
		return map[string]any{"inbound": []string{payload}}
	case "IN-USER":
		return map[string]any{"auth_user": []string{payload}}
	case "PROCESS-PATH-WILDCARD":
		return map[string]any{"process_path_regex": []string{wildcardToRegex(payload)}}
	case "IP-SUFFIX", "SRC-IP-SUFFIX", "IN-PORT",
		"IP-ASN", "SRC-IP-ASN", "DSCP",
		"PROCESS-NAME-REGEX", "PROCESS-NAME-WILDCARD",
		"SUB-RULE":
		t.warn(fmt.Sprintf("unsupported sub-rule type in logical rule: %s", ruleType))
		return nil
	default:
		t.warn(fmt.Sprintf("unsupported sub-rule type in logical rule: %s", ruleType))
		return nil
	}
}

// isValidOutbound checks if the target outbound exists in proxy or group tags,
// or is a built-in (DIRECT, REJECT).
func isValidOutbound(target string, t *translation) bool {
	switch target {
	case "DIRECT", "REJECT":
		return true
	}
	return t.proxyTags[target] || t.groupTags[target]
}

// ensureRuleSetDef creates a rule_set definition for GEOIP/GEOSITE if not already present.
func ensureRuleSetDef(tag string, geoType string, name string, t *translation) {
	if _, exists := t.ruleSetDefs[tag]; exists {
		return
	}

	var baseURL string
	if geoType == "geoip" {
		baseURL = "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@sing/geo/geoip/"
	} else {
		baseURL = "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@sing/geo/geosite/"
	}

	t.ruleSetDefs[tag] = map[string]any{
		"type":            "remote",
		"tag":             tag,
		"format":          "binary",
		"url":             baseURL + name + ".srs",
		"update_interval": "1d",
	}
}

// parseNotRule handles NOT rules: NOT,((sub-rule)),target
func parseNotRule(parts []string, t *translation) {
	if len(parts) < 3 {
		t.warn("invalid NOT rule: " + strings.Join(parts, ","))
		return
	}

	target := parts[len(parts)-1]

	if !isValidOutbound(target, t) {
		t.warn("NOT rule references non-existent outbound \"" + target + "\"")
		return
	}

	payload := strings.Join(parts[1:len(parts)-1], ",")
	subRules := extractSubRules(payload, t)
	if len(subRules) == 0 {
		t.warn("NOT rule has no valid sub-rules: " + strings.Join(parts, ","))
		return
	}

	t.config.Route.Rules = append(t.config.Route.Rules, map[string]any{
		"type":     "logical",
		"mode":     "and",
		"rules":    subRules,
		"invert":   true,
		"outbound": target,
	})
}

// wildcardToRegex converts a mihomo DOMAIN-WILDCARD glob pattern to a regex.
func wildcardToRegex(pattern string) string {
	var buf strings.Builder
	buf.WriteString("^")
	for _, ch := range pattern {
		if ch == '*' {
			buf.WriteString(".*")
		} else if ch == '?' {
			buf.WriteString(".")
		} else {
			buf.WriteString(regexp.QuoteMeta(string(ch)))
		}
	}
	buf.WriteString("$")
	return buf.String()
}
