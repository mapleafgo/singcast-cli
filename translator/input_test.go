package translator

import (
	"encoding/base64"
	"testing"
)

func TestConvertURIListSSIPv6BracketHost(t *testing.T) {
	raw := "ss://" +
		base64.StdEncoding.EncodeToString([]byte("aes-128-gcm:pass")) +
		"@[2001:db8::1]:8388#v6\n"

	jsonStr, _, err := Convert([]byte(raw))
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	out := findSSOutbound(t, parseJSON(t, jsonStr))
	if out["server"] != "2001:db8::1" {
		t.Fatalf("server = %v, want 2001:db8::1", out["server"])
	}
	if out["server_port"] != float64(8388) {
		t.Fatalf("server_port = %v, want 8388", out["server_port"])
	}
}

func TestConvertURIListSSEncodedIPv6Host(t *testing.T) {
	raw := "ss://" +
		base64.StdEncoding.EncodeToString(
			[]byte("aes-128-gcm:pass@[2001:db8::1]:8388"),
		) + "\n"

	jsonStr, _, err := Convert([]byte(raw))
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	out := findSSOutbound(t, parseJSON(t, jsonStr))
	if out["server"] != "2001:db8::1" {
		t.Fatalf("server = %v, want 2001:db8::1", out["server"])
	}
	if out["server_port"] != float64(8388) {
		t.Fatalf("server_port = %v, want 8388", out["server_port"])
	}
}

func findSSOutbound(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	raw, ok := doc["outbounds"].([]any)
	if !ok {
		t.Fatal("expected outbounds")
	}
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if ok && m["type"] == "shadowsocks" {
			return m
		}
	}
	t.Fatal("expected shadowsocks outbound")
	return nil
}
