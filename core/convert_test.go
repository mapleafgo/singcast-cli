package core

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
)

func TestConvertYAML(t *testing.T) {
	yaml := "proxies:\n  - name: p\n    type: ss\n    server: 1.2.3.4\n    port: 443\n    cipher: aes-128-gcm\n    password: x\n"
	jsonStr, err := Convert(yaml)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !strings.Contains(jsonStr, `"outbounds"`) {
		t.Fatalf("expected sing-box JSON, got: %s", jsonStr)
	}
}

func TestConvertBase64URIList(t *testing.T) {
	raw := "ss://" + base64.StdEncoding.EncodeToString([]byte("aes-128-gcm:x@1.2.3.4:443")) + "#p\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))
	jsonStr, err := Convert(encoded)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !strings.Contains(jsonStr, `"outbounds"`) {
		t.Fatalf("expected sing-box JSON, got: %s", jsonStr)
	}
}

func TestCheckConfigBase64URIList(t *testing.T) {
	raw := "trojan://pass@example.com:443#tr\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))
	if err := CheckConfig(context.Background(), encoded); err != nil {
		t.Fatalf("CheckConfig: %v", err)
	}
}
