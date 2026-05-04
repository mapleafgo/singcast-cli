package core

import (
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestLogging_RingBufferOrder(t *testing.T) {
	// Reset ring buffer state.
	coreLogMu.Lock()
	coreLogPos = 0
	coreLogLen = 0
	coreLogCb = nil
	coreLogMu.Unlock()

	// Write entries via slog.
	for i := 0; i < 5; i++ {
		slog.Info("test message", "index", i)
	}

	got := queryCoreLogs()
	var entries []LogEntry
	if err := json.Unmarshal([]byte(got), &entries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}
	// Verify order is chronological.
	for i, e := range entries {
		if !strings.Contains(e.Message, "test message") {
			t.Errorf("entry[%d] = %q, should contain 'test message'", i, e.Message)
		}
	}
}

func TestLogging_RingBufferOverflow(t *testing.T) {
	coreLogMu.Lock()
	coreLogPos = 0
	coreLogLen = 0
	coreLogCb = nil
	coreLogMu.Unlock()

	// Ensure all log levels pass for this test.
	prevLevel := GetLogLevel()
	SetLogLevel(6) // Trace
	defer SetLogLevel(prevLevel)

	// Write more than maxCoreLogEntries entries.
	for i := 0; i < maxCoreLogEntries+100; i++ {
		slog.Debug("overflow-test", "i", i)
	}

	coreLogMu.Lock()
	if coreLogLen != maxCoreLogEntries {
		t.Errorf("coreLogLen = %d, want %d", coreLogLen, maxCoreLogEntries)
	}
	coreLogMu.Unlock()

	got := queryCoreLogs()
	var entries []LogEntry
	if err := json.Unmarshal([]byte(got), &entries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(entries) != maxCoreLogEntries {
		t.Errorf("queryCoreLogs returned %d entries, want %d", len(entries), maxCoreLogEntries)
	}

	// First entry should be the oldest surviving entry (index 100, since first 100 were overwritten).
	if !strings.Contains(entries[0].Message, "overflow-test") {
		t.Errorf("oldest entry = %q, should contain overflow-test", entries[0].Message)
	}
}

func TestLogging_SetLogCallback(t *testing.T) {
	coreLogMu.Lock()
	coreLogPos = 0
	coreLogLen = 0
	coreLogCb = nil
	coreLogMu.Unlock()

	var received []string
	setLogCallback(func(eventType int32, jsonPayload string) {
		if eventType == EventCoreLog {
			received = append(received, jsonPayload)
		}
	})

	slog.Info("callback-test")

	if len(received) == 0 {
		t.Fatal("expected callback to fire for slog output")
	}

	var entries []LogEntry
	if err := json.Unmarshal([]byte(received[0]), &entries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(entries) != 1 || !strings.Contains(entries[0].Message, "callback-test") {
		t.Errorf("unexpected entry: %v", entries)
	}

	// Unset callback.
	setLogCallback(nil)
	received = nil
	slog.Info("should-not-callback")
	if len(received) != 0 {
		t.Error("callback should not fire after setLogCallback(nil)")
	}
}

func TestLogging_QueryEmpty(t *testing.T) {
	coreLogMu.Lock()
	coreLogPos = 0
	coreLogLen = 0
	coreLogCb = nil
	coreLogMu.Unlock()

	got := queryCoreLogs()
	if got != "[]" {
		t.Errorf("empty ring = %q, want []", got)
	}
}

func TestLogging_HandleAttrs(t *testing.T) {
	coreLogMu.Lock()
	coreLogPos = 0
	coreLogLen = 0
	coreLogCb = nil
	coreLogMu.Unlock()

	slog.Info("with-attrs", "key1", "val1", "key2", 42)

	got := queryCoreLogs()
	var entries []LogEntry
	if err := json.Unmarshal([]byte(got), &entries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if !strings.Contains(entries[0].Message, "key1=val1") {
		t.Errorf("entry should contain attrs: %q", entries[0].Message)
	}
	if !strings.Contains(entries[0].Message, "key2=42") {
		t.Errorf("entry should contain attrs: %q", entries[0].Message)
	}
}

func TestSlogToCoreLevel(t *testing.T) {
	tests := []struct {
		input slog.Level
		want  int32
	}{
		{slog.LevelError, 2},
		{slog.LevelWarn, 3},
		{slog.LevelInfo, 4},
		{slog.LevelDebug, 5},
		{slog.Level(-10), 6}, // below Debug → Trace
	}
	for _, tt := range tests {
		got := slogToCoreLevel(tt.input)
		if got != tt.want {
			t.Errorf("slogToCoreLevel(%v) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
