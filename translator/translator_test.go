package translator

import (
	"encoding/json"
	"testing"
)

// parseJSON parses a JSON string into map[string]any. Fatals on error.
func parseJSON(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("parseJSON: %v\ninput: %s", err, s)
	}
	return m
}

// mustTranslate translates YAML and parses the result JSON. Fatals on error.
func mustTranslate(t *testing.T, yaml string) (map[string]any, []string) {
	t.Helper()
	jsonStr, warnings, err := Translate([]byte(yaml))
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	return parseJSON(t, jsonStr), warnings
}

// translateMustFail translates and expects an error.
func translateMustFail(t *testing.T, yaml string) error {
	t.Helper()
	_, _, err := Translate([]byte(yaml))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	return err
}

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect Format
	}{
		{"json object", `{"type":"vless"}`, FormatJSON},
		{"json array", `[1,2,3]`, FormatYAML},
		{"json number", `42`, FormatYAML},
		{"json bool true", `true`, FormatYAML},
		{"json null", `null`, FormatYAML},
		{"yaml mapping", "mixed-port: 7890\n", FormatYAML},
		{"yaml with proxy", "proxies:\n  - name: test\n    type: ss\n", FormatYAML},
		{"empty", "", FormatYAML},
		{"plain text", "hello world", FormatYAML},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectFormat([]byte(tt.input))
			if got != tt.expect {
				t.Errorf("DetectFormat(%q) = %v, want %v", tt.input, got, tt.expect)
			}
		})
	}
}

