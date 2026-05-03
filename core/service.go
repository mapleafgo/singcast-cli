package core

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/sagernet/sing-box/experimental/libbox"
	_ "github.com/sagernet/sing-box/include" // register all protocols

	"github.com/mapleafgo/singcast/translator"
)

// Version is set via -ldflags at build time.
var Version = "dev"

// VersionJSON returns version info as JSON.
func VersionJSON() string {
	data, _ := json.Marshal(map[string]string{
		"version": Version,
		"core":    "sing-box",
	})
	return string(data)
}

// CheckConfig validates a config string (Clash YAML or sing-box JSON).
func CheckConfig(content string) error {
	data := []byte(content)
	if translator.DetectFormat(data) == translator.FormatYAML {
		result, _, err := translator.TranslateWithOptions(data, nil)
		if err != nil {
			return fmt.Errorf("translate config: %w", err)
		}
		return libbox.CheckConfig(result)
	}
	return libbox.CheckConfig(content)
}

// FormatBytes formats a byte count as a human-readable string (KB).
func FormatBytes(length int64) string { return libbox.FormatBytes(length) }

// FormatDuration formats a millisecond duration as a human-readable string.
func FormatDuration(duration int64) string { return libbox.FormatDuration(duration) }

// AvailablePort finds the next available TCP port starting from startPort.
func AvailablePort(startPort int32) (int32, error) { return libbox.AvailablePort(startPort) }

// ForceGC triggers a manual garbage collection and returns memory to the OS.
func ForceGC() {
	runtime.GC()
	debug.FreeOSMemory()
}

// SetMemoryLimit sets a soft memory limit for the Go runtime (OOM protection).
// Set to 0 to disable. This is a package-level convenience without monitoring.
func SetMemoryLimit(bytes int64) { debug.SetMemoryLimit(bytes) }

// SetMemoryLimitWithMonitor sets a soft memory limit and enables adaptive OOM monitoring.
// When the limit is breached, the monitor triggers ResetNetwork + GC asynchronously.
// Set to 0 to disable monitoring and restore the default memory limit.
// No-op if the service is destroyed.
func (s *Service) SetMemoryLimitWithMonitor(bytes int64) {
	s.mu.Lock()
	if s.state == StateDestroyed {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	if bytes <= 0 {
		debug.SetMemoryLimit(0)
	} else {
		debug.SetMemoryLimit(bytes)
	}
	s.oom.setLimit(bytes)
}

// State represents the lifecycle state of a Service.
type State int32

const (
	StateCreated     State = iota // NewService() returns this
	StateInitialized              // Init() succeeded
	StateRunning                  // StartWithContent() succeeded
	StateDestroyed                // Destroy() called, terminal
)

func (s State) String() string {
	switch s {
	case StateCreated:
		return "created"
	case StateInitialized:
		return "initialized"
	case StateRunning:
		return "running"
	case StateDestroyed:
		return "destroyed"
	default:
		return "unknown"
	}
}

// libboxSetupMu guards the process-wide libbox.Setup call.
var libboxSetupMu sync.Mutex

// libboxReady is true after a successful libbox.Setup.
var libboxReady bool

func resetLibboxForTesting() {
	libboxSetupMu.Lock()
	libboxReady = false
	libboxSetupMu.Unlock()
}

// Service manages the sing-box service lifecycle.
//
// Lifecycle:
//
//	NewService → Init → [SetTunFd, SetSocketProtector, …] → StartWithContent → running
//	  ↑                                                              │
//	  └── Stop ←─────────────────────────────────────────────────────┘
//	Destroy is terminal; the instance cannot be reused.
type Service struct {
	mu            sync.Mutex
	state         State
	commandServer *libbox.CommandServer
	commandClient *libbox.CommandClient
	handler       *ClientHandler
	platformIO    *PlatformIO
	currentConfig string // translated sing-box JSON (kept for ReloadTUN)
	oom           *oomMonitor
}

// NewService creates a Service in StateCreated.
func NewService() *Service {
	s := &Service{platformIO: NewPlatformIO()}
	s.oom = newOOMMonitor(func() { s.ResetNetwork() })
	return s
}

// State returns the current lifecycle state (thread-safe).
func (s *Service) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Handler returns the client handler.
func (s *Service) Handler() *ClientHandler { return s.handler }

// PlatformIO returns the platform IO handler.
func (s *Service) PlatformIO() *PlatformIO { return s.platformIO }

// Init initializes the libbox runtime and command server.
// Must be called once after NewService.
func (s *Service) Init(homeDir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateCreated {
		return fmt.Errorf("init: invalid state %s", s.state)
	}

	slog.Info("service init", "homeDir", homeDir)

	tempDir := filepath.Join(homeDir, "temp")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}

	libboxSetupMu.Lock()
	if !libboxReady {
		if err := libbox.Setup(&libbox.SetupOptions{
			BasePath:    homeDir,
			WorkingPath: homeDir,
			TempPath:    tempDir,
		}); err != nil {
			libboxSetupMu.Unlock()
			return fmt.Errorf("libbox setup: %w", err)
		}
		libboxReady = true
	}
	libboxSetupMu.Unlock()

	handler := NewClientHandler()
	commandServer, err := libbox.NewCommandServer(defaultServerHandler, s.platformIO)
	if err != nil {
		return fmt.Errorf("create command server: %w", err)
	}
	if err := commandServer.Start(); err != nil {
		return fmt.Errorf("start command server: %w", err)
	}

	s.commandServer = commandServer
	s.handler = handler
	s.state = StateInitialized

	slog.Info("service init done")
	return nil
}

