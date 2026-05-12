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
	if len(rules) != 6 {
		t.Fatalf("expected 6 geo rules for CN, got %d", len(rules))
	}

	// Rule 0: overseas-ai → PROXY
	assertRuleSet(t, rules[0], "overseas-ai", "PROXY", "overseas-ai")

	// Rule 1: geolocation-!cn → PROXY (must come before cn rule)
	assertRuleSet(t, rules[1], "geosite-geolocation-!cn", "PROXY", "geolocation-!cn")

	// Rule 2: logical AND — geolocation-cn AND NOT geoip-cn → PROXY
	assertGeoIPMismatchGuard(t, rules[2], "geosite-geolocation-cn", "geoip-cn", "PROXY")

	// Rule 3: geolocation-cn → DIRECT
	assertRuleSet(t, rules[3], "geosite-geolocation-cn", "DIRECT", "geolocation-cn")

	// Rule 4: logical AND — (NOT geolocation-!cn) AND geoip-cn → DIRECT
	logical := rules[4]
	if logical["type"] != "logical" {
		t.Fatalf("rule 4 type = %v, want logical", logical["type"])
	}
	if logical["mode"] != "and" {
		t.Fatalf("rule 4 mode = %v, want and", logical["mode"])
	}
	if logical["outbound"] != "DIRECT" {
		t.Fatalf("rule 4 outbound = %v, want DIRECT", logical["outbound"])
	}
	subRules, ok := logical["rules"].([]map[string]any)
	if !ok || len(subRules) != 2 {
		t.Fatalf("rule 4 sub-rules: ok=%v len=%d", ok, len(subRules))
	}
	// Sub-rule 0: geolocation-!cn inverted
	if subRules[0]["invert"] != true {
		t.Error("sub-rule 0 should have invert=true")
	}
	rs0, _ := subRules[0]["rule_set"].([]string)
	if len(rs0) != 1 || rs0[0] != "geosite-geolocation-!cn" {
		t.Errorf("sub-rule 0 rule_set = %v, want [geosite-geolocation-!cn]", rs0)
	}
	// Sub-rule 1: geoip-cn
	rs1, _ := subRules[1]["rule_set"].([]string)
	if len(rs1) != 1 || rs1[0] != "geoip-cn" {
		t.Errorf("sub-rule 1 rule_set = %v, want [geoip-cn]", rs1)
	}

	// Rule 5: .cn domain suffix → DIRECT (fallback after all geo rules)
	assertDomainSuffix(t, rules[5], ".cn", "DIRECT")

	// Verify rule_set definitions
	expectedDefs := []string{"geosite-geolocation-cn", "geosite-geolocation-!cn", "geoip-cn", "overseas-ai"}
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

	generateGeoRoute("us", "PROXY", tr)

	rules := tr.config.Route.Rules
	if len(rules) != 5 {
		t.Fatalf("expected 5 geo rules for US, got %d", len(rules))
	}

	// Rule 0: geolocation-!cn → PROXY
	assertRuleSet(t, rules[0], "geosite-geolocation-!cn", "PROXY", "geolocation-!cn")

	// Rule 1: logical AND — geosite-us AND NOT geoip-us → PROXY
	assertGeoIPMismatchGuard(t, rules[1], "geosite-us", "geoip-us", "PROXY")

	// Rule 2: geosite-us → DIRECT
	assertRuleSet(t, rules[2], "geosite-us", "DIRECT", "geosite-us")

	// Rule 3: logical AND — NOT geolocation-!cn AND geoip-us → DIRECT
	geoIPRule := rules[3]
	if geoIPRule["type"] != "logical" {
		t.Fatalf("rule 3 type = %v, want logical", geoIPRule["type"])
	}
	if geoIPRule["mode"] != "and" {
		t.Fatalf("rule 3 mode = %v, want and", geoIPRule["mode"])
	}
	if geoIPRule["outbound"] != "DIRECT" {
		t.Fatalf("rule 3 outbound = %v, want DIRECT", geoIPRule["outbound"])
	}
	subRules, ok := geoIPRule["rules"].([]map[string]any)
	if !ok || len(subRules) != 2 {
		t.Fatalf("rule 3 sub-rules: ok=%v len=%d", ok, len(subRules))
	}
	if subRules[0]["invert"] != true {
		t.Error("sub-rule 0 should have invert=true")
	}
	rs0, _ := subRules[0]["rule_set"].([]string)
	if len(rs0) != 1 || rs0[0] != "geosite-geolocation-!cn" {
		t.Errorf("sub-rule 0 rule_set = %v, want [geosite-geolocation-!cn]", rs0)
	}
	rs1, _ := subRules[1]["rule_set"].([]string)
	if len(rs1) != 1 || rs1[0] != "geoip-us" {
		t.Errorf("sub-rule 1 rule_set = %v, want [geoip-us]", rs1)
	}

	// Rule 4: .us domain suffix → DIRECT (fallback)
	assertDomainSuffix(t, rules[4], ".us", "DIRECT")

	// Verify rule_set definitions
	expectedDefs := []string{"geosite-geolocation-!cn", "geosite-us", "geoip-us"}
	for _, tag := range expectedDefs {
		if _, ok := tr.ruleSetDefs[tag]; !ok {
			t.Errorf("missing rule_set definition for %q", tag)
		}
	}
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

func TestGenerateGeoRoute_RuleSetURLPrefix(t *testing.T) {
	tr := testTranslation(t)
	tr.groupTagOrder = []string{"PROXY"}
	tr.opts = &Options{RuleSetURLPrefix: "https://gh-proxy.org"}

	generateGeoRoute("cn", "PROXY", tr)

	for tag, def := range tr.ruleSetDefs {
		url, _ := def["url"].(string)
		if !strings.HasPrefix(url, "https://gh-proxy.org/") {
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

func assertGeoIPMismatchGuard(t *testing.T, rule map[string]any, geositeTag, geoipTag, expectedOutbound string) {
	t.Helper()
	if rule["type"] != "logical" {
		b, _ := json.Marshal(rule)
		t.Fatalf("expected logical rule, got: %s", b)
	}
	if rule["mode"] != "and" {
		t.Fatalf("mode = %v, want and", rule["mode"])
	}
	ob, _ := rule["outbound"].(string)
	if ob != expectedOutbound {
		t.Fatalf("outbound = %q, want %q", ob, expectedOutbound)
	}
	subRules, ok := rule["rules"].([]map[string]any)
	if !ok || len(subRules) != 2 {
		t.Fatalf("sub-rules: ok=%v len=%d", ok, len(subRules))
	}
	// Sub-rule 0: geosite match (no invert)
	rs0, _ := subRules[0]["rule_set"].([]string)
	if len(rs0) != 1 || rs0[0] != geositeTag {
		t.Errorf("sub-rule 0 rule_set = %v, want [%s]", rs0, geositeTag)
	}
	if subRules[0]["invert"] != nil {
		t.Error("sub-rule 0 should not have invert")
	}
	// Sub-rule 1: geoip match with invert
	rs1, _ := subRules[1]["rule_set"].([]string)
	if len(rs1) != 1 || rs1[0] != geoipTag {
		t.Errorf("sub-rule 1 rule_set = %v, want [%s]", rs1, geoipTag)
	}
	if subRules[1]["invert"] != true {
		t.Error("sub-rule 1 should have invert=true")
	}
}
