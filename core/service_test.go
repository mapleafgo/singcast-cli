package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sagernet/sing-box/experimental/libbox"
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

func TestService_QueryMemoryStats(t *testing.T) {
	resetLibboxForTesting()
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	mustMkdirAll(t, homeDir)

	svc := NewService()
	if err := svc.Init(initJSON(homeDir)); err != nil {
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

func TestService_SetOnEventBeforeInit(t *testing.T) {
	svc := NewService()
	svc.SetOnEvent(func(eventType int32, jsonPayload string) {})
}

func TestService_StopWhenNotRunning(t *testing.T) {
	resetLibboxForTesting()
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
	resetLibboxForTesting()
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
	resetLibboxForTesting()
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
	resetLibboxForTesting()
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
	resetLibboxForTesting()
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

func TestParseOverrideOptions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantInc  int
		wantExc  int
		wantAuto bool
		wantErr  bool
	}{
		{"empty", "", 0, 0, false, false},
		{"valid", `{"include_packages":["a","b"],"exclude_packages":["c"]}`, 2, 1, false, false},
		{"auto_redirect", `{"auto_redirect":true,"include_packages":["x"]}`, 1, 0, true, false},
		{"invalid_json", `{bad`, 0, 0, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, cfg, err := parseOverrideOptions(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if opts == nil {
				t.Fatal("opts should not be nil on success")
			}
			if cfg.AutoRedirect != tt.wantAuto {
				t.Errorf("AutoRedirect = %v, want %v", cfg.AutoRedirect, tt.wantAuto)
			}
			countInc := countIter(opts.IncludePackage)
			countExc := countIter(opts.ExcludePackage)
			if countInc != tt.wantInc {
				t.Errorf("include count = %d, want %d", countInc, tt.wantInc)
			}
			if countExc != tt.wantExc {
				t.Errorf("exclude count = %d, want %d", countExc, tt.wantExc)
			}
		})
	}
}

func countIter(it libbox.StringIterator) int {
	if it == nil {
		return 0
	}
	n := 0
	for it.HasNext() {
		it.Next()
		n++
	}
	return n
}

func TestSetOverridePackages_StateChecks(t *testing.T) {
	resetLibboxForTesting()
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

	t.Run("InvalidJSON", func(t *testing.T) {
		// parseOverrideOptions validates JSON regardless of state.
		// Test it directly to ensure the parse path is covered.
		_, _, err := parseOverrideOptions(`{bad`)
		if err == nil {
			t.Error("expected parse error for invalid JSON")
		}
	})
}
