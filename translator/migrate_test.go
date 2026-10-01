package translator

import (
	"encoding/json"
	"testing"
)

// legacyGeoDNSConfig 复刻旧版本转换出的订阅档案形态：
// DNS 规则直接引用纯 IP 的 geoip-cn，在 sing-box 1.14 会被拒绝启动。
const legacyGeoDNSConfig = `{
  "log": {"level": "warn"},
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

func TestUpgradeLegacyDNSRulesMigratesGeoIPRule(t *testing.T) {
	upgraded, changed, err := UpgradeLegacyDNSRules(legacyGeoDNSConfig)
	if err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}
	if !changed {
		t.Fatal("expected legacy geo DNS rule to be upgraded")
	}

	var root map[string]any
	if err := json.Unmarshal([]byte(upgraded), &root); err != nil {
		t.Fatalf("upgraded output is not valid JSON: %v", err)
	}
	dns := root["dns"].(map[string]any)
	rules := dns["rules"].([]any)

	// 规则 0 是 clash_mode 直连，不含 rule_set，必须原样保留且不被插入 evaluate。
	if mode, _ := rules[0].(map[string]any)["clash_mode"].(string); mode != "Direct" {
		t.Fatalf("rule[0] changed unexpectedly: %v", rules[0])
	}

	// 规则 1 前应插入 evaluate，且规则 1 自身开启 match_response。
	eval, ok := rules[1].(map[string]any)
	if !ok || eval["action"] != "evaluate" {
		t.Fatalf("expected evaluate before geo rule, got %v", rules[1])
	}
	if eval["server"] != "def-0" {
		t.Errorf("evaluate server = %v, want def-0", eval["server"])
	}

	geo, _ := rules[2].(map[string]any)
	if matched, _ := geo["match_response"].(bool); !matched {
		t.Errorf("geo rule must enable match_response, got %v", geo["match_response"])
	}
	if geo["server"] != "def-0" {
		t.Errorf("geo rule server = %v, want def-0", geo["server"])
	}

	// fakeip 规则不含纯 IP rule-set，不应被改动。
	last, _ := rules[len(rules)-1].(map[string]any)
	if _, has := last["match_response"]; has {
		t.Errorf("fakeip rule should not be upgraded: %v", last)
	}
}

func TestUpgradeLegacyDNSRulesIdempotent(t *testing.T) {
	upgraded, changed, err := UpgradeLegacyDNSRules(legacyGeoDNSConfig)
	if err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}
	if !changed {
		t.Fatal("expected first pass to change config")
	}
	again, changedAgain, err := UpgradeLegacyDNSRules(upgraded)
	if err != nil {
		t.Fatalf("second upgrade failed: %v", err)
	}
	if changedAgain {
		t.Error("upgrade must be idempotent, but second pass reported changes")
	}
	if again != upgraded {
		t.Error("second pass must return the input unchanged")
	}
}

func TestUpgradeLegacyDNSRulesNoopWhenNoGeoIP(t *testing.T) {
	config := `{"dns":{"rules":[{"action":"route","server":"def-0"}]},"route":{"rule_set":[]}}`
	out, changed, err := UpgradeLegacyDNSRules(config)
	if err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}
	if changed {
		t.Error("config without GeoIP DNS rules should not change")
	}
	if out != config {
		t.Error("unchanged config must be returned as-is")
	}
}

func TestUpgradeLegacyDNSRulesKeepsInvalidJSONUntouched(t *testing.T) {
	invalid := "not json at all"
	out, changed, err := UpgradeLegacyDNSRules(invalid)
	if err != nil {
		t.Fatalf("invalid JSON should not error: %v", err)
	}
	if changed || out != invalid {
		t.Error("invalid JSON must be returned untouched")
	}
}
