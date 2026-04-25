package translator

import "testing"

func TestTranslateProvidersHTTP(t *testing.T) {
	cfg := &RawConfig{
		RuleProvider: map[string]map[string]any{
			"my-provider": {
				"type":     "http",
				"url":      "https://example.com/rules.yaml",
				"behavior": "classical",
				"interval": 3600,
			},
		},
	}
	tt := newTestTranslation()

	translateProviders(cfg, tt)

	tag := "rp-my-provider"
	def, exists := tt.ruleSetDefs[tag]
	if !exists {
		t.Fatal("expected rule_set definition for rp-my-provider")
	}

	if def["type"] != "remote" {
		t.Errorf("type = %v, want remote", def["type"])
	}
	if def["format"] != "source" {
		t.Errorf("format = %v, want source", def["format"])
	}
	if def["url"] != "https://example.com/rules.yaml" {
		t.Errorf("url = %v, want https://example.com/rules.yaml", def["url"])
	}
	if def["update_interval"] != "1h" {
		t.Errorf("update_interval = %v, want 1h", def["update_interval"])
	}
}

func TestTranslateProvidersDomain(t *testing.T) {
	cfg := &RawConfig{
		RuleProvider: map[string]map[string]any{
			"domain-provider": {
				"type":     "http",
				"url":      "https://example.com/domain.list",
				"behavior": "domain",
				"interval": 86400,
			},
		},
	}
	tt := newTestTranslation()

	translateProviders(cfg, tt)

	def := tt.ruleSetDefs["rp-domain-provider"]
	if def == nil {
		t.Fatal("expected rule_set definition for rp-domain-provider")
	}
	if def["format"] != "binary" {
		t.Errorf("format = %v, want binary", def["format"])
	}
}

func TestTranslateProvidersIPCidr(t *testing.T) {
	cfg := &RawConfig{
		RuleProvider: map[string]map[string]any{
			"ipcidr-provider": {
				"type":     "http",
				"url":      "https://example.com/ipcidr.list",
				"behavior": "ipcidr",
				"interval": 86400,
			},
		},
	}
	tt := newTestTranslation()

	translateProviders(cfg, tt)

	def := tt.ruleSetDefs["rp-ipcidr-provider"]
	if def == nil {
		t.Fatal("expected rule_set definition for rp-ipcidr-provider")
	}
	if def["format"] != "binary" {
		t.Errorf("format = %v, want binary", def["format"])
	}
}

func TestTranslateProvidersNil(t *testing.T) {
	cfg := &RawConfig{
		RuleProvider: nil,
	}
	tt := newTestTranslation()

	// Should not panic.
	translateProviders(cfg, tt)

	if len(tt.ruleSetDefs) != 0 {
		t.Errorf("expected 0 rule_set definitions, got %d", len(tt.ruleSetDefs))
	}
}

func TestTranslateProvidersWithProxy(t *testing.T) {
	cfg := &RawConfig{
		RuleProvider: map[string]map[string]any{
			"proxy-provider": {
				"type":     "http",
				"url":      "https://example.com/rules.yaml",
				"behavior": "classical",
				"interval": 3600,
				"proxy":    "my-proxy",
			},
		},
	}
	tt := newTestTranslation()

	translateProviders(cfg, tt)

	def := tt.ruleSetDefs["rp-proxy-provider"]
	if def == nil {
		t.Fatal("expected rule_set definition for rp-proxy-provider")
	}
	if def["download_detour"] != "my-proxy" {
		t.Errorf("download_detour = %v, want my-proxy", def["download_detour"])
	}
}
