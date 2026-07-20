package translator

import "testing"

// TestTranslateDNSNoCircularWithMultipleDoHProxyServerNameservers
// 回归：clash-verge v2 订阅常见配置 —— proxy-server-nameserver 是多条 DoH（域名）时，
// 早先版本会把 psn-* 作为 domain_resolver 的候选，导致 psn-0 → psn-1 → psn-0 循环，
// sing-box 启动时报 "circular server dependency: ns-0 -> psn-0 -> psn-1 -> psn-0"。
// 必须始终优先用 default-nameserver（IP UDP）做 bootstrap。
func TestTranslateDNSNoCircularWithMultipleDoHProxyServerNameservers(t *testing.T) {
	tr := newTestTranslation()
	tr.groupTagOrder = []string{"PROXY"}
	tr.groupTags["PROXY"] = true

	cfg := &RawConfig{
		DNS: RawDNS{
			Enable:            true,
			DefaultNameserver: []string{"223.5.5.5", "119.29.29.29"},
			NameServer: []string{
				"https://dns.cloudflare.com/dns-query#PROXY",
				"https://dns.google/dns-query#PROXY",
			},
			ProxyServerNameserver: []string{
				"https://dns.alidns.com/dns-query",
				"https://doh.pub/dns-query",
			},
		},
	}

	translateDNS(cfg, tr)
	if tr.config.DNS == nil {
		t.Fatal("DNS config should not be nil")
	}

	// 检查每个域名 DNS server 的 domain_resolver 必须指向 def-*（IP UDP），
	// 不能再指向其它 psn-*/ns-* 之类的域名 server，否则会形成循环。
	for _, srv := range tr.config.DNS.Servers {
		tag, _ := srv["tag"].(string)
		if tag == "" {
			continue
		}
		dr, _ := srv["domain_resolver"].(string)
		if dr == "" {
			continue
		}
		if dr == tag {
			t.Errorf("%s.domain_resolver 指向自身: %q", tag, dr)
		}
		if len(dr) < 4 || dr[:4] != "def-" {
			t.Errorf("%s.domain_resolver = %q, 期望 def-* (default-nameserver, IP UDP) 以避免 circular server dependency", tag, dr)
		}
	}
}
