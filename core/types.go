package core

// TrafficSnapshot holds traffic and resource usage statistics.
type TrafficSnapshot struct {
	Up          int64 `json:"up"`
	Down        int64 `json:"down"`
	UpTotal     int64 `json:"up_total"`
	DownTotal   int64 `json:"down_total"`
	Memory      int64 `json:"memory"`
	Goroutines  int32 `json:"goroutines"`
	ConnsIn     int32 `json:"connections_in"`
	ConnsOut    int32 `json:"connections_out"`
}

// ProxyGroup represents a proxy group with its items.
type ProxyGroup struct {
	Tag        string           `json:"tag"`
	Type       string           `json:"type"`
	Selectable bool             `json:"selectable"`
	Selected   string           `json:"selected"`
	Items      []ProxyGroupItem `json:"items,omitempty"`
}

// ProxyGroupItem represents a single proxy within a group.
type ProxyGroupItem struct {
	Tag   string `json:"tag"`
	Type  string `json:"type"`
	Delay int32  `json:"delay,omitempty"`
}

// LogEntry represents a single log message.
type LogEntry struct {
	Level   int32  `json:"level"`
	Message string `json:"message"`
}

// ConnectionEventSnapshot represents a connection event for external consumers.
type ConnectionEventSnapshot struct {
	EventType int32  `json:"event_type"`
	ID        string `json:"id"`
}
