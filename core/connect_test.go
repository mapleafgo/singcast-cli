package core

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"golang.org/x/net/proxy"

	"github.com/mapleafgo/singcast/translator"
	"github.com/sagernet/sing/common/json"
)

func testConfigPath() string {
	if p := os.Getenv("SINGCAST_TEST_CONFIG"); p != "" {
		return p
	}
	return ""
}

// injectMixedInbound replaces all inbounds with a single mixed inbound on the given port.
// This avoids port conflicts with production services.
func injectMixedInbound(t *testing.T, jsonContent string, port uint16) string {
	t.Helper()

	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonContent), &top); err != nil {
		t.Fatalf("parse config for injection: %v", err)
	}

	mixed := map[string]any{
		"type":        "mixed",
		"tag":         "mixed-in",
		"listen":      "127.0.0.1",
		"listen_port": port,
	}
	mixedJSON, _ := json.Marshal(mixed)
	top["inbounds"] = []byte("[" + string(mixedJSON) + "]")

	out, _ := json.Marshal(top)
	return string(out)
}

// TestConnectivity_Google starts the service with a real config and verifies
// that google.com is reachable through the mixed proxy inbound.
func TestConnectivity_Google(t *testing.T) {
	cfgPath := testConfigPath()
	if cfgPath == "" {
		t.Skip("SINGCAST_TEST_CONFIG not set, skipping")
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	jsonContent, warns, err := translator.TranslateWithOptions(data, &translator.Options{
		RuleSetURLPrefix: "https://gh-proxy.org",
	})
	if err != nil {
		t.Fatalf("translate config: %v", err)
	}
	for _, w := range warns {
		t.Logf("WARN: %s", w)
	}

	// Inject a mixed inbound if the config doesn't have one (e.g. profiles without mixed-port).
	const testMixedPort uint16 = 10800
	jsonContent = injectMixedInbound(t, jsonContent, testMixedPort)

	mixedPort := extractMixedPort(t, jsonContent)
	if mixedPort == 0 {
		t.Fatal("no mixed inbound port found after injection")
	}

	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		t.Fatal(err)
	}

	svc := NewService()
	if err := svc.Init(initJSON(homeDir)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer svc.Destroy()

	if err := svc.StartWithContent(jsonContent, ""); err != nil {
		t.Fatalf("StartWithContent: %v", err)
	}
	defer svc.Stop()

	proxyAddr := fmt.Sprintf("127.0.0.1:%d", mixedPort)
	if !waitForListen(t, proxyAddr, 10*time.Second) {
		t.Fatalf("proxy %s not listening after 10s", proxyAddr)
	}
	t.Logf("proxy listening on %s", proxyAddr)

	time.Sleep(2 * time.Second)

	proxyURL, _ := url.Parse(fmt.Sprintf("http://%s", proxyAddr))
	transport := &http.Transport{
		Proxy:                 http.ProxyURL(proxyURL),
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		ResponseHeaderTimeout: 15 * time.Second,
	}

	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}

	reqCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "https://www.google.com", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("User-Agent", "singcast-connectivity-test/1.0")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET google.com through proxy: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	t.Logf("Status: %d", resp.StatusCode)
	t.Logf("Body (first 200 bytes): %.200s", string(body))

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Test domestic site: aliyun.com should be routed DIRECT (not through proxy).
	// If auto-routing works correctly, the proxy handles DNS + routing internally.
	aliyunReq, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "https://www.aliyun.com", nil)
	if err != nil {
		t.Fatalf("create aliyun request: %v", err)
	}
	aliyunReq.Header.Set("User-Agent", "singcast-connectivity-test/1.0")

	aliyunResp, err := client.Do(aliyunReq)
	if err != nil {
		t.Fatalf("GET aliyun.com through proxy: %v", err)
	}
	defer aliyunResp.Body.Close()

	aliyunBody, _ := io.ReadAll(io.LimitReader(aliyunResp.Body, 4096))
	t.Logf("aliyun.com Status: %d", aliyunResp.StatusCode)
	t.Logf("aliyun.com Body (first 200 bytes): %.200s", string(aliyunBody))

	if aliyunResp.StatusCode != http.StatusOK {
		t.Errorf("aliyun.com: expected status 200, got %d", aliyunResp.StatusCode)
	}

	// Test domestic site: flutter-io.cn (.cn suffix) should also route DIRECT.
	flutterReq, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "https://pub-web.flutter-io.cn/", nil)
	if err != nil {
		t.Fatalf("create flutter-io.cn request: %v", err)
	}
	flutterReq.Header.Set("User-Agent", "singcast-connectivity-test/1.0")

	flutterResp, err := client.Do(flutterReq)
	if err != nil {
		t.Fatalf("GET flutter-io.cn through proxy: %v", err)
	}
	defer flutterResp.Body.Close()

	flutterBody, _ := io.ReadAll(io.LimitReader(flutterResp.Body, 4096))
	t.Logf("flutter-io.cn Status: %d", flutterResp.StatusCode)
	t.Logf("flutter-io.cn Body (first 200 bytes): %.200s", string(flutterBody))

	if flutterResp.StatusCode != http.StatusOK {
		t.Errorf("flutter-io.cn: expected status 200, got %d", flutterResp.StatusCode)
	}
}

