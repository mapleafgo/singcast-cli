package translator

import (
	"testing"
)

func ptrBool(v bool) *bool { return &v }

func TestTranslateDNSEnabled(t *testing.T) {
	tr := newTestTranslation()
	tr.groupTagOrder = []string{"PROXY"}
	tr.groupTags["PROXY"] = true

	cfg := &RawConfig{
		DNS: RawDNS{
			Enable:            true,
			NameServer:        []string{"8.8.8.8"},
			DefaultNameserver: []string{"223.5.5.5"},
		},
	}

	translateDNS(cfg, tr)
	if tr.config.DNS == nil {
		t.Fatal("DNS config should not be nil when enabled")
	}

	if len(tr.config.DNS.Servers) == 0 {
		t.Error("expected DNS servers to be configured")
	}

	// Should have default-nameserver + nameserver entries
	// default-nameserver: 1 entry (def-0)
	// nameserver: 1 entry (ns-0)
	if len(tr.config.DNS.Servers) != 2 {
		t.Errorf("expected 2 DNS servers, got %d", len(tr.config.DNS.Servers))
	}

	// Verify one is the default nameserver and one is the regular nameserver
	tags := make(map[string]bool)
	for _, srv := range tr.config.DNS.Servers {
		if tag, ok := srv["tag"].(string); ok {
			tags[tag] = true
		}
	}
	if !tags["def-0"] {
		t.Error("missing default-nameserver tag def-0")
	}
	if !tags["ns-0"] {
		t.Error("missing nameserver tag ns-0")
	}
}

func TestTranslateDNSDisabled(t *testing.T) {
	tr := newTestTranslation()

	cfg := &RawConfig{
		DNS: RawDNS{
			Enable:     false,
			NameServer: []string{"8.8.8.8"},
		},
	}

	translateDNS(cfg, tr)
	// DNS 未启用时仍输出最小 DNS 模块（仅 bootstrap server）做兜底，
	// 避免 default_domain_resolver 为空导致 sing-box 回退到 local transport。
	if tr.config.DNS == nil {
		t.Error("DNS config should not be nil when disabled (bootstrap resolver required)")
	}
}

// TestTranslateDNSDisabledHasBootstrapResolver 验证 DNS 未启用时仍设置
// route.default_domain_resolver 指向 IP UDP DNS server。Android VpnService 下
// 系统 resolver 读到 ::1:53 导致 connection refused（issue #69）。
func TestTranslateDNSDisabledHasBootstrapResolver(t *testing.T) {
	tr := newTestTranslation()

	cfg := &RawConfig{
		DNS: RawDNS{
			Enable:     false,
			NameServer: []string{"8.8.8.8"},
		},
	}

	translateDNS(cfg, tr)

	if tr.config.Route.DefaultDomainResolver == "" {
		t.Fatal("default_domain_resolver should not be empty when DNS is disabled")
	}

	// default_domain_resolver 必须指向一个 IP UDP DNS server，不能是 local 类型
	for _, srv := range tr.config.DNS.Servers {
		if tag, _ := srv["tag"].(string); tag == tr.config.Route.DefaultDomainResolver {
			if srv["type"] == "local" {
				t.Error("default_domain_resolver points to a local DNS server; expected IP UDP to avoid ::1:53 on Android VPN")
			}
			if srv["type"] != "udp" {
				t.Errorf("default_domain_resolver server type = %v, want udp", srv["type"])
			}
			server, _ := srv["server"].(string)
			if !isIPAddress(server) {
				t.Errorf("default_domain_resolver server = %q, expected an IP address", server)
			}
			return
		}
	}
	t.Fatalf("default_domain_resolver %q not found in DNS servers", tr.config.Route.DefaultDomainResolver)
}

// TestTranslateDNSNoIPServerHasBootstrap 验证 DNS 启用但无 IP 地址 DNS server 时
// default_domain_resolver 仍指向 IP UDP DNS（issue #69）。
// 场景：default-nameserver 为空，nameserver 全是 DoH 域名，无处可 bootstrap。
func TestTranslateDNSNoIPServerHasBootstrap(t *testing.T) {
	tr := newTestTranslation()
	tr.groupTagOrder = []string{"PROXY"}
	tr.groupTags["PROXY"] = true

	cfg := &RawConfig{
		DNS: RawDNS{
			Enable:     true,
			NameServer: []string{"https://dns.cloudflare.com/dns-query"},
		},
	}

	translateDNS(cfg, tr)

	if tr.config.Route.DefaultDomainResolver == "" {
		t.Fatal("default_domain_resolver should not be empty")
	}

	for _, srv := range tr.config.DNS.Servers {
		if tag, _ := srv["tag"].(string); tag == tr.config.Route.DefaultDomainResolver {
			if srv["type"] == "local" {
				t.Error("default_domain_resolver points to a local DNS server; expected IP UDP to avoid ::1:53 on Android VPN")
			}
			server, _ := srv["server"].(string)
			if !isIPAddress(server) {
				t.Errorf("default_domain_resolver server = %q, expected an IP address", server)
			}
			return
		}
	}
	t.Fatalf("default_domain_resolver %q not found in DNS servers", tr.config.Route.DefaultDomainResolver)
}

