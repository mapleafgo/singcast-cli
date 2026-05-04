package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mapleafgo/singcast/translator"
)

// minimalYAML is the smallest valid mihomo config for integration testing.
const minimalYAML = `mixed-port: 1080
mode: rule
dns:
  enable: true
  enhanced-mode: redir-host
  nameserver: [8.8.8.8]
proxies:
  - name: test-proxy
    type: socks5
    server: 127.0.0.1
    port: 1081
proxy-groups:
  - name: PROXY
    type: select
    proxies: [test-proxy, DIRECT]
rules:
  - MATCH,PROXY
`

const fakeipYAML = `mixed-port: 1080
mode: rule
dns:
  enable: true
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
  default-nameserver: [223.5.5.5, 119.29.29.29]
  nameserver: [https://dns.cloudflare.com/dns-query, https://dns.google/dns-query]
  proxy-server-nameserver: [https://dns.alidns.com/dns-query]
  fallback: [tls://8.8.8.8:853]
  fallback-filter:
    geoip: true
    geoip-code: CN
    ipcidr: [240.0.0.0/4, 0.0.0.0/32]
  nameserver-policy:
    "geosite:cn": [https://doh.pub/dns-query]
  fake-ip-filter: ["*.lan", "*.local"]
proxies:
  - name: test-proxy
    type: socks5
    server: 127.0.0.1
    port: 1081
proxy-groups:
  - name: PROXY
    type: select
    proxies: [test-proxy, DIRECT]
rules:
  - DOMAIN-SUFFIX,google.com,PROXY
  - IP-CIDR,10.0.0.0/8,DIRECT,no-resolve
  - MATCH,PROXY
`

// mustTranslateYAML translates YAML to sing-box JSON, fatals on error.
func mustTranslateYAML(t *testing.T, yaml string) string {
	t.Helper()
	out, _, err := translator.Translate([]byte(yaml))
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	return out
}

// parseJSONMap parses JSON into map[string]any.
func parseJSONMap(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("parseJSON: %v", err)
	}
	return m
}

// TestCheckConfig_MinimalYAML verifies the minimal config passes sing-box validation.
func TestCheckConfig_MinimalYAML(t *testing.T) {
	jsonStr := mustTranslateYAML(t, minimalYAML)
	if err := CheckConfig(jsonStr); err != nil {
		if strings.Contains(err.Error(), "clash api is not included in this build") {
			t.Skip("requires -tags with_clash_api")
		}
		t.Fatalf("CheckConfig failed: %v\nJSON:\n%s", err, jsonStr)
	}
}

// TestCheckConfig_FakeipYAML verifies a realistic fakeip config passes validation.
func TestCheckConfig_FakeipYAML(t *testing.T) {
	jsonStr := mustTranslateYAML(t, fakeipYAML)
	if err := CheckConfig(jsonStr); err != nil {
		if strings.Contains(err.Error(), "clash api is not included in this build") {
			t.Skip("requires -tags with_clash_api")
		}
		t.Fatalf("CheckConfig failed: %v\nJSON:\n%s", err, jsonStr)
	}
}

// TestCheckConfig_PassThroughJSON verifies a direct sing-box JSON config passes validation.
func TestCheckConfig_PassThroughJSON(t *testing.T) {
	singboxJSON := `{
			"log": {"level": "warn"},
			"inbounds": [{"type": "mixed", "tag": "mixed-in", "listen": "127.0.0.1", "listen_port": 1080}],
			"outbounds": [{"type": "direct", "tag": "DIRECT"}],
			"route": {"final": "DIRECT"}
		}`
	if err := CheckConfig(singboxJSON); err != nil {
		t.Fatalf("CheckConfig for raw JSON failed: %v", err)
	}
}

// TestDNS_NoCircularDependency verifies DNS domain_resolver never points to itself.
func TestDNS_NoCircularDependency(t *testing.T) {
	jsonStr := mustTranslateYAML(t, fakeipYAML)
	m := parseJSONMap(t, jsonStr)
	dns := m["dns"].(map[string]any)
	servers := dns["servers"].([]any)

	for _, srv := range servers {
		srvMap := srv.(map[string]any)
		tag, _ := srvMap["tag"].(string)
		dr, _ := srvMap["domain_resolver"].(string)
		if dr != "" && dr == tag {
			t.Errorf("circular domain_resolver: server %q points to itself", tag)
		}
	}
}

// TestDNS_FinalNotFakeIP verifies DNS final is never a fakeip server.
func TestDNS_FinalNotFakeIP(t *testing.T) {
	jsonStr := mustTranslateYAML(t, fakeipYAML)
	m := parseJSONMap(t, jsonStr)
	dns := m["dns"].(map[string]any)
	final, _ := dns["final"].(string)
	if final == "" {
		return
	}

	servers := dns["servers"].([]any)
	for _, srv := range servers {
		srvMap := srv.(map[string]any)
		if srvMap["tag"] == final && srvMap["type"] == "fakeip" {
			t.Fatalf("dns.final = %q is a fakeip server, which sing-box rejects", final)
		}
	}
}

