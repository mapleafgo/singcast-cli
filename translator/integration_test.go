package translator

import (
	"strings"
	"testing"
)

func TestIntegrationNoDuplicateTags(t *testing.T) {
	yaml := `mixed-port: 7890
proxies:
  - name: p1
    type: socks5
    server: 1.2.3.4
    port: 1080
  - name: p2
    type: socks5
    server: 5.6.7.8
    port: 1081
proxy-groups:
  - name: PROXY
    type: select
    proxies: [p1, p2, DIRECT]
`
	m, _ := mustTranslate(t, yaml)
	outbounds := m["outbounds"].([]any)
	seen := map[string]bool{}
	for _, ob := range outbounds {
		tag := ob.(map[string]any)["tag"].(string)
		if seen[tag] {
			t.Errorf("duplicate outbound tag: %s", tag)
		}
		seen[tag] = true
	}
}

// TestIntegrationAutoRoutingCN verifies CN auto-routing with a minimal config.
func TestIntegrationAutoRoutingCN(t *testing.T) {
	yaml := `mixed-port: 7890
proxies:
  - name: p1
    type: socks5
    server: 1.2.3.4
    port: 1080
proxy-groups:
  - name: PROXY
    type: select
    proxies: [p1, DIRECT]
`
	// 固定 Country=CN：该用例验证 CN 自动分流规则，不能依赖运行环境 IP 地理定位。
	// CI runner 通常在非 CN 区域，DetectCountry 成功时会走 generateCountryRoutes（7 条规则）导致失败。
	out, _, _, err := ConvertWithMeta([]byte(yaml), &Options{Country: "CN"})
	if err != nil {
		t.Fatal(err)
	}
	m := parseJSON(t, out)
	route := m["route"].(map[string]any)

	// final should be PROXY
	if route["final"] != "PROXY" {
		t.Errorf("route.final = %v, want PROXY", route["final"])
	}

	rules := route["rules"].([]any)
	// sniff + hijack-dns + clash_mode:Direct + clash_mode:Global + ip_is_private +
	// overseas-ai + geolocation-!cn + geosite-cn + geoip-cn + domain_suffix:.cn = 10
	if len(rules) != 10 {
		t.Fatalf("expected 10 rules for CN, got %d", len(rules))
	}

	// Verify clash_mode:Direct catch-all
	directRule := rules[2].(map[string]any)
	if directRule["clash_mode"] != "Direct" || directRule["outbound"] != "DIRECT" {
		t.Errorf("rule 2 should be clash_mode:Direct → DIRECT, got %v", directRule)
	}

	// Verify clash_mode:Global catch-all
	globalRule := rules[3].(map[string]any)
	if globalRule["clash_mode"] != "Global" || globalRule["outbound"] != "PROXY" {
		t.Errorf("rule 3 should be clash_mode:Global → PROXY, got %v", globalRule)
	}

	// Verify ip_is_private rule (has clash_mode:"Rule")
	privateRule := rules[4].(map[string]any)
	if privateRule["ip_is_private"] != true {
		t.Error("rule 4 should have ip_is_private=true")
	}
	if privateRule["clash_mode"] != "Rule" {
		t.Error("ip_is_private rule should have clash_mode=Rule")
	}
	if privateRule["outbound"] != "DIRECT" {
		t.Errorf("ip_is_private outbound = %v, want DIRECT", privateRule["outbound"])
	}

	// Verify overseas-ai → proxy rule
	aiRule := rules[5].(map[string]any)
	aiRS, _ := aiRule["rule_set"].([]any)
	if len(aiRS) != 1 || aiRS[0] != "overseas-ai" {
		t.Errorf("rule 5 rule_set = %v, want [overseas-ai]", aiRS)
	}
	if aiRule["outbound"] != "PROXY" {
		t.Errorf("rule 5 outbound = %v, want PROXY", aiRule["outbound"])
	}

	// Verify geosite-geolocation-!cn → proxy rule (comes BEFORE cn rule)
	notCNRule := rules[6].(map[string]any)
	notCNRS, _ := notCNRule["rule_set"].([]any)
	if len(notCNRS) != 1 || notCNRS[0] != "geosite-geolocation-!cn" {
		t.Errorf("rule 6 rule_set = %v, want [geosite-geolocation-!cn]", notCNRS)
	}
	if notCNRule["outbound"] != "PROXY" {
		t.Errorf("rule 6 outbound = %v, want PROXY", notCNRule["outbound"])
	}
	if notCNRule["clash_mode"] != "Rule" {
		t.Error("geolocation-!cn rule should have clash_mode=Rule")
	}

	// Verify geosite-cn → DIRECT rule
	geoCNRule := rules[7].(map[string]any)
	geoCNRS, _ := geoCNRule["rule_set"].([]any)
	if len(geoCNRS) != 1 || geoCNRS[0] != "geosite-cn" {
		t.Errorf("rule 7 rule_set = %v, want [geosite-cn]", geoCNRS)
	}
	if geoCNRule["outbound"] != "DIRECT" {
		t.Errorf("rule 7 outbound = %v, want DIRECT", geoCNRule["outbound"])
	}

	// Verify geoip-cn → DIRECT rule
	geoipCNRule := rules[8].(map[string]any)
	geoipCNRS, _ := geoipCNRule["rule_set"].([]any)
	if len(geoipCNRS) != 1 || geoipCNRS[0] != "geoip-cn" {
		t.Errorf("rule 8 rule_set = %v, want [geoip-cn]", geoipCNRS)
	}
	if geoipCNRule["outbound"] != "DIRECT" {
		t.Errorf("rule 8 outbound = %v, want DIRECT", geoipCNRule["outbound"])
	}

	// Verify domain_suffix:.cn → DIRECT rule (fallback after geo rules)
	cnSuffixRule := rules[9].(map[string]any)
	cnDS, _ := cnSuffixRule["domain_suffix"].([]any)
	if len(cnDS) != 1 || cnDS[0] != ".cn" {
		t.Errorf("rule 9 domain_suffix = %v, want [.cn]", cnDS)
	}
	if cnSuffixRule["outbound"] != "DIRECT" {
		t.Errorf("rule 9 outbound = %v, want DIRECT", cnSuffixRule["outbound"])
	}

	// Verify rule_set definitions
	rsDefs := route["rule_set"].([]any)
	rsTags := map[string]bool{}
	for _, rs := range rsDefs {
		rsMap := rs.(map[string]any)
		rsTags[rsMap["tag"].(string)] = true
	}
	for _, tag := range []string{"geosite-geolocation-!cn", "geoip-cn", "geosite-cn", "overseas-ai"} {
		if !rsTags[tag] {
			t.Errorf("missing rule_set: %s", tag)
		}
	}
}