// StartWithContent starts or restarts the service with raw YAML or JSON config.
func (s *Service) StartWithContent(content, ruleSetProxy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateInitialized && s.state != StateRunning {
		return fmt.Errorf("start: invalid state %s", s.state)
	}
	if s.commandServer == nil {
		return fmt.Errorf("start: service closed")
	}

	return s.startWithContent(content, ruleSetProxy)
}

func (s *Service) startWithContent(content, ruleSetProxy string) error {
	slog.Info("startWithContent", "bytes", len(content), "proxy", ruleSetProxy, "state", s.state)

	data := []byte(content)
	jsonContent, err := s.translateConfig(data, translator.DetectFormat(data), ruleSetProxy)
	if err != nil {
		return err
	}

	return s.startWithJSON(jsonContent)
}

func (s *Service) translateConfig(data []byte, format translator.Format, ruleSetProxy string) (string, error) {
	if format == translator.FormatYAML {
		opts := &translator.Options{RuleSetURLPrefix: ruleSetProxy}
		result, _, err := translator.TranslateWithOptions(data, opts)
		if err != nil {
			return "", fmt.Errorf("translate config: %w", err)
		}
		return result, nil
	}
	return string(data), nil
}

// startWithJSON feeds translated JSON config to libbox and reconnects
// the command client. Caller must hold s.mu.
func (s *Service) startWithJSON(jsonContent string) error {
	slog.Info("starting/reloading service", "bytes", len(jsonContent))

	s.currentConfig = jsonContent

	// Disconnect old client BEFORE starting new service to avoid stale callbacks.
	if s.commandClient != nil {
		s.commandClient.Disconnect()
		s.commandClient = nil
	}

	if err := s.commandServer.StartOrReloadService(jsonContent, &libbox.OverrideOptions{}); err != nil {
		slog.Error("StartOrReloadService failed", "error", err)
		s.state = StateInitialized
		return fmt.Errorf("start service: %w", err)
	}

	opts := &libbox.CommandClientOptions{StatusInterval: int64(time.Second)}
	opts.AddCommand(libbox.CommandLog)
	opts.AddCommand(libbox.CommandStatus)
	opts.AddCommand(libbox.CommandGroup)
	opts.AddCommand(libbox.CommandConnections)

	newClient := libbox.NewCommandClient(s.handler, opts)
	if err := newClient.Connect(); err != nil {
		slog.Error("command client Connect failed", "error", err)
		s.commandServer.CloseService()
		s.state = StateInitialized
		return fmt.Errorf("connect command client: %w", err)
	}

	s.commandClient = newClient
	s.state = StateRunning
	slog.Info("service running")
	return nil
}

