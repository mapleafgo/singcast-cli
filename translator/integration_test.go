package translator

import (
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
