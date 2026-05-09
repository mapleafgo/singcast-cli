package core

import (
	"encoding/json"
	"sync"

	"github.com/sagernet/sing-box/experimental/libbox"
)

// Event type constants passed to the onEvent callback.
const (
	EventTraffic      = 0
	EventLogs         = 1
	EventConnections  = 2
	EventProxyUpdate  = 3
	EventModeUpdate   = 4
	EventCoreLog      = 6
	EventConnected    = 7
	EventDisconnected = 8
)

// ClientHandler implements libbox.CommandClientHandler.
// Each callback converts the libbox data to JSON and forwards it
// through the onEvent function.
type ClientHandler struct {
	onEvent func(eventType int32, jsonPayload string)
	mu      sync.Mutex

	startedAt int64 // set after successful service start

	// cached data for query APIs
	cachedGroups []ProxyGroup
	cachedStatus *TrafficSnapshot
	cachedLogs   []LogEntry
	cachedConns  any // map[string]any for connection events
}

// NewClientHandler creates a ClientHandler.
// Call SetOnEvent to register the callback.
func NewClientHandler() *ClientHandler {
	return &ClientHandler{}
}

// SetStartedAt records the service start timestamp for inclusion in traffic snapshots.
func (h *ClientHandler) SetStartedAt(ts int64) {
	h.mu.Lock()
	h.startedAt = ts
	h.mu.Unlock()
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
		if s, ok := payload.(TrafficSnapshot); ok {
			h.cachedStatus = &s
		}
	case EventLogs:
		if entries, ok := payload.([]LogEntry); ok {
			h.cachedLogs = entries
		}
	case EventConnections:
		h.cachedConns = payload
	case EventProxyUpdate:
		if groups, ok := payload.([]ProxyGroup); ok {
			h.cachedGroups = groups
		}
	}
	h.mu.Unlock()

	if fn != nil {
		fn(eventType, jsonStr)
	}
}

// GetCachedGroupsJSON returns the last cached proxy group JSON.
func (h *ClientHandler) GetCachedGroupsJSON() string {
	h.mu.Lock()
	groups := h.cachedGroups
	h.mu.Unlock()
	if groups == nil {
		return "[]"
	}
	data, _ := json.Marshal(groups)
	return string(data)
}

// GetCachedStatusJSON returns the last cached traffic status JSON.
func (h *ClientHandler) GetCachedStatusJSON() string {
	h.mu.Lock()
	s := h.cachedStatus
	h.mu.Unlock()
	if s == nil {
		return "{}"
	}
	data, _ := json.Marshal(s)
	return string(data)
}

// GetCachedLogs returns the last cached log entries.
func (h *ClientHandler) GetCachedLogs() []LogEntry {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cachedLogs
}

// GetCachedConnectionsJSON returns the last cached connection events JSON.
func (h *ClientHandler) GetCachedConnectionsJSON() string {
	h.mu.Lock()
	conns := h.cachedConns
	h.mu.Unlock()
	if conns == nil {
		return "[]"
	}
	data, _ := json.Marshal(conns)
	return string(data)
}

// UpdateDelay updates the cached delay for a specific proxy tag.
func (h *ClientHandler) UpdateDelay(tag string, delay int32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range h.cachedGroups {
		for j := range h.cachedGroups[i].Items {
			if h.cachedGroups[i].Items[j].Tag == tag {
				h.cachedGroups[i].Items[j].Delay = delay
			}
		}
	}
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
// Entries exceeding the current log level are filtered out.
func (h *ClientHandler) WriteLogs(messageList libbox.LogIterator) {
	maxLevel := GetLogLevel()
	var entries []LogEntry
	for messageList.HasNext() {
		entry := messageList.Next()
		if entry.Level > maxLevel {
			continue
		}
		entries = append(entries, LogEntry{
			Level:   entry.Level,
			Message: entry.Message,
		})
	}
	h.emit(EventLogs, entries)
}

// WriteStatus receives traffic and resource usage statistics.
func (h *ClientHandler) WriteStatus(message *libbox.StatusMessage) {
	h.mu.Lock()
	startedAt := h.startedAt
	h.mu.Unlock()
	snapshot := TrafficSnapshot{
		Up:         message.Uplink,
		Down:       message.Downlink,
		UpTotal:    message.UplinkTotal,
		DownTotal:  message.DownlinkTotal,
		Memory:     message.Memory,
		Goroutines: message.Goroutines,
		ConnsIn:    message.ConnectionsIn,
		ConnsOut:   message.ConnectionsOut,
		StartedAt:  startedAt,
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
// Ensure all three base modes are always present regardless of rule config.
func (h *ClientHandler) InitializeClashMode(modeList libbox.StringIterator, currentMode string) {
	var modes []string
	for modeList.HasNext() {
		modes = append(modes, modeList.Next())
	}
	modeSet := make(map[string]bool, len(modes))
	for _, m := range modes {
		modeSet[m] = true
	}
	for _, m := range []string{"Rule", "Global", "Direct"} {
		if !modeSet[m] {
			modes = append(modes, m)
		}
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
		snap := ConnectionEventSnapshot{
			EventType: evt.Type,
			ID:        evt.ID,
		}
		if conn := evt.Connection; conn != nil {
			snap.Inbound = conn.Inbound
			snap.InboundType = conn.InboundType
			snap.IPVersion = conn.IPVersion
			snap.Network = conn.Network
			snap.Source = conn.Source
			snap.Destination = conn.Destination
			snap.Domain = conn.Domain
			snap.Protocol = conn.Protocol
			snap.User = conn.User
			snap.FromOutbound = conn.FromOutbound
			snap.CreatedAt = conn.CreatedAt
			snap.Uplink = conn.Uplink
			snap.Downlink = conn.Downlink
			snap.UplinkTotal = conn.UplinkTotal
			snap.DownlinkTotal = conn.DownlinkTotal
			snap.Rule = conn.Rule
			snap.Outbound = conn.Outbound
			snap.OutboundType = conn.OutboundType
			chain := conn.Chain()
			for chain.HasNext() {
				snap.Chain = append(snap.Chain, chain.Next())
			}
			if pi := conn.ProcessInfo; pi != nil {
				snap.ProcessID = pi.ProcessID
				snap.UserID = pi.UserID
				snap.UserName = pi.UserName
				snap.ProcessPath = pi.ProcessPath
				pkgs := pi.PackageNames()
				for pkgs.HasNext() {
					snap.Packages = append(snap.Packages, pkgs.Next())
				}
			}
		}
		snap.ClosedAt = evt.ClosedAt
		snapshots = append(snapshots, snap)
	}
	h.emit(EventConnections, map[string]any{
		"reset": events.Reset,
		"items": snapshots,
	})
}
