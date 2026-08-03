package translator

import (
	"os"
	"strings"
	"testing"
)

func convertFixture(t *testing.T, name string) (string, []string) {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	jsonStr, warnings, err := Convert(data)
	if err != nil {
		t.Fatalf("Convert %s: %v", name, err)
	}
	return jsonStr, warnings
}

func TestConvertFixtureProfile(t *testing.T) {
	jsonStr, warnings := convertFixture(t, "multi-proxy.yaml")
	out := parseJSON(t, jsonStr)

	outbounds := out["outbounds"].([]any)
	if len(outbounds) < 10 {
		t.Errorf("outbounds 太少: %d", len(outbounds))
	}
	hasDirect := false
	proxyCount := 0
	for _, ob := range outbounds {
		m, _ := ob.(map[string]any)
		switch m["type"] {
		case "direct":
			hasDirect = true
		case "block", "dns":
		default:
			proxyCount++
		}
	}
	if !hasDirect {
		t.Error("缺少 direct outbound")
	}
	if proxyCount == 0 {
		t.Error("没有代理节点")
	}

	route, _ := out["route"].(map[string]any)
	if route == nil {
		t.Fatal("缺少 route 字段")
	}
	if route["final"] == nil || route["final"] == "" {
		t.Error("route.final 为空")
	}
	rules, _ := route["rules"].([]any)
	if len(rules) == 0 {
		t.Error("route.rules 为空")
	}
	ruleSets, _ := route["rule_set"].([]any)
	if len(ruleSets) == 0 {
		t.Error("route.rule_set 为空")
	}

	dns, _ := out["dns"].(map[string]any)
	if dns == nil {
		t.Fatal("缺少 dns 字段")
	}
	servers, _ := dns["servers"].([]any)
	if len(servers) == 0 {
		t.Error("dns.servers 为空")
	}
	t.Logf("outbounds=%d (proxy=%d), rules=%d, rule_set=%d, dns.servers=%d, final=%v, warnings=%d",
		len(outbounds), proxyCount, len(rules), len(ruleSets), len(servers), route["final"], len(warnings))
}

// Convert 是纯翻译，rule_set URL 必须保持原始值，不混入任何前缀改写
func TestConvertFixtureProfile_RawRuleSetURL(t *testing.T) {
	jsonStr, _ := convertFixture(t, "multi-proxy.yaml")
	out := parseJSON(t, jsonStr)
	route, _ := out["route"].(map[string]any)
	ruleSets, _ := route["rule_set"].([]any)
	if len(ruleSets) == 0 {
		t.Fatal("route.rule_set 为空")
	}
	for _, rs := range ruleSets {
		m, _ := rs.(map[string]any)
		url, _ := m["url"].(string)
		if !strings.HasPrefix(url, RawGitHubPrefix) {
			t.Errorf("rule_set %v: URL 应保持原始值，got %s", m["tag"], url)
		}
	}
}
