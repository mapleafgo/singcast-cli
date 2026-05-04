package core

import (
	"encoding/json"
	"sync"

	"github.com/sagernet/sing-box/experimental/libbox"
)

// Event type constants passed to the onEvent callback.
const (
	EventTraffic     = 0
	EventLogs        = 1
	EventConnections = 2
	EventProxyUpdate = 3
	EventModeUpdate  = 4
	EventCoreLog     = 6
	EventConnected    = 7
	EventDisconnected = 8
)

// ClientHandler implements libbox.CommandClientHandler.
// Each callback converts the libbox data to JSON and forwards it
// through the onEvent function.
type ClientHandler struct {
	onEvent func(eventType int32, jsonPayload string)
	mu      sync.Mutex

	// cached JSON strings for query APIs
	cachedGroups string
	cachedStatus string
	cachedLogs   string
	cachedConns  string
}

// NewClientHandler creates a ClientHandler.
// Call SetOnEvent to register the callback.
func NewClientHandler() *ClientHandler {
	return &ClientHandler{}
}

// SetOnEvent replaces the event callback.
func (h *ClientHandler) SetOnEvent(fn func(eventType int32, jsonPayload string)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onEvent = fn
}

func (h *ClientHandler) emit(eventType int32, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	jsonStr := string(data)

	h.mu.Lock()
	fn := h.onEvent
	switch eventType {
	case EventTraffic:
		h.cachedStatus = jsonStr
	case EventLogs:
		h.cachedLogs = jsonStr
	case EventConnections:
		h.cachedConns = jsonStr
	case EventProxyUpdate:
		h.cachedGroups = jsonStr
	}
	h.mu.Unlock()

	if fn != nil {
		fn(eventType, jsonStr)
	}
}

func (h *ClientHandler) getCached(cache *string, fallback string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if *cache == "" {
		return fallback
	}
	return *cache
}

// GetCachedGroupsJSON returns the last cached proxy group JSON.
func (h *ClientHandler) GetCachedGroupsJSON() string {
	return h.getCached(&h.cachedGroups, "[]")
}

// GetCachedStatusJSON returns the last cached traffic status JSON.
func (h *ClientHandler) GetCachedStatusJSON() string {
	return h.getCached(&h.cachedStatus, "{}")
}

// GetCachedLogsJSON returns the last cached log entries JSON.
func (h *ClientHandler) GetCachedLogsJSON() string {
	return h.getCached(&h.cachedLogs, "[]")
}

// GetCachedConnectionsJSON returns the last cached connection events JSON.
func (h *ClientHandler) GetCachedConnectionsJSON() string {
	return h.getCached(&h.cachedConns, "[]")
}

// Connected is called when the command client connects to the server.
func (h *ClientHandler) Connected() {
	h.emit(EventConnected, map[string]bool{"connected": true})
}

// Disconnected is called when the command client disconnects.
func (h *ClientHandler) Disconnected(message string) {
	h.emit(EventDisconnected, map[string]string{"message": message})
}

// SetDefaultLogLevel is called with the initial log level from the server.
func (h *ClientHandler) SetDefaultLogLevel(level int32) {
	SetLogLevel(level)
}

// ClearLogs is called when the log buffer is cleared on the server side.
func (h *ClientHandler) ClearLogs() {
	h.emit(EventLogs, []LogEntry{})
}

// WriteLogs receives a batch of log entries from the server.
func (h *ClientHandler) WriteLogs(messageList libbox.LogIterator) {
	var entries []LogEntry
	for messageList.HasNext() {
		entry := messageList.Next()
		entries = append(entries, LogEntry{
			Level:   entry.Level,
			Message: entry.Message,
		})
	}
	h.emit(EventLogs, entries)
}

// WriteStatus receives traffic and resource usage statistics.
func (h *ClientHandler) WriteStatus(message *libbox.StatusMessage) {
	snapshot := TrafficSnapshot{
		Up:         message.Uplink,
		Down:       message.Downlink,
		UpTotal:    message.UplinkTotal,
		DownTotal:  message.DownlinkTotal,
		Memory:     message.Memory,
		Goroutines: message.Goroutines,
		ConnsIn:    message.ConnectionsIn,
		ConnsOut:   message.ConnectionsOut,
	}
	h.emit(EventTraffic, snapshot)
}

// WriteGroups receives the current proxy group snapshot.
func (h *ClientHandler) WriteGroups(message libbox.OutboundGroupIterator) {
	var groups []ProxyGroup
	for message.HasNext() {
		g := message.Next()
		group := ProxyGroup{
			Tag:        g.Tag,
			Type:       g.Type,
			Selectable: g.Selectable,
			Selected:   g.Selected,
		}
		items := g.GetItems()
		for items.HasNext() {
			item := items.Next()
			group.Items = append(group.Items, ProxyGroupItem{
				Tag:   item.Tag,
				Type:  item.Type,
				Delay: item.URLTestDelay,
			})
		}
		groups = append(groups, group)
	}
	h.emit(EventProxyUpdate, groups)
}

// InitializeClashMode is called once with the available clash modes.
func (h *ClientHandler) InitializeClashMode(modeList libbox.StringIterator, currentMode string) {
	var modes []string
	for modeList.HasNext() {
		modes = append(modes, modeList.Next())
	}
	h.emit(EventModeUpdate, map[string]any{
		"modes":        modes,
		"current_mode": currentMode,
	})
}

// UpdateClashMode is called when the clash mode changes.
func (h *ClientHandler) UpdateClashMode(newMode string) {
	h.emit(EventModeUpdate, map[string]string{
		"current_mode": newMode,
	})
}

// WriteConnectionEvents receives connection tracker events.
func (h *ClientHandler) WriteConnectionEvents(events *libbox.ConnectionEvents) {
	var snapshots []ConnectionEventSnapshot
	it := events.Iterator()
	for it.HasNext() {
		evt := it.Next()
		snapshots = append(snapshots, ConnectionEventSnapshot{
			EventType: evt.Type,
			ID:        evt.ID,
		})
	}
	h.emit(EventConnections, map[string]any{
		"reset": events.Reset,
		"items": snapshots,
	})
}
