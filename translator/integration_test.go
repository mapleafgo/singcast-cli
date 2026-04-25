package translator

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// realConfigPath points to the actual production YAML profile.
const realConfigPath = "/home/mapleafgo/.local/share/cn.mapleafgo.clash_for_flutter/profiles/1777039692584.yaml"

// TestIntegrationRealProfile loads the real production YAML file,
// translates it, and validates the output structure.
func TestIntegrationRealProfile(t *testing.T) {
	data, err := os.ReadFile(realConfigPath)
	if err != nil {
		t.Skipf("real profile not found: %v", err)
	}

	out, warns, err := Translate(data)
	if err != nil {
		t.Fatalf("Translate failed: %v", err)
	}
	m := parseJSON(t, out)

	rawJSON, _ := json.MarshalIndent(m, "", "  ")
	t.Logf("Output JSON size: %d bytes, warnings: %d", len(rawJSON), len(warns))
	for _, w := range warns {
		t.Logf("WARN: %s", w)
	}

	// === Inbounds ===
	inbounds := m["inbounds"].([]any)
	if len(inbounds) < 1 {
		t.Fatal("expected at least 1 inbound")
	}
	mixedInb := inbounds[0].(map[string]any)
	if mixedInb["type"] != "mixed" {
		t.Errorf("first inbound type = %v, want mixed", mixedInb["type"])
	}
	if mixedInb["listen_port"].(float64) != 7890 {
		t.Errorf("listen_port = %v, want 7890", mixedInb["listen_port"])
	}
	if mixedInb["listen"] != "0.0.0.0" {
		t.Errorf("listen = %v, want 0.0.0.0 (allow-lan=true)", mixedInb["listen"])
	}

	// === Outbounds ===
	outbounds := m["outbounds"].([]any)
	tags := collectTags(outbounds)
	t.Logf("Total outbounds: %d", len(outbounds))

	// Must have DIRECT
	if !tags["DIRECT"] {
		t.Error("missing DIRECT outbound")
	}

	// Must have both proxy groups
	if !tags["节点选择"] {
		t.Error("missing 节点选择 group")
	}
	if !tags["自动选择"] {
		t.Error("missing 自动选择 group")
	}

	// Verify group types
	proxySel := findOutbound(outbounds, "节点选择")
	if proxySel == nil {
		t.Fatal("节点选择 group not found")
	}
	if proxySel["type"] != "selector" {
		t.Errorf("节点选择 type = %v, want selector", proxySel["type"])
	}

	autoTest := findOutbound(outbounds, "自动选择")
	if autoTest == nil {
		t.Fatal("自动选择 group not found")
	}
	if autoTest["type"] != "urltest" {
		t.Errorf("自动选择 type = %v, want urltest", autoTest["type"])
	}
	if autoTest["url"] != "http://cp.cloudflare.com/generate_204" {
		t.Errorf("自动选择 url = %v", autoTest["url"])
	}
	if autoTest["interval"] != "5m" {
		t.Errorf("自动选择 interval = %v, want 5m", autoTest["interval"])
	}

	// Verify all 36 proxy outbounds exist
	expectedProxies := []string{
		"剩余流量：127.73 GB",
		"距离下次重置剩余：19 天",
		"套餐到期：2028-01-13",
		"🇺🇸United States 01",
		"🇺🇸United States 02",
		"🇺🇸United States 03",
		"🇺🇸United States 04",
		"🇸🇬Singapore 01",
		"🇸🇬Singapore 02",
		"🇸🇬Singapore 03",
		"🇸🇬Singapore 04",
		"🇯🇵Japan 01",
		"🇯🇵Japan 02",
		"🇯🇵Japan 03",
		"🇯🇵Japan 04",
		"🇰🇷Korea 03",
		"🇰🇷Korea 04",
		"🇳🇱Netherlands 01",
		"🇳🇱Netherlands 02",
		"🇬🇧United Kingdom 01",
		"🇬🇧United Kingdom 02",
		"🇬🇧United Kingdom 03",
		"🇬🇧United Kingdom 04",
		"🇭🇰Hong Kong 01 [Home]",
		"🇭🇰Hong Kong 02 [Home]",
		"🇭🇰Hong Kong 03",
		"🇭🇰Hong Kong 04",
		"🇭🇰Hong Kong 05",
		"🇭🇰Hong Kong 06",
		"🇹🇼Taiwan 01 [Home]",
		"🇹🇼Taiwan 02 [Home]",
		"🇹🇼Taiwan 03",
		"🇹🇼Taiwan 04",
		"🇩🇪Deutschland 01 IPv6",
		"🇩🇪Deutschland 02 IPv6",
	}
	for _, name := range expectedProxies {
		if !tags[name] {
			t.Errorf("missing proxy outbound: %s", name)
		}
	}

	// Verify proxy types: first vless should have proper REALITY config
	us01 := findOutbound(outbounds, "🇺🇸United States 01")
	if us01 == nil {
		t.Fatal("🇺🇸United States 01 not found")
	}
	if us01["type"] != "vless" {
		t.Errorf("us01 type = %v, want vless", us01["type"])
	}
	if us01["flow"] != "xtls-rprx-vision" {
		t.Errorf("us01 flow = %v, want xtls-rprx-vision", us01["flow"])
	}
	tls, _ := us01["tls"].(map[string]any)
	if tls == nil {
		t.Fatal("us01 tls is nil")
	}
	reality, _ := tls["reality"].(map[string]any)
	if reality == nil {
		t.Fatal("us01 reality is nil")
	}
	if reality["public_key"] == "" {
		t.Error("us01 reality.public_key is empty")
	}
	if reality["short_id"] == "" {
		t.Error("us01 reality.short_id is empty")
	}

	// Verify anytls proxy
	sg01 := findOutbound(outbounds, "🇸🇬Singapore 01")
	if sg01 == nil {
		t.Fatal("🇸🇬Singapore 01 not found")
	}
	if sg01["type"] != "anytls" {
		t.Errorf("sg01 type = %v, want anytls", sg01["type"])
	}

	// === Route ===
	route := m["route"].(map[string]any)

	// route.final should be 节点选择 (from MATCH rule)
	if route["final"] != "节点选择" {
		t.Errorf("route.final = %v, want 节点选择", route["final"])
	}

	rules := route["rules"].([]any)
	t.Logf("Total route rules: %d", len(rules))

	// First two rules: sniff + hijack-dns
	if rules[0].(map[string]any)["action"] != "sniff" {
		t.Errorf("first rule action = %v, want sniff", rules[0].(map[string]any)["action"])
	}
	if rules[1].(map[string]any)["action"] != "hijack-dns" {
		t.Errorf("second rule action = %v, want hijack-dns", rules[1].(map[string]any)["action"])
	}

	// Verify REJECT rules are converted to action:reject
	rejectCount := 0
	for _, r := range rules {
		rm := r.(map[string]any)
		if rm["action"] == "reject" {
			rejectCount++
		}
		// Ensure no outbound:REJECT in output
		if ob, ok := rm["outbound"]; ok && ob == "REJECT" {
			t.Error("found outbound:REJECT in rules, should be action:reject")
		}
	}
	// Real config has many REJECT rules (STUN ports + ad domains + tracking)
	if rejectCount == 0 {
		t.Error("expected some reject rules from STUN/ad/tracking entries")
	}
	t.Logf("REJECT action rules: %d", rejectCount)

	// Verify rule types present in output
	ruleJSON, _ := json.Marshal(rules)
	ruleStr := string(ruleJSON)

	type check struct{ substr, desc string }
	checks := []check{
		{`"domain":["feed1.chitanda-eru.com"]`, "DOMAIN"},
		{`"domain_suffix":["stun.qq.com"]`, "DOMAIN-SUFFIX"},
		{`"ip_cidr":["127.0.0.0/8"]`, "IP-CIDR"},
		{`"ip_cidr":["::1/128"]`, "IP-CIDR6"},
		{`"port":[3478]`, "DST-PORT single"},
		{`"rule_set":["geosite-google"]`, "GEOSITE google"},
		{`"rule_set":["geosite-telegram"]`, "GEOSITE telegram"},
		{`"rule_set":["geosite-cn"]`, "GEOSITE cn"},
		{`"rule_set":["geoip-CN"]`, "GEOIP CN"},
		{`"rule_set":["rp-overseas-ai"]`, "RULE-SET"},
	}
	for _, c := range checks {
		if !strings.Contains(ruleStr, c.substr) {
			t.Errorf("missing %s in rules", c.desc)
		}
	}

	// === rule_set definitions ===
	ruleSetDefs := route["rule_set"].([]any)
	t.Logf("Total rule_set definitions: %d", len(ruleSetDefs))

	rsTagMap := map[string]bool{}
	for _, rs := range ruleSetDefs {
		rsMap := rs.(map[string]any)
		tag := rsMap["tag"].(string)
		rsTagMap[tag] = true
	}
	// Verify GEOSITE rule_set definitions
	for _, gs := range []string{"private", "category-ads-all", "google", "telegram", "twitter", "facebook", "github", "cn", "geolocation-cn", "geolocation-!cn"} {
		tag := "geosite-" + gs
		if !rsTagMap[tag] {
			t.Errorf("missing rule_set definition: %s", tag)
		}
	}
	// Verify GEOIP rule_set definition
	if !rsTagMap["geoip-CN"] {
		t.Error("missing geoip-CN rule_set definition")
	}
	// Verify custom RULE-SET definition
	if !rsTagMap["rp-overseas-ai"] {
		t.Error("missing rp-overseas-ai rule_set definition")
	}

	// === DNS ===
	dns := m["dns"].(map[string]any)
	dnsServers := dns["servers"].([]any)
	if len(dnsServers) < 2 {
		t.Errorf("expected at least 2 DNS servers, got %d", len(dnsServers))
	}
	t.Logf("DNS servers: %d", len(dnsServers))

	// DNS rules (from nameserver-policy)
	dnsRules := dns["rules"].([]any)
	if len(dnsRules) < 1 {
		t.Error("expected at least 1 DNS rule (from nameserver-policy)")
	}

	// FakeIP server should exist (type "fakeip" in DNS servers)
	foundFakeIP := false
	for _, srv := range dnsServers {
		srvMap := srv.(map[string]any)
		if srvMap["type"] == "fakeip" {
			foundFakeIP = true
			if srvMap["inet4_range"] == "" {
				t.Error("fakeip server missing inet4_range")
			}
			break
		}
	}
	if !foundFakeIP {
		t.Error("expected fakeip DNS server (enhanced-mode: fake-ip)")
	}

	// === Experimental / Clash API ===
	exp := m["experimental"].(map[string]any)
	clashAPI := exp["clash_api"].(map[string]any)
	if clashAPI["external_controller"] != "127.0.0.1:9090" {
		t.Errorf("external_controller = %v, want 127.0.0.1:9090", clashAPI["external_controller"])
	}
}

