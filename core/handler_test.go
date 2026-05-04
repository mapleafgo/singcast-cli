package core

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/sagernet/sing-box/experimental/libbox"
)

// mockLogIterator implements libbox.LogIterator.
type mockLogIterator struct {
	items []*libbox.LogEntry
	index int
}

func (m *mockLogIterator) Len() int32     { return int32(len(m.items)) }
func (m *mockLogIterator) HasNext() bool  { return m.index < len(m.items) }
func (m *mockLogIterator) Next() *libbox.LogEntry {
	if m.index >= len(m.items) {
		return nil
	}
	e := m.items[m.index]
	m.index++
	return e
}

// --- Tests ---

func TestHandler_SetOnEvent(t *testing.T) {
	h := NewClientHandler()

	var mu sync.Mutex
	var received []struct {
		eventType int32
		payload   string
	}
	h.SetOnEvent(func(eventType int32, jsonPayload string) {
		mu.Lock()
		received = append(received, struct {
			eventType int32
			payload   string
		}{eventType, jsonPayload})
		mu.Unlock()
	})

	h.ClearLogs()

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 callback, got %d", len(received))
	}
	if received[0].eventType != EventLogs {
		t.Errorf("eventType = %d, want %d", received[0].eventType, EventLogs)
	}
}

func TestHandler_ClearLogs(t *testing.T) {
	h := NewClientHandler()
	var got string
	h.SetOnEvent(func(eventType int32, jsonPayload string) {
		if eventType == EventLogs {
			got = jsonPayload
		}
	})

	h.ClearLogs()

	if got != "[]" {
		t.Errorf("ClearLogs payload = %q, want []", got)
	}
	if cached := h.GetCachedLogs(); len(cached) != 0 {
		t.Errorf("cached logs after ClearLogs = %v, want empty", cached)
	}
}

