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

	generateCNRoutes("PROXY", tr)

	rules := tr.config.Route.Rules
	// overseas-ai + geolocation-!cn + geosite-cn + geoip-cn + .cn = 5
	if len(rules) != 5 {
		t.Fatalf("expected 5 geo rules for CN, got %d", len(rules))
	}

	// Rule 0: overseas-ai → PROXY
	assertRuleSet(t, rules[0], "overseas-ai", "PROXY", "overseas-ai")

	// Rule 1: geolocation-!cn → PROXY (must come before cn rule)
	assertRuleSet(t, rules[1], "geosite-geolocation-!cn", "PROXY", "geolocation-!cn")

	// Rule 2: geosite-cn → DIRECT
	assertRuleSet(t, rules[2], "geosite-cn", "DIRECT", "geosite-cn")

	// Rule 3: geoip-cn → DIRECT
	assertRuleSet(t, rules[3], "geoip-cn", "DIRECT", "geoip-cn")

	// Rule 4: .cn domain suffix → DIRECT (fallback)
	assertDomainSuffix(t, rules[4], ".cn", "DIRECT")

	// Verify rule_set definitions
	expectedDefs := []string{"geosite-cn", "geosite-geolocation-!cn", "geoip-cn", "overseas-ai"}
	for _, tag := range expectedDefs {
		if _, ok := tr.ruleSetDefs[tag]; !ok {
			t.Errorf("missing rule_set definition for %q", tag)
		}
	}

	// Verify overseas-ai uses custom URL
	aiDef := tr.ruleSetDefs["overseas-ai"]
	aiURL, _ := aiDef["url"].(string)
	if !strings.Contains(aiURL, "viewer12/OverseasAI.list") {
		t.Errorf("overseas-ai URL = %q, want viewer12/OverseasAI.list", aiURL)
	}
}

func TestGenerateGeoRoute_OtherCountry(t *testing.T) {
	tr := testTranslation(t)
	tr.groupTagOrder = []string{"PROXY"}

	generateCountryRoutes("bd", tr)

	rules := tr.config.Route.Rules
	// geoip-bd + .bd = 2；官方 sing-geosite 没有按国家代码的 geosite-bd
	if len(rules) != 2 {
		t.Fatalf("expected 2 geo rules for BD, got %d", len(rules))
	}

	// Rule 0: geoip-bd → DIRECT
	assertRuleSet(t, rules[0], "geoip-bd", "DIRECT", "geoip-bd")

	// Rule 1: .bd domain suffix → DIRECT (fallback)
	assertDomainSuffix(t, rules[1], ".bd", "DIRECT")

	// Verify rule_set definitions
	expectedDefs := []string{"geoip-bd"}
	for _, tag := range expectedDefs {
		if _, ok := tr.ruleSetDefs[tag]; !ok {
			t.Errorf("missing rule_set definition for %q", tag)
		}
	}
	if _, ok := tr.ruleSetDefs["geosite-bd"]; ok {
		t.Error("must not register geosite-bd: SagerNet/sing-geosite has no per-country rule-set")
	}
}

func TestGenerateGeoRoute_NoGroups(t *testing.T) {
	tr := testTranslation(t)

	generateCNRoutes("DIRECT", tr)

	for _, rule := range tr.config.Route.Rules {
		if ob, ok := rule["outbound"].(string); ok && ob != "DIRECT" {
			t.Errorf("expected all outbounds to be DIRECT when no groups, got %q", ob)
		}
	}
}

func TestGenerateGeoRoute_RuleSetURLs(t *testing.T) {
	tr := testTranslation(t)
	tr.groupTagOrder = []string{"PROXY"}

	generateCNRoutes("PROXY", tr)

	for tag, def := range tr.ruleSetDefs {
		url, _ := def["url"].(string)
		if !strings.HasSuffix(url, ".srs") {
			t.Errorf("rule_set %q URL does not end with .srs: %s", tag, url)
		}
		if def["format"] != "binary" {
			t.Errorf("rule_set %q format = %v, want binary", tag, def["format"])
		}
		// SagerNet geo rules must use the standard prefix; custom rules are exempt.
		if tag != "overseas-ai" && !strings.HasPrefix(url, "https://raw.githubusercontent.com/SagerNet/") {
			t.Errorf("rule_set %q has non-SagerNet URL: %s", tag, url)
		}
	}
}

func TestGenerateGeoRoute_RawURL(t *testing.T) {
	tr := testTranslation(t)
	tr.groupTagOrder = []string{"PROXY"}

	generateCNRoutes("PROXY", tr)

	// translator 只做纯翻译，rule_set URL 保持原始值，前缀改写在 core 层做
	for tag, def := range tr.ruleSetDefs {
		url, _ := def["url"].(string)
		if strings.Contains(url, "gh-proxy") {
			t.Errorf("rule_set %q URL should be raw, got: %s", tag, url)
		}
		if !strings.HasPrefix(url, "https://raw.githubusercontent.com") {
			t.Errorf("rule_set %q URL unexpected: %s", tag, url)
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

func assertDomainSuffix(t *testing.T, rule map[string]any, expectedSuffix, expectedOutbound string) {
	t.Helper()
	ds, _ := rule["domain_suffix"].([]string)
	if len(ds) != 1 || ds[0] != expectedSuffix {
		b, _ := json.Marshal(rule)
		t.Errorf("domain_suffix rule: suffix = %v, want [%s] (rule: %s)", ds, expectedSuffix, b)
	}
	ob, _ := rule["outbound"].(string)
	if ob != expectedOutbound {
		t.Errorf("domain_suffix rule: outbound = %q, want %q", ob, expectedOutbound)
	}
}
