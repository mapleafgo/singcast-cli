// Package ffi provides the singcast API for mobile platforms via gomobile.
// Desktop c-shared builds use cmd/lib; gomobile binds this package directly.
package ffi

import (
	runtimeDebug "runtime/debug"

	"github.com/mapleafgo/singcast/core"
	"github.com/sagernet/sing-box/experimental/libbox"
	_ "golang.org/x/mobile/bind" // retained for gomobile bind
)

// EventHandler receives event callbacks from the core runtime.
type EventHandler interface {
	OnEvent(eventType int32, jsonPayload string)
}

// SocketProtector protects socket file descriptors from VPN routing.
// On Android, implement this to call VpnService.protect(fd).
type SocketProtector interface {
	Protect(fd int32) bool
}

// Singcast is the primary API object for mobile consumers.
//
// Lifecycle:
//
//	New() → Init(homeDir) → [SetTunFd/SetSocketProtector/…] → StartWithContent → running
//	   ↑                                                                     │
//	   └── Stop ←────────────────────────────────────────────────────────────┘
//	Destroy is terminal; call it when the OS reclaims the VPN service.
type Singcast struct {
	svc *core.Service
}

// New creates a new Singcast instance.
func New() *Singcast {
	return &Singcast{svc: core.NewService()}
}

// Init initializes the core runtime.
// optionsJSON: {"home_dir":"/path","log_max_lines":500,"debug":false,"fix_android_stack":false}
func (s *Singcast) Init(optionsJSON string) error {
	return s.svc.Init(optionsJSON)
}

// StartWithContent starts or restarts the service with raw YAML or JSON config.
func (s *Singcast) StartWithContent(content, ruleSetProxy string) error {
	return s.svc.StartWithContent(content, ruleSetProxy)
}

// Stop stops the running service. Safe to call multiple times.
func (s *Singcast) Stop() error {
	return s.svc.Stop()
}

// Destroy releases all resources. The instance cannot be reused.
func (s *Singcast) Destroy() {
	s.svc.Destroy()
}

// --- Platform IO ---

// SetTunFd stores a TUN file descriptor for mobile platforms.
func (s *Singcast) SetTunFd(fd int32) {
	s.svc.PlatformIO().SetTunFd(fd)
}

// SetSocketProtector registers a socket protector for VPN bypass.
func (s *Singcast) SetSocketProtector(p SocketProtector) {
	s.svc.PlatformIO().SetSocketProtector(func(fd int32) bool {
		if p != nil {
			return p.Protect(fd)
		}
		return false
	})
}

// SetInterfacesJSON provides network interface data from the mobile platform.
func (s *Singcast) SetInterfacesJSON(json string) {
	s.svc.PlatformIO().SetInterfacesJSON(json)
}

// UpdateDefaultInterface reports the current default network interface.
func (s *Singcast) UpdateDefaultInterface(name string, index int64, expensive bool) {
	s.svc.PlatformIO().UpdateDefaultInterface(name, index, expensive)
}

// SetIncludeAllNetworks sets whether the VPN configuration uses includeAllNetworks (iOS).
func (s *Singcast) SetIncludeAllNetworks(v bool) {
	s.svc.PlatformIO().SetIncludeAllNetworks(v)
}

// SetWIFIState reports the current WiFi SSID and BSSID from the mobile platform.
func (s *Singcast) SetWIFIState(ssid, bssid string) {
	s.svc.PlatformIO().SetWIFIState(ssid, bssid)
}

// QueryTunOptions returns TUN configuration as JSON for mobile consumers.
// Call after the service has started (OpenTun has been invoked by sing-box).
func (s *Singcast) QueryTunOptions() string {
	return s.svc.PlatformIO().QueryTunOptions()
}

// --- Logging ---

// SetLogLevel sets the minimum log level (2=Error, 3=Warn, 4=Info, 5=Debug, 6=Trace).
func (s *Singcast) SetLogLevel(level int32) { s.svc.SetLogLevel(level) }

// SetError pushes an error message to connected clients.
func (s *Singcast) SetError(message string) { s.svc.SetError(message) }

// --- Pause / Wake / Network ---

// Pause suspends network activity. On iOS, auto-wakes after 1 minute.
func (s *Singcast) Pause() { s.svc.Pause() }

// Wake resumes network activity after Pause.
func (s *Singcast) Wake() { s.svc.Wake() }

// ResetNetwork resets all connections, DNS cache, and forces outbounds to reconnect.
func (s *Singcast) ResetNetwork() { s.svc.ResetNetwork() }

// --- Config ---

