package translator

import "testing"

func TestAssembleBuiltins(t *testing.T) {
	tt := newTestTranslation()

	assemble(tt)

	if len(tt.config.Outbounds) < 1 {
		t.Fatalf("expected at least 1 outbound, got %d", len(tt.config.Outbounds))
	}

	wantBuiltins := []struct {
		typ string
		tag string
	}{
		{"direct", "DIRECT"},
	}

	for i, w := range wantBuiltins {
		ob := tt.config.Outbounds[i]
		if ob["type"] != w.typ {
			t.Errorf("outbound[%d].type = %v, want %v", i, ob["type"], w.typ)
		}
		if ob["tag"] != w.tag {
			t.Errorf("outbound[%d].tag = %v, want %v", i, ob["tag"], w.tag)
		}
	}
}

func TestAssembleDefaultRules(t *testing.T) {
	tt := newTestTranslation()

	assemble(tt)

	if len(tt.config.Route.Rules) < 2 {
		t.Fatalf("expected at least 2 rules, got %d", len(tt.config.Route.Rules))
	}

	sniffRule := tt.config.Route.Rules[0]
	if sniffRule["action"] != "sniff" {
		t.Errorf("first rule action = %v, want sniff", sniffRule["action"])
	}

	dnsRule := tt.config.Route.Rules[1]
	if dnsRule["protocol"] != "dns" {
		t.Errorf("second rule protocol = %v, want dns", dnsRule["protocol"])
	}
	if dnsRule["action"] != "hijack-dns" {
		t.Errorf("second rule action = %v, want hijack-dns", dnsRule["action"])
	}
}

func TestAssembleFinalFallback(t *testing.T) {
	// When groupTagOrder has entries, final should be the first group tag.
	tt := newTestTranslation()
	tt.groupTagOrder = []string{"PROXY"}
	tt.groupTags["PROXY"] = true

	assemble(tt)

	if tt.config.Route.Final != "PROXY" {
		t.Errorf("final = %v, want PROXY", tt.config.Route.Final)
	}

	// When no group tags exist, final should fall back to DIRECT.
	tt2 := newTestTranslation()

	assemble(tt2)

	if tt2.config.Route.Final != "DIRECT" {
		t.Errorf("final = %v, want DIRECT", tt2.config.Route.Final)
	}
}

func TestAssembleRuleSetDefs(t *testing.T) {
	tt := newTestTranslation()
	tt.ruleSetDefs["rp-test1"] = map[string]any{
		"tag":    "rp-test1",
		"type":   "remote",
		"format": "source",
	}
	tt.ruleSetDefs["rp-test2"] = map[string]any{
		"tag":    "rp-test2",
		"type":   "remote",
		"format": "binary",
	}

	assemble(tt)

	if len(tt.config.Route.RuleSet) != 2 {
		t.Fatalf("expected 2 rule_set entries, got %d", len(tt.config.Route.RuleSet))
	}

	foundTags := make(map[string]bool)
	for _, rs := range tt.config.Route.RuleSet {
		tag, _ := rs["tag"].(string)
		foundTags[tag] = true
	}

	if !foundTags["rp-test1"] {
		t.Error("missing rule_set entry with tag rp-test1")
	}
	if !foundTags["rp-test2"] {
		t.Error("missing rule_set entry with tag rp-test2")
	}
}