// TestConnectivity_SOCKS5 verifies google.com is reachable through SOCKS5.
func TestConnectivity_SOCKS5(t *testing.T) {
	cfgPath := testConfigPath()
	if cfgPath == "" {
		t.Skip("SINGCAST_TEST_CONFIG not set, skipping")
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	jsonContent, warns, err := translator.TranslateWithOptions(data, &translator.Options{
		RuleSetURLPrefix: "https://gh-proxy.org",
	})
	if err != nil {
		t.Fatalf("translate config: %v", err)
	}
	for _, w := range warns {
		t.Logf("WARN: %s", w)
	}

	const testMixedPort uint16 = 10801
	jsonContent = injectMixedInbound(t, jsonContent, testMixedPort)
	mixedPort := extractMixedPort(t, jsonContent)
	if mixedPort == 0 {
		t.Fatal("no mixed inbound port found after injection")
	}

	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		t.Fatal(err)
	}

	svc := NewService()
	if err := svc.Init(initJSON(homeDir)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer svc.Destroy()

	if err := svc.StartWithContent(jsonContent, ""); err != nil {
		t.Fatalf("StartWithContent: %v", err)
	}
	defer svc.Stop()

	proxyAddr := fmt.Sprintf("127.0.0.1:%d", mixedPort)
	if !waitForListen(t, proxyAddr, 10*time.Second) {
		t.Fatalf("proxy %s not listening after 10s", proxyAddr)
	}
	t.Logf("proxy listening on %s", proxyAddr)

	time.Sleep(2 * time.Second)

	socksDialer, err := proxy.SOCKS5("tcp", proxyAddr, nil, &net.Dialer{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("create SOCKS5 dialer: %v", err)
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return socksDialer.Dial(network, addr)
		},
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
	}

	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}

	reqCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "https://www.google.com", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("User-Agent", "singcast-connectivity-test/1.0")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET google.com through SOCKS5 proxy: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	t.Logf("Status: %d", resp.StatusCode)
	t.Logf("Body (first 200 bytes): %.200s", string(body))

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestConnectivity_HTTPS verifies an HTTPS site is reachable through the proxy.
func TestConnectivity_HTTPS(t *testing.T) {
	cfgPath := testConfigPath()
	if cfgPath == "" {
		t.Skip("SINGCAST_TEST_CONFIG not set, skipping")
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	jsonContent, warns, err := translator.TranslateWithOptions(data, &translator.Options{
		RuleSetURLPrefix: "https://gh-proxy.org",
	})
	if err != nil {
		t.Fatalf("translate config: %v", err)
	}
	for _, w := range warns {
		t.Logf("WARN: %s", w)
	}

	const testMixedPort uint16 = 10802
	jsonContent = injectMixedInbound(t, jsonContent, testMixedPort)
	mixedPort := extractMixedPort(t, jsonContent)
	if mixedPort == 0 {
		t.Fatal("no mixed inbound port found after injection")
	}

	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		t.Fatal(err)
	}

	svc := NewService()
	if err := svc.Init(initJSON(homeDir)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer svc.Destroy()

	if err := svc.StartWithContent(jsonContent, ""); err != nil {
		t.Fatalf("StartWithContent: %v", err)
	}
	defer svc.Stop()

	proxyAddr := fmt.Sprintf("127.0.0.1:%d", mixedPort)
	if !waitForListen(t, proxyAddr, 10*time.Second) {
		t.Fatalf("proxy %s not listening after 10s", proxyAddr)
	}
	t.Logf("proxy listening on %s", proxyAddr)

	time.Sleep(2 * time.Second)

	proxyURL, _ := url.Parse(fmt.Sprintf("http://%s", proxyAddr))
	transport := &http.Transport{
		Proxy:                 http.ProxyURL(proxyURL),
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		ResponseHeaderTimeout: 15 * time.Second,
	}

	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}

	reqCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "https://www.gstatic.com/generate_204", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("User-Agent", "singcast-connectivity-test/1.0")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET gstatic.com through proxy: %v", err)
	}
	defer resp.Body.Close()

	t.Logf("Status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", resp.StatusCode)
	}
}