// TestIntegrationCountryOverride 验证 Options.Country 能覆盖自动检测，
// 使非 CN 国家走 generateCountryRoutes 分支。
func TestIntegrationCountryOverride(t *testing.T) {
	yaml := `mixed-port: 7890
proxies:
  - name: p1
    type: socks5
    server: 1.2.3.4
    port: 1080
proxy-groups:
  - name: PROXY
    type: select
    proxies: [p1, DIRECT]
`
	out, _, _, err := ConvertWithMeta([]byte(yaml), &Options{Country: "JP"})
	if err != nil {
		t.Fatal(err)
	}
	m := parseJSON(t, out)
	route := m["route"].(map[string]any)

	// geoip-jp rule_set 应存在（geosite-jp 不存在上游）
	rsDefs := route["rule_set"].([]any)
	rsTags := map[string]bool{}
	for _, rs := range rsDefs {
		rsMap := rs.(map[string]any)
		rsTags[rsMap["tag"].(string)] = true
	}
	if !rsTags["geoip-jp"] {
		t.Error("missing rule_set: geoip-jp")
	}
	if rsTags["geosite-jp"] {
		t.Error("must not register geosite-jp: SagerNet/sing-geosite has no per-country rule-set")
	}
}

// TestIntegrationCountryOverrideInvalid 验证非两位的国家覆盖会回退自动检测，
// 不再生成 geoip-usa/domain_suffix ".usa" 这类坏规则。
func TestIntegrationCountryOverrideInvalid(t *testing.T) {
	yaml := `mixed-port: 7890
proxies:
  - name: p1
    type: socks5
    server: 1.2.3.4
    port: 1080
proxy-groups:
  - name: PROXY
    type: select
    proxies: [p1, DIRECT]
`
	out, _, _, err := ConvertWithMeta([]byte(yaml), &Options{Country: "USA"})
	if err != nil {
		t.Fatal(err)
	}
	m := parseJSON(t, out)
	route := m["route"].(map[string]any)

	rsDefs := route["rule_set"].([]any)
	for _, rs := range rsDefs {
		def := rs.(map[string]any)
		if u, _ := def["url"].(string); strings.Contains(u, "/geoip-usa.srs") {
			t.Errorf("invalid country override must fallback, got %s", u)
		}
	}
	rules := route["rules"].([]any)
	for _, r := range rules {
		rule := r.(map[string]any)
		if ds, ok := rule["domain_suffix"].([]any); ok {
			for _, d := range ds {
				if d == ".usa" {
					t.Error("invalid country override must fallback, got domain_suffix .usa")
				}
			}
		}
	}
}