// Stop stops the running service. No-op if not running.
func (s *Service) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateRunning {
		return nil
	}

	slog.Info("stopping service")
	s.platformIO.ResetTunFd()

	if s.commandClient != nil {
		s.commandClient.Disconnect()
		s.commandClient = nil
	}

	err := s.commandServer.CloseService()
	if err != nil {
		slog.Error("stop: CloseService error", "error", err)
	} else {
		slog.Info("service stopped")
	}

	s.state = StateInitialized
	return err
}

// Destroy releases all resources. The instance cannot be reused.
func (s *Service) Destroy() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateDestroyed {
		return
	}

	slog.Info("destroying service")

	s.oom.stop()

	if s.state == StateRunning {
		s.platformIO.ResetTunFd()
		s.commandServer.CloseService()
	}

	if s.commandClient != nil {
		s.commandClient.Disconnect()
		s.commandClient = nil
	}

	if s.commandServer != nil {
		s.commandServer.Close()
		s.commandServer = nil
	}

	s.state = StateDestroyed
	slog.Info("service destroyed")
}

// ReloadConfig reloads the service with new configuration content.
func (s *Service) ReloadConfig(content, ruleSetProxy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateRunning {
		return fmt.Errorf("reload config: invalid state %s", s.state)
	}
	return s.startWithContent(content, ruleSetProxy)
}

// ReloadTUN restarts the TUN interface without changing configuration.
func (s *Service) ReloadTUN() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateRunning {
		return fmt.Errorf("reload TUN: invalid state %s", s.state)
	}
	if s.currentConfig == "" {
		return fmt.Errorf("reload TUN: no configuration stored")
	}

	slog.Info("reloading TUN with existing config", "configBytes", len(s.currentConfig))
	return s.startWithJSON(s.currentConfig)
}

// withServer executes fn with the command server under lock protection.
func (s *Service) withServer(fn func(*libbox.CommandServer)) {
	s.mu.Lock()
	srv := s.commandServer
	s.mu.Unlock()
	if srv != nil {
		fn(srv)
	}
}

// Pause suspends network activity. On iOS, auto-wakes after 1 minute.
func (s *Service) Pause() { s.withServer(func(srv *libbox.CommandServer) { srv.Pause() }) }

// Wake resumes network activity after Pause.
func (s *Service) Wake() { s.withServer(func(srv *libbox.CommandServer) { srv.Wake() }) }

// ResetNetwork resets all connections, DNS cache, and forces outbounds to reconnect.
func (s *Service) ResetNetwork() {
	s.withServer(func(srv *libbox.CommandServer) { srv.ResetNetwork() })
}

// NeedWIFIState reports whether the current config requires WIFI state monitoring.
func (s *Service) NeedWIFIState() bool {
	var result bool
	s.withServer(func(srv *libbox.CommandServer) { result = srv.NeedWIFIState() })
	return result
}

// NeedFindProcess reports whether the current config requires process finding.
func (s *Service) NeedFindProcess() bool {
	var result bool
	s.withServer(func(srv *libbox.CommandServer) { result = srv.NeedFindProcess() })
	return result
}

// UpdateWIFIState triggers the platform to report current WIFI state.
func (s *Service) UpdateWIFIState() {
	s.withServer(func(srv *libbox.CommandServer) { srv.UpdateWIFIState() })
}

// WriteMessage writes a log message to the core at the given level.
func (s *Service) WriteMessage(level int32, message string) {
	s.withServer(func(srv *libbox.CommandServer) { srv.WriteMessage(level, message) })
}

// FlushSystemDNS attempts to flush the system DNS cache.
func (s *Service) FlushSystemDNS() { s.platformIO.FlushSystemDNS() }

