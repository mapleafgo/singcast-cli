package translator

import "testing"

func TestTranslateProvidersSkipped(t *testing.T) {
	cfg := &RawConfig{
		RuleProvider: map[string]map[string]any{
			"my-provider": {
				"type":     "http",
				"url":      "https://example.com/rules.yaml",
				"behavior": "classical",
				"interval": 3600,
			},
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

	// All providers should be skipped
	if len(tt.ruleSetDefs) != 0 {
		t.Errorf("expected 0 rule_set definitions, got %d", len(tt.ruleSetDefs))
	}
	if len(tt.warnings) != 2 {
		t.Errorf("expected 2 warnings, got %d", len(tt.warnings))
	}
}

func TestTranslateProvidersNil(t *testing.T) {
	cfg := &RawConfig{
		RuleProvider: nil,
	}
	tt := newTestTranslation()

	translateProviders(cfg, tt)

	if len(tt.ruleSetDefs) != 0 {
		t.Errorf("expected 0 rule_set definitions, got %d", len(tt.ruleSetDefs))
	}
	if len(tt.warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d", len(tt.warnings))
	}
}
