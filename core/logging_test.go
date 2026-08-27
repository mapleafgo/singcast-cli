package core

import (
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"

	singboxlog "github.com/sagernet/sing-box/log"
)

func TestLogging_Callback(t *testing.T) {
	var mu sync.Mutex
	var gotEntries []LogEntry

	SetOnLogEvent(func(eventType int32, jsonStr string) {
		if eventType != EventLog {
			t.Errorf("eventType = %d, want %d", eventType, EventLog)
		}
		var entry LogEntry
		if err := json.Unmarshal([]byte(jsonStr), &entry); err != nil {
			t.Errorf("unmarshal: %v", err)
		}
		mu.Lock()
		gotEntries = append(gotEntries, entry)
		mu.Unlock()
	})
	defer SetOnLogEvent(nil)

	slog.Info("callback-test", "key", "val")

	mu.Lock()
	defer mu.Unlock()
	if len(gotEntries) == 0 {
		t.Fatal("expected at least one log entry via callback")
	}
	if !strings.Contains(gotEntries[len(gotEntries)-1].Message, "callback-test") {
		t.Errorf("entry = %q, should contain callback-test", gotEntries[len(gotEntries)-1].Message)
	}
}

func TestLogging_HandleAttrs(t *testing.T) {
	var mu sync.Mutex
	var messages []string

	SetOnLogEvent(func(_ int32, jsonStr string) {
		var entry LogEntry
		_ = json.Unmarshal([]byte(jsonStr), &entry)
		mu.Lock()
		messages = append(messages, entry.Message)
		mu.Unlock()
	})
	defer SetOnLogEvent(nil)

	slog.Info("with-attrs", "key1", "val1", "key2", 42)

	mu.Lock()
	defer mu.Unlock()
	if len(messages) == 0 {
		t.Fatal("expected at least one entry")
	}
	if !strings.Contains(messages[len(messages)-1], "key1=val1") {
		t.Errorf("entry should contain key1=val1: %q", messages[len(messages)-1])
	}
	if !strings.Contains(messages[len(messages)-1], "key2=42") {
		t.Errorf("entry should contain key2=42: %q", messages[len(messages)-1])
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

func TestPlatformLogWriterUsesLogLevelFilter(t *testing.T) {
	SetLogLevel(LogLevelError)
	defer SetLogLevel(LogLevelInfo)

	var mu sync.Mutex
	var messages []string
	SetOnLogEvent(func(_ int32, jsonStr string) {
		var entry LogEntry
		if err := json.Unmarshal([]byte(jsonStr), &entry); err != nil {
			t.Errorf("unmarshal: %v", err)
			return
		}
		mu.Lock()
		messages = append(messages, entry.Message)
		mu.Unlock()
	})
	defer SetOnLogEvent(nil)

	writer := &platformLogWriter{}
	writer.WriteMessage(singboxlog.LevelDebug, "platform-debug")
	writer.WriteMessage(singboxlog.LevelError, "platform-error")

	mu.Lock()
	defer mu.Unlock()
	if len(messages) != 1 || messages[0] != "platform-error" {
		t.Fatalf("messages = %v, want only platform-error", messages)
	}
}
