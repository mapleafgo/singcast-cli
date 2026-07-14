package proxy

import (
	"strings"
	"testing"
)

// captureWarn returns a warn callback and a pointer to the collected warnings.
func captureWarn() (func(string), *[]string) {
	var w []string
	return func(msg string) { w = append(w, msg) }, &w
}

func TestTranslateVLESS(t *testing.T) {
	warn, warnings := captureWarn()
	m := map[string]any{
		"name":               "vless-test",
		"server":             "example.com",
		"port":               443,
		"uuid":               "abc-123-uuid",
		"flow":               "xtls-rprx-vision",
		"tls":                true,
		"sni":                "example.com",
		"client-fingerprint": "chrome",
		"reality-opts": map[string]any{
			"public-key": "pkABC",
			"short-id":   "sidABC",
		},
		"network": "ws",
		"ws-opts": map[string]any{
			"path": "/ws-path",
		},
	}

	out := TranslateVLESS(m, warn)
	if out == nil {
		t.Fatal("expected non-nil result")
	}
	if out["type"] != "vless" {
		t.Errorf("type = %v, want vless", out["type"])
	}
	if out["tag"] != "vless-test" {
		t.Errorf("tag = %v, want vless-test", out["tag"])
	}
	if out["server"] != "example.com" {
		t.Errorf("server = %v, want example.com", out["server"])
	}
	if out["server_port"] != 443 {
		t.Errorf("server_port = %v, want 443", out["server_port"])
	}
	if out["uuid"] != "abc-123-uuid" {
		t.Errorf("uuid = %v, want abc-123-uuid", out["uuid"])
	}
	if out["flow"] != "xtls-rprx-vision" {
		t.Errorf("flow = %v, want xtls-rprx-vision", out["flow"])
	}

	// TLS should have reality and utls
	tls, ok := out["tls"].(map[string]any)
	if !ok {
		t.Fatal("expected tls to be map[string]any")
	}
	reality, ok := tls["reality"].(map[string]any)
	if !ok {
		t.Fatal("expected tls.reality")
	}
	if reality["public_key"] != "pkABC" {
		t.Errorf("reality.public_key = %v, want pkABC", reality["public_key"])
	}
	if reality["short_id"] != "sidABC" {
		t.Errorf("reality.short_id = %v, want sidABC", reality["short_id"])
	}
	utls, ok := tls["utls"].(map[string]any)
	if !ok {
		t.Fatal("expected tls.utls")
	}
	if utls["fingerprint"] != "chrome" {
		t.Errorf("utls.fingerprint = %v, want chrome", utls["fingerprint"])
	}

	// Transport should be ws
	transport, ok := out["transport"].(map[string]any)
	if !ok {
		t.Fatal("expected transport")
	}
	if transport["type"] != "ws" {
		t.Errorf("transport.type = %v, want ws", transport["type"])
	}
	if transport["path"] != "/ws-path" {
		t.Errorf("transport.path = %v, want /ws-path", transport["path"])
	}

	if len(*warnings) > 0 {
		t.Errorf("unexpected warnings: %v", *warnings)
	}
}

func TestTranslateVMess(t *testing.T) {
	warn, warnings := captureWarn()
	m := map[string]any{
		"name":               "vmess-test",
		"server":             "vm.example.com",
		"port":               443,
		"uuid":               "vmess-uuid",
		"alterId":            0,
		"cipher":             "auto",
		"tls":                true,
		"skip-cert-verify":   true,
		"client-fingerprint": "chrome",
	}

	out := TranslateVMess(m, warn)
	if out == nil {
		t.Fatal("expected non-nil result")
	}
	if out["type"] != "vmess" {
		t.Errorf("type = %v, want vmess", out["type"])
	}
	if out["security"] != "auto" {
		t.Errorf("security = %v, want auto", out["security"])
	}
	if out["alter_id"] != 0 {
		t.Errorf("alter_id = %v, want 0", out["alter_id"])
	}

	tls, ok := out["tls"].(map[string]any)
	if !ok {
		t.Fatal("expected tls")
	}
	if tls["insecure"] != true {
		t.Errorf("tls.insecure = %v, want true", tls["insecure"])
	}

	if len(*warnings) > 0 {
		t.Errorf("unexpected warnings: %v", *warnings)
	}
}

