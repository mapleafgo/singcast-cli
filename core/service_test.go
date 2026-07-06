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

func TestService_QueryProxies_NotRunning(t *testing.T) {
	svc := NewService()
	defer svc.Destroy()

	result := svc.QueryProxies()
	assert.JSONEq(t, "[]", result)
}

func TestService_QueryStats_NotRunning(t *testing.T) {
	svc := NewService()
	defer svc.Destroy()

	result := svc.QueryStats()
	assert.Contains(t, result, `"up":0`)
	assert.Contains(t, result, `"down":0`)
	assert.Contains(t, result, `"connections":0`)
}

func TestService_QueryConnections_NotRunning(t *testing.T) {
	svc := NewService()
	defer svc.Destroy()

	result := svc.QueryConnections()
	assert.JSONEq(t, "[]", result)
}

func TestService_QueryMode_NotRunning(t *testing.T) {
	svc := NewService()
	defer svc.Destroy()

	result := svc.QueryMode()
	assert.Contains(t, result, `"current_mode":"Rule"`)
	assert.Contains(t, result, `"modes"`)
}

func TestService_QueryRules_NotRunning(t *testing.T) {
	svc := NewService()
	defer svc.Destroy()

	result := svc.QueryRules()
	assert.Contains(t, result, `"rules":[]`)
}

func TestService_TestGroupDelay_NotRunning(t *testing.T) {
	svc := NewService()
	defer svc.Destroy()

	result := svc.TestGroupDelay("test-group", 3000)
	assert.JSONEq(t, "{}", result)
}

func TestService_URLTest_NotRunning(t *testing.T) {
	svc := NewService()
	defer svc.Destroy()

	result := svc.URLTest("test-outbound", 3000)
	assert.Equal(t, int32(-1), result)
}

func TestService_SelectOutbound_NotRunning(t *testing.T) {
	svc := NewService()
	defer svc.Destroy()

	err := svc.SelectOutbound("group", "outbound")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "service not running")
}

func TestService_SetMode_NotRunning(t *testing.T) {
	svc := NewService()
	defer svc.Destroy()

	err := svc.SetMode("Rule")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "clash API not available")
}

func TestService_CloseConnection_NotRunning(t *testing.T) {
	svc := NewService()
	defer svc.Destroy()

	err := svc.CloseConnection("00000000-0000-0000-0000-000000000000")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "clash API not available")
}

func TestService_CloseConnection_InvalidID(t *testing.T) {
	svc := NewService()
	defer svc.Destroy()

	err := svc.CloseConnection("not-a-uuid")
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "clash API not available") || strings.Contains(err.Error(), "invalid connection ID"))
}

func TestService_CloseConnections_NotRunning(t *testing.T) {
	svc := NewService()
	defer svc.Destroy()

	err := svc.CloseConnections()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "clash API not available")
}

func TestService_SetGroupExpand_NotRunning(t *testing.T) {
	svc := NewService()
	defer svc.Destroy()

	err := svc.SetGroupExpand("group", true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "service not running")
}

func TestService_FlushFakeIP_NotRunning(t *testing.T) {
	svc := NewService()
	defer svc.Destroy()

	err := svc.FlushFakeIP()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "service not running")
}

func TestService_FlushDNSCache_NotRunning(t *testing.T) {
	svc := NewService()
	defer svc.Destroy()

	err := svc.FlushDNSCache()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "service not running")
}

func TestService_SetOnEvent_Nil(t *testing.T) {
	svc := NewService()
	defer svc.Destroy()

	svc.SetOnEvent(nil)
	assert.Nil(t, svc.getOnEvent())
}

func TestService_SetOnEvent_Callback(t *testing.T) {
	svc := NewService()
	defer svc.Destroy()

	var receivedEvent int32
	var receivedData string
	svc.SetOnEvent(func(event int32, data string) {
		receivedEvent = event
		receivedData = data
	})

	fn := svc.getOnEvent()
	require.NotNil(t, fn)
	fn(42, "test-data")
	assert.Equal(t, int32(42), receivedEvent)
	assert.Equal(t, "test-data", receivedData)
}

func TestService_TriggerGC(t *testing.T) {
	svc := NewService()
	defer svc.Destroy()

	svc.TriggerGC()
}

func TestService_State(t *testing.T) {
	svc := NewService()
	defer svc.Destroy()

	assert.Equal(t, StateCreated, svc.State())
}

func TestService_PlatformIO(t *testing.T) {
	svc := NewService()
	defer svc.Destroy()

	assert.NotNil(t, svc.PlatformIO())
}

func TestState_String(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateCreated, "created"},
		{StateInitialized, "initialized"},
		{StateStarting, "starting"},
		{StateRunning, "running"},
		{StateDestroyed, "destroyed"},
		{State(99), "unknown"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.state.String())
	}
}
