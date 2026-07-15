package translator

import (
	"strings"
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
		proxyTags:                   make(map[string]bool),
		groupTags:                   make(map[string]bool),
		stubTags:                    make(map[string]string),
		invalidHealthCheckProxyTags: make(map[string]bool),
		ruleSetDefs:                 make(map[string]map[string]any),
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

// TestTranslateURLTestGroupIntervalClamp 验证过大的 interval 被截断并产生警告
func TestTranslateURLTestGroupIntervalClamp(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["p1"] = true
	tr.proxyTags["p2"] = true

	cfg := &RawConfig{
		ProxyGroup: []map[string]any{
			{
				"name":     "Auto",
				"type":     "url-test",
				"proxies":  []any{"p1", "p2"},
				"interval": 86400, // 24 hours, should be clamped to 1800
			},
		},
	}

	groups := translateGroups(cfg, tr)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	g := groups[0]
	interval := g["interval"]
	if interval != proxy.SecondsToDuration(1800) {
		t.Errorf("interval = %v, want %v (clamped)", interval, proxy.SecondsToDuration(1800))
	}
	if interval != "30m" {
		t.Errorf("interval = %v, want 30m", interval)
	}
	// idle_timeout = max(1800*2, 1800) = 3600
	idleTimeout := g["idle_timeout"]
	if idleTimeout != proxy.SecondsToDuration(3600) {
		t.Errorf("idle_timeout = %v, want %v", idleTimeout, proxy.SecondsToDuration(3600))
	}
	// 应该有警告
	foundWarn := false
	for _, w := range tr.warnings {
		if strings.Contains(w, "clamped") {
			foundWarn = true
			break
		}
	}
	if !foundWarn {
		t.Error("expected a warning about interval being clamped")
	}
}

func TestTranslateURLTestGroupFiltersInvalidSubscriptionInfoProxy(t *testing.T) {
	tr := newTestTranslation()
	cfg := &RawConfig{
		Proxy: []map[string]any{
			{
				"name":     "剩余流量：172.37 GB",
				"type":     "hysteria2",
				"server":   "127.0.0.1",
				"port":     65535,
				"password": "test-password",
			},
			{
				"name":     "valid-node",
				"type":     "hysteria2",
				"server":   "example.com",
				"port":     443,
				"password": "test-password",
			},
		},
		ProxyGroup: []map[string]any{
			{
				"name":    "Manual",
				"type":    "select",
				"proxies": []any{"剩余流量：172.37 GB", "valid-node"},
			},
			{
				"name":    "Auto",
				"type":    "url-test",
				"proxies": []any{"剩余流量：172.37 GB", "valid-node"},
			},
		},
	}

	translateProxies(cfg, tr)
	groups := translateGroups(cfg, tr)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d: %v", len(groups), groups)
	}

	selectOutbounds, _ := groups[0]["outbounds"].([]string)
	if len(selectOutbounds) != 2 {
		t.Fatalf("select group outbounds = %v, want invalid proxy preserved with valid node", selectOutbounds)
	}

	urlTestOutbounds, _ := groups[1]["outbounds"].([]string)
	if len(urlTestOutbounds) != 1 || urlTestOutbounds[0] != "valid-node" {
		t.Fatalf("url-test group outbounds = %v, want [valid-node]", urlTestOutbounds)
	}
}

func TestTranslateFallbackGroupFiltersInvalidSubscriptionInfoProxy(t *testing.T) {
	tr := newTestTranslation()
	cfg := &RawConfig{
		Proxy: []map[string]any{
			{
				"name":     "官网：keluosi.top",
				"type":     "hysteria2",
				"server":   "127.0.0.1",
				"port":     65535,
				"password": "test-password",
			},
			{
				"name":     "valid-node",
				"type":     "hysteria2",
				"server":   "example.com",
				"port":     443,
				"password": "test-password",
			},
		},
		ProxyGroup: []map[string]any{
			{
				"name":    "Fallback",
				"type":    "fallback",
				"proxies": []any{"官网：keluosi.top", "valid-node"},
			},
		},
	}

	translateProxies(cfg, tr)
	groups := translateGroups(cfg, tr)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d: %v", len(groups), groups)
	}

	outbounds, _ := groups[0]["outbounds"].([]string)
	if len(outbounds) != 1 || outbounds[0] != "valid-node" {
		t.Fatalf("fallback group outbounds = %v, want [valid-node]", outbounds)
	}
}