func TestTranslateTrojan(t *testing.T) {
	warn, warnings := captureWarn()
	m := map[string]any{
		"name":     "trojan-test",
		"server":   "tr.example.com",
		"port":     443,
		"password": "trojan-pass",
		"sni":      "tr.example.com",
	}

	out := TranslateTrojan(m, warn)
	if out == nil {
		t.Fatal("expected non-nil result")
	}
	if out["type"] != "trojan" {
		t.Errorf("type = %v, want trojan", out["type"])
	}
	if out["password"] != "trojan-pass" {
		t.Errorf("password = %v, want trojan-pass", out["password"])
	}

	tls, ok := out["tls"].(map[string]any)
	if !ok {
		t.Fatal("expected tls")
	}
	if tls["server_name"] != "tr.example.com" {
		t.Errorf("tls.server_name = %v, want tr.example.com", tls["server_name"])
	}

	if len(*warnings) > 0 {
		t.Errorf("unexpected warnings: %v", *warnings)
	}
}

func TestTranslateShadowsocks(t *testing.T) {
	warn, warnings := captureWarn()
	m := map[string]any{
		"name":     "ss-test",
		"server":   "ss.example.com",
		"port":     8388,
		"cipher":   "aes-256-gcm",
		"password": "ss-pass",
	}

	out := TranslateShadowsocks(m, warn)
	if out == nil {
		t.Fatal("expected non-nil result")
	}
	if out["type"] != "shadowsocks" {
		t.Errorf("type = %v, want shadowsocks", out["type"])
	}
	if out["method"] != "aes-256-gcm" {
		t.Errorf("method = %v, want aes-256-gcm", out["method"])
	}
	if out["password"] != "ss-pass" {
		t.Errorf("password = %v, want ss-pass", out["password"])
	}
	if len(*warnings) > 0 {
		t.Errorf("unexpected warnings: %v", *warnings)
	}

	// Test with plugin and plugin-opts (SIP003)
	t.Run("with_plugin", func(t *testing.T) {
		warn2, warnings2 := captureWarn()
		m2 := map[string]any{
			"name":     "ss-plugin",
			"server":   "ss2.example.com",
			"port":     8388,
			"cipher":   "aes-256-gcm",
			"password": "ss-pass2",
			"plugin":   "obfs-local",
			"plugin-opts": map[string]any{
				"mode": "tls",
				"host": "bing.com",
			},
		}

		out2 := TranslateShadowsocks(m2, warn2)
		if out2 == nil {
			t.Fatal("expected non-nil result")
		}
		if out2["plugin"] != "obfs-local" {
			t.Errorf("plugin = %v, want obfs-local", out2["plugin"])
		}
		opts, ok := out2["plugin_opts"].(string)
		if !ok {
			t.Fatal("expected plugin_opts to be string")
		}
		if !strings.Contains(opts, "obfs=tls") || !strings.Contains(opts, "obfs-host=bing.com") {
			t.Errorf("plugin_opts = %v, want to contain obfs=tls and obfs-host=bing.com", opts)
		}
		if len(*warnings2) > 0 {
			t.Errorf("unexpected warnings: %v", *warnings2)
		}
	})
}

func TestTranslateShadowsocksUnsupported(t *testing.T) {
	warn, warnings := captureWarn()
	m := map[string]any{
		"name":     "ss-bad",
		"server":   "ss.example.com",
		"port":     8388,
		"cipher":   "rc4-md5",
		"password": "ss-pass",
	}

	out := TranslateShadowsocks(m, warn)
	if out != nil {
		t.Errorf("expected nil result for unsupported cipher, got %v", out)
	}
	if len(*warnings) == 0 {
		t.Error("expected warning for unsupported cipher")
	}
	found := false
	for _, w := range *warnings {
		if strings.Contains(w, "rc4-md5") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning mentioning rc4-md5, got %v", *warnings)
	}
}

