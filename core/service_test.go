package core

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mapleafgo/singcast/translator"
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

var testOriginalWD = func() string {
	wd, err := os.Getwd()
	if err != nil {
		return os.TempDir()
	}
	return wd
}()

func chdirInTest(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(testOriginalWD))
	})
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

func TestApplyRuleSetProxy_GitHubOnly(t *testing.T) {
	input := `{"route":{"rule_set":[
		{"tag":"gh","url":"https://raw.githubusercontent.com/a/b/c.srs"},
		{"tag":"other","url":"https://example.com/r.srs"}
	]}}`
	result, err := translator.ApplyRuleSetProxy(input, "https://mirror.example.com")
	require.NoError(t, err)
	var cfg map[string]any
	require.NoError(t, json.Unmarshal([]byte(result), &cfg))
	rs := cfg["route"].(map[string]any)["rule_set"].([]any)
	assert.Equal(t, "https://mirror.example.com/https://raw.githubusercontent.com/a/b/c.srs",
		rs[0].(map[string]any)["url"])
	assert.Equal(t, "https://example.com/r.srs",
		rs[1].(map[string]any)["url"])
}

func TestApplyRuleSetProxy_EmptyProxy(t *testing.T) {
	input := `{"route":{"rule_set":[{"tag":"t","url":"https://raw.githubusercontent.com/x.srs"}]}}`
	result, err := translator.ApplyRuleSetProxy(input, "")
	require.NoError(t, err)
	assert.Equal(t, input, result)
}
func TestService_QueryProxies_NotRunning(t *testing.T) {
	svc := NewService()
	defer svc.Destroy()

	result := svc.QueryProxies()
	assert.JSONEq(t, "[]", result)
}

// TestTranslateConfigRestoresStubTagsFromJSON verifies that stub metadata
// embedded in a converted JSON config is recovered through the real
// translateConfig path, so restarting from the saved profile keeps the
// "unsupported" marker instead of degrading to a plain socks node.
func TestTranslateConfigRestoresStubTagsFromJSON(t *testing.T) {
	yaml := `mixed-port: 1080
proxies:
  - name: ssr-node
    type: ssr
    server: 1.2.3.4
    port: 1080
  - name: good-node
    type: socks5
    server: 5.6.7.8
    port: 1081
proxy-groups:
  - name: PROXY
    type: select
    proxies: [ssr-node, good-node, DIRECT]
`
	jsonStr, _, _, err := translator.ConvertWithMeta([]byte(yaml), &translator.Options{Country: "CN"})
	require.NoError(t, err)

	svc := NewService()
	defer svc.Destroy()
	_, tags, err := svc.translateConfig([]byte(jsonStr), "")
	require.NoError(t, err)
	assert.Equal(t, "ssr", tags["ssr-node"])
	assert.NotContains(t, tags, "good-node")
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

func TestService_SetOnEvent_ReceivesCoreLogs(t *testing.T) {
	SetLogLevel(LogLevelInfo)
	svc := NewService()
	defer svc.Destroy()

	var receivedEvent int32
	var receivedData string
	svc.SetOnEvent(func(eventType int32, data string) {
		receivedEvent = eventType
		receivedData = data
	})
	defer svc.SetOnEvent(nil)

	slog.Info("service-log-test")

	assert.Equal(t, EventLog, receivedEvent)
	assert.Contains(t, receivedData, "service-log-test")
}

func TestService_InitContextNormalizesRelativeHomeDir(t *testing.T) {
	root := t.TempDir()
	chdirInTest(t, root)

	svc := NewService()
	require.NoError(t, svc.InitContext(context.Background(), initJSON("home")))
	defer svc.Destroy()

	if _, err := os.Stat(filepath.Join(root, "home", "temp")); err != nil {
		t.Fatalf("stat home temp dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "home", "home", "temp")); err == nil {
		t.Fatal("relative home path was resolved again after chdir")
	}
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

// 事件回调是在状态变更路径上同步调用的，回调内必须能安全地反手调用 Stop()。
// 若 Stop 持有 start 路径的锁，这里会因 sync.Mutex 不可重入而死锁。
func TestService_StopFromEventCallback(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	opts, err := json.Marshal(InitOptions{HomeDir: dir})
	require.NoError(t, err)

	var stopErr error
	var stopped atomic.Bool
	svc.SetOnEvent(func(event int32, data string) {
		if event == EventStateChange && data == StateStarting.String() && stopped.CompareAndSwap(false, true) {
			stopErr = svc.Stop()
		}
	})
	require.NoError(t, svc.Init(string(opts)))
	defer svc.Destroy()

	// 配置无关：只要状态推进到 Starting 就会触发上面的回调。
	// 用 done channel 加超时兜底，死锁时给出明确失败而不是拖到 go test 超时。
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = svc.StartWithContent(`{"log":{"level":"error"}}`, "")
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("deadlock: Stop() called from event callback did not return")
	}
	require.True(t, stopped.Load(), "callback should have fired on Starting")
	require.NoError(t, stopErr)
}
