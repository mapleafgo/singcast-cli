package translator

import (
	"testing"

	"github.com/mapleafgo/singcast/translator/proxy"
)

func newTestTranslation() *translation {
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

func TestTranslateSelectGroup(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["a"] = true
	tr.proxyTags["b"] = true

	cfg := &RawConfig{
		ProxyGroup: []map[string]any{
			{
				"name":    "MySelect",
				"type":    "select",
				"proxies": []any{"a", "b", "DIRECT"},
			},
		},
	}

	groups := translateGroups(cfg, tr)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	g := groups[0]
	if g["type"] != "selector" {
		t.Errorf("type = %v, want selector", g["type"])
	}
	if g["tag"] != "MySelect" {
		t.Errorf("tag = %v, want MySelect", g["tag"])
	}
	if g["default"] != "a" {
		t.Errorf("default = %v, want a", g["default"])
	}
	if g["interrupt_exist_connections"] != true {
		t.Error("interrupt_exist_connections should be true")
	}
	outbounds, _ := g["outbounds"].([]string)
	if len(outbounds) != 3 || outbounds[0] != "a" || outbounds[1] != "b" || outbounds[2] != "DIRECT" {
		t.Errorf("outbounds = %v, want [a b DIRECT]", outbounds)
	}
}

func TestTranslateURLTestGroup(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["p1"] = true
	tr.proxyTags["p2"] = true

	cfg := &RawConfig{
		ProxyGroup: []map[string]any{
			{
				"name":     "Auto",
				"type":     "url-test",
				"proxies":  []any{"p1", "p2"},
				"url":      "http://www.gstatic.com/generate_204",
				"interval": 300,
			},
		},
	}

	groups := translateGroups(cfg, tr)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	g := groups[0]
	if g["type"] != "urltest" {
		t.Errorf("type = %v, want urltest", g["type"])
	}
	if g["tag"] != "Auto" {
		t.Errorf("tag = %v, want Auto", g["tag"])
	}
	if g["url"] != "http://www.gstatic.com/generate_204" {
		t.Errorf("url = %v, want http://www.gstatic.com/generate_204", g["url"])
	}
	interval := g["interval"]
	if interval != proxy.SecondsToDuration(300) {
		t.Errorf("interval = %v, want %v", interval, proxy.SecondsToDuration(300))
	}
	if interval != "5m" {
		t.Errorf("interval = %v, want 5m", interval)
	}
}

func TestTranslateFallbackGroup(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["p1"] = true
	tr.proxyTags["p2"] = true

	cfg := &RawConfig{
		ProxyGroup: []map[string]any{
			{
				"name":    "Fallback",
				"type":    "fallback",
				"proxies": []any{"p1", "p2"},
				"url":     "http://www.gstatic.com/generate_204",
			},
		},
	}

	groups := translateGroups(cfg, tr)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	g := groups[0]
	if g["type"] != "urltest" {
		t.Errorf("type = %v, want urltest", g["type"])
	}
	if g["tolerance"] != 65535 {
		t.Errorf("tolerance = %v, want 65535", g["tolerance"])
	}
}

func TestTranslateLoadBalanceGroup(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["p1"] = true
	tr.proxyTags["p2"] = true

	cfg := &RawConfig{
		ProxyGroup: []map[string]any{
			{
				"name":    "LB",
				"type":    "load-balance",
				"proxies": []any{"p1", "p2"},
			},
		},
	}

	groups := translateGroups(cfg, tr)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	g := groups[0]
	if g["type"] != "selector" {
		t.Errorf("type = %v, want selector (degraded from load-balance)", g["type"])
	}

	// Check that a warning was produced about load-balance
	found := false
	for _, w := range tr.warnings {
		if len(w) > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a warning for load-balance degradation")
	}
}

func TestTranslateRelayGroup(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["p1"] = true
	tr.proxyTags["p2"] = true

	cfg := &RawConfig{
		ProxyGroup: []map[string]any{
			{
				"name":    "Relay",
				"type":    "relay",
				"proxies": []any{"p1", "p2"},
			},
		},
	}

	groups := translateGroups(cfg, tr)
	if len(groups) != 0 {
		t.Fatalf("expected 0 groups (relay unsupported), got %d", len(groups))
	}

	// Check that a warning was produced
	found := false
	for _, w := range tr.warnings {
		if len(w) > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a warning for relay being unsupported")
	}
}

func TestFilterGroupProxies(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["known-proxy"] = true
	tr.groupTags["known-group"] = true

	g := map[string]any{
		"proxies": []any{
			"known-proxy",
			"known-group",
			"DIRECT",
			"unknown-proxy",
			123, // non-string
		},
	}

	filtered := filterGroupProxies(g, tr)
	if len(filtered) != 3 {
		t.Fatalf("expected 3 filtered proxies, got %d: %v", len(filtered), filtered)
	}

	expected := map[string]bool{
		"known-proxy": true,
		"known-group": true,
		"DIRECT":      true,
	}
	for _, p := range filtered {
		if !expected[p] {
			t.Errorf("unexpected proxy in filtered result: %s", p)
		}
	}
}

func TestToInt(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		wantVal int
		wantOk  bool
	}{
		{"int", 5, 5, true},
		{"int64", int64(5), 5, true},
		{"float64", float64(5.0), 5, true},
		{"string", "5", 0, false},
		{"nil", nil, 0, false},
		{"bool", true, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, ok := toInt(tt.input)
			if val != tt.wantVal {
				t.Errorf("toInt(%v) value = %d, want %d", tt.input, val, tt.wantVal)
			}
			if ok != tt.wantOk {
				t.Errorf("toInt(%v) ok = %v, want %v", tt.input, ok, tt.wantOk)
			}
		})
	}
}
