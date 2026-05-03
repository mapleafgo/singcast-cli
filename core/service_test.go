package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", dir, err)
	}
}

func TestService_QueryMemoryStats(t *testing.T) {
	resetLibboxForTesting()
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	mustMkdirAll(t, homeDir)

	svc := NewService()
	if err := svc.Init(homeDir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer svc.Destroy()

	got := svc.QueryMemoryStats()
	var m map[string]int64
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"sys", "heap_alloc", "heap_sys", "stack_inuse", "goroutines", "limit"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing field %q in %s", key, got)
		}
	}
	if m["goroutines"] < 1 {
		t.Errorf("goroutines = %d, want >= 1", m["goroutines"])
	}
}

func TestService_QueryLogsReturnsJSON(t *testing.T) {
	resetLibboxForTesting()
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	mustMkdirAll(t, homeDir)

	svc := NewService()
	if err := svc.Init(homeDir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer svc.Destroy()

	got := svc.QueryLogs()
	var entries []LogEntry
	if err := json.Unmarshal([]byte(got), &entries); err != nil {
		t.Fatalf("QueryLogs returned invalid JSON: %v", err)
	}
}

func TestService_SetOnEventBeforeInit(t *testing.T) {
	svc := NewService()
	svc.SetOnEvent(func(eventType int32, jsonPayload string) {})
}

func TestService_SetMemoryLimitWithMonitorAfterDestroy(t *testing.T) {
	resetLibboxForTesting()
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	mustMkdirAll(t, homeDir)

	svc := NewService()
	if err := svc.Init(homeDir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	svc.Destroy()

	svc.SetMemoryLimitWithMonitor(1 << 30)
	if svc.oom.running.Load() {
		t.Error("SetMemoryLimitWithMonitor should not start monitor after Destroy")
	}
}

func TestService_SetMemoryLimitWithMonitorZero(t *testing.T) {
	resetLibboxForTesting()
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	mustMkdirAll(t, homeDir)

	svc := NewService()
	if err := svc.Init(homeDir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer svc.Destroy()

	svc.SetMemoryLimitWithMonitor(1 << 20)
	if !svc.oom.running.Load() {
		t.Fatal("expected monitor running after setLimit")
	}
	svc.SetMemoryLimitWithMonitor(0)
	for i := 0; i < 50; i++ {
		if !svc.oom.running.Load() {
			return
		}
	}
	t.Error("expected monitor stopped after setLimit(0)")
}

func TestService_StopWhenNotRunning(t *testing.T) {
	resetLibboxForTesting()
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	mustMkdirAll(t, homeDir)

	svc := NewService()
	if err := svc.Init(homeDir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer svc.Destroy()

	if err := svc.Stop(); err != nil {
		t.Errorf("Stop on non-running should return nil, got %v", err)
	}
}

func TestService_DestroyTwice(t *testing.T) {
	resetLibboxForTesting()
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	mustMkdirAll(t, homeDir)

	svc := NewService()
	if err := svc.Init(homeDir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	svc.Destroy()
	svc.Destroy()
}

func TestService_StartOnDestroyed(t *testing.T) {
	resetLibboxForTesting()
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	mustMkdirAll(t, homeDir)

	svc := NewService()
	if err := svc.Init(homeDir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	svc.Destroy()

	err := svc.StartWithContent(minimalYAML, "")
	if err == nil || !strings.Contains(err.Error(), "invalid state") {
		t.Errorf("StartWithContent after Destroy should fail with state error, got %v", err)
	}
}

func TestService_FlushDNS(t *testing.T) {
	resetLibboxForTesting()
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	mustMkdirAll(t, homeDir)

	svc := NewService()
	if err := svc.Init(homeDir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer svc.Destroy()

	svc.FlushSystemDNS()
}

func TestService_QueryLogsWithCoreLogs(t *testing.T) {
	resetLibboxForTesting()
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	mustMkdirAll(t, homeDir)

	svc := NewService()
	if err := svc.Init(homeDir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer svc.Destroy()

	got := svc.QueryLogs()
	var entries []LogEntry
	if err := json.Unmarshal([]byte(got), &entries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected at least some core log entries from init")
	}
}