// ReloadConfig reloads the service with new configuration.
func (s *Singcast) ReloadConfig(content, ruleSetProxy string) error {
	return s.svc.ReloadConfig(content, ruleSetProxy)
}

// ReloadTUN restarts the TUN interface without changing configuration.
func (s *Singcast) ReloadTUN() error { return s.svc.ReloadTUN() }

// SetOverridePackages updates the include/exclude package lists for VPN split tunneling.
// The service restarts with the current config and new package overrides.
// Pass empty string to clear all overrides.
func (s *Singcast) SetOverridePackages(overrideJSON string) error {
	return s.svc.SetOverridePackages(overrideJSON)
}

// CheckConfig validates a config string (Clash YAML or sing-box JSON).
func (s *Singcast) CheckConfig(content string) error { return core.CheckConfig(content) }

// --- Proxy Control ---

// SelectProxy selects a proxy in the given group.
func (s *Singcast) SelectProxy(group, tag string) error {
	return s.svc.SelectOutbound(group, tag)
}

// TestDelay runs a URL test for the given outbound tag.
func (s *Singcast) TestDelay(name string) error { return s.svc.URLTest(name) }

// SetMode sets the clash routing mode (rule, global, or direct).
func (s *Singcast) SetMode(mode string) error { return s.svc.SetClashMode(mode) }

// SetGroupExpand sets the UI expand state for a proxy group.
func (s *Singcast) SetGroupExpand(group string, expand bool) error {
	return s.svc.SetGroupExpand(group, expand)
}

// --- Queries ---

// QueryProxies returns cached proxy groups as JSON.
func (s *Singcast) QueryProxies() string {
	h := s.svc.Handler()
	if h == nil {
		return "[]"
	}
	return h.GetCachedGroupsJSON()
}

// QueryTraffic returns cached traffic stats as JSON.
func (s *Singcast) QueryTraffic() string {
	h := s.svc.Handler()
	if h == nil {
		return "{}"
	}
	return h.GetCachedStatusJSON()
}

// QueryLogs returns combined sing-box and core internal logs as JSON.
// If clear is true, the log buffer is cleared after querying.
func (s *Singcast) QueryLogs(clear bool) string {
	result := s.svc.QueryLogs()
	if clear {
		_ = s.svc.ClearLogs()
	}
	return result
}

// QueryConnections returns cached connections as JSON.
func (s *Singcast) QueryConnections() string {
	h := s.svc.Handler()
	if h == nil {
		return "[]"
	}
	return h.GetCachedConnectionsJSON()
}

// CloseConnection closes a connection by ID.
func (s *Singcast) CloseConnection(id string) error { return s.svc.CloseConnection(id) }

// CloseAllConnections closes all active connections.
func (s *Singcast) CloseAllConnections() error { return s.svc.CloseConnections() }

// --- Platform Queries ---

// NeedWIFIState reports whether the current config requires WIFI state monitoring.
func (s *Singcast) NeedWIFIState() bool { return s.svc.NeedWIFIState() }

// NeedFindProcess reports whether the current config requires process finding.
func (s *Singcast) NeedFindProcess() bool { return s.svc.NeedFindProcess() }

// UpdateWIFIState triggers the platform to report current WIFI state.
func (s *Singcast) UpdateWIFIState() { s.svc.UpdateWIFIState() }

// WriteMessage writes a log message to the core at the given level.
func (s *Singcast) WriteMessage(level int32, message string) {
	s.svc.WriteMessage(level, message)
}

// FlushSystemDNS attempts to flush the system DNS cache.
func (s *Singcast) FlushSystemDNS() { s.svc.FlushSystemDNS() }

// QueryMemoryStats returns current Go runtime memory statistics as JSON.
func (s *Singcast) QueryMemoryStats() string { return s.svc.QueryMemoryStats() }

// --- Memory ---

// SetMemoryLimit sets a soft memory limit for Go runtime OOM protection.
// Set to 0 to disable.
func (s *Singcast) SetMemoryLimit(bytes int64) {
	runtimeDebug.SetMemoryLimit(bytes)
}

// Version returns version info as JSON.
func (s *Singcast) Version() string { return core.VersionJSON() }

// --- Events ---

// SetOnEvent registers an event handler. Call after Init.
func (s *Singcast) SetOnEvent(handler EventHandler) {
	s.svc.SetOnEvent(func(eventType int32, jsonPayload string) {
		if handler != nil {
			handler.OnEvent(eventType, jsonPayload)
		}
	})
}

// --- Package-level Utilities ---

// SetLocale sets the locale for sing-box internal error messages.
func SetLocale(localeID string) { libbox.SetLocale(localeID) }