// TestNoIPCIDRResolveNo verifies ip_cidr_resolve_no never appears in output.
func TestNoIPCIDRResolveNo(t *testing.T) {
	jsonStr := mustTranslateYAML(t, fakeipYAML)
	if strings.Contains(jsonStr, "ip_cidr_resolve_no") {
		t.Error("found ip_cidr_resolve_no in output; sing-box v1.13 does not support this field")
	}
}

// TestServiceLifecycle tests Init/StartWithContent/Stop/Destroy with a minimal config.
func TestServiceLifecycle(t *testing.T) {
	resetLibboxForTesting()

	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		t.Fatal(err)
	}

	svc := NewService()
	if s := svc.State(); s != StateCreated {
		t.Fatalf("expected StateCreated, got %s", s)
	}

	if err := svc.Init(initJSON(homeDir)); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if s := svc.State(); s != StateInitialized {
		t.Fatalf("expected StateInitialized, got %s", s)
	}

	err := svc.StartWithContent(minimalYAML, "")
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "decode config") ||
			strings.Contains(errMsg, "unknown field") ||
			strings.Contains(errMsg, "invalid") {
			t.Fatalf("config parse error (not network): %v", err)
		}
		t.Logf("StartWithContent returned expected network error: %v", err)
	}

	svc.Stop()
	svc.Destroy()
	if s := svc.State(); s != StateDestroyed {
		t.Fatalf("expected StateDestroyed, got %s", s)
	}
}

// TestServiceDoubleInit verifies calling Init twice on the same instance returns an error.
func TestServiceDoubleInit(t *testing.T) {
	resetLibboxForTesting()

	tmpDir := t.TempDir()
	svc := NewService()

	if err := svc.Init(initJSON(filepath.Join(tmpDir, "home1"))); err != nil {		t.Fatalf("first Init: %v", err)
	}
	if err := svc.Init(initJSON(filepath.Join(tmpDir, "home2"))); err == nil {
		t.Fatal("second Init on same instance should return error")
	}
	svc.Destroy()
}

// TestDetectFormat_PassThrough verifies JSON passthrough is detected correctly.
func TestDetectFormat_PassThrough(t *testing.T) {
	singboxJSON := `{"log":{"level":"warn"},"inbounds":[{"type":"mixed","tag":"in"}],"outbounds":[{"type":"direct","tag":"DIRECT"}],"route":{"final":"DIRECT"}}`

	got := translator.DetectFormat([]byte(singboxJSON))
	if got != translator.FormatJSON {
		t.Errorf("DetectFormat for valid sing-box JSON = %v, want FormatJSON", got)
	}
}

// TestReloadConfig_NotInitialized verifies ReloadConfig returns error on uninitialized service.
func TestReloadConfig_NotInitialized(t *testing.T) {
	svc := NewService() // StateCreated

	err := svc.ReloadConfig(minimalYAML, "")
	if err == nil {
		t.Fatal("ReloadConfig should return error when not initialized")
	}
	if !strings.Contains(err.Error(), "invalid state") {
		t.Errorf("ReloadConfig error = %q, want state error", err.Error())
	}
}

// TestReloadTUN_NotInitialized verifies ReloadTUN returns error on uninitialized service.
func TestReloadTUN_NotInitialized(t *testing.T) {
	svc := NewService() // StateCreated

	err := svc.ReloadTUN()
	if err == nil {
		t.Fatal("ReloadTUN should return error when not initialized")
	}
	if !strings.Contains(err.Error(), "invalid state") {
		t.Errorf("ReloadTUN error = %q, want state error", err.Error())
	}
}

// TestReloadTUN_NoConfigStored verifies ReloadTUN returns error when service is not running.
func TestReloadTUN_NoConfigStored(t *testing.T) {
	resetLibboxForTesting()

	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		t.Fatal(err)
	}

	svc := NewService()
	if err := svc.Init(initJSON(homeDir)); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer svc.Destroy()

	// ReloadTUN should fail because state is Initialized (not Running)
	err := svc.ReloadTUN()
	if err == nil {
		t.Fatal("ReloadTUN should return error when not running")
	}
	if !strings.Contains(err.Error(), "invalid state") {
		t.Errorf("ReloadTUN error = %q, want state error", err.Error())
	}
}

// TestStartWithJSON_RollbackOnFailure verifies currentConfig is rolled back when startWithJSON fails.
func TestStartWithJSON_RollbackOnFailure(t *testing.T) {
	resetLibboxForTesting()

	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		t.Fatal(err)
	}

	svc := NewService()
	if err := svc.Init(initJSON(homeDir)); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer svc.Destroy()

	// StartWithContent fails (no clash_api tag), currentConfig should remain empty.
	_ = svc.StartWithContent(minimalYAML, "")
	if svc.currentConfig != "" {
		t.Error("currentConfig should be empty after failed StartWithContent")
	}
}
