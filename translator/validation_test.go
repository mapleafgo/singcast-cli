package translator

import (
	"strings"
	"testing"
)

func TestValidateNoDuplicates(t *testing.T) {
	tt := newTestTranslation()
	tt.config.Outbounds = []map[string]any{
		{"type": "http", "tag": "proxy1"},
		{"type": "socks", "tag": "proxy2"},
	}

	err := validate(tt)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidateDuplicateTag(t *testing.T) {
	tt := newTestTranslation()
	tt.config.Outbounds = []map[string]any{
		{"type": "http", "tag": "proxy1"},
		{"type": "socks", "tag": "proxy1"},
	}

	err := validate(tt)

	if err == nil {
		t.Fatal("expected error for duplicate tag, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error should contain 'duplicate', got: %v", err)
	}
}

func TestValidateBuiltinOutbounds(t *testing.T) {
	tt := newTestTranslation()
	tt.config.Outbounds = []map[string]any{
		{"type": "http", "tag": "my-proxy"},
	}
	tt.config.Route.Rules = []map[string]any{
		{"outbound": "DIRECT"},
	}

	err := validate(tt)

	if err != nil {
		t.Errorf("expected no error for builtin outbound reference, got %v", err)
	}
}

func TestValidateFinalInvalid(t *testing.T) {
	tt := newTestTranslation()
	tt.config.Route.Final = "nonexistent"

	err := validate(tt)

	if err == nil {
		t.Fatal("expected error for invalid route.final, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should reference 'nonexistent', got: %v", err)
	}
}

func TestValidateFinalValid(t *testing.T) {
	tt := newTestTranslation()
	tt.config.Route.Final = "DIRECT"

	err := validate(tt)

	if err != nil {
		t.Errorf("expected no error for valid route.final 'DIRECT', got %v", err)
	}
}