func TestTranslateJSONPassthrough(t *testing.T) {
	input := `{"log":{"level":"info"},"outbounds":[{"type":"direct","tag":"DIRECT"}]}`
	out, warns, err := Translate([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) > 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	m := parseJSON(t, out)
	if m["log"] == nil {
		t.Error("expected log field in passthrough")
	}
}

func TestTranslateMinimalYAML(t *testing.T) {
	yaml := `mixed-port: 7890
proxies:
  - name: "my-proxy"
    type: socks5
    server: 1.2.3.4
    port: 1080
rules:
  - MATCH,my-proxy
`
	m, warns := mustTranslate(t, yaml)

	// Check inbounds
	inbounds := m["inbounds"].([]any)
	if len(inbounds) < 1 {
		t.Fatal("expected at least 1 inbound")
	}
	mixed := inbounds[0].(map[string]any)
	if mixed["type"] != "mixed" {
		t.Errorf("inbound type = %v, want mixed", mixed["type"])
	}
	if mixed["listen_port"].(float64) != 7890 {
		t.Errorf("listen_port = %v, want 7890", mixed["listen_port"])
	}

	// Check outbounds contain builtins + proxy
	outbounds := m["outbounds"].([]any)
	tags := collectTags(outbounds)
	for _, tag := range []string{"DIRECT", "my-proxy"} {
		if !tags[tag] {
			t.Errorf("missing outbound tag: %s", tag)
		}
	}

	// Check route.final — no proxy groups, so falls back to DIRECT
	route := m["route"].(map[string]any)
	if route["final"] != "DIRECT" {
		t.Errorf("route.final = %v, want DIRECT (no proxy groups)", route["final"])
	}

	// Check default rules (sniff, hijack-dns + auto-routing rules)
	rules := route["rules"].([]any)
	if len(rules) < 2 {
		t.Errorf("expected at least 2 default rules, got %d", len(rules))
	}
	if rules[0].(map[string]any)["action"] != "sniff" {
		t.Errorf("first rule should be sniff")
	}

	_ = warns // warnings ok
}

func TestTranslateFullYAML(t *testing.T) {
	yaml := `mixed-port: 7890
allow-lan: true
mode: rule
log-level: info
external-controller: 127.0.0.1:9090
secret: "my-secret"
ipv6: true
dns:
  enable: true
  listen: 0.0.0.0:1053
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
  nameserver:
    - 114.114.114.114
    - tls://dns.google:853
  fallback:
    - https://1.1.1.1/dns-query
  default-nameserver:
    - 223.5.5.5
  nameserver-policy:
    "+.google.com": https://dns.google/dns-query
tun:
  enable: true
  stack: system
  auto-route: true
  strict-route: true
  mtu: 9000
proxies:
  - name: "vless-reality"
    type: vless
    server: example.com
    port: 443
    uuid: abc-def-123
    flow: xtls-rprx-vision
    tls: true
    sni: example.com
    client-fingerprint: chrome
    reality-opts:
      public-key: testkey123
      short-id: abcd1234
    network: ws
    ws-opts:
      path: /ws
  - name: "vmess-proxy"
    type: vmess
    server: vm.example.com
    port: 443
    uuid: 12345678-abcd
    alterId: 0
    cipher: auto
    tls: true
    skip-cert-verify: true
  - name: "trojan-proxy"
    type: trojan
    server: tr.example.com
    port: 443
    password: mypassword
    sni: tr.example.com
  - name: "ss-proxy"
    type: ss
    server: ss.example.com
    port: 8388
    cipher: aes-256-gcm
    password: sspass
  - name: "hy2-proxy"
    type: hysteria2
    server: hy.example.com
    port: 443
    password: hy2pass
    up: "50 Mbps"
    down: "100 Mbps"
    obfs: salamander
    obfs-password: obfspass
proxy-groups:
  - name: PROXY
    type: select
    proxies:
      - vless-reality
      - vmess-proxy
      - trojan-proxy
      - ss-proxy
      - hy2-proxy
      - DIRECT
  - name: AUTO
    type: url-test
    proxies:
      - vless-reality
      - vmess-proxy
      - trojan-proxy
    url: http://www.gstatic.com/generate_204
    interval: 300
rules:
  - DOMAIN-SUFFIX,google.com,PROXY
  - DOMAIN,example.com,PROXY
  - GEOIP,CN,DIRECT
  - GEOSITE,google,PROXY
  - IP-CIDR,10.0.0.0/8,DIRECT
  - DST-PORT,443,PROXY
  - NETWORK,udp,REJECT
  - MATCH,PROXY
`
	m, warns := mustTranslate(t, yaml)

	// Check inbounds: mixed + tun
	inbounds := m["inbounds"].([]any)
	inboundTypes := map[string]bool{}
	for _, ib := range inbounds {
		ibMap := ib.(map[string]any)
		inboundTypes[ibMap["type"].(string)] = true
	}
	if !inboundTypes["mixed"] {
		t.Error("missing mixed inbound")
	}
	if !inboundTypes["tun"] {
		t.Error("missing tun inbound")
	}

	// Check outbounds
	outbounds := m["outbounds"].([]any)
	tags := collectTags(outbounds)
	for _, tag := range []string{"DIRECT", "vless-reality", "vmess-proxy", "trojan-proxy", "ss-proxy", "hy2-proxy", "PROXY", "AUTO"} {
		if !tags[tag] {
			t.Errorf("missing outbound tag: %s", tag)
		}
	}

	// Check VLESS outbound details
	vless := findOutbound(outbounds, "vless-reality")
	if vless == nil {
		t.Fatal("vless-reality outbound not found")
	}
	if vless["type"] != "vless" {
		t.Errorf("vless type = %v", vless["type"])
	}
	if vless["uuid"] != "abc-def-123" {
		t.Errorf("vless uuid = %v", vless["uuid"])
	}
	if vless["flow"] != "xtls-rprx-vision" {
		t.Errorf("vless flow = %v", vless["flow"])
	}

	// Check VLESS TLS (REALITY)
	tls, _ := vless["tls"].(map[string]any)
	if tls == nil {
		t.Fatal("vless tls is nil")
	}
	reality, _ := tls["reality"].(map[string]any)
	if reality == nil || reality["public_key"] != "testkey123" {
		t.Errorf("vless reality = %v", reality)
	}
	utls, _ := tls["utls"].(map[string]any)
	if utls == nil || utls["fingerprint"] != "chrome" {
		t.Errorf("vless utls = %v", utls)
	}

	// Check transport
	transport, _ := vless["transport"].(map[string]any)
	if transport == nil || transport["type"] != "ws" {
		t.Errorf("vless transport = %v", transport)
	}
	if transport["path"] != "/ws" {
		t.Errorf("vless ws path = %v", transport["path"])
	}

	// Check VMess
	vmess := findOutbound(outbounds, "vmess-proxy")
	if vmess["security"] != "auto" {
		t.Errorf("vmess security = %v", vmess["security"])
	}

	// Check Hysteria2
	hy2 := findOutbound(outbounds, "hy2-proxy")
	if hy2["up_mbps"].(float64) != 50 {
		t.Errorf("hy2 up_mbps = %v", hy2["up_mbps"])
	}
	obfs, _ := hy2["obfs"].(map[string]any)
	if obfs == nil || obfs["type"] != "salamander" {
		t.Errorf("hy2 obfs = %v", obfs)
	}

	// Check PROXY group
	proxyGrp := findOutbound(outbounds, "PROXY")
	if proxyGrp["type"] != "selector" {
		t.Errorf("PROXY type = %v", proxyGrp["type"])
	}
	if proxyGrp["interrupt_exist_connections"] != true {
		t.Error("PROXY missing interrupt_exist_connections")
	}

	// Check AUTO group
	autoGrp := findOutbound(outbounds, "AUTO")
	if autoGrp["type"] != "urltest" {
		t.Errorf("AUTO type = %v", autoGrp["type"])
	}

	// Check route rules (auto-routing)
	route := m["route"].(map[string]any)
	rules := route["rules"].([]any)
	// Should have: sniff, hijack-dns, ip_is_private, geo rules
	if len(rules) < 5 {
		t.Errorf("expected at least 5 route rules, got %d", len(rules))
	}
	// First two rules are always sniff + hijack-dns
	if rules[0].(map[string]any)["action"] != "sniff" {
		t.Error("first rule should be sniff")
	}
	if rules[1].(map[string]any)["action"] != "hijack-dns" {
		t.Error("second rule should be hijack-dns")
	}
	// Verify ip_is_private rule exists
	foundPrivate := false
	for _, r := range rules {
		rm := r.(map[string]any)
		if rm["ip_is_private"] == true {
			foundPrivate = true
		}
	}
	if !foundPrivate {
		t.Error("missing ip_is_private rule")
	}

	// Check DNS
	dns, _ := m["dns"].(map[string]any)
	if dns == nil {
		t.Fatal("dns section is nil")
	}
	if dns["final"] == nil {
		t.Error("dns.final is nil")
	}

	// Check experimental
	exp, _ := m["experimental"].(map[string]any)
	if exp == nil {
		t.Fatal("experimental section is nil")
	}
	clashAPI, _ := exp["clash_api"].(map[string]any)
	if clashAPI == nil {
		t.Fatal("clash_api is nil")
	}
	if clashAPI["external_controller"] != "127.0.0.1:9090" {
		t.Errorf("clash_api.external_controller = %v", clashAPI["external_controller"])
	}
	if clashAPI["secret"] != "my-secret" {
		t.Errorf("clash_api.secret = %v", clashAPI["secret"])
	}

	t.Logf("warnings (%d): %v", len(warns), warns)
}

func TestTranslateInvalidYAML(t *testing.T) {
	translateMustFail(t, "\t\n  invalid: [\n yaml: {")
}

func TestTranslateDuplicateTag(t *testing.T) {
	yaml := `mixed-port: 7890
proxies:
  - name: dup
    type: socks5
    server: 1.2.3.4
    port: 1080
  - name: dup
    type: socks5
    server: 5.6.7.8
    port: 1081
rules:
  - MATCH,dup
`
	translateMustFail(t, yaml)
}

func TestTranslateUnsupportedProxy(t *testing.T) {
	yaml := `mixed-port: 7890
proxies:
  - name: my-ssr
    type: ssr
    server: 1.2.3.4
    port: 1080
rules:
  - MATCH,DIRECT
`
	_, warns, err := Translate([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, w := range warns {
		if len(w) > 0 {
			found = true
		}
	}
	if !found {
		t.Error("expected at least one warning for unsupported proxy")
	}
}

func TestTranslateEmptyProxies(t *testing.T) {
	yaml := `mixed-port: 7890
rules:
  - MATCH,DIRECT
`
	m, _ := mustTranslate(t, yaml)
	route := m["route"].(map[string]any)
	if route["final"] != "DIRECT" {
		t.Errorf("route.final = %v, want DIRECT", route["final"])
	}
}

// collectTags extracts all "tag" values from an outbound slice.
func collectTags(outbounds []any) map[string]bool {
	tags := make(map[string]bool)
	for _, ob := range outbounds {
		if m, ok := ob.(map[string]any); ok {
			if tag, ok := m["tag"].(string); ok {
				tags[tag] = true
			}
		}
	}
	return tags
}

// findOutbound finds an outbound by tag in the slice.
func findOutbound(outbounds []any, tag string) map[string]any {
	for _, ob := range outbounds {
		if m, ok := ob.(map[string]any); ok {
			if m["tag"] == tag {
				return m
			}
		}
	}
	return nil
}

func TestHostsTranslation(t *testing.T) {
	yaml := `mixed-port: 7890
dns:
  enable: true
  nameserver:
    - 8.8.8.8
hosts:
  example.com: 1.2.3.4
  test.local: 10.0.0.1
`
	m, _ := mustTranslate(t, yaml)
	dns := m["dns"].(map[string]any)
	servers := dns["servers"].([]any)

	// Find the hosts server
	var hostsSrv map[string]any
	for _, s := range servers {
		srv := s.(map[string]any)
		if srv["type"] == "hosts" {
			hostsSrv = srv
			break
		}
	}
	if hostsSrv == nil {
		t.Fatal("hosts DNS server not found")
	}
	if hostsSrv["tag"] != "hosts" {
		t.Errorf("hosts tag = %v, want hosts", hostsSrv["tag"])
	}
	predefined, _ := hostsSrv["predefined"].(map[string]any)
	if predefined["example.com"] != "1.2.3.4" {
		t.Errorf("hosts predefined example.com = %v, want 1.2.3.4", predefined["example.com"])
	}
	if predefined["test.local"] != "10.0.0.1" {
		t.Errorf("hosts predefined test.local = %v, want 10.0.0.1", predefined["test.local"])
	}

	// Check DNS rule routing host domains to hosts server
	rules := dns["rules"].([]any)
	found := false
	for _, r := range rules {
		rule := r.(map[string]any)
		if rule["server"] == "hosts" {
			found = true
			domains, _ := rule["domain"].([]any)
			domainSet := make(map[string]bool)
			for _, d := range domains {
				domainSet[d.(string)] = true
			}
			if !domainSet["example.com"] || !domainSet["test.local"] {
				t.Errorf("hosts rule domains = %v, want [example.com, test.local]", domains)
			}
			break
		}
	}
	if !found {
		t.Error("DNS rule routing to hosts server not found")
	}
}

func TestGlobalClientFingerprint(t *testing.T) {
	yaml := `mixed-port: 7890
global-client-fingerprint: chrome
proxies:
  - name: p1
    type: vmess
    server: 1.2.3.4
    port: 443
    uuid: test-uuid
    tls: true
  - name: p2
    type: vmess
    server: 5.6.7.8
    port: 443
    uuid: test-uuid-2
    tls: true
    client-fingerprint: firefox
`
	m, _ := mustTranslate(t, yaml)
	outbounds := m["outbounds"].([]any)

	// p1: no per-proxy fingerprint -> should get global "chrome"
	p1 := findOutbound(outbounds, "p1")
	if p1 == nil {
		t.Fatal("p1 outbound not found")
	}
	p1tls := p1["tls"].(map[string]any)
	p1utls := p1tls["utls"].(map[string]any)
	if p1utls["fingerprint"] != "chrome" {
		t.Errorf("p1 utls fingerprint = %v, want chrome (from global)", p1utls["fingerprint"])
	}

	// p2: has per-proxy "firefox" -> should keep it, not overridden by global
	p2 := findOutbound(outbounds, "p2")
	if p2 == nil {
		t.Fatal("p2 outbound not found")
	}
	p2tls := p2["tls"].(map[string]any)
	p2utls := p2tls["utls"].(map[string]any)
	if p2utls["fingerprint"] != "firefox" {
		t.Errorf("p2 utls fingerprint = %v, want firefox (per-proxy override)", p2utls["fingerprint"])
	}
}
