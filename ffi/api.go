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

// State returns the current service lifecycle state.
// 0=Created, 1=Initialized, 2=Running, 3=Destroyed.
func (s *Singcast) State() int32 { return int32(s.svc.State()) }

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

// QueryLogs returns combined sing-box and core internal logs as JSON.
func (s *Singcast) QueryLogs() string { return s.svc.QueryLogs() }

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

// QueryDNS performs a DNS query and returns the result as JSON.
func (s *Singcast) QueryDNS(name string, qType uint16) string { return s.svc.QueryDNS(name, qType) }

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

// URLTestUpdateListener is called when URL test delay history is updated.
type URLTestUpdateListener interface {
	OnURLTestUpdate()
}

// ModeUpdateListener is called when the clash routing mode changes.
type ModeUpdateListener interface {
	OnModeUpdate(mode string)
}

// ConnectionEventListener is called on connection create/update/close.
// eventType: 0=New, 1=Update, 2=Closed. connJSON is a connection object.
type ConnectionEventListener interface {
	OnConnectionEvent(eventType int32, connJSON string)
}

func (s *Singcast) SetOnURLTestUpdate(l URLTestUpdateListener) {
	if l == nil {
		s.svc.SetOnURLTestUpdate(nil)
	} else {
		s.svc.SetOnURLTestUpdate(func() { l.OnURLTestUpdate() })
	}
}

func (s *Singcast) SetOnModeUpdate(l ModeUpdateListener) {
	if l == nil {
		s.svc.SetOnModeUpdate(nil)
	} else {
		s.svc.SetOnModeUpdate(func(mode string) { l.OnModeUpdate(mode) })
	}
}

func (s *Singcast) SetOnConnEvent(l ConnectionEventListener) {
	if l == nil {
		s.svc.SetOnConnEvent(nil)
	} else {
		s.svc.SetOnConnEvent(func(eventType int32, connJSON string) {
			l.OnConnectionEvent(eventType, connJSON)
		})
	}
}

// SetCallbackFunc registers raw function callbacks. Used by desktop FFI.
func (s *Singcast) SetCallbackFuncs(onURLTest func(), onMode func(string), onConn func(int32, string)) {
	s.svc.SetOnURLTestUpdate(onURLTest)
	s.svc.SetOnModeUpdate(onMode)
	s.svc.SetOnConnEvent(onConn)
}

// --- Memory ---

// SetMemoryLimit sets a soft memory limit for Go runtime OOM protection.
// Set to 0 to disable.
func (s *Singcast) SetMemoryLimit(bytes int64) {
	runtimeDebug.SetMemoryLimit(bytes)
}

// Version returns version info as JSON.
func (s *Singcast) Version() string { return core.VersionJSON() }