func TestHealthCheckGroupsFilterStaticallyInvalidProxyEndpoints(t *testing.T) {
	tr := newTestTranslation()
	invalidProxies := []map[string]any{
		{
			"name":     "loopback-any-port",
			"type":     "hysteria2",
			"server":   "127.0.0.1",
			"port":     443,
			"password": "test-password",
		},
		{
			"name":     "localhost-hostname",
			"type":     "hysteria2",
			"server":   "localhost",
			"port":     443,
			"password": "test-password",
		},
		{
			"name":     "ipv6-loopback",
			"type":     "hysteria2",
			"server":   "::1",
			"port":     443,
			"password": "test-password",
		},
		{
			"name":     "bracketed-ipv6-loopback",
			"type":     "hysteria2",
			"server":   "[::1]",
			"port":     443,
			"password": "test-password",
		},
		{
			"name":     "unspecified-ipv4",
			"type":     "hysteria2",
			"server":   "0.0.0.0",
			"port":     443,
			"password": "test-password",
		},
		{
			"name":     "unspecified-ipv6",
			"type":     "hysteria2",
			"server":   "::",
			"port":     443,
			"password": "test-password",
		},
		{
			"name":     "invalid-high-port",
			"type":     "hysteria2",
			"server":   "example.net",
			"port":     70000,
			"password": "test-password",
		},
		{
			"name":     "invalid-negative-port",
			"type":     "hysteria2",
			"server":   "example.org",
			"port":     -1,
			"password": "test-password",
		},
	}
	proxies := append([]map[string]any{}, invalidProxies...)
	proxies = append(proxies, map[string]any{
		"name":     "valid-node",
		"type":     "hysteria2",
		"server":   "example.com",
		"port":     443,
		"password": "test-password",
	})

	groupMembers := []any{
		"loopback-any-port",
		"localhost-hostname",
		"ipv6-loopback",
		"bracketed-ipv6-loopback",
		"unspecified-ipv4",
		"unspecified-ipv6",
		"invalid-high-port",
		"invalid-negative-port",
		"valid-node",
	}
	cfg := &RawConfig{
		Proxy: proxies,
		ProxyGroup: []map[string]any{
			{
				"name":    "Manual",
				"type":    "select",
				"proxies": groupMembers,
			},
			{
				"name":    "Auto",
				"type":    "url-test",
				"proxies": groupMembers,
			},
			{
				"name":    "Fallback",
				"type":    "fallback",
				"proxies": groupMembers,
			},
		},
	}

	translateProxies(cfg, tr)
	groups := translateGroups(cfg, tr)
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d: %v", len(groups), groups)
	}

	selectOutbounds, _ := groups[0]["outbounds"].([]string)
	if len(selectOutbounds) != len(groupMembers) {
		t.Fatalf("select group outbounds = %v, want all members preserved", selectOutbounds)
	}

	for _, index := range []int{1, 2} {
		outbounds, _ := groups[index]["outbounds"].([]string)
		if len(outbounds) != 1 || outbounds[0] != "valid-node" {
			t.Fatalf("%s outbounds = %v, want [valid-node]", groups[index]["tag"], outbounds)
		}
	}
}

func TestHealthCheckGroupsFilterDegradedStubProxies(t *testing.T) {
	tr := newTestTranslation()
	cfg := &RawConfig{
		Proxy: []map[string]any{
			{
				"name":   "unsupported-node",
				"type":   "ssr",
				"server": "example.net",
				"port":   443,
			},
			{
				"name":     "valid-node",
				"type":     "hysteria2",
				"server":   "example.com",
				"port":     443,
				"password": "test-password",
			},
		},
		ProxyGroup: []map[string]any{
			{
				"name":    "Auto",
				"type":    "url-test",
				"proxies": []any{"unsupported-node", "valid-node"},
			},
			{
				"name":    "Fallback",
				"type":    "fallback",
				"proxies": []any{"unsupported-node", "valid-node"},
			},
			{
				"name":    "Manual",
				"type":    "select",
				"proxies": []any{"unsupported-node", "valid-node"},
			},
		},
	}

	translateProxies(cfg, tr)
	groups := translateGroups(cfg, tr)
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d: %v", len(groups), groups)
	}

	for _, index := range []int{0, 1} {
		outbounds, _ := groups[index]["outbounds"].([]string)
		if len(outbounds) != 1 || outbounds[0] != "valid-node" {
			t.Fatalf("%s outbounds = %v, want [valid-node]", groups[index]["tag"], outbounds)
		}
	}

	selectOutbounds, _ := groups[2]["outbounds"].([]string)
	if len(selectOutbounds) != 2 {
		t.Fatalf("select group outbounds = %v, want unsupported stub preserved with valid node", selectOutbounds)
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
	if g["tolerance"] != 10000 {
		t.Errorf("tolerance = %v, want 10000", g["tolerance"])
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