// TestIntegrationNoDuplicateTags verifies outbound tags are unique.
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
rules:
  - MATCH,PROXY
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

// TestIntegrationRejectConvertedToAction verifies REJECT becomes action:reject.
func TestIntegrationRejectConvertedToAction(t *testing.T) {
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
rules:
  - DOMAIN,ads.example.com,REJECT
  - MATCH,PROXY
`
	m, _ := mustTranslate(t, yaml)
	route := m["route"].(map[string]any)
	rules := route["rules"].([]any)
	for _, r := range rules {
		rm := r.(map[string]any)
		if ob, ok := rm["outbound"]; ok && ob == "REJECT" {
			t.Error("found outbound:REJECT in rules, should be action:reject")
		}
	}
	ruleJSON, _ := json.Marshal(rules)
	if !strings.Contains(string(ruleJSON), `"action":"reject"`) {
		t.Error("expected action:reject in rules")
	}
}

// TestIntegrationGeoIPWithSrcParam verifies GEOIP,CN,target,src sets source matching.
func TestIntegrationGeoIPWithSrcParam(t *testing.T) {
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
rules:
  - GEOIP,CN,DIRECT,src
  - MATCH,PROXY
`
	m, _ := mustTranslate(t, yaml)
	route := m["route"].(map[string]any)
	rules := route["rules"].([]any)
	for _, r := range rules {
		rm := r.(map[string]any)
		if rs, ok := rm["rule_set"]; ok {
			rsTags, _ := rs.([]any)
			if len(rsTags) > 0 && rsTags[0] == "geoip-CN" {
				if rm["rule_set_ip_cidr_match_source"] != true {
					t.Errorf("GEOIP with src should have rule_set_ip_cidr_match_source=true")
				}
				return
			}
		}
	}
	t.Error("GEOIP CN rule not found")
}
