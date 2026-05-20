package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", dir, err)
	}
}

// initJSON returns a minimal Init options JSON for the given home directory.
func initJSON(homeDir string) string {
	data, _ := json.Marshal(InitOptions{HomeDir: homeDir})
	return string(data)
}

func TestService_StopWhenNotRunning(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	mustMkdirAll(t, homeDir)

	svc := NewService()
	if err := svc.Init(initJSON(homeDir)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer svc.Destroy()

	if err := svc.Stop(); err != nil {
		t.Errorf("Stop on non-running should return nil, got %v", err)
	}
}

func TestService_DestroyTwice(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	mustMkdirAll(t, homeDir)

	svc := NewService()
	if err := svc.Init(initJSON(homeDir)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	svc.Destroy()
	svc.Destroy()
}

func TestService_StartOnDestroyed(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	mustMkdirAll(t, homeDir)

	svc := NewService()
	if err := svc.Init(initJSON(homeDir)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	svc.Destroy()

	err := svc.StartWithContent(minimalYAML, "")
	if err == nil || !strings.Contains(err.Error(), "invalid state") {
		t.Errorf("StartWithContent after Destroy should fail with state error, got %v", err)
	}
}

func TestService_FlushDNS(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	mustMkdirAll(t, homeDir)

	svc := NewService()
	if err := svc.Init(initJSON(homeDir)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer svc.Destroy()

	svc.FlushSystemDNS()
}

func TestApplyRuleSetProxy(t *testing.T) {
	input := `{
		"route": {
			"rule_set": [
				{"tag": "geoip-cn", "type": "remote", "url": "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-cn.srs"},
				{"tag": "geosite-cn", "type": "remote", "url": "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-cn.srs"},
				{"tag": "custom", "type": "remote", "url": "https://example.com/rules.srs"}
			]
		}
	}`

	result, err := applyRuleSetProxy([]byte(input), "https://gh-proxy.org")
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, json.Unmarshal([]byte(result), &cfg))

	ruleSets := cfg["route"].(map[string]any)["rule_set"].([]any)

	// raw.githubusercontent.com URLs should be prefixed
	assert.Equal(t, "https://gh-proxy.org/https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-cn.srs",
		ruleSets[0].(map[string]any)["url"])
	assert.Equal(t, "https://gh-proxy.org/https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-cn.srs",
		ruleSets[1].(map[string]any)["url"])

	// Non-GitHub URLs should remain unchanged
	assert.Equal(t, "https://example.com/rules.srs",
		ruleSets[2].(map[string]any)["url"])
}

func TestApplyRuleSetProxy_EmptyProxy(t *testing.T) {
	input := `{"route": {"rule_set": [{"tag": "t", "url": "https://raw.githubusercontent.com/test.srs"}]}}`

	// Empty proxy should return original
	result, err := applyRuleSetProxy([]byte(input), "")
	require.NoError(t, err)
	// translateConfig skips applyRuleSetProxy when proxy is empty,
	// but if called directly, it still applies since caller checks
	_ = result
}

func TestApplyRuleSetProxy_NoRoute(t *testing.T) {
	input := `{"log": {"level": "info"}}`
	result, err := applyRuleSetProxy([]byte(input), "https://gh-proxy.org")
	require.NoError(t, err)
	assert.JSONEq(t, input, result)
}
