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

// initJSON returns a minimal Init options JSON for the given home directory.
func initJSON(homeDir string) string {
	data, _ := json.Marshal(InitOptions{HomeDir: homeDir})
	return string(data)
}

func TestService_QueryLogsReturnsJSON(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	mustMkdirAll(t, homeDir)

	svc := NewService()
	if err := svc.Init(initJSON(homeDir)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer svc.Destroy()

	got := svc.QueryLogs()
	var entries []LogEntry
	if err := json.Unmarshal([]byte(got), &entries); err != nil {
		t.Fatalf("QueryLogs returned invalid JSON: %v", err)
	}
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

func TestService_QueryLogsWithCoreLogs(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	mustMkdirAll(t, homeDir)

	svc := NewService()
	if err := svc.Init(initJSON(homeDir)); err != nil {
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

func TestSetOverridePackages_StateChecks(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	mustMkdirAll(t, homeDir)

	svc := NewService()
	if err := svc.Init(initJSON(homeDir)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer svc.Destroy()

	t.Run("NotRunning", func(t *testing.T) {
		if err := svc.SetOverridePackages(`{"include_packages":["a"]}`); err == nil {
			t.Error("expected error when not running")
		}
	})
}
