package translator

import (
	"encoding/json"
	"strings"
	"testing"
)

func testTranslation(t *testing.T) *translation {
	t.Helper()
	return &translation{
		config: &singboxConfig{
			Inbounds:  []map[string]any{},
			Outbounds: []map[string]any{},
			Route:     &singboxRoute{},
		},
		proxyTags:   make(map[string]bool),
		groupTags:   make(map[string]bool),
		ruleSetDefs: make(map[string]map[string]any),
	}
}

func TestGenerateGeoRoute_China(t *testing.T) {
	tr := testTranslation(t)
	tr.groupTagOrder = []string{"PROXY"}
	tr.opts = &Options{Country: "cn"}

	generateGeoRoute("cn", "PROXY", tr)

	rules := tr.config.Route.Rules
	if len(rules) != 3 {
		t.Fatalf("expected 3 geo rules for CN, got %d", len(rules))
	}

	// Rule 0: geolocation-!cn → PROXY
	assertRuleSet(t, rules[0], "geosite-geolocation-!cn", "PROXY", "geolocation-!cn")

	// Rule 1: geoip-cn → DIRECT
	assertRuleSet(t, rules[1], "geoip-cn", "DIRECT", "geoip-cn")

	// Rule 2: geosite-geolocation-cn → DIRECT
	assertRuleSet(t, rules[2], "geosite-geolocation-cn", "DIRECT", "geolocation-cn")

	// Verify rule_set definitions
	expectedDefs := []string{"geosite-geolocation-!cn", "geoip-cn", "geosite-geolocation-cn"}
	for _, tag := range expectedDefs {
		if _, ok := tr.ruleSetDefs[tag]; !ok {
			t.Errorf("missing rule_set definition for %q", tag)
		}
	}
}

func TestGenerateGeoRoute_OtherCountry(t *testing.T) {
	tr := testTranslation(t)
	tr.groupTagOrder = []string{"PROXY"}

	generateGeoRoute("us", "PROXY", tr)

	rules := tr.config.Route.Rules
	if len(rules) != 2 {
		t.Fatalf("expected 2 geo rules for US, got %d", len(rules))
	}

	// Rule 0: geoip-us → DIRECT
	assertRuleSet(t, rules[0], "geoip-us", "DIRECT", "geoip-us")

	// Rule 1: geosite-us → DIRECT
	assertRuleSet(t, rules[1], "geosite-us", "DIRECT", "geosite-us")
}

func TestGenerateGeoRoute_NoGroups(t *testing.T) {
	tr := testTranslation(t)

	generateGeoRoute("cn", "DIRECT", tr)

	for _, rule := range tr.config.Route.Rules {
		if ob, ok := rule["outbound"].(string); ok && ob != "DIRECT" {
			t.Errorf("expected all outbounds to be DIRECT when no groups, got %q", ob)
		}
	}
}

func TestGenerateGeoRoute_RuleSetURLs(t *testing.T) {
	tr := testTranslation(t)
	tr.groupTagOrder = []string{"PROXY"}

	generateGeoRoute("cn", "PROXY", tr)

	for tag, def := range tr.ruleSetDefs {
		url, _ := def["url"].(string)
		if !strings.HasPrefix(url, "https://raw.githubusercontent.com/SagerNet/") {
			t.Errorf("rule_set %q has non-SagerNet URL: %s", tag, url)
		}
		if !strings.HasSuffix(url, ".srs") {
			t.Errorf("rule_set %q URL does not end with .srs: %s", tag, url)
		}
		if def["format"] != "binary" {
			t.Errorf("rule_set %q format = %v, want binary", tag, def["format"])
		}
	}
}

func TestGenerateGeoRoute_RuleSetURLPrefix(t *testing.T) {
	tr := testTranslation(t)
	tr.groupTagOrder = []string{"PROXY"}
	tr.opts = &Options{RuleSetURLPrefix: "https://gh-proxy.org"}

	generateGeoRoute("cn", "PROXY", tr)

	for tag, def := range tr.ruleSetDefs {
		url, _ := def["url"].(string)
		if !strings.HasPrefix(url, "https://gh-proxy.org/https://raw.githubusercontent.com/SagerNet/") {
			t.Errorf("rule_set %q URL not proxied: %s", tag, url)
		}
	}
}

func assertRuleSet(t *testing.T, rule map[string]any, expectedTag string, expectedOutbound string, name string) {
	t.Helper()
	rs, _ := rule["rule_set"].([]string)
	if len(rs) != 1 || rs[0] != expectedTag {
		b, _ := json.Marshal(rule)
		t.Errorf("%s: rule_set = %v, want [%s] (rule: %s)", name, rs, expectedTag, b)
	}
	ob, _ := rule["outbound"].(string)
	if ob != expectedOutbound {
		t.Errorf("%s: outbound = %q, want %q", name, ob, expectedOutbound)
	}
}
