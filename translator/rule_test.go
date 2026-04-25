package translator

import (
	"strings"
	"testing"
)

func TestParseRuleDomain(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["PROXY"] = true

	cfg := &RawConfig{
		Rule: []string{"DOMAIN,example.com,PROXY"},
	}

	translateRules(cfg, tr)
	rules := tr.config.Route.Rules
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	r := rules[0]
	domains, _ := r["domain"].([]string)
	if len(domains) != 1 || domains[0] != "example.com" {
		t.Errorf("domain = %v, want [example.com]", domains)
	}
	if r["outbound"] != "PROXY" {
		t.Errorf("outbound = %v, want PROXY", r["outbound"])
	}
}

func TestParseRuleDomainSuffix(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["DIRECT"] = true

	cfg := &RawConfig{
		Rule: []string{"DOMAIN-SUFFIX,google.com,DIRECT"},
	}

	translateRules(cfg, tr)
	rules := tr.config.Route.Rules
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	r := rules[0]
	suffixes, _ := r["domain_suffix"].([]string)
	if len(suffixes) != 1 || suffixes[0] != "google.com" {
		t.Errorf("domain_suffix = %v, want [google.com]", suffixes)
	}
	if r["outbound"] != "DIRECT" {
		t.Errorf("outbound = %v, want DIRECT", r["outbound"])
	}
}

func TestParseRuleGeoSite(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["PROXY"] = true

	cfg := &RawConfig{
		Rule: []string{"GEOSITE,google,PROXY"},
	}

	translateRules(cfg, tr)
	rules := tr.config.Route.Rules
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	r := rules[0]
	rs, _ := r["rule_set"].([]string)
	if len(rs) != 1 || rs[0] != "geosite-google" {
		t.Errorf("rule_set = %v, want [geosite-google]", rs)
	}
	if r["outbound"] != "PROXY" {
		t.Errorf("outbound = %v, want PROXY", r["outbound"])
	}

	// Check rule_set definition
	def, exists := tr.ruleSetDefs["geosite-google"]
	if !exists {
		t.Fatal("rule_set definition for geosite-google not found")
	}
	if def["type"] != "remote" {
		t.Errorf("def type = %v, want remote", def["type"])
	}
	url, _ := def["url"].(string)
	if !strings.Contains(url, "meta-rules-dat") || !strings.Contains(url, "geosite") {
		t.Errorf("def url = %v, want MetaCubeX geosite URL", url)
	}
	if !strings.HasSuffix(url, "google.srs") {
		t.Errorf("def url = %v, want to end with google.srs", url)
	}
}

func TestParseRuleGeoIP(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["DIRECT"] = true

	cfg := &RawConfig{
		Rule: []string{"GEOIP,CN,DIRECT"},
	}

	translateRules(cfg, tr)
	rules := tr.config.Route.Rules
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	r := rules[0]
	rs, _ := r["rule_set"].([]string)
	if len(rs) != 1 || rs[0] != "geoip-CN" {
		t.Errorf("rule_set = %v, want [geoip-CN]", rs)
	}
	if r["outbound"] != "DIRECT" {
		t.Errorf("outbound = %v, want DIRECT", r["outbound"])
	}

	// Check rule_set definition
	def, exists := tr.ruleSetDefs["geoip-CN"]
	if !exists {
		t.Fatal("rule_set definition for geoip-CN not found")
	}
	if def["type"] != "remote" {
		t.Errorf("def type = %v, want remote", def["type"])
	}
	url, _ := def["url"].(string)
	if !strings.Contains(url, "meta-rules-dat") || !strings.Contains(url, "geoip") {
		t.Errorf("def url = %v, want MetaCubeX geoip URL", url)
	}
	if !strings.HasSuffix(url, "CN.srs") {
		t.Errorf("def url = %v, want to end with CN.srs", url)
	}
}

func TestParseRuleIPCIDR(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["DIRECT"] = true

	cfg := &RawConfig{
		Rule: []string{"IP-CIDR,10.0.0.0/8,DIRECT"},
	}

	translateRules(cfg, tr)
	rules := tr.config.Route.Rules
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	r := rules[0]
	cidrs, _ := r["ip_cidr"].([]string)
	if len(cidrs) != 1 || cidrs[0] != "10.0.0.0/8" {
		t.Errorf("ip_cidr = %v, want [10.0.0.0/8]", cidrs)
	}
	if r["outbound"] != "DIRECT" {
		t.Errorf("outbound = %v, want DIRECT", r["outbound"])
	}
}