// TestIntegrationUnsupportedProxyStub verifies that unsupported protocol proxies
// (e.g. ssr) are converted to a socks stub outbound instead of being dropped.
// The stub keeps the node visible in the Clash API UI but fails on use.
func TestIntegrationUnsupportedProxyStub(t *testing.T) {
	yaml := `mixed-port: 7890
proxies:
  - name: ssr-node
    type: ssr
    server: 1.2.3.4
    port: 1080
  - name: good-node
    type: socks5
    server: 5.6.7.8
    port: 1081
proxy-groups:
  - name: PROXY
    type: select
    proxies: [ssr-node, good-node, DIRECT]
`
	out, warnings, err := Convert([]byte(yaml))
	if err != nil {
		t.Fatalf("translation failed: %v", err)
	}

	m := parseJSON(t, out)
	outbounds := m["outbounds"].([]any)

	// ssr-node should appear as a socks stub
	var ssrStub map[string]any
	for _, ob := range outbounds {
		obMap := ob.(map[string]any)
		if obMap["tag"] == "ssr-node" {
			ssrStub = obMap
			break
		}
	}
	if ssrStub == nil {
		t.Fatal("ssr-node stub outbound not found")
	}
	if ssrStub["type"] != "socks" {
		t.Errorf("ssr-node type = %v, want socks (stub)", ssrStub["type"])
	}
	if ssrStub["server"] != "127.0.0.1" {
		t.Errorf("ssr-node server = %v, want 127.0.0.1", ssrStub["server"])
	}
	if ssrStub["server_port"] != float64(1) {
		t.Errorf("ssr-node server_port = %v, want 1", ssrStub["server_port"])
	}

	// PROXY group should include ssr-node in its outbounds
	var proxyGroup map[string]any
	for _, ob := range outbounds {
		obMap := ob.(map[string]any)
		if obMap["tag"] == "PROXY" {
			proxyGroup = obMap
			break
		}
	}
	if proxyGroup == nil {
		t.Fatal("PROXY group not found")
	}
	groupObs, _ := proxyGroup["outbounds"].([]any)
	foundSSR := false
	for _, o := range groupObs {
		if o == "ssr-node" {
			foundSSR = true
		}
	}
	if !foundSSR {
		t.Error("PROXY group should include ssr-node in outbounds")
	}

	// Should have a warning about ssr being unsupported
	foundWarn := false
	for _, w := range warnings {
		if strings.Contains(w, "ssr") {
			foundWarn = true
		}
	}
	if !foundWarn {
		t.Error("expected warning mentioning ssr")
	}
}

// TestIntegrationUnsupportedCipherStub verifies that proxies with unsupported
// ciphers (within a supported protocol) also get stub outbounds.
func TestIntegrationUnsupportedCipherStub(t *testing.T) {
	yaml := `mixed-port: 7890
proxies:
  - name: bad-ss
    type: ss
    server: 1.2.3.4
    port: 8388
    cipher: rc4-md5
    password: pass
  - name: good-node
    type: socks5
    server: 5.6.7.8
    port: 1081
proxy-groups:
  - name: PROXY
    type: select
    proxies: [bad-ss, good-node]
`
	out, _, err := Convert([]byte(yaml))
	if err != nil {
		t.Fatalf("translation failed: %v", err)
	}

	m := parseJSON(t, out)
	outbounds := m["outbounds"].([]any)

	// bad-ss should appear as a socks stub
	for _, ob := range outbounds {
		obMap := ob.(map[string]any)
		if obMap["tag"] == "bad-ss" {
			if obMap["type"] != "socks" {
				t.Errorf("bad-ss type = %v, want socks (stub)", obMap["type"])
			}
			return
		}
	}
	t.Fatal("bad-ss stub outbound not found")
}

// TestIntegrationStubTagsMeta verifies that ConvertWithMeta returns correct
// stubTags mapping for proxies with unsupported protocols.
func TestIntegrationStubTagsMeta(t *testing.T) {
	yaml := `mixed-port: 7890
proxies:
  - name: ssr-node
    type: ssr
    server: 1.2.3.4
    port: 1080
  - name: good-node
    type: socks5
    server: 5.6.7.8
    port: 1081
proxy-groups:
  - name: PROXY
    type: select
    proxies: [ssr-node, good-node, DIRECT]
`
	_, _, meta, err := ConvertWithMeta([]byte(yaml), &Options{Country: "CN"})
	if err != nil {
		t.Fatalf("translation failed: %v", err)
	}

	if len(meta.StubTags) != 1 {
		t.Fatalf("expected 1 stub tag, got %d: %v", len(meta.StubTags), meta.StubTags)
	}
	origType, ok := meta.StubTags["ssr-node"]
	if !ok {
		t.Fatal("ssr-node not found in stubTags")
	}
	if origType != "ssr" {
		t.Errorf("ssr-node original type = %v, want ssr", origType)
	}

	// good-node should NOT be in stubTags
	if _, ok := meta.StubTags["good-node"]; ok {
		t.Error("good-node should not be in stubTags")
	}
}