func TestTranslateHysteria2(t *testing.T) {
	warn, warnings := captureWarn()
	m := map[string]any{
		"name":          "hy2-test",
		"server":        "hy2.example.com",
		"port":          443,
		"password":      "hy2-pass",
		"up":            "50 Mbps",
		"down":          "100 Mbps",
		"obfs":          "salamander",
		"obfs-password": "test",
	}

	out := TranslateHysteria2(m, warn)
	if out == nil {
		t.Fatal("expected non-nil result")
	}
	if out["type"] != "hysteria2" {
		t.Errorf("type = %v, want hysteria2", out["type"])
	}
	if out["up_mbps"] != 50 {
		t.Errorf("up_mbps = %v, want 50", out["up_mbps"])
	}
	if out["down_mbps"] != 100 {
		t.Errorf("down_mbps = %v, want 100", out["down_mbps"])
	}

	obfs, ok := out["obfs"].(map[string]any)
	if !ok {
		t.Fatal("expected obfs")
	}
	if obfs["type"] != "salamander" {
		t.Errorf("obfs.type = %v, want salamander", obfs["type"])
	}
	if obfs["password"] != "test" {
		t.Errorf("obfs.password = %v, want test", obfs["password"])
	}

	// TLS is always present for Hysteria2
	if _, ok := out["tls"]; !ok {
		t.Error("expected tls to be set")
	}

	if len(*warnings) > 0 {
		t.Errorf("unexpected warnings: %v", *warnings)
	}

	// Test port hopping
	t.Run("port_hopping", func(t *testing.T) {
		warn2, warnings2 := captureWarn()
		m2 := map[string]any{
			"name":     "hy2-ports",
			"server":   "hy2.example.com",
			"port":     443,
			"password": "hy2-pass",
			"ports":    "1000-2000",
		}

		out2 := TranslateHysteria2(m2, warn2)
		serverPorts, ok := out2["server_ports"].([]string)
		if !ok {
			t.Fatal("expected server_ports to be []string")
		}
		if len(serverPorts) != 1 || serverPorts[0] != "1000:2000" {
			t.Errorf("server_ports = %v, want [1000:2000]", serverPorts)
		}
		if len(*warnings2) > 0 {
			t.Errorf("unexpected warnings: %v", *warnings2)
		}
	})
}

func TestTranslateTUIC(t *testing.T) {
	warn, warnings := captureWarn()
	m := map[string]any{
		"name":                  "tuic-test",
		"server":                "tuic.example.com",
		"port":                  443,
		"uuid":                  "00000000-0000-0000-0000-000000000010",
		"password":              "tuic-pass",
		"congestion-controller": "cubic",
		"sni":                   "tuic.example.com",
	}

	out := TranslateTUIC(m, warn)
	if out == nil {
		t.Fatal("expected non-nil result")
	}
	if out["type"] != "tuic" {
		t.Errorf("type = %v, want tuic", out["type"])
	}
	if out["congestion_control"] != "cubic" {
		t.Errorf("congestion_control = %v, want cubic", out["congestion_control"])
	}

	tls, ok := out["tls"].(map[string]any)
	if !ok {
		t.Fatal("expected tls")
	}
	alpn, ok := tls["alpn"].([]string)
	if !ok {
		t.Fatal("expected tls.alpn to be []string")
	}
	if len(alpn) != 1 || alpn[0] != "h3" {
		t.Errorf("tls.alpn = %v, want [h3]", alpn)
	}

	if len(*warnings) > 0 {
		t.Errorf("unexpected warnings: %v", *warnings)
	}

	// TUIC v4 (token) should return nil + warning
	t.Run("v4_token", func(t *testing.T) {
		warn2, warnings2 := captureWarn()
		m2 := map[string]any{
			"name":   "tuic-v4",
			"server": "tuic.example.com",
			"port":   443,
			"token":  "some-token",
		}

		out2 := TranslateTUIC(m2, warn2)
		if out2 != nil {
			t.Errorf("expected nil for TUIC v4, got %v", out2)
		}
		if len(*warnings2) == 0 {
			t.Error("expected warning for TUIC v4 token")
		}
	})
}

func TestTranslateSOCKS(t *testing.T) {
	warn, warnings := captureWarn()
	m := map[string]any{
		"name":     "socks-test",
		"server":   "socks.example.com",
		"port":     1080,
		"username": "user1",
		"password": "pass1",
	}

	out := TranslateSOCKS(m, warn)
	if out == nil {
		t.Fatal("expected non-nil result")
	}
	if out["type"] != "socks" {
		t.Errorf("type = %v, want socks", out["type"])
	}
	if out["version"] != "5" {
		t.Errorf("version = %v, want 5", out["version"])
	}
	if out["username"] != "user1" {
		t.Errorf("username = %v, want user1", out["username"])
	}
	if out["password"] != "pass1" {
		t.Errorf("password = %v, want pass1", out["password"])
	}
	if len(*warnings) > 0 {
		t.Errorf("unexpected warnings: %v", *warnings)
	}
}