// extractMixedPort reads the mixed inbound listen_port from sing-box JSON config.
func extractMixedPort(t *testing.T, jsonStr string) uint16 {
	t.Helper()
	var cfg struct {
		Inbounds []struct {
			Type       string `json:"type"`
			ListenPort uint16 `json:"listen_port"`
		} `json:"inbounds"`
	}
	if err := parseJSONTo(jsonStr, &cfg); err != nil {
		t.Fatalf("parse config for port: %v", err)
	}
	for _, in := range cfg.Inbounds {
		if in.Type == "mixed" {
			return in.ListenPort
		}
	}
	return 0
}

func parseJSONTo(s string, v any) error {
	return json.Unmarshal([]byte(s), v)
}

// waitForListen polls until the address is accepting TCP connections.
func waitForListen(t *testing.T, addr string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(t.Context(), "tcp", addr)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// --- TUN integration test ---

// injectTunConfig injects a TUN inbound into sing-box JSON config.
func injectTunConfig(jsonContent string) string {
	var top map[string]json.RawMessage
	if json.Unmarshal([]byte(jsonContent), &top) != nil {
		return jsonContent
	}

	tun := map[string]any{
		"type":           "tun",
		"tag":            "tun-in",
		"interface_name": "sing-box",
		"address":        "172.19.0.1/30",
		"auto_route":     true,
		"strict_route":   true,
	}

	var rawInbounds []json.RawMessage
	if raw, ok := top["inbounds"]; ok {
		_ = json.Unmarshal(raw, &rawInbounds)
	}
	tunJSON, _ := json.Marshal(tun)
	rawInbounds = append([]json.RawMessage{tunJSON}, rawInbounds...)
	top["inbounds"], _ = json.Marshal(rawInbounds)

	out, _ := json.Marshal(top)
	return string(out)
}

// waitForInterface polls until a network interface with the given name appears.
func waitForInterface(t *testing.T, name string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		iface, err := net.InterfaceByName(name)
		if err == nil && iface != nil {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// TestConnectivity_TUN verifies TUN mode: injects a tun inbound into the real
// config, starts the service, checks the TUN interface is created, and confirms
// google.com is reachable through the mixed proxy (TUN routes system traffic;
// mixed proxy confirms the service stack is functional).
func TestConnectivity_TUN(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("TUN test only runs on Linux and macOS")
	}
	if os.Getuid() != 0 {
		t.Skip("TUN test requires root (run with sudo)")
	}
	cfgPath := testConfigPath()
	if cfgPath == "" {
		t.Skip("SINGCAST_TEST_CONFIG not set, skipping")
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	jsonContent, warns, err := translator.TranslateWithOptions(data, &translator.Options{
		RuleSetURLPrefix: "https://gh-proxy.org",
	})
	if err != nil {
		t.Fatalf("translate config: %v", err)
	}
	for _, w := range warns {
		t.Logf("WARN: %s", w)
	}

	// Extract mixed port for health check (TUN routes all traffic,
	// but we use mixed proxy to confirm the service works).
	mixedPort := extractMixedPort(t, jsonContent)

	// Inject TUN inbound.
	jsonContent = injectTunConfig(jsonContent)

	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		t.Fatal(err)
	}

	svc := NewService()
	if err := svc.Init(initJSON(homeDir)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer svc.Destroy()

	if err := svc.StartWithContent(jsonContent, ""); err != nil {
		t.Fatalf("StartWithContent: %v", err)
	}
	defer svc.Stop()

	// Verify TUN interface is created.
	if !waitForInterface(t, "sing-box", 15*time.Second) {
		ifaces, _ := net.Interfaces()
		var names []string
		for _, i := range ifaces {
			names = append(names, i.Name)
		}
		t.Fatalf("TUN interface 'sing-box' not found after 15s; available: %v", names)
	}
	t.Log("TUN interface 'sing-box' is up")

	// If mixed inbound exists, verify proxy still works through the service.
	if mixedPort != 0 {
		proxyAddr := fmt.Sprintf("127.0.0.1:%d", mixedPort)
		if !waitForListen(t, proxyAddr, 10*time.Second) {
			t.Fatalf("mixed proxy %s not listening", proxyAddr)
		}

		proxyURL, _ := url.Parse(fmt.Sprintf("http://%s", proxyAddr))
		transport := &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			DialContext:     (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		}
		client := &http.Client{Transport: transport, Timeout: 30 * time.Second}

		reqCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "https://www.gstatic.com/generate_204", nil)
		if err != nil {
			t.Fatalf("create request: %v", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("GET gstatic.com through proxy: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("expected 204, got %d", resp.StatusCode)
		} else {
			t.Log("confirmed: proxy request through TUN-enabled service returned 204")
		}
	}
}
