package translator

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// realConfigPath points to the actual production YAML profile.
const realConfigPath = "/home/mapleafgo/.local/share/cn.mapleafgo.singcast/profiles/1778575671321.yaml"

// TestIntegrationRealProfile loads the real production YAML file,
// translates it with auto-routing, and validates every aspect of the output
// against sing-box specification requirements.
func TestIntegrationRealProfile(t *testing.T) {
	data, err := os.ReadFile(realConfigPath)
	if err != nil {
		t.Skipf("real profile not found: %v", err)
	}

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

	// === Log ===
	logSec := m["log"].(map[string]any)
	if logSec["level"] != "info" {
		t.Errorf("log.level = %v, want info", logSec["level"])
	}

	// === Inbounds ===
	inbounds := m["inbounds"].([]any)
	if len(inbounds) != 1 {
		t.Fatalf("expected 1 inbound, got %d", len(inbounds))
	}
	mixedInb := inbounds[0].(map[string]any)
	if mixedInb["type"] != "mixed" {
		t.Errorf("inbound type = %v, want mixed", mixedInb["type"])
	}
	if mixedInb["listen"] != "0.0.0.0" {
		t.Errorf("listen = %v, want 0.0.0.0 (allow-lan=true)", mixedInb["listen"])
	}
	if mixedInb["listen_port"].(float64) != 7890 {
		t.Errorf("listen_port = %v, want 7890", mixedInb["listen_port"])
	}

	// === Outbounds ===
	outbounds := m["outbounds"].([]any)
	tags := collectTags(outbounds)
	t.Logf("Total outbounds: %d", len(outbounds))

	// First outbound must be DIRECT
	ob0 := outbounds[0].(map[string]any)
	if ob0["type"] != "direct" || ob0["tag"] != "DIRECT" {
		t.Errorf("first outbound = %v %v, want direct DIRECT", ob0["type"], ob0["tag"])
	}

	// Must have proxy groups from the real profile
	firstGroup := "CrossWall (克洛斯)"
	if !tags[firstGroup] {
		t.Errorf("missing %q group", firstGroup)
	}
	if !tags["🌍自动选择"] {
		t.Error("missing 🌍自动选择 urltest group")
	}

	// Verify selector group
	sel := findOutbound(outbounds, firstGroup)
	if sel == nil {
		t.Fatalf("%q group not found", firstGroup)
	}
	if sel["type"] != "selector" {
		t.Errorf("%q type = %v, want selector", firstGroup, sel["type"])
	}

	// Verify urltest groups
	for _, gName := range []string{"🌍自动选择", "🇺🇸美国自动选择", "🇭🇰香港自动选择", "🇪🇺欧洲自动选择"} {
		g := findOutbound(outbounds, gName)
		if g == nil {
			t.Errorf("group %q not found", gName)
			continue
		}
		if g["type"] != "urltest" {
			t.Errorf("%q type = %v, want urltest", gName, g["type"])
		}
		if g["url"] == nil {
			t.Errorf("%q missing url field", gName)
		}
	}

	// Verify fallback group (sing-box maps fallback → urltest with high tolerance)
	fb := findOutbound(outbounds, "故障转移")
	if fb == nil {
		t.Fatal("故障转移 group not found")
	}
	if fb["type"] != "urltest" {
		t.Errorf("故障转移 type = %v, want urltest (fallback→urltest)", fb["type"])
	}

	// Verify proxy node types: anytls and hysteria2
	foundAnyTLS, foundHysteria2 := false, false
	for _, ob := range outbounds {
		obMap := ob.(map[string]any)
		switch obMap["type"] {
		case "anytls":
			foundAnyTLS = true
		case "hysteria2":
			foundHysteria2 = true
		}
	}
	if !foundAnyTLS {
		t.Error("expected at least one anytls outbound")
	}
	if !foundHysteria2 {
		t.Error("expected at least one hysteria2 outbound")
	}

	// === Route ===
	route := m["route"].(map[string]any)

	if route["final"] != firstGroup {
		t.Errorf("route.final = %v, want %q", route["final"], firstGroup)
	}
	if route["auto_detect_interface"] != true {
		t.Error("expected auto_detect_interface = true")
	}
	if route["find_process"] != true {
		t.Error("expected find_process = true (find-process-mode: strict)")
	}

	rules := route["rules"].([]any)
	t.Logf("Total route rules: %d", len(rules))

	// Rule 0-1: sniff + hijack-dns (no outbound, no clash_mode)
	assertAction(t, rules[0], "sniff", "rule 0")
	assertAction(t, rules[1], "hijack-dns", "rule 1")

	// Rule 2: clash_mode:Direct → DIRECT
	assertClashModeRule(t, rules[2], "Direct", "DIRECT", "rule 2")

	// Rule 3: clash_mode:Global → first group
	assertClashModeRule(t, rules[3], "Global", firstGroup, "rule 3")

	// Rule 4: ip_is_private → DIRECT (clash_mode:Rule)
	r4 := asMap(t, rules[4], "rule 4")
	if r4["ip_is_private"] != true {
		t.Error("rule 4 should have ip_is_private=true")
	}
	if r4["clash_mode"] != "Rule" {
		t.Error("rule 4 should have clash_mode=Rule")
	}
	if r4["outbound"] != "DIRECT" {
		t.Errorf("rule 4 outbound = %v, want DIRECT", r4["outbound"])
	}

	// Rule 5: overseas-ai → proxy
	r5 := asMap(t, rules[5], "rule 5")
	assertRuleSetOutbound(t, r5, "overseas-ai", firstGroup)
	if r5["clash_mode"] != "Rule" {
		t.Error("rule 5 should have clash_mode=Rule")
	}

	// Rule 6: geosite-geolocation-!cn → proxy
	r6 := asMap(t, rules[6], "rule 6")
	assertRuleSetOutbound(t, r6, "geosite-geolocation-!cn", firstGroup)
	if r6["clash_mode"] != "Rule" {
		t.Error("rule 6 should have clash_mode=Rule")
	}

	// Rule 7: logical(geosite-cn AND NOT geoip-cn AND clash_mode=Rule) → proxy
	r7 := asMap(t, rules[7], "rule 7")
	assertLogicalRule(t, r7, "and", firstGroup, 3, "rule 7")
	// Sub-rule 2 must be clash_mode:Rule
	subRules7 := r7["rules"].([]any)
	subRule7x2 := subRules7[2].(map[string]any)
	if subRule7x2["clash_mode"] != "Rule" {
		t.Error("rule 7 sub-rule 2 should have clash_mode=Rule")
	}

	// Rule 8: geosite-cn → DIRECT (clash_mode:Rule)
	r8 := asMap(t, rules[8], "rule 8")
	assertRuleSetOutbound(t, r8, "geosite-cn", "DIRECT")
	if r8["clash_mode"] != "Rule" {
		t.Error("rule 8 should have clash_mode=Rule")
	}

	// Rule 9: logical(NOT geolocation-!cn AND geoip-cn AND clash_mode=Rule) → DIRECT
	r9 := asMap(t, rules[9], "rule 9")
	assertLogicalRule(t, r9, "and", "DIRECT", 3, "rule 9")

	// Rule 10: domain_suffix:.cn → DIRECT (clash_mode:Rule)
	r10 := asMap(t, rules[10], "rule 10")
	if r10["clash_mode"] != "Rule" {
		t.Error("rule 10 should have clash_mode=Rule")
	}
	ds10, _ := r10["domain_suffix"].([]any)
	if len(ds10) != 1 || ds10[0] != ".cn" {
		t.Errorf("rule 10 domain_suffix = %v, want [.cn]", ds10)
	}
	if r10["outbound"] != "DIRECT" {
		t.Errorf("rule 10 outbound = %v, want DIRECT", r10["outbound"])
	}

	// === rule_set definitions ===
	ruleSetDefs := route["rule_set"].([]any)
	t.Logf("Total rule_set definitions: %d", len(ruleSetDefs))

	rsTagMap := map[string]bool{}
	for _, rs := range ruleSetDefs {
		rsMap := rs.(map[string]any)
		tag := rsMap["tag"].(string)
		rsTagMap[tag] = true

		if rsMap["type"] != "remote" {
			t.Errorf("rule_set %q type = %v, want remote", tag, rsMap["type"])
		}
		if rsMap["format"] != "binary" {
			t.Errorf("rule_set %q format = %v, want binary", tag, rsMap["format"])
		}
		if rsMap["download_detour"] != "DIRECT" {
			t.Errorf("rule_set %q download_detour = %v, want DIRECT", tag, rsMap["download_detour"])
		}
		url, _ := rsMap["url"].(string)
		if url == "" {
			t.Errorf("rule_set %q missing url", tag)
		}
		if !strings.HasSuffix(url, ".srs") {
			t.Errorf("rule_set %q url should end with .srs: %s", tag, url)
		}
	}
	for _, tag := range []string{"overseas-ai", "geosite-geolocation-!cn", "geosite-cn", "geoip-cn"} {
		if !rsTagMap[tag] {
			t.Errorf("missing rule_set definition: %s", tag)
		}
	}

	// === DNS ===
	dns := m["dns"].(map[string]any)
	dnsServers := dns["servers"].([]any)
	if len(dnsServers) < 2 {
		t.Fatalf("expected at least 2 DNS servers, got %d", len(dnsServers))
	}
	t.Logf("DNS servers: %d", len(dnsServers))

	// Verify DNS server types
	dnsTypes := map[string]bool{"quic": false, "udp": false, "fakeip": false}
	for _, srv := range dnsServers {
		srvMap := srv.(map[string]any)
		t := srvMap["type"].(string)
		dnsTypes[t] = true
	}
	for dt, found := range dnsTypes {
		if !found {
			t.Errorf("expected DNS server type %q not found", dt)
		}
	}

	// DNS final
	if dns["final"] == nil || dns["final"] == "" {
		t.Error("dns.final should be set")
	}

	// DNS rules (from nameserver-policy)
	dnsRules := dns["rules"].([]any)
	if len(dnsRules) < 1 {
		t.Error("expected at least 1 DNS rule (from nameserver-policy)")
	}

	// DNS strategy
	if dns["strategy"] != "prefer_ipv6" {
		t.Errorf("dns.strategy = %v, want prefer_ipv6 (ipv6=true)", dns["strategy"])
	}

	// === Experimental / Clash API ===
	exp := m["experimental"].(map[string]any)
	clashAPI := exp["clash_api"].(map[string]any)
	if clashAPI["external_controller"] != "127.0.0.1:9090" {
		t.Errorf("external_controller = %v, want 127.0.0.1:9090", clashAPI["external_controller"])
	}
	if clashAPI["default_mode"] != "Rule" {
		t.Errorf("default_mode = %v, want Rule", clashAPI["default_mode"])
	}

	// Cache file
	cache := exp["cache_file"].(map[string]any)
	if cache["enabled"] != true {
		t.Error("cache_file should be enabled")
	}
	if cache["store_fakeip"] != true {
		t.Error("store_fakeip should be true (profile.store-fake-ip)")
	}
	if cache["store_rdrc"] != true {
		t.Error("store_rdrc should be true")
	}
}

