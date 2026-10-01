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

// legacyGeoDNSJSON 复刻旧版本转换出的已保存 profile：DNS 规则直接引用纯 IP 的
// geoip-cn，在 sing-box 1.14 会被拒绝启动，必须由透传路径升级为响应匹配。
const legacyGeoDNSJSON = `{
  "dns": {
    "servers": [
      {"type": "udp", "tag": "def-0", "server": "223.5.5.5"},
      {"type": "fakeip", "tag": "fakeip-dns", "inet4_range": "198.18.0.0/15"}
    ],
    "rules": [
      {"action": "route", "clash_mode": "Direct", "server": "def-0"},
      {"action": "route", "rule_set": ["geosite-private", "geosite-cn", "geoip-cn"], "server": "def-0"},
      {"action": "route", "query_type": ["A", "AAAA"], "server": "fakeip-dns"}
    ],
    "final": "ns-0"
  },
  "outbounds": [{"type": "direct", "tag": "DIRECT"}],
  "route": {
    "rules": [],
    "rule_set": [
      {"type": "remote", "tag": "geoip-cn", "format": "binary",
       "url": "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-cn.srs"},
      {"type": "remote", "tag": "geosite-cn", "format": "binary",
       "url": "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-cn.srs"}
    ],
    "final": "DIRECT"
  }
}`

func TestConvertUpgradesLegacyGeoDNSRules(t *testing.T) {
	jsonStr, err := Convert(legacyGeoDNSJSON)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !strings.Contains(jsonStr, `"match_response":true`) {
		t.Fatalf("legacy geo DNS rule was not upgraded to response matching:\n%s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"action":"evaluate"`) {
		t.Fatalf("missing evaluate action for legacy geo DNS rule:\n%s", jsonStr)
	}
}

func TestCheckConfigUpgradesLegacyGeoDNSRules(t *testing.T) {
	if err := CheckConfig(context.Background(), legacyGeoDNSJSON); err != nil {
		t.Fatalf("legacy converted profile must stay loadable: %v", err)
	}
}
