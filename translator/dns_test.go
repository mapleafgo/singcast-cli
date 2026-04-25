package translator

import (
	"testing"
)

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
	if tr.config.DNS != nil {
		t.Error("DNS config should be nil when disabled")
	}
}

func TestParseDNSServerUDP(t *testing.T) {
	srv := parseDNSServer("8.8.8.8", "test-udp", "")
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
	srv := parseDNSServer("https://dns.google/dns-query", "test-https", "")
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

func TestParseDNSServerTLS(t *testing.T) {
	srv := parseDNSServer("tls://dns.google:853", "test-tls", "")
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
	srv := parseDNSServer("dhcp://eth0", "test-dhcp", "")
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
	srv := parseDNSServer("system", "test-sys", "")
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

func TestParseDNSServerRcode(t *testing.T) {
	srv := parseDNSServer("rcode://success", "test-rcode", "")
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	if srv["type"] != "rcode" {
		t.Errorf("type = %v, want rcode", srv["type"])
	}
	if srv["rcode"] != "success" {
		t.Errorf("rcode = %v, want success", srv["rcode"])
	}
}

func TestParseDNSServerFragment(t *testing.T) {
	srv := parseDNSServer("https://dns.google#proxy&skip-cert-verify", "test-frag", "my-proxy")
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

func TestNameserverPolicyToRule(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		wantKey  string
		wantVals []string
	}{
		{
			name:     "plus-dot prefix becomes domain_suffix",
			pattern:  "+.example.com",
			wantKey:  "domain_suffix",
			wantVals: []string{".example.com"},
		},
		{
			name:     "star-dot prefix becomes domain_suffix",
			pattern:  "*.example.com",
			wantKey:  "domain_suffix",
			wantVals: []string{".example.com"},
		},
		{
			name:     "plain domain becomes domain",
			pattern:  "example.com",
			wantKey:  "domain",
			wantVals: []string{"example.com"},
		},
		{
			name:     "mid-wildcard becomes domain_keyword",
			pattern:  "*xample*",
			wantKey:  "domain_keyword",
			wantVals: []string{"xample"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := nameserverPolicyToRule(tt.pattern, "test-srv")
			if rule == nil {
				t.Fatal("expected non-nil rule")
			}
			if rule["server"] != "test-srv" {
				t.Errorf("server = %v, want test-srv", rule["server"])
			}

			vals, ok := rule[tt.wantKey].([]string)
			if !ok {
				t.Fatalf("expected key %q with []string value, got %v", tt.wantKey, rule[tt.wantKey])
			}
			if len(vals) != len(tt.wantVals) {
				t.Fatalf("expected %d values for %q, got %d: %v", len(tt.wantVals), tt.wantKey, len(vals), vals)
			}
			for i, v := range vals {
				if v != tt.wantVals[i] {
					t.Errorf("value[%d] = %q, want %q", i, v, tt.wantVals[i])
				}
			}
		})
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