// TestTranslateDNSSystemDefaultNameserverNotLocal 验证 default-nameserver 只含
// system 时 default_domain_resolver 不指向 local 类型 server（issue #69）。
// local 类型在 Android VPN 下读到 ::1:53 导致 connection refused。
func TestTranslateDNSSystemDefaultNameserverNotLocal(t *testing.T) {
	tr := newTestTranslation()
	tr.groupTagOrder = []string{"PROXY"}
	tr.groupTags["PROXY"] = true

	cfg := &RawConfig{
		DNS: RawDNS{
			Enable:            true,
			DefaultNameserver: []string{"system"},
			NameServer:        []string{"https://dns.cloudflare.com/dns-query"},
		},
	}

	translateDNS(cfg, tr)

	if tr.config.Route.DefaultDomainResolver == "" {
		t.Fatal("default_domain_resolver should not be empty")
	}

	for _, srv := range tr.config.DNS.Servers {
		if tag, _ := srv["tag"].(string); tag == tr.config.Route.DefaultDomainResolver {
			if srv["type"] == "local" {
				t.Error("default_domain_resolver points to a local DNS server; expected IP UDP to avoid ::1:53 on Android VPN")
			}
			server, _ := srv["server"].(string)
			if !isIPAddress(server) {
				t.Errorf("default_domain_resolver server = %q, expected an IP address", server)
			}
			return
		}
	}
	t.Fatalf("default_domain_resolver %q not found in DNS servers", tr.config.Route.DefaultDomainResolver)
}

func TestParseDNSServerUDP(t *testing.T) {
	srv := parseDNSServer("8.8.8.8", "test-udp", "", nil)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	if srv["type"] != "udp" {
		t.Errorf("type = %v, want udp", srv["type"])
	}
	if srv["server"] != "8.8.8.8" {
		t.Errorf("server = %v, want 8.8.8.8", srv["server"])
	}
	if srv["server_port"] != 53 {
		t.Errorf("server_port = %v, want 53", srv["server_port"])
	}
	if srv["tag"] != "test-udp" {
		t.Errorf("tag = %v, want test-udp", srv["tag"])
	}
}

func TestParseDNSServerHTTPS(t *testing.T) {
	srv := parseDNSServer("https://dns.google/dns-query", "test-https", "", nil)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	if srv["type"] != "https" {
		t.Errorf("type = %v, want https", srv["type"])
	}
	if srv["server"] != "dns.google" {
		t.Errorf("server = %v, want dns.google", srv["server"])
	}
	if srv["path"] != "/dns-query" {
		t.Errorf("path = %v, want /dns-query", srv["path"])
	}
	if srv["tag"] != "test-https" {
		t.Errorf("tag = %v, want test-https", srv["tag"])
	}
}