func TestParseRuleMatch(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["PROXY"] = true

	cfg := &RawConfig{
		Rule: []string{"MATCH,PROXY"},
	}

	translateRules(cfg, tr)
	rules := tr.config.Route.Rules
	if len(rules) != 0 {
		t.Fatalf("expected 0 rules (MATCH sets final), got %d", len(rules))
	}
	if tr.config.Route.Final != "PROXY" {
		t.Errorf("Route.Final = %q, want PROXY", tr.config.Route.Final)
	}
}

func TestParseRuleNetwork(t *testing.T) {
	tr := newTestTranslation()

	cfg := &RawConfig{
		Rule: []string{"NETWORK,udp,REJECT"},
	}

	translateRules(cfg, tr)
	rules := tr.config.Route.Rules
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	r := rules[0]
	net, ok := r["network"].([]string)
	if !ok || len(net) != 1 || net[0] != "udp" {
		t.Errorf("network = %v, want [udp]", r["network"])
	}
	if r["outbound"] != "REJECT" {
		t.Errorf("outbound = %v, want REJECT", r["outbound"])
	}
}

func TestParseRuleDSTPort(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["PROXY"] = true

	// Single port
	cfg := &RawConfig{
		Rule: []string{"DST-PORT,443,PROXY"},
	}

	translateRules(cfg, tr)
	rules := tr.config.Route.Rules
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	r := rules[0]
	ports, _ := r["port"].([]int)
	if len(ports) != 1 || ports[0] != 443 {
		t.Errorf("port = %v, want [443]", ports)
	}
	if r["outbound"] != "PROXY" {
		t.Errorf("outbound = %v, want PROXY", r["outbound"])
	}

	// Port range
	tr2 := newTestTranslation()
	tr2.proxyTags["PROXY"] = true

	cfg2 := &RawConfig{
		Rule: []string{"DST-PORT,80-443,PROXY"},
	}

	translateRules(cfg2, tr2)
	rules2 := tr2.config.Route.Rules
	if len(rules2) != 1 {
		t.Fatalf("expected 1 rule for port range, got %d", len(rules2))
	}

	r2 := rules2[0]
	portRanges, _ := r2["port_range"].([]string)
	if len(portRanges) != 1 || portRanges[0] != "80:443" {
		t.Errorf("port_range = %v, want [80:443]", portRanges)
	}
	if r2["outbound"] != "PROXY" {
		t.Errorf("outbound = %v, want PROXY", r2["outbound"])
	}
}

func TestParseRuleInvalid(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["PROXY"] = true

	cfg := &RawConfig{
		Rule: []string{"DOMAIN,onlytwo"},
	}

	translateRules(cfg, tr)
	rules := tr.config.Route.Rules
	if len(rules) != 0 {
		t.Fatalf("expected 0 rules for invalid rule, got %d", len(rules))
	}

	if len(tr.warnings) == 0 {
		t.Error("expected a warning for invalid rule")
	}
}

func TestEnsureRuleSetDefIdempotent(t *testing.T) {
	tr := newTestTranslation()

	ensureRuleSetDef("geosite-test", "geosite", "test", tr)
	ensureRuleSetDef("geosite-test", "geosite", "test", tr)

	if len(tr.ruleSetDefs) != 1 {
		t.Errorf("expected 1 rule_set definition, got %d", len(tr.ruleSetDefs))
	}

	def, exists := tr.ruleSetDefs["geosite-test"]
	if !exists {
		t.Fatal("rule_set definition not found")
	}
	if def["tag"] != "geosite-test" {
		t.Errorf("tag = %v, want geosite-test", def["tag"])
	}
}