func TestTranslateHTTP(t *testing.T) {
	warn, warnings := captureWarn()
	m := map[string]any{
		"name":     "http-test",
		"server":   "http.example.com",
		"port":     8080,
		"username": "user1",
		"password": "pass1",
		"headers": map[string]any{
			"X-Custom": "value",
		},
	}

	out := TranslateHTTP(m, warn)
	if out == nil {
		t.Fatal("expected non-nil result")
	}
	if out["type"] != "http" {
		t.Errorf("type = %v, want http", out["type"])
	}
	if out["username"] != "user1" {
		t.Errorf("username = %v, want user1", out["username"])
	}
	if out["password"] != "pass1" {
		t.Errorf("password = %v, want pass1", out["password"])
	}
	headers, ok := out["headers"].(map[string]any)
	if !ok {
		t.Fatal("expected headers to be map[string]any")
	}
	if headers["X-Custom"] != "value" {
		t.Errorf("headers[X-Custom] = %v, want value", headers["X-Custom"])
	}
	if len(*warnings) > 0 {
		t.Errorf("unexpected warnings: %v", *warnings)
	}
}

func TestTranslateTLSEnabled(t *testing.T) {
	m := map[string]any{
		"tls":                true,
		"sni":                "test.com",
		"skip-cert-verify":   true,
		"alpn":               []string{"h2", "http/1.1"},
		"client-fingerprint": "chrome",
	}

	tls := TranslateTLS(m)
	if tls == nil {
		t.Fatal("expected non-nil tls")
	}
	if tls["enabled"] != true {
		t.Errorf("enabled = %v, want true", tls["enabled"])
	}
	if tls["server_name"] != "test.com" {
		t.Errorf("server_name = %v, want test.com", tls["server_name"])
	}
	if tls["insecure"] != true {
		t.Errorf("insecure = %v, want true", tls["insecure"])
	}
	alpn, ok := tls["alpn"].([]string)
	if !ok {
		t.Fatal("expected alpn to be []string")
	}
	if len(alpn) != 2 || alpn[0] != "h2" || alpn[1] != "http/1.1" {
		t.Errorf("alpn = %v, want [h2 http/1.1]", alpn)
	}
}

func TestTranslateTLSReality(t *testing.T) {
	m := map[string]any{
		"tls": true,
		"reality-opts": map[string]any{
			"public-key": "pkReality",
			"short-id":   "sidReality",
		},
	}

	tls := TranslateTLS(m)
	if tls == nil {
		t.Fatal("expected non-nil tls")
	}
	reality, ok := tls["reality"].(map[string]any)
	if !ok {
		t.Fatal("expected reality")
	}
	if reality["public_key"] != "pkReality" {
		t.Errorf("reality.public_key = %v, want pkReality", reality["public_key"])
	}
	if reality["short_id"] != "sidReality" {
		t.Errorf("reality.short_id = %v, want sidReality", reality["short_id"])
	}
}

func TestTranslateTransportWS(t *testing.T) {
	m := map[string]any{
		"network": "ws",
		"ws-opts": map[string]any{
			"path": "/ws",
			"headers": map[string]any{
				"Host": "example.com",
			},
		},
	}

	transport := TranslateTransport(m)
	if transport == nil {
		t.Fatal("expected non-nil transport")
	}
	if transport["type"] != "ws" {
		t.Errorf("type = %v, want ws", transport["type"])
	}
	if transport["path"] != "/ws" {
		t.Errorf("path = %v, want /ws", transport["path"])
	}
	headers, ok := transport["headers"].(map[string]any)
	if !ok {
		t.Fatal("expected headers")
	}
	if headers["Host"] != "example.com" {
		t.Errorf("headers[Host] = %v, want example.com", headers["Host"])
	}
}

func TestTranslateTransportGRPC(t *testing.T) {
	m := map[string]any{
		"network": "grpc",
		"grpc-opts": map[string]any{
			"grpc-service-name": "grpcService",
		},
	}

	transport := TranslateTransport(m)
	if transport == nil {
		t.Fatal("expected non-nil transport")
	}
	if transport["type"] != "grpc" {
		t.Errorf("type = %v, want grpc", transport["type"])
	}
	if transport["service_name"] != "grpcService" {
		t.Errorf("service_name = %v, want grpcService", transport["service_name"])
	}
}

func TestTranslateTransportWSEarlyData(t *testing.T) {
	m := map[string]any{
		"network": "ws",
		"ws-opts": map[string]any{
			"path":                   "/ws",
			"max-early-data":         2560,
			"early-data-header-name": "Sec-WebSocket-Protocol",
		},
	}

	transport := TranslateTransport(m)
	if transport == nil {
		t.Fatal("expected non-nil transport")
	}
	if transport["type"] != "ws" {
		t.Errorf("type = %v, want ws", transport["type"])
	}
	if transport["max_early_data"] != 2560 {
		t.Errorf("max_early_data = %v, want 2560", transport["max_early_data"])
	}
	if transport["early_data_header_name"] != "Sec-WebSocket-Protocol" {
		t.Errorf("early_data_header_name = %v, want Sec-WebSocket-Protocol", transport["early_data_header_name"])
	}
}