// QueryMemoryStats returns current Go runtime memory statistics as JSON.
func (s *Service) QueryMemoryStats() string {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	data, _ := json.Marshal(map[string]int64{
		"sys":         int64(m.Sys),
		"heap_alloc":  int64(m.HeapAlloc),
		"heap_sys":    int64(m.HeapSys),
		"stack_inuse": int64(m.StackInuse),
		"goroutines":  int64(runtime.NumGoroutine()),
		"limit":       debug.SetMemoryLimit(-1),
	})
	return string(data)
}

// SetOnEvent sets the event callback.
func (s *Service) SetOnEvent(fn func(eventType int32, jsonPayload string)) {
	s.mu.Lock()
	handler := s.handler
	s.mu.Unlock()

	if handler != nil {
		handler.SetOnEvent(fn)
	}
	setLogCallback(fn)
}

// QueryLogs returns combined sing-box and core internal logs as JSON.
func (s *Service) QueryLogs() string {
	s.mu.Lock()
	handler := s.handler
	s.mu.Unlock()

	var entries []LogEntry
	if handler != nil {
		_ = json.Unmarshal([]byte(handler.GetCachedLogsJSON()), &entries)
	}
	var coreEntries []LogEntry
	_ = json.Unmarshal([]byte(queryCoreLogs()), &coreEntries)
	entries = append(entries, coreEntries...)

	if len(entries) == 0 {
		return "[]"
	}
	data, _ := json.Marshal(entries)
	return string(data)
}

// client returns the command client. Caller must hold s.mu.
func (s *Service) client() (*libbox.CommandClient, error) {
	if s.state != StateRunning || s.commandClient == nil {
		return nil, fmt.Errorf("service not running")
	}
	return s.commandClient, nil
}

// withClient locks, validates the client, then calls fn.
func (s *Service) withClient(fn func(*libbox.CommandClient) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.client()
	if err != nil {
		return err
	}
	return fn(c)
}

// SelectOutbound selects a proxy in the given group.
func (s *Service) SelectOutbound(groupTag, outboundTag string) error {
	return s.withClient(func(c *libbox.CommandClient) error {
		return c.SelectOutbound(groupTag, outboundTag)
	})
}

// URLTest runs a URL test for the given outbound tag.
func (s *Service) URLTest(outboundTag string) error {
	return s.withClient(func(c *libbox.CommandClient) error { return c.URLTest(outboundTag) })
}

// SetClashMode sets the clash routing mode.
func (s *Service) SetClashMode(mode string) error {
	return s.withClient(func(c *libbox.CommandClient) error { return c.SetClashMode(mode) })
}

// CloseConnection closes a connection by ID.
func (s *Service) CloseConnection(connID string) error {
	return s.withClient(func(c *libbox.CommandClient) error { return c.CloseConnection(connID) })
}

// CloseConnections closes all active connections.
func (s *Service) CloseConnections() error {
	return s.withClient(func(c *libbox.CommandClient) error { return c.CloseConnections() })
}

// ClearLogs clears the server-side log buffer.
func (s *Service) ClearLogs() error {
	return s.withClient(func(c *libbox.CommandClient) error { return c.ClearLogs() })
}

// SetGroupExpand sets the UI expand state for a proxy group.
func (s *Service) SetGroupExpand(groupTag string, isExpand bool) error {
	return s.withClient(func(c *libbox.CommandClient) error {
		return c.SetGroupExpand(groupTag, isExpand)
	})
}

// GetStartedAt returns the unix timestamp when the service was started.
func (s *Service) GetStartedAt() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.client()
	if err != nil {
		return 0, err
	}
	return c.GetStartedAt()
}

// serverHandler implements libbox.CommandServerHandler with no-op methods.
type serverHandler struct{}

var defaultServerHandler = &serverHandler{}

func (h *serverHandler) ServiceStop() error   { return nil }
func (h *serverHandler) ServiceReload() error { return nil }
func (h *serverHandler) GetSystemProxyStatus() (*libbox.SystemProxyStatus, error) {
	return &libbox.SystemProxyStatus{}, nil
}
func (h *serverHandler) SetSystemProxyEnabled(bool) error { return nil }
func (h *serverHandler) WriteDebugMessage(message string) { slog.Debug(message) }
