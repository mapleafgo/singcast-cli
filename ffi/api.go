// Package ffi provides the singcast API for external consumers.
// Desktop c-shared builds use cmd/cffi; gomobile binds this package directly.
package ffi

import (
	"errors"

	"github.com/mapleafgo/singcast/core"
	_ "golang.org/x/mobile/bind" // retained for gomobile bind
)

var errNotInit = errors.New("core not initialized")

// EventHandler receives event callbacks from the core runtime.
// Implement this interface on the mobile side and pass it to SetOnEvent.
type EventHandler interface {
	OnEvent(eventType int, jsonPayload string)
}

// SocketProtector protects socket file descriptors from VPN routing.
// On Android, implement this to call VpnService.protect(fd).
// On iOS, this is not needed (NetworkExtension handles routing).
type SocketProtector interface {
	Protect(fd int32) bool
}

// Singcast is the primary API object.
type Singcast struct {
	eventHandler EventHandler
}

// New creates a new Singcast instance.
func New() *Singcast { return &Singcast{} }

// Init initializes the core runtime.
func (s *Singcast) Init(homeDir string) error {
	return core.Init(homeDir)
}

// Stop stops the running service.
func (s *Singcast) Stop() error {
	return core.Stop()
}

// Destroy shuts down and releases all resources.
func (s *Singcast) Destroy() {
	core.Destroy()
}

// CheckConfig validates a config string (Clash YAML or sing-box JSON).
func (s *Singcast) CheckConfig(content string) error {
	return core.CheckConfig(content)
}

// SelectProxy selects a proxy in the given group.
func (s *Singcast) SelectProxy(group, tag string) error {
	svc := core.GetService()
	if svc == nil {
		return errNotInit
	}
	return svc.SelectOutbound(group, tag)
}

// TestDelay runs a URL test for the given outbound tag.
func (s *Singcast) TestDelay(name string) error {
	svc := core.GetService()
	if svc == nil {
		return errNotInit
	}
	return svc.URLTest(name)
}

// SetMode sets the clash routing mode (rule, global, or direct).
func (s *Singcast) SetMode(mode string) error {
	svc := core.GetService()
	if svc == nil {
		return errNotInit
	}
	return svc.SetClashMode(mode)
}

// QueryProxies returns cached proxy groups as JSON.
func (s *Singcast) QueryProxies() string {
	svc := core.GetService()
	if svc == nil || svc.Handler() == nil {
		return "[]"
	}
	return svc.Handler().GetCachedGroupsJSON()
}

// QueryTraffic returns cached traffic stats as JSON.
func (s *Singcast) QueryTraffic() string {
	svc := core.GetService()
	if svc == nil || svc.Handler() == nil {
		return "{}"
	}
	return svc.Handler().GetCachedStatusJSON()
}

// QueryLogs returns combined sing-box and core internal logs as JSON.
func (s *Singcast) QueryLogs() string {
	return core.QueryLogs()
}

// QueryConnections returns cached connections as JSON.
func (s *Singcast) QueryConnections() string {
	svc := core.GetService()
	if svc == nil || svc.Handler() == nil {
		return "[]"
	}
	return svc.Handler().GetCachedConnectionsJSON()
}

// CloseConnection closes a connection by ID.
func (s *Singcast) CloseConnection(id string) error {
	svc := core.GetService()
	if svc == nil {
		return errNotInit
	}
	return svc.CloseConnection(id)
}

// CloseAllConnections closes all active connections.
func (s *Singcast) CloseAllConnections() error {
	svc := core.GetService()
	if svc == nil {
		return errNotInit
	}
	return svc.CloseConnections()
}

// Version returns version info as JSON.
func (s *Singcast) Version() string {
	return core.VersionJSON()
}

// SetOnEvent sets the event handler.
// Mobile apps should implement the EventHandler interface and pass it here.
func (s *Singcast) SetOnEvent(handler EventHandler) {
	s.eventHandler = handler
	core.SetOnEvent(func(eventType int, jsonPayload string) {
		if handler != nil {
			handler.OnEvent(eventType, jsonPayload)
		}
	})
}

// SetTunFd stores a TUN file descriptor for mobile platforms.
// Call after creating the TUN interface and before StartWithContent.
func (s *Singcast) SetTunFd(fd int32) {
	core.SetTunFd(fd)
}

// SetSocketProtector registers a socket protector for VPN bypass.
// On Android, pass an implementation that calls VpnService.protect(fd).
// Must be called before StartWithContent when VPN is active.
func (s *Singcast) SetSocketProtector(p SocketProtector) {
	core.SetSocketProtector(func(fd int32) bool {
		if p != nil {
			return p.Protect(fd)
		}
		return false
	})
}

// SetInterfacesJSON provides network interface data from the mobile platform.
// On Android, this is populated via ConnectivityManager since Go's net.Interfaces()
// fails with netlink permission denied.
func (s *Singcast) SetInterfacesJSON(json string) {
	core.SetInterfacesJSON(json)
}

// StartWithContent starts the service with raw YAML or JSON content.
func (s *Singcast) StartWithContent(content, ruleSetProxy string) error {
	return core.StartWithContent(content, ruleSetProxy)
}

// UpdateDefaultInterface reports the current default network interface
// detected by the mobile platform (e.g. via Android ConnectivityManager).
func (s *Singcast) UpdateDefaultInterface(name string, index int64, expensive bool) {
	core.UpdateDefaultInterface(name, index, expensive)
}
