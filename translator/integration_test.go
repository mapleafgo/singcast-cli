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
// translates it with auto-routing, and validates the output structure.
func TestIntegrationRealProfile(t *testing.T) {
	data, err := os.ReadFile(realConfigPath)
	if err != nil {
		t.Skipf("real profile not found: %v", err)
	}

	// Force CN country for deterministic test
	out, warns, err := TranslateWithOptions(data, &Options{Country: "CN"})
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
	if mixedInb["listen_port"].(float64) != 9870 {
		t.Errorf("listen_port = %v, want 9870", mixedInb["listen_port"])
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

	// === Route (auto-routing) ===
	route := m["route"].(map[string]any)

	// route.final should be 节点选择 (first group)
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

	// Verify auto-routing rules present (ip_is_private instead of geoip-private)
	ruleJSON, _ := json.Marshal(rules)
	ruleStr := string(ruleJSON)

	type check struct{ substr, desc string }
	checks := []check{
		{`"ip_is_private":true`, "private IP inline rule"},
		{`"geosite-geolocation-!cn"`, "non-CN geolocation rule_set"},
		{`"geoip-cn"`, "CN geoip rule_set"},
		{`"geosite-cn"`, "CN geolocation rule_set"},
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

		// Verify download_detour is DIRECT
		if rsMap["download_detour"] != "DIRECT" {
			t.Errorf("rule_set %q download_detour = %v, want DIRECT", tag, rsMap["download_detour"])
		}
	}
	// Verify auto-routing rule_set definitions (no geoip-private)
	for _, tag := range []string{"geosite-geolocation-!cn", "geoip-cn", "geosite-cn"} {
		if !rsTagMap[tag] {
			t.Errorf("missing rule_set definition: %s", tag)
		}
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

	// FakeIP server should exist
	foundFakeIP := false
	for _, srv := range dnsServers {
		srvMap := srv.(map[string]any)
		if srvMap["type"] == "fakeip" {
			foundFakeIP = true
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
	out, _, err := TranslateWithOptions([]byte(yaml), &Options{Country: "CN"})
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
	// overseas-ai + geolocation-!cn + logical(geolocation-cn AND NOT geoip-cn) +
	// geolocation-cn + logical(NOT !cn AND geoip-cn) + domain_suffix:.cn = 11
	if len(rules) != 11 {
		t.Fatalf("expected 11 rules for CN, got %d", len(rules))
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

	// Verify domain_suffix:.cn → DIRECT rule (fallback after geo rules)
	cnSuffixRule := rules[10].(map[string]any)
	cnDS, _ := cnSuffixRule["domain_suffix"].([]any)
	if len(cnDS) != 1 || cnDS[0] != ".cn" {
		t.Errorf("rule 10 domain_suffix = %v, want [.cn]", cnDS)
	}
	if cnSuffixRule["outbound"] != "DIRECT" {
		t.Errorf("rule 10 outbound = %v, want DIRECT", cnSuffixRule["outbound"])
	}

	// Verify geosite-cn → DIRECT rule
	geoCNRule := rules[8].(map[string]any)
	geoCNRS, _ := geoCNRule["rule_set"].([]any)
	if len(geoCNRS) != 1 || geoCNRS[0] != "geosite-cn" {
		t.Errorf("rule 8 rule_set = %v, want [geosite-cn]", geoCNRS)
	}
	if geoCNRule["outbound"] != "DIRECT" {
		t.Errorf("rule 8 outbound = %v, want DIRECT", geoCNRule["outbound"])
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

// TestIntegrationAutoRoutingOtherCountry verifies non-CN auto-routing.
func TestIntegrationAutoRoutingOtherCountry(t *testing.T) {
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
	out, _, err := TranslateWithOptions([]byte(yaml), &Options{Country: "JP"})
	if err != nil {
		t.Fatal(err)
	}
	m := parseJSON(t, out)
	route := m["route"].(map[string]any)

	rules := route["rules"].([]any)
	// sniff + hijack-dns + clash_mode:Direct + clash_mode:Global + ip_is_private +
	// logical(geosite-jp AND NOT geoip-jp) + geosite-jp + geoip-jp +
	// domain_suffix:.jp = 9
	if len(rules) != 9 {
		t.Fatalf("expected 9 rules for JP, got %d", len(rules))
	}

	// Verify geoip-jp and geosite-jp rule_set definitions exist
	rsDefs := route["rule_set"].([]any)
	rsTags := map[string]bool{}
	for _, rs := range rsDefs {
		rsMap := rs.(map[string]any)
		rsTags[rsMap["tag"].(string)] = true
	}
	for _, tag := range []string{"geoip-jp", "geosite-jp"} {
		if !rsTags[tag] {
			t.Errorf("missing rule_set: %s", tag)
		}
	}
}
