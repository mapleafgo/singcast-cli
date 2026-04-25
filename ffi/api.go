// Package ffi provides the singcast API for external consumers.
// Desktop c-shared builds use cmd/cffi; gomobile binds this package directly.
package ffi

import (
	"encoding/json"
	"errors"

	"github.com/mapleafgo/singcast/core"
	"github.com/mapleafgo/singcast/translator"
	_ "golang.org/x/mobile/bind" // retained for gomobile bind
)

var errNotInit = errors.New("core not initialized")

// Singcast is the primary API object.
type Singcast struct{}

// New creates a new Singcast instance.
func New() *Singcast { return &Singcast{} }

func mustMarshal(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// Init initializes the core runtime.
func (s *Singcast) Init(homeDir string) error {
	return core.Init(homeDir)
}

// Start starts the proxy service.
func (s *Singcast) Start(configPath, ruleSetProxy string) error {
	return core.Start(configPath, ruleSetProxy)
}

// Stop stops the running service.
func (s *Singcast) Stop() error {
	return core.Stop()
}

// Close shuts down and releases all resources.
func (s *Singcast) Close() {
	core.Close()
}

// ReloadConfig reloads the config from the last used path.
func (s *Singcast) ReloadConfig() error {
	return core.ReloadConfig()
}

// CheckConfig validates a sing-box JSON config string.
func (s *Singcast) CheckConfig(jsonContent string) error {
	return core.CheckConfig(jsonContent)
}

// TranslateConfig translates a Mihomo YAML config to sing-box JSON.
// Returns a JSON string with "config" and "warnings" fields, or "error".
func (s *Singcast) TranslateConfig(yamlContent, ruleSetProxy string) string {
	opts := &translator.Options{RuleSetURLPrefix: ruleSetProxy}
	jsonStr, warnings, err := translator.TranslateWithOptions([]byte(yamlContent), opts)
	if err != nil {
		return mustMarshal(map[string]string{"error": err.Error()})
	}
	return mustMarshal(map[string]any{
		"config":   jsonStr,
		"warnings": warnings,
	})
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

// QueryLogs returns cached log entries as JSON.
func (s *Singcast) QueryLogs() string {
	svc := core.GetService()
	if svc == nil || svc.Handler() == nil {
		return "[]"
	}
	return svc.Handler().GetCachedLogsJSON()
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

// SetOnEvent sets the event callback.
func (s *Singcast) SetOnEvent(fn func(eventType int, jsonPayload string)) {
	core.SetOnEvent(fn)
}

// SetTunFd stores a TUN file descriptor for mobile platforms.
// Call after creating the TUN interface and before StartWithContent.
func (s *Singcast) SetTunFd(fd int32) {
	core.SetTunFd(fd)
}

// StartWithContent starts the service with raw YAML or JSON content.
func (s *Singcast) StartWithContent(content, ruleSetProxy string) error {
	return core.StartWithContent(content, ruleSetProxy)
}