func TestIsValidOutbound(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["my-proxy"] = true
	tr.groupTags["my-group"] = true

	tests := []struct {
		target string
		want   bool
	}{
		{"DIRECT", true},
		{"REJECT", true},
		{"dns-out", false},
		{"my-proxy", true},
		{"my-group", true},
		{"nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			got := isValidOutbound(tt.target, tr)
			if got != tt.want {
				t.Errorf("isValidOutbound(%q) = %v, want %v", tt.target, got, tt.want)
			}
		})
	}
}

func TestTranslateLogicalRules(t *testing.T) {
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
  - AND,((DOMAIN,example.com),(IP-CIDR,10.0.0.0/8)),PROXY
  - OR,((DOMAIN-SUFFIX,google.com),(DOMAIN-KEYWORD,facebook)),PROXY
  - MATCH,DIRECT
`
	m, _ := mustTranslate(t, yaml)
	route := m["route"].(map[string]any)
	rules := route["rules"].([]any)

	// Find logical rules (type == "logical")
	var logicalRules []map[string]any
	for _, r := range rules {
		rm := r.(map[string]any)
		if rm["type"] == "logical" {
			logicalRules = append(logicalRules, rm)
		}
	}
	if len(logicalRules) != 2 {
		t.Fatalf("expected 2 logical rules, got %d", len(logicalRules))
	}

	andRule := logicalRules[0]
	if andRule["mode"] != "and" {
		t.Errorf("AND rule mode = %v, want and", andRule["mode"])
	}
	if andRule["outbound"] != "PROXY" {
		t.Errorf("AND rule outbound = %v, want PROXY", andRule["outbound"])
	}
	subRules := andRule["rules"].([]any)
	if len(subRules) != 2 {
		t.Errorf("AND rule sub-rules count = %d, want 2", len(subRules))
	}

	orRule := logicalRules[1]
	if orRule["mode"] != "or" {
		t.Errorf("OR rule mode = %v, want or", orRule["mode"])
	}
}

func TestParseRuleDomainWildcard(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["PROXY"] = true

	cfg := &RawConfig{
		Rule: []string{"DOMAIN-WILDCARD,*.example.com,PROXY"},
	}

	translateRules(cfg, tr)
	rules := tr.config.Route.Rules
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	r := rules[0]
	regexes, _ := r["domain_regex"].([]string)
	if len(regexes) != 1 || regexes[0] != `^.*\.example\.com$` {
		t.Errorf("domain_regex = %v, want [^.*\\.example\\.com$]", regexes)
	}
	if r["outbound"] != "PROXY" {
		t.Errorf("outbound = %v, want PROXY", r["outbound"])
	}
}

func TestParseRuleSrcGeoIP(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["PROXY"] = true

	cfg := &RawConfig{
		Rule: []string{"SRC-GEOIP,CN,PROXY"},
	}

	translateRules(cfg, tr)
	rules := tr.config.Route.Rules
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	r := rules[0]
	rs, _ := r["rule_set"].([]string)
	if len(rs) != 1 || rs[0] != "geoip-CN" {
		t.Errorf("rule_set = %v, want [geoip-CN]", rs)
	}
	if r["rule_set_ip_cidr_match_source"] != true {
		t.Errorf("rule_set_ip_cidr_match_source = %v, want true", r["rule_set_ip_cidr_match_source"])
	}
	if r["outbound"] != "PROXY" {
		t.Errorf("outbound = %v, want PROXY", r["outbound"])
	}
}

func TestParseRuleProcessPathRegex(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["PROXY"] = true

	cfg := &RawConfig{
		Rule: []string{"PROCESS-PATH-REGEX,.*chrome.*,PROXY"},
	}

	translateRules(cfg, tr)
	rules := tr.config.Route.Rules
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	r := rules[0]
	ppr, _ := r["process_path_regex"].([]string)
	if len(ppr) != 1 || ppr[0] != ".*chrome.*" {
		t.Errorf("process_path_regex = %v, want [.*chrome.*]", ppr)
	}
}

func TestParseRuleUID(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["DIRECT"] = true

	cfg := &RawConfig{
		Rule: []string{"UID,1000,DIRECT"},
	}

	translateRules(cfg, tr)
	rules := tr.config.Route.Rules
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	r := rules[0]
	uids, _ := r["user_id"].([]int)
	if len(uids) != 1 || uids[0] != 1000 {
		t.Errorf("user_id = %v, want [1000]", uids)
	}
}

func TestParseRuleInType(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["PROXY"] = true

	cfg := &RawConfig{
		Rule: []string{"IN-TYPE,mixed,PROXY"},
	}

	translateRules(cfg, tr)
	rules := tr.config.Route.Rules
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	r := rules[0]
	inbs, _ := r["inbound"].([]string)
	if len(inbs) != 1 || inbs[0] != "mixed" {
		t.Errorf("inbound = %v, want [mixed]", inbs)
	}
}

func TestParseRuleInUser(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["PROXY"] = true

	cfg := &RawConfig{
		Rule: []string{"IN-USER,admin,PROXY"},
	}

	translateRules(cfg, tr)
	rules := tr.config.Route.Rules
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	r := rules[0]
	users, _ := r["auth_user"].([]string)
	if len(users) != 1 || users[0] != "admin" {
		t.Errorf("auth_user = %v, want [admin]", users)
	}
}

func TestParseRuleNot(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["PROXY"] = true

	cfg := &RawConfig{
		Rule: []string{"NOT,((DOMAIN,example.com)),PROXY"},
	}

	translateRules(cfg, tr)
	rules := tr.config.Route.Rules
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	r := rules[0]
	if r["type"] != "logical" {
		t.Errorf("type = %v, want logical", r["type"])
	}
	if r["mode"] != "and" {
		t.Errorf("mode = %v, want and", r["mode"])
	}
	if r["invert"] != true {
		t.Errorf("invert = %v, want true", r["invert"])
	}
	if r["outbound"] != "PROXY" {
		t.Errorf("outbound = %v, want PROXY", r["outbound"])
	}
	subRules, _ := r["rules"].([]map[string]any)
	if len(subRules) != 1 {
		t.Fatalf("sub-rules count = %d, want 1", len(subRules))
	}
	domains, _ := subRules[0]["domain"].([]string)
	if len(domains) != 1 || domains[0] != "example.com" {
		t.Errorf("domain = %v, want [example.com]", domains)
	}
}

func TestParseRuleUnsupportedSkips(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["PROXY"] = true

	cfg := &RawConfig{
		Rule: []string{
			"IP-SUFFIX,1.2.3/8,PROXY",
			"SRC-IP-SUFFIX,1.2.3/8,PROXY",
			"IN-PORT,8080,PROXY",
			"IP-ASN,AS13335,PROXY",
			"SRC-IP-ASN,AS13335,PROXY",
			"DSCP,46,PROXY",
			"PROCESS-NAME-REGEX,chrome,PROXY",
			"PROCESS-NAME-WILDCARD,*chrome*,PROXY",
			"SUB-RULE,,PROXY",
		},
	}

	translateRules(cfg, tr)
	rules := tr.config.Route.Rules
	if len(rules) != 0 {
		t.Fatalf("expected 0 rules (all unsupported), got %d", len(rules))
	}
	if len(tr.warnings) != 9 {
		t.Errorf("expected 9 warnings, got %d: %v", len(tr.warnings), tr.warnings)
	}
}

func TestParseRuleSrcPortRange(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["PROXY"] = true

	cfg := &RawConfig{
		Rule: []string{"SRC-PORT,80-443,PROXY"},
	}

	translateRules(cfg, tr)
	rules := tr.config.Route.Rules
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	r := rules[0]
	ranges, _ := r["source_port_range"].([]string)
	if len(ranges) != 1 || ranges[0] != "80:443" {
		t.Errorf("source_port_range = %v, want [80:443]", ranges)
	}
}

func TestParseRuleProcessPathWildcard(t *testing.T) {
	tr := newTestTranslation()
	tr.proxyTags["PROXY"] = true

	cfg := &RawConfig{
		Rule: []string{"PROCESS-PATH-WILDCARD,*chrome*,PROXY"},
	}

	translateRules(cfg, tr)
	rules := tr.config.Route.Rules
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	r := rules[0]
	ppr, _ := r["process_path_regex"].([]string)
	if len(ppr) != 1 || ppr[0] != `^.*chrome.*$` {
		t.Errorf("process_path_regex = %v, want [^.*chrome.*$]", ppr)
	}
}