func TestParseDNSServerPlainHTTPIsSkipped(t *testing.T) {
	var warnings []string
	warn := func(msg string) {
		warnings = append(warnings, msg)
	}

	srv := parseDNSServer("http://dns.example/dns-query", "test-http", "", warn)
	if srv != nil {
		t.Fatalf("expected plain HTTP DNS server to be skipped, got %v", srv)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
	if warnings[0] != `DNS server "dns.example": plain HTTP DNS is not supported by sing-box, skipping` {
		t.Errorf("warning = %q", warnings[0])
	}
}

func TestParseDNSServerTLS(t *testing.T) {
	srv := parseDNSServer("tls://dns.google:853", "test-tls", "", nil)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	if srv["type"] != "tls" {
		t.Errorf("type = %v, want tls", srv["type"])
	}
	if srv["server"] != "dns.google" {
		t.Errorf("server = %v, want dns.google", srv["server"])
	}
	if srv["server_port"] != 853 {
		t.Errorf("server_port = %v, want 853", srv["server_port"])
	}
}

func TestParseDNSServerDHCP(t *testing.T) {
	srv := parseDNSServer("dhcp://eth0", "test-dhcp", "", nil)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	if srv["type"] != "dhcp" {
		t.Errorf("type = %v, want dhcp", srv["type"])
	}
	if srv["interface"] != "eth0" {
		t.Errorf("interface = %v, want eth0", srv["interface"])
	}
	if srv["tag"] != "test-dhcp" {
		t.Errorf("tag = %v, want test-dhcp", srv["tag"])
	}
}

func TestParseDNSServerSystem(t *testing.T) {
	srv := parseDNSServer("system", "test-sys", "", nil)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	if srv["type"] != "local" {
		t.Errorf("type = %v, want local", srv["type"])
	}
	if srv["tag"] != "test-sys" {
		t.Errorf("tag = %v, want test-sys", srv["tag"])
	}
}

// sing-box 没有 rcode DNS server 类型，输出它会让配置反序列化阶段就失败
// （unknown transport type: rcode）。必须跳过并告警，而不是产出非法条目。
func TestParseDNSServerRcode(t *testing.T) {
	var warnings []string
	srv := parseDNSServer("rcode://success", "test-rcode", "", func(m string) {
		warnings = append(warnings, m)
	})
	if srv != nil {
		t.Fatalf("expected nil (unsupported), got %v", srv)
	}
	if len(warnings) == 0 {
		t.Error("expected a warning for unsupported rcode nameserver")
	}
}

func TestParseDNSServerFragment(t *testing.T) {
	srv := parseDNSServer("https://dns.google#proxy&skip-cert-verify", "test-frag", "my-proxy", nil)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	if srv["type"] != "https" {
		t.Errorf("type = %v, want https", srv["type"])
	}
	// "#proxy" should set detour to defaultDetour
	if srv["detour"] != "my-proxy" {
		t.Errorf("detour = %v, want my-proxy", srv["detour"])
	}
	// "#skip-cert-verify" should set tls.insecure
	tls, _ := srv["tls"].(map[string]any)
	if tls == nil {
		t.Fatal("expected tls config")
	}
	if tls["insecure"] != true {
		t.Error("tls.insecure should be true")
	}
}

func TestIsIPAddress(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"1.2.3.4", true},
		{"::1", true},
		{"2001:db8::1", true},
		{"example.com", false},
		{"", false},
		{"256.256.256.256", false},
		{"not-an-ip", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isIPAddress(tt.input)
			if got != tt.want {
				t.Errorf("isIPAddress(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeFakeIPRange(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "198.18 range normalized to /15",
			input: "198.18.0.1/16",
			want:  "198.18.0.0/15",
		},
		{
			name:  "non-198.18 range kept as-is",
			input: "10.0.0.0/8",
			want:  "10.0.0.0/8",
		},
		{
			name:  "invalid string returns default",
			input: "invalid",
			want:  "198.18.0.0/15",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeFakeIPRange(tt.input)
			if got != tt.want {
				t.Errorf("normalizeFakeIPRange(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractHostPort(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantHost   string
		wantPort   int
		wantScheme string
	}{
		{"plain ip", "8.8.8.8", "8.8.8.8", 53, ""},
		{"ip with port", "8.8.8.8:5353", "8.8.8.8", 5353, ""},
		{"https url", "https://dns.google/dns-query", "dns.google", 443, "https"},
		{"https with port", "https://dns.google:8443/dns-query", "dns.google", 8443, "https"},
		{"tls url", "tls://dns.google:853", "dns.google", 853, "tls"},
		{"quic url", "quic://dns.adguard.com", "dns.adguard.com", 853, "quic"},
		{"h3 url", "h3://dns.cloudflare.com", "dns.cloudflare.com", 443, "h3"},
		{"with fragment", "https://dns.google#proxy", "dns.google", 443, "https"},
		{"ipv6 bracket", "[2001:4860:4860::8888]", "2001:4860:4860::8888", 53, ""},
		{"ipv6 with port", "[2001:4860:4860::8888]:5353", "2001:4860:4860::8888", 5353, ""},
		// mihomo 惯例允许不带方括号的裸 IPv6；地址自身含冒号，不得被当成 host:port 截断
		{"bare ipv6", "2001:4860:4860::8888", "2001:4860:4860::8888", 53, ""},
		{"bare ipv6 loopback", "::1", "::1", 53, ""},
		{"bare ipv6 with scheme", "tls://2001:4860:4860::8888", "2001:4860:4860::8888", 853, "tls"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port, _, scheme, _ := extractHostPort(tt.input)
			if host != tt.wantHost {
				t.Errorf("host = %q, want %q", host, tt.wantHost)
			}
			if port != tt.wantPort {
				t.Errorf("port = %d, want %d", port, tt.wantPort)
			}
			if scheme != tt.wantScheme {
				t.Errorf("scheme = %q, want %q", scheme, tt.wantScheme)
			}
		})
	}
}

func TestTranslateDNSPreferH3(t *testing.T) {
	tr := newTestTranslation()
	tr.groupTagOrder = []string{"PROXY"}
	tr.groupTags["PROXY"] = true

	cfg := &RawConfig{
		DNS: RawDNS{
			Enable:            true,
			PreferH3:          true,
			NameServer:        []string{"https://dns.google/dns-query"},
			DefaultNameserver: []string{"223.5.5.5"},
		},
	}

	translateDNS(cfg, tr)

	for _, srv := range tr.config.DNS.Servers {
		if srv["type"] == "https" {
			t.Errorf("found https server after prefer-h3, should be upgraded to h3: %v", srv)
		}
		if srv["server"] == "dns.google" && srv["type"] != "h3" {
			t.Errorf("dns.google type = %v, want h3", srv["type"])
		}
	}
}

func TestTranslateDNSFakeIPWhitelist(t *testing.T) {
	tr := newTestTranslation()
	tr.groupTagOrder = []string{"PROXY"}
	tr.groupTags["PROXY"] = true

	cfg := &RawConfig{
		DNS: RawDNS{
			Enable:            true,
			EnhancedMode:      "fake-ip",
			FakeIPFilter:      []string{"*.example.com", "test.local"},
			FakeIPFilterMode:  "whitelist",
			NameServer:        []string{"8.8.8.8"},
			DefaultNameserver: []string{"223.5.5.5"},
		},
	}

	translateDNS(cfg, tr)

	if tr.config.DNS == nil {
		t.Fatal("DNS config should not be nil")
	}

	rules := tr.config.DNS.Rules
	if len(rules) < 2 {
		t.Fatalf("expected at least 2 rules for whitelist mode, got %d", len(rules))
	}

	// First rule: suffix match -> fakeip
	if rules[0]["server"] != "fakeip-dns" {
		t.Errorf("whitelist suffix rule server = %v, want fakeip-dns", rules[0]["server"])
	}

	// 无条件兜底规则不进 Rules，改由 generateDNSRules 追加到规则表末尾，
	// 避免遮蔽其后更精确的规则。
	if len(tr.dnsTerminalRules) != 1 {
		t.Fatalf("expected 1 terminal rule, got %d", len(tr.dnsTerminalRules))
	}
	if got := tr.dnsTerminalRules[0]["server"]; got != "ns-0" {
		t.Errorf("whitelist catch-all server = %v, want ns-0", got)
	}

	// 走完排序后兜底规则必须落在最后一条
	tr.country = "cn"
	generateDNSRules(tr)
	final := tr.config.DNS.Rules
	if got := final[len(final)-1]["server"]; got != "ns-0" {
		t.Errorf("catch-all not last after ordering: %v", got)
	}
}

func TestTranslateDNSUseHostsFalse(t *testing.T) {
	tr := newTestTranslation()
	tr.groupTagOrder = []string{"PROXY"}

	cfg := &RawConfig{
		Hosts: map[string]any{
			"local.test": "127.0.0.1",
		},
		DNS: RawDNS{
			Enable:            true,
			UseHosts:          ptrBool(false),
			NameServer:        []string{"8.8.8.8"},
			DefaultNameserver: []string{"223.5.5.5"},
		},
	}

	translateDNS(cfg, tr)

	for _, srv := range tr.config.DNS.Servers {
		if srv["type"] == "hosts" {
			t.Error("hosts DNS server should not be created when use-hosts is false")
		}
	}
}

func TestTranslateDNSHostsArray(t *testing.T) {
	tr := newTestTranslation()
	tr.groupTagOrder = []string{"PROXY"}

	cfg := &RawConfig{
		Hosts: map[string]any{
			"local.test": "127.0.0.1",
			"array.test": []any{"10.0.0.1", "10.0.0.2"},
			"v6.test":    []any{"::1"},
			"empty.test": []any{},
		},
		DNS: RawDNS{
			Enable:            true,
			NameServer:        []string{"8.8.8.8"},
			DefaultNameserver: []string{"223.5.5.5"},
		},
	}

	translateDNS(cfg, tr)

	// Find the hosts server
	var hostsSrv map[string]any
	for _, srv := range tr.config.DNS.Servers {
		if srv["type"] == "hosts" {
			hostsSrv = srv
			break
		}
	}
	if hostsSrv == nil {
		t.Fatal("hosts DNS server not created")
	}

	predefined, ok := hostsSrv["predefined"].(map[string]any)
	if !ok {
		t.Fatalf("predefined type = %T, want map[string]any", hostsSrv["predefined"])
	}

	// Single string value
	if predefined["local.test"] != "127.0.0.1" {
		t.Errorf("local.test = %v, want 127.0.0.1", predefined["local.test"])
	}
	// Array value: all IPs passed through
	arrVal, ok := predefined["array.test"].([]string)
	if !ok {
		t.Fatalf("array.test type = %T, want []string", predefined["array.test"])
	}
	if len(arrVal) != 2 || arrVal[0] != "10.0.0.1" || arrVal[1] != "10.0.0.2" {
		t.Errorf("array.test = %v, want [10.0.0.1 10.0.0.2]", arrVal)
	}
	// Single-element array: flattened to string
	if predefined["v6.test"] != "::1" {
		t.Errorf("v6.test = %v, want ::1", predefined["v6.test"])
	}
	// Empty array: no entry (or empty string)
	if ip, exists := predefined["empty.test"]; exists && ip != "" {
		t.Errorf("empty.test = %v, want empty or absent", ip)
	}
}

func TestPreferUDPServerEmpty(t *testing.T) {
	result := &singboxDNS{Servers: []map[string]any{}}
	tag := preferUDPServer(nil, result)
	if tag != "" {
		t.Errorf("expected empty string for nil candidates, got %q", tag)
	}
	tag = preferUDPServer([]string{}, result)
	if tag != "" {
		t.Errorf("expected empty string for empty candidates, got %q", tag)
	}
}

func TestFindFirstDirectDNSTagPrefersUDP(t *testing.T) {
	// sing-box 的 DNS 规则路由到单个服务器，无法自动故障转移，
	// 因此国内域名应优先选明文 UDP（最抗封锁），而非列表第一个。
	tr := newTestTranslation()
	tr.config.DNS = &singboxDNS{
		Servers: []map[string]any{
			{"tag": "def-0", "type": "quic", "server": "223.5.5.5"},
			{"tag": "def-1", "type": "h3", "server": "223.5.5.5"},
			{"tag": "def-2", "type": "udp", "server": "114.114.114.114"},
		},
	}

	got := findFirstDirectDNSTag(tr)
	if got != "def-2" {
		t.Errorf("findFirstDirectDNSTag = %q, want def-2 (UDP preferred over quic/h3)", got)
	}
}

func TestFindFirstDirectDNSTagFallbackToFirst(t *testing.T) {
	// 无 UDP 服务器时，退回取第一个直连服务器，保持原行为。
	tr := newTestTranslation()
	tr.config.DNS = &singboxDNS{
		Servers: []map[string]any{
			{"tag": "def-0", "type": "quic", "server": "223.5.5.5"},
			{"tag": "def-1", "type": "h3", "server": "223.5.5.5"},
		},
	}

	got := findFirstDirectDNSTag(tr)
	if got != "def-0" {
		t.Errorf("findFirstDirectDNSTag = %q, want def-0 (fallback to first)", got)
	}
}

func TestFindFirstDirectDNSTagSkipsDetour(t *testing.T) {
	// 带 detour 的（走代理的）DNS 不应被选为国内直连 DNS。
	tr := newTestTranslation()
	tr.config.DNS = &singboxDNS{
		Servers: []map[string]any{
			{"tag": "ns-0", "type": "udp", "server": "8.8.8.8", "detour": "PROXY"},
			{"tag": "def-0", "type": "udp", "server": "114.114.114.114"},
		},
	}

	got := findFirstDirectDNSTag(tr)
	if got != "def-0" {
		t.Errorf("findFirstDirectDNSTag = %q, want def-0 (skip detour servers)", got)
	}
}

func TestFindFirstECHCapableDNSTagPrefersEncrypted(t *testing.T) {
	// ECH 查询 type65 记录应优先加密传输（DoH/DoQ/DoH3），避免明文 UDP
	// 因偶发干扰/超时导致所有 ECH 节点同时不可用（sing-box 单 server 无故障转移）。
	tr := newTestTranslation()
	tr.config.DNS = &singboxDNS{
		Servers: []map[string]any{
			{"tag": "def-0", "type": "quic", "server": "223.5.5.5"},
			{"tag": "def-1", "type": "h3", "server": "223.5.5.5"},
			{"tag": "def-2", "type": "udp", "server": "114.114.114.114"},
		},
	}

	got := findFirstECHCapableDNSTag(tr)
	if got != "def-0" {
		t.Errorf("findFirstECHCapableDNSTag = %q, want def-0 (encrypted preferred over udp)", got)
	}
}

func TestFindFirstECHCapableDNSTagFallbackToAnyDirect(t *testing.T) {
	// 无加密直连服务器时，退回取第一个直连（可能为 udp），保证不返回空。
	tr := newTestTranslation()
	tr.config.DNS = &singboxDNS{
		Servers: []map[string]any{
			{"tag": "def-0", "type": "udp", "server": "114.114.114.114"},
			{"tag": "def-1", "type": "udp", "server": "223.5.5.5"},
		},
	}

	got := findFirstECHCapableDNSTag(tr)
	if got != "def-0" {
		t.Errorf("findFirstECHCapableDNSTag = %q, want def-0 (fallback to first direct)", got)
	}
}

func TestFindFirstECHCapableDNSTagSkipsDetour(t *testing.T) {
	// 带 detour（走代理）的加密 DNS 不能用于 ECH：会重新引入 proxy→ECH→DNS→proxy 循环。
	tr := newTestTranslation()
	tr.config.DNS = &singboxDNS{
		Servers: []map[string]any{
			{"tag": "ns-0", "type": "https", "server": "1.1.1.1", "detour": "PROXY"},
			{"tag": "def-0", "type": "quic", "server": "223.5.5.5"},
		},
	}

	got := findFirstECHCapableDNSTag(tr)
	if got != "def-0" {
		t.Errorf("findFirstECHCapableDNSTag = %q, want def-0 (skip detour servers)", got)
	}
}

// TestTranslateDNSDisabledNoDNSRules 验证 DNS 未启用时不生成 DNS 路由规则。
// 虽然 bootstrap DNS server 需要存在，但用户未启用 DNS，不应有 geo-based DNS
// 路由规则——这些规则会注册 rule_set 定义并改变用户的 DNS 行为意图。
func TestTranslateDNSDisabledNoDNSRules(t *testing.T) {
	tr := newTestTranslation()
	tr.groupTagOrder = []string{"PROXY"}
	tr.groupTags["PROXY"] = true

	cfg := &RawConfig{
		DNS: RawDNS{
			Enable:     false,
			NameServer: []string{"8.8.8.8"},
		},
	}

	translateDNS(cfg, tr)

	// generateDNSRules 在 translateDNS 之后调用，模拟 translateInternal 的流程
	tr.country = "cn"
	generateDNSRules(tr)

	if tr.config.DNS == nil {
		t.Fatal("DNS config should not be nil (bootstrap resolver required)")
	}
	if len(tr.config.DNS.Rules) != 0 {
		t.Errorf("expected 0 DNS rules when DNS disabled, got %d", len(tr.config.DNS.Rules))
		for i, rule := range tr.config.DNS.Rules {
			t.Logf("  rule[%d]: %v", i, rule)
		}
	}
}

func TestGenerateDNSRulesIncludesPrivate(t *testing.T) {
	tr := newTestTranslation()
	tr.country = "cn"
	tr.dnsEnabled = true
	tr.config.DNS = &singboxDNS{
		Servers: []map[string]any{
			{"tag": "ns-0", "type": "udp", "server": "8.8.8.8"},
			{"tag": "fakeip-dns", "type": "fakeip"},
		},
	}
	tr.dnsTerminalRules = []map[string]any{{"server": "ns-0"}}

	generateDNSRules(tr)

	var geoRule map[string]any
	for _, rule := range tr.config.DNS.Rules {
		if _, ok := rule["rule_set"]; ok {
			geoRule = rule
			break
		}
	}
	if geoRule == nil {
		t.Fatal("expected a geo-based DNS rule")
	}
	rs, _ := geoRule["rule_set"].([]string)
	found := false
	for _, tag := range rs {
		if tag == "geosite-private" {
			found = true
		}
	}
	if !found {
		t.Errorf("geo DNS rule_set = %v, want geosite-private", rs)
	}
	if _, ok := tr.ruleSetDefs["geosite-private"]; !ok {
		t.Error("missing rule_set definition for geosite-private")
	}
}