func TestTranslateTransportHTTPUpgrade(t *testing.T) {
	m := map[string]any{
		"network": "ws",
		"ws-opts": map[string]any{
			"path":               "/upgrade",
			"v2ray-http-upgrade": true,
		},
	}

	transport := TranslateTransport(m)
	if transport == nil {
		t.Fatal("expected non-nil transport")
	}
	if transport["type"] != "httpupgrade" {
		t.Errorf("type = %v, want httpupgrade", transport["type"])
	}
	if transport["path"] != "/upgrade" {
		t.Errorf("path = %v, want /upgrade", transport["path"])
	}
}

func TestSecondsToDuration(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0s"},
		{30, "30s"},
		{300, "5m"},
		{3600, "1h"},
		{3661, "1h1m1s"},
	}
	for _, tt := range tests {
		got := SecondsToDuration(tt.input)
		if got != tt.want {
			t.Errorf("SecondsToDuration(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseBandwidth(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"50", 50},
		{"50 Mbps", 50},
		{"200 mbps", 200},
		{"", 0},
		{"abc", 0},
	}
	for _, tt := range tests {
		got := ParseBandwidth(tt.input)
		if got != tt.want {
			t.Errorf("ParseBandwidth(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestTranslateAnyTLS(t *testing.T) {
	warn, warnings := captureWarn()
	m := map[string]any{
		"name":             "anytls-test",
		"server":           "at.example.com",
		"port":             443,
		"password":         "at-pass",
		"sni":              "at.example.com",
		"skip-cert-verify": true,
	}

	out := TranslateAnyTLS(m, warn)
	if out == nil {
		t.Fatal("expected non-nil result")
	}
	if out["type"] != "anytls" {
		t.Errorf("type = %v, want anytls", out["type"])
	}
	if out["tag"] != "anytls-test" {
		t.Errorf("tag = %v, want anytls-test", out["tag"])
	}
	if out["server"] != "at.example.com" {
		t.Errorf("server = %v, want at.example.com", out["server"])
	}
	if out["server_port"] != 443 {
		t.Errorf("server_port = %v, want 443", out["server_port"])
	}
	if out["password"] != "at-pass" {
		t.Errorf("password = %v, want at-pass", out["password"])
	}
	tls, ok := out["tls"].(map[string]any)
	if !ok {
		t.Fatal("expected tls to be map[string]any")
	}
	if tls["enabled"] != true {
		t.Errorf("tls.enabled = %v, want true", tls["enabled"])
	}
	if tls["insecure"] != true {
		t.Errorf("tls.insecure = %v, want true", tls["insecure"])
	}
	if tls["server_name"] != "at.example.com" {
		t.Errorf("tls.server_name = %v, want at.example.com", tls["server_name"])
	}
	if len(*warnings) > 0 {
		t.Errorf("unexpected warnings: %v", *warnings)
	}
}

func TestTranslateAnyTLS_MissingPassword(t *testing.T) {
	warn, warnings := captureWarn()
	m := map[string]any{
		"name":   "anytls-nopass",
		"server": "at.example.com",
		"port":   443,
	}

	out := TranslateAnyTLS(m, warn)
	if out != nil {
		t.Errorf("expected nil result for missing password, got %v", out)
	}
	if len(*warnings) == 0 {
		t.Error("expected warning for missing password")
	}
	found := false
	for _, w := range *warnings {
		if strings.Contains(w, "missing password") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning mentioning 'missing password', got %v", *warnings)
	}
}

func TestTranslateHysteria(t *testing.T) {
	warn, warnings := captureWarn()
	m := map[string]any{
		"name":     "hysteria-test",
		"server":   "hy.example.com",
		"port":     443,
		"auth-str": "hy-auth",
		"up":       "50 Mbps",
		"down":     "100 Mbps",
		"obfs":     "salamander",
		"ports":    "1000-2000",
		"protocol": "udp",
	}

	out := TranslateHysteria(m, warn)
	if out == nil {
		t.Fatal("expected non-nil result")
	}
	if out["type"] != "hysteria" {
		t.Errorf("type = %v, want hysteria", out["type"])
	}
	if out["auth_str"] != "hy-auth" {
		t.Errorf("auth_str = %v, want hy-auth", out["auth_str"])
	}
	if out["up_mbps"] != 50 {
		t.Errorf("up_mbps = %v, want 50", out["up_mbps"])
	}
	if out["down_mbps"] != 100 {
		t.Errorf("down_mbps = %v, want 100", out["down_mbps"])
	}
	if out["obfs"] != "salamander" {
		t.Errorf("obfs = %v, want salamander", out["obfs"])
	}
	serverPorts, ok := out["server_ports"].([]string)
	if !ok {
		t.Fatal("expected server_ports to be []string")
	}
	if len(serverPorts) != 1 || serverPorts[0] != "1000:2000" {
		t.Errorf("server_ports = %v, want [1000:2000]", serverPorts)
	}
	if out["network"] != "udp" {
		t.Errorf("network = %v, want udp", out["network"])
	}
	tls, ok := out["tls"].(map[string]any)
	if !ok {
		t.Fatal("expected tls")
	}
	if tls["enabled"] != true {
		t.Errorf("tls.enabled = %v, want true", tls["enabled"])
	}
	if len(*warnings) > 0 {
		t.Errorf("unexpected warnings: %v", *warnings)
	}
}

func TestTranslateHysteria_Auth(t *testing.T) {
	warn, warnings := captureWarn()
	m := map[string]any{
		"name":   "hy-auth-test",
		"server": "hy.example.com",
		"port":   443,
		"auth":   "my-auth-token",
	}

	out := TranslateHysteria(m, warn)
	if out == nil {
		t.Fatal("expected non-nil result")
	}
	if out["auth_str"] != "my-auth-token" {
		t.Errorf("auth_str = %v, want my-auth-token", out["auth_str"])
	}
	if len(*warnings) > 0 {
		t.Errorf("unexpected warnings: %v", *warnings)
	}
}

func TestTranslateUnsupported(t *testing.T) {
	warn, warnings := captureWarn()
	m := map[string]any{
		"name": "test-ssr",
	}

	out := TranslateUnsupported("ssr", m, warn)
	if out != nil {
		t.Errorf("expected nil result, got %v", out)
	}
	if len(*warnings) == 0 {
		t.Error("expected warning")
	}
	foundSSR, foundNotSupported := false, false
	for _, w := range *warnings {
		if strings.Contains(w, "ssr") {
			foundSSR = true
		}
		if strings.Contains(w, "not supported") {
			foundNotSupported = true
		}
	}
	if !foundSSR {
		t.Errorf("expected warning mentioning 'ssr', got %v", *warnings)
	}
	if !foundNotSupported {
		t.Errorf("expected warning mentioning 'not supported', got %v", *warnings)
	}
}

func TestTranslateHTTP_Transport(t *testing.T) {
	m := map[string]any{
		"network": "http",
		"http-opts": map[string]any{
			"method": "POST",
			"path":   "/test",
			"headers": map[string]any{
				"Host": "example.com",
			},
		},
	}

	transport := TranslateTransport(m)
	if transport == nil {
		t.Fatal("expected non-nil transport")
	}
	if transport["type"] != "http" {
		t.Errorf("type = %v, want http", transport["type"])
	}
	if transport["method"] != "POST" {
		t.Errorf("method = %v, want POST", transport["method"])
	}
	if transport["path"] != "/test" {
		t.Errorf("path = %v, want /test", transport["path"])
	}
	headers, ok := transport["headers"].(map[string]any)
	if !ok {
		t.Fatal("expected headers to be map[string]any")
	}
	if headers["Host"] != "example.com" {
		t.Errorf("headers[Host] = %v, want example.com", headers["Host"])
	}
}

func TestTranslateH2_Transport(t *testing.T) {
	m := map[string]any{
		"network": "h2",
		"h2-opts": map[string]any{
			"host": "example.com",
			"path": "/h2",
		},
	}

	transport := TranslateTransport(m)
	if transport == nil {
		t.Fatal("expected non-nil transport")
	}
	if transport["type"] != "http" {
		t.Errorf("type = %v, want http", transport["type"])
	}
	host, ok := transport["host"].([]string)
	if !ok {
		t.Fatal("expected host to be []string")
	}
	if len(host) != 1 || host[0] != "example.com" {
		t.Errorf("host = %v, want [example.com]", host)
	}
	if transport["path"] != "/h2" {
		t.Errorf("path = %v, want /h2", transport["path"])
	}
}
