package translator

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestNormalizeInputBase64URIList(t *testing.T) {
	ssBody := base64.StdEncoding.EncodeToString([]byte("aes-128-gcm:pass@host.com:8080"))
	raw := "ss://" + ssBody + "#p1\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))
	out, err := NormalizeInput([]byte(encoded))
	if err != nil {
		t.Fatalf("NormalizeInput: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "proxies:") || !strings.Contains(s, "p1") {
		t.Fatalf("expected mihomo YAML with proxies, got: %s", s)
	}
}

func TestNormalizeInputBase64JSON(t *testing.T) {
	jsonStr := `{"log":{"level":"info"},"outbounds":[{"type":"direct","tag":"direct"}]}`
	encoded := base64.StdEncoding.EncodeToString([]byte(jsonStr))
	out, err := NormalizeInput([]byte(encoded))
	if err != nil {
		t.Fatalf("NormalizeInput: %v", err)
	}
	if string(out) != jsonStr {
		t.Fatalf("expected decoded JSON, got: %s", out)
	}
}

func TestNormalizeInputRawYAMLPassthrough(t *testing.T) {
	yaml := "mixed-port: 7890\nproxies:\n  - name: p\n    type: ss\n"
	out, err := NormalizeInput([]byte(yaml))
	if err != nil {
		t.Fatalf("NormalizeInput: %v", err)
	}
	if string(out) != yaml {
		t.Fatalf("expected passthrough, got: %s", out)
	}
}
