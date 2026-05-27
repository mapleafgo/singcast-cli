// Package mobile provides the singcast API for mobile platforms via gomobile.
package mobile

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
	svc := core.NewService()
	svc.PlatformIO().SetMobile(true)
	return &Singcast{svc: svc}
}

// Init initializes the core runtime.
// optionsJSON: {"home_dir":"/path","debug":false}
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

// State returns the current service lifecycle state as a string.
// "created", "initialized", "starting", "running", "destroyed".
func (s *Singcast) State() string { return s.svc.State().String() }

// --- Platform IO ---

// SetTunFd stores a TUN file descriptor for mobile platforms.
func (s *Singcast) SetTunFd(fd int32) {
	s.svc.PlatformIO().SetTunFd(fd)
}

// SetSocketProtector registers a socket protector for VPN bypass.
// Pass nil to clear the protector (e.g. when VPN disconnects).
func (s *Singcast) SetSocketProtector(p SocketProtector) {
	if p == nil {
		s.svc.PlatformIO().SetSocketProtector(nil)
		return
	}
	s.svc.PlatformIO().SetSocketProtector(func(fd int32) bool {
		return p.Protect(fd)
	})
}

// InterfaceProvider provides network interface data on demand.
// GetInterfaces must return a JSON array of interface objects.
type InterfaceProvider interface {
	GetInterfaces() string
}

// WiFiStateProvider provides WiFi state on demand.
// GetWiFiState must return {"ssid":"...","bssid":"..."}.
type WiFiStateProvider interface {
	GetWiFiState() string
}

func (s *Singcast) SetInterfaceProvider(p InterfaceProvider) {
	if p == nil {
		s.svc.PlatformIO().SetInterfaceProvider(nil)
		return
	}
	s.svc.PlatformIO().SetInterfaceProvider(func() string {
		return p.GetInterfaces()
	})
}

func (s *Singcast) SetWiFiStateProvider(p WiFiStateProvider) {
	if p == nil {
		s.svc.PlatformIO().SetWiFiStateProvider(nil)
		return
	}
	s.svc.PlatformIO().SetWiFiStateProvider(func() string {
		return p.GetWiFiState()
	})
}

// UpdateDefaultInterface reports the current default network interface.
func (s *Singcast) UpdateDefaultInterface(name string, index int64, expensive bool) {
	s.svc.PlatformIO().UpdateDefaultInterface(name, index, expensive)
}

// SetIncludeAllNetworks sets whether the VPN uses includeAllNetworks (iOS).
func (s *Singcast) SetIncludeAllNetworks(v bool) {
	s.svc.PlatformIO().SetIncludeAllNetworks(v)
}

// --- Logging ---

// SetLogLevel sets the minimum log level (2=Error, 3=Warn, 4=Info, 5=Debug, 6=Trace).
func (s *Singcast) SetLogLevel(level int32) { s.svc.SetLogLevel(level) }

// --- Network ---

// ResetNetwork resets all connections, DNS cache, and forces outbounds to reconnect.
func (s *Singcast) ResetNetwork() { s.svc.ResetNetwork() }

// --- Config ---

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
func (s *Singcast) SetMode(mode string) error { return s.svc.SetMode(mode) }

// SetGroupExpand sets the UI expand state for a proxy group.
func (s *Singcast) SetGroupExpand(group string, expand bool) error {
	return s.svc.SetGroupExpand(group, expand)
}

// --- Queries ---

// QueryProxies returns proxy groups as JSON.
func (s *Singcast) QueryProxies() string { return s.svc.QueryProxies() }

// QueryStats returns traffic and memory stats as JSON.
func (s *Singcast) QueryStats() string { return s.svc.QueryStats() }

// QueryConnections returns active connections as JSON.
func (s *Singcast) QueryConnections() string { return s.svc.QueryConnections() }

// QueryMode returns current routing mode and available modes as JSON.
func (s *Singcast) QueryMode() string { return s.svc.QueryMode() }

// CloseConnection closes a connection by ID.
func (s *Singcast) CloseConnection(id string) error { return s.svc.CloseConnection(id) }

// CloseAllConnections closes all active connections.
func (s *Singcast) CloseAllConnections() error { return s.svc.CloseConnections() }

// --- System ---

// FlushSystemDNS attempts to flush the system DNS cache.
func (s *Singcast) FlushSystemDNS() { s.svc.FlushSystemDNS() }

// --- Rules / DNS / Cache ---

// QueryRules returns the routing rule list as JSON.
func (s *Singcast) QueryRules() string { return s.svc.QueryRules() }

// FlushFakeIP clears the FakeIP address cache.
func (s *Singcast) FlushFakeIP() error { return s.svc.FlushFakeIP() }

// FlushDNSCache clears the internal DNS query cache.
func (s *Singcast) FlushDNSCache() error { return s.svc.FlushDNSCache() }

// TestGroupDelay runs URL tests for all outbounds in a group.
// Returns a JSON map of {tag: delay_ms}. -1 means failure/timeout.
func (s *Singcast) TestGroupDelay(groupTag string, timeoutMs int32) string {
	return s.svc.TestGroupDelay(groupTag, timeoutMs)
}

// TriggerGC forces a garbage collection.
func (s *Singcast) TriggerGC() { s.svc.TriggerGC() }

// --- Event Listeners ---

// EventListener is the unified callback interface for all core events.
// eventType: 0=Log, 1=URLTest, 2=ModeUpdate, 3=ConnEvent, 4=StateChange, 5=Stats.
type EventListener interface {
	OnEvent(eventType int32, json string)
}

func (s *Singcast) SetOnEvent(l EventListener) {
	var fn func(int32, string)
	if l != nil {
		fn = func(eventType int32, json string) { l.OnEvent(eventType, json) }
	}
	s.svc.SetOnEvent(fn)
	core.SetOnLogEvent(fn)
}

// --- Memory ---

// SetMemoryLimit sets a soft memory limit for Go runtime OOM protection.
// Set to 0 to disable.
func (s *Singcast) SetMemoryLimit(bytes int64) {
	runtimeDebug.SetMemoryLimit(bytes)
}

// Version returns version info as JSON.
func (s *Singcast) Version() string { return core.VersionJSON() }