func TestHandler_WriteLogs(t *testing.T) {
	h := NewClientHandler()
	var got string
	h.SetOnEvent(func(eventType int32, jsonPayload string) {
		if eventType == EventLogs {
			got = jsonPayload
		}
	})

	h.WriteLogs(&mockLogIterator{
		items: []*libbox.LogEntry{
			{Level: 4, Message: "hello"},
			{Level: 2, Message: "error occurred"},
		},
	})

	var entries []LogEntry
	if err := json.Unmarshal([]byte(got), &entries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Message != "hello" {
		t.Errorf("entry[0].Message = %q, want hello", entries[0].Message)
	}
	if entries[1].Level != 2 {
		t.Errorf("entry[1].Level = %d, want 2", entries[1].Level)
	}

	cached := h.GetCachedLogs()
	cachedJSON, _ := json.Marshal(cached)
	if string(cachedJSON) != got {
		t.Error("cached logs should match callback payload")
	}
}

func TestHandler_WriteLogsLevelFilter(t *testing.T) {
	// Default level is Info(4). Debug(5) and Trace(6) should be filtered.
	prevLevel := GetLogLevel()
	defer SetLogLevel(prevLevel)
	SetLogLevel(4) // Info

	h := NewClientHandler()
	var got string
	h.SetOnEvent(func(eventType int32, jsonPayload string) {
		if eventType == EventLogs {
			got = jsonPayload
		}
	})

	h.WriteLogs(&mockLogIterator{
		items: []*libbox.LogEntry{
			{Level: 2, Message: "error"},  // Error → pass
			{Level: 3, Message: "warn"},   // Warn → pass
			{Level: 4, Message: "info"},   // Info → pass
			{Level: 5, Message: "debug"},  // Debug → filtered
			{Level: 6, Message: "trace"},  // Trace → filtered
		},
	})

	var entries []LogEntry
	if err := json.Unmarshal([]byte(got), &entries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries (debug/trace filtered), got %d: %+v", len(entries), entries)
	}
	for _, e := range entries {
		if e.Level > 4 {
			t.Errorf("entry level %d should have been filtered: %s", e.Level, e.Message)
		}
	}

	// Raise level to Error(2) — only errors should pass.
	SetLogLevel(2)
	got = ""
	h.WriteLogs(&mockLogIterator{
		items: []*libbox.LogEntry{
			{Level: 2, Message: "error"},
			{Level: 4, Message: "info"},
		},
	})
	if err := json.Unmarshal([]byte(got), &entries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(entries) != 1 || entries[0].Message != "error" {
		t.Errorf("expected only error entry, got %+v", entries)
	}

	// Set to Trace(6) — all should pass.
	SetLogLevel(6)
	got = ""
	h.WriteLogs(&mockLogIterator{
		items: []*libbox.LogEntry{
			{Level: 5, Message: "debug"},
			{Level: 6, Message: "trace"},
		},
	})
	if err := json.Unmarshal([]byte(got), &entries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries at trace level, got %d", len(entries))
	}
}

func TestHandler_WriteStatus(t *testing.T) {
	h := NewClientHandler()
	var got string
	h.SetOnEvent(func(eventType int32, jsonPayload string) {
		if eventType == EventTraffic {
			got = jsonPayload
		}
	})

	h.WriteStatus(&libbox.StatusMessage{
		Uplink: 100, Downlink: 200,
		UplinkTotal: 1000, DownlinkTotal: 2000,
		Memory: 50_000_000, Goroutines: 42,
		ConnectionsIn: 10, ConnectionsOut: 20,
	})

	var snap TrafficSnapshot
	if err := json.Unmarshal([]byte(got), &snap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if snap.Up != 100 || snap.Down != 200 {
		t.Errorf("Up/Down = %d/%d, want 100/200", snap.Up, snap.Down)
	}
	if snap.Memory != 50_000_000 {
		t.Errorf("Memory = %d, want 50000000", snap.Memory)
	}
	if snap.Goroutines != 42 {
		t.Errorf("Goroutines = %d, want 42", snap.Goroutines)
	}

	if cached := h.GetCachedStatusJSON(); cached != got {
		t.Error("cached status should match callback payload")
	}
}

func TestHandler_WriteGroups(t *testing.T) {
	h := NewClientHandler()
	var got string
	h.SetOnEvent(func(eventType int32, jsonPayload string) {
		if eventType == EventProxyUpdate {
			got = jsonPayload
		}
	})

	// OutboundGroup.itemList is unexported, so test via emit which exercises
	// the same cache+callback path that WriteGroups uses internally.
	groups := []ProxyGroup{
		{
			Tag: "PROXY", Type: "Selector", Selectable: true, Selected: "proxy-a",
			Items: []ProxyGroupItem{
				{Tag: "proxy-a", Type: "shadowsocks", Delay: 50},
				{Tag: "DIRECT", Type: "direct"},
			},
		},
	}
	h.emit(EventProxyUpdate, groups)

	var result []ProxyGroup
	if err := json.Unmarshal([]byte(got), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result) != 1 || result[0].Tag != "PROXY" {
		t.Fatalf("unexpected result: %v", result)
	}
	if len(result[0].Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result[0].Items))
	}
	if result[0].Items[0].Delay != 50 {
		t.Errorf("item[0].Delay = %d, want 50", result[0].Items[0].Delay)
	}
	if cached := h.GetCachedGroupsJSON(); cached != got {
		t.Error("cached groups should match callback payload")
	}
}

func TestHandler_InitializeClashMode(t *testing.T) {
	h := NewClientHandler()
	var got string
	h.SetOnEvent(func(eventType int32, jsonPayload string) {
		if eventType == EventModeUpdate {
			got = jsonPayload
		}
	})

	h.InitializeClashMode(&stringIterator{items: []string{"rule", "global", "direct"}}, "rule")

	var result map[string]any
	if err := json.Unmarshal([]byte(got), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	modes, _ := result["modes"].([]any)
	if len(modes) != 3 {
		t.Errorf("modes count = %d, want 3", len(modes))
	}
	if result["current_mode"] != "rule" {
		t.Errorf("current_mode = %v, want rule", result["current_mode"])
	}
}

func TestHandler_UpdateClashMode(t *testing.T) {
	h := NewClientHandler()
	var got string
	h.SetOnEvent(func(eventType int32, jsonPayload string) {
		if eventType == EventModeUpdate {
			got = jsonPayload
		}
	})

	h.UpdateClashMode("global")

	var result map[string]string
	if err := json.Unmarshal([]byte(got), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["current_mode"] != "global" {
		t.Errorf("current_mode = %q, want global", result["current_mode"])
	}
}

func TestHandler_EmitCachesCorrectly(t *testing.T) {
	h := NewClientHandler()

	// Emit traffic → should cache under status.
	h.emit(EventTraffic, TrafficSnapshot{Up: 42, Down: 99})
	cached := h.GetCachedStatusJSON()
	if cached == "{}" {
		t.Error("traffic emit should update status cache")
	}

	// Emit logs → should cache under logs.
	h.emit(EventLogs, []LogEntry{{Level: 4, Message: "test"}})
	if h.GetCachedLogs() == nil {
		t.Error("logs emit should update logs cache")
	}

	// Emit connections → should cache.
	h.emit(EventConnections, map[string]any{"reset": true, "items": []any{}})
	if h.GetCachedConnectionsJSON() == "[]" {
		t.Error("connections emit should update connections cache")
	}

	// Emit proxy update → should cache.
	h.emit(EventProxyUpdate, []ProxyGroup{{Tag: "G"}})
	if h.GetCachedGroupsJSON() == "[]" {
		t.Error("proxy emit should update groups cache")
	}
}

func TestHandler_GetCachedDefaults(t *testing.T) {
	h := NewClientHandler()
	if h.GetCachedGroupsJSON() != "[]" {
		t.Errorf("default groups = %q, want []", h.GetCachedGroupsJSON())
	}
	if h.GetCachedStatusJSON() != "{}" {
		t.Errorf("default status = %q, want {}", h.GetCachedStatusJSON())
	}
	if h.GetCachedLogs() != nil {
		t.Errorf("default logs = %v, want nil", h.GetCachedLogs())
	}
	if h.GetCachedConnectionsJSON() != "[]" {
		t.Errorf("default conns = %q, want []", h.GetCachedConnectionsJSON())
	}
}

func TestHandler_NilCallbackNoPanic(t *testing.T) {
	h := NewClientHandler()
	// Should not panic with nil callback.
	h.WriteLogs(&mockLogIterator{})
	h.WriteStatus(&libbox.StatusMessage{})
	h.ClearLogs()
	h.InitializeClashMode(&stringIterator{}, "rule")
	h.UpdateClashMode("global")
}

func TestHandler_ConnectedDisconnected(t *testing.T) {
	h := NewClientHandler()
	// Should be no-ops without panic.
	h.Connected()
	h.Disconnected("")
	h.SetDefaultLogLevel(4)
}
