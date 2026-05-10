package core

// Event type constants for the unified callback.
const (
	EventLog        int32 = 0
	EventURLTest    int32 = 1
	EventModeUpdate int32 = 2
	EventConnEvent  int32 = 3
)

// TrafficSnapshot holds traffic and resource usage statistics.
type TrafficSnapshot struct {
	Up         int64 `json:"up"`
	Down       int64 `json:"down"`
	UpTotal    int64 `json:"up_total"`
	DownTotal  int64 `json:"down_total"`
	Memory     int64 `json:"memory"`
	Goroutines int32 `json:"goroutines"`
	ConnsIn    int32 `json:"connections_in"`
	ConnsOut   int32 `json:"connections_out"`
	StartedAt  int64 `json:"started_at,omitempty"`
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

// ConnectionEventSnapshot represents a connection event for external consumers.
type ConnectionEventSnapshot struct {
	EventType int32  `json:"event_type"`
	ID        string `json:"id"`
	// Connection details (available for open/update events, empty for close).
	Inbound       string   `json:"inbound,omitempty"`
	InboundType   string   `json:"inbound_type,omitempty"`
	IPVersion     int32    `json:"ip_version,omitempty"`
	Network       string   `json:"network,omitempty"`
	Source        string   `json:"source,omitempty"`
	Destination   string   `json:"destination,omitempty"`
	Domain        string   `json:"domain,omitempty"`
	Protocol      string   `json:"protocol,omitempty"`
	User          string   `json:"user,omitempty"`
	FromOutbound  string   `json:"from_outbound,omitempty"`
	CreatedAt     int64    `json:"created_at,omitempty"`
	ClosedAt      int64    `json:"closed_at,omitempty"`
	Uplink        int64    `json:"uplink,omitempty"`
	Downlink      int64    `json:"downlink,omitempty"`
	UplinkTotal   int64    `json:"uplink_total,omitempty"`
	DownlinkTotal int64    `json:"downlink_total,omitempty"`
	Rule          string   `json:"rule,omitempty"`
	Outbound      string   `json:"outbound,omitempty"`
	OutboundType  string   `json:"outbound_type,omitempty"`
	Chain         []string `json:"chain,omitempty"`
	// Process info (only when process finding is enabled).
	ProcessID   int64    `json:"process_id,omitempty"`
	UserID      int32    `json:"user_id,omitempty"`
	UserName    string   `json:"user_name,omitempty"`
	ProcessPath string   `json:"process_path,omitempty"`
	Packages    []string `json:"packages,omitempty"`
}

// TunOptionsSnapshot is a JSON-serializable snapshot of TUN configuration.
type TunOptionsSnapshot struct {
	Inet4Address             []string `json:"inet4_address,omitempty"`
	Inet6Address             []string `json:"inet6_address,omitempty"`
	DNSServerAddress         string   `json:"dns_server_address,omitempty"`
	MTU                      int32    `json:"mtu"`
	AutoRoute                bool     `json:"auto_route"`
	StrictRoute              bool     `json:"strict_route"`
	Inet4RouteAddress        []string `json:"inet4_route_address,omitempty"`
	Inet6RouteAddress        []string `json:"inet6_route_address,omitempty"`
	Inet4RouteExcludeAddress []string `json:"inet4_route_exclude_address,omitempty"`
	Inet6RouteExcludeAddress []string `json:"inet6_route_exclude_address,omitempty"`
	Inet4RouteRange          []string `json:"inet4_route_range,omitempty"`
	Inet6RouteRange          []string `json:"inet6_route_range,omitempty"`
	IncludePackage           []string `json:"include_package,omitempty"`
	ExcludePackage           []string `json:"exclude_package,omitempty"`
	HTTPProxyEnabled         bool     `json:"http_proxy_enabled"`
	HTTPProxyServer          string   `json:"http_proxy_server,omitempty"`
	HTTPProxyServerPort      int32    `json:"http_proxy_server_port,omitempty"`
	HTTPProxyBypassDomain    []string `json:"http_proxy_bypass_domain,omitempty"`
	HTTPProxyMatchDomain     []string `json:"http_proxy_match_domain,omitempty"`
}

// OverrideConfig holds override options for VPN split tunneling.
type OverrideConfig struct {
	AutoRedirect    bool     `json:"auto_redirect,omitempty"`
	IncludePackages []string `json:"include_packages,omitempty"`
	ExcludePackages []string `json:"exclude_packages,omitempty"`
}

// InitOptions holds initialization parameters passed as JSON to Service.Init.
type InitOptions struct {
	HomeDir         string `json:"home_dir"`
	LogMaxLines     int    `json:"log_max_lines,omitempty"`
	Debug           bool   `json:"debug,omitempty"`
	FixAndroidStack bool   `json:"fix_android_stack,omitempty"`
}