func assertAction(t *testing.T, rule any, wantAction, ctx string) {
	t.Helper()
	r := asMap(t, rule, ctx)
	if r["action"] != wantAction {
		t.Errorf("%s action = %v, want %s", ctx, r["action"], wantAction)
	}
}

func assertClashModeRule(t *testing.T, rule any, mode, wantOutbound, ctx string) {
	t.Helper()
	r := asMap(t, rule, ctx)
	if r["clash_mode"] != mode {
		t.Errorf("%s clash_mode = %v, want %s", ctx, r["clash_mode"], mode)
	}
	if r["outbound"] != wantOutbound {
		t.Errorf("%s outbound = %v, want %s", ctx, r["outbound"], wantOutbound)
	}
}

func assertRuleSetOutbound(t *testing.T, r map[string]any, wantRS, wantOutbound string) {
	t.Helper()
	rs, _ := r["rule_set"].([]any)
	if len(rs) != 1 || rs[0] != wantRS {
		t.Errorf("rule_set = %v, want [%s]", rs, wantRS)
	}
	if r["outbound"] != wantOutbound {
		t.Errorf("outbound = %v, want %s", r["outbound"], wantOutbound)
	}
}

func assertLogicalRule(t *testing.T, r map[string]any, wantMode, wantOutbound string, wantSubRules int, ctx string) {
	t.Helper()
	if r["type"] != "logical" {
		t.Fatalf("%s type = %v, want logical", ctx, r["type"])
	}
	if r["mode"] != wantMode {
		t.Errorf("%s mode = %v, want %s", ctx, r["mode"], wantMode)
	}
	if r["outbound"] != wantOutbound {
		t.Errorf("%s outbound = %v, want %s", ctx, r["outbound"], wantOutbound)
	}
	if r["action"] != "route" {
		t.Errorf("%s action = %v, want route", ctx, r["action"])
	}
	subRules, ok := r["rules"].([]any)
	if !ok || len(subRules) != wantSubRules {
		t.Fatalf("%s sub-rules: ok=%v len=%d, want %d", ctx, ok, len(subRules), wantSubRules)
	}
}

func asMap(t *testing.T, v any, ctx string) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s: expected map, got %T", ctx, v)
	}
	return m
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
	// overseas-ai + geolocation-!cn + logical(geosite-cn AND NOT geoip-cn) +
	// geosite-cn + logical(NOT !cn AND geoip-cn) + domain_suffix:.cn = 11
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
