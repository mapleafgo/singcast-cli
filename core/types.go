package core

// Event type constants for the unified callback.
const (
	EventLog         int32 = 0
	EventURLTest     int32 = 1
	EventModeUpdate  int32 = 2
	EventConnEvent   int32 = 3
	EventStateChange int32 = 4
	EventStats       int32 = 5
)

// StatsSnapshot holds traffic and resource usage statistics for QueryStats output.
type StatsSnapshot struct {
	Up          int64  `json:"up"`
	Down        int64  `json:"down"`
	Connections int    `json:"connections"`
	Memory      uint64 `json:"memory"`
	StartedAt   int64  `json:"started_at"`
}

// ProxyGroup represents a proxy group with its items.
type ProxyGroup struct {
	Tag        string           `json:"tag"`
	Type       string           `json:"type"`
	Selectable bool             `json:"selectable"`
	Selected   string           `json:"selected"`
	Expand     bool             `json:"expand,omitempty"`
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
	Level     int32  `json:"level"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

// InitOptions holds initialization parameters passed as JSON to Service.Init.
type InitOptions struct {
	HomeDir string `json:"home_dir"`
	Debug   bool   `json:"debug,omitempty"`
}

// ModeInfo is the JSON response for QueryMode.
type ModeInfo struct {
	Modes       []string `json:"modes"`
	CurrentMode string   `json:"current_mode"`
}

// RulesInfo is the JSON response for QueryRules.
type RulesInfo struct {
	Rules []ruleEntry `json:"rules"`
}

// VersionInfo is the JSON response for VersionJSON.
type VersionInfo struct {
	Version string `json:"version"`
	Core    string `json:"core"`
}

type connEntry struct {
	Event       int32  `json:"event"`
	ID          string `json:"id"`
	Network     string `json:"network"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Domain      string `json:"domain,omitempty"`
	Outbound    string `json:"outbound"`
	Rule        string `json:"rule,omitempty"`
	Upload      int64  `json:"upload"`
	Download    int64  `json:"download"`
	Start       string `json:"start"`
}

type ruleEntry struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
	Proxy   string `json:"proxy"`
}
