// Package ffi provides the singcast API for mobile platforms via gomobile.
// Desktop c-shared builds use cmd/lib; gomobile binds this package directly.
package ffi

import (
	runtimeDebug "runtime/debug"

	"github.com/mapleafgo/singcast/core"
	_ "golang.org/x/mobile/bind" // retained for gomobile bind
)

// SocketProtector protects socket file descriptors from VPN routing.
// On Android, implement this to call VpnService.protect(fd).
type SocketProtector interface {
	Protect(fd int32) bool
}

// Singcast is the primary API object for mobile consumers.
//
// Lifecycle:
//
//	Create() → Init(homeDir) → [SetTunFd/SetSocketProtector/…] → StartWithContent → running
//	   ↑                                                                     │
//	   └── Stop ←────────────────────────────────────────────────────────────┘
//	Destroy is terminal; call it when the OS reclaims the VPN service.
type Singcast struct {
	svc *core.Service
}

// Create creates a new Singcast instance.
func Create() *Singcast {
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
func (s *Singcast) Stop() error { return s.svc.Stop() }

// Destroy releases all resources. The instance cannot be reused.
func (s *Singcast) Destroy() { s.svc.Destroy() }

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

// SetIncludeAllNetworks sets whether the VPN uses includeAllNetworks (iOS).
func (s *Singcast) SetIncludeAllNetworks(v bool) {
	s.svc.PlatformIO().SetIncludeAllNetworks(v)
}

// SetWIFIState reports the current WiFi SSID and BSSID from the mobile platform.
func (s *Singcast) SetWIFIState(ssid, bssid string) {
	s.svc.PlatformIO().SetWIFIState(ssid, bssid)
}

// --- Logging ---

// SetLogLevel sets the minimum log level (2=Error, 3=Warn, 4=Info, 5=Debug, 6=Trace).
func (s *Singcast) SetLogLevel(level int32) { s.svc.SetLogLevel(level) }

// --- Network ---

// ResetNetwork resets all connections, DNS cache, and forces outbounds to reconnect.
func (s *Singcast) ResetNetwork() { s.svc.ResetNetwork() }

// --- Config ---

// ReloadTUN restarts the TUN interface without changing configuration.
func (s *Singcast) ReloadTUN() error { return s.svc.ReloadTUN() }

// SetOverridePackages updates the include/exclude package lists for VPN split tunneling.
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
// Returns delay in milliseconds, -1 on error or timeout.
func (s *Singcast) TestDelay(name string, timeoutMs int32) int32 {
	return s.svc.URLTest(name, timeoutMs)
}

// SetMode sets the clash routing mode (rule, global, or direct).
func (s *Singcast) SetMode(mode string) error { return s.svc.SetClashMode(mode) }

// SetGroupExpand sets the UI expand state for a proxy group.
func (s *Singcast) SetGroupExpand(group string, expand bool) error {
	return s.svc.SetGroupExpand(group, expand)
}

// --- Queries ---

// QueryProxies returns proxy groups as JSON.
func (s *Singcast) QueryProxies() string { return s.svc.QueryProxies() }

// QueryTraffic returns traffic stats as JSON.
func (s *Singcast) QueryTraffic() string { return s.svc.QueryTraffic() }

// QueryLogs returns combined sing-box and core internal logs as JSON.
func (s *Singcast) QueryLogs() string { return s.svc.QueryLogs() }

// QueryConnections returns active connections as JSON.
func (s *Singcast) QueryConnections() string { return s.svc.QueryConnections() }

// CloseConnection closes a connection by ID.
func (s *Singcast) CloseConnection(id string) error { return s.svc.CloseConnection(id) }

// CloseAllConnections closes all active connections.
func (s *Singcast) CloseAllConnections() error { return s.svc.CloseConnections() }

// --- System ---

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
