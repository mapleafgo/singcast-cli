package core

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
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

// libboxReady is true after a successful libbox.Setup.
var libboxReady bool

func resetLibboxForTesting() {
	libboxReady = false
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
	mu              sync.Mutex
	state           State
	commandServer   *libbox.CommandServer
	commandClient   *libbox.CommandClient
	handler         *ClientHandler
	platformIO      *PlatformIO
	currentConfig     string // translated sing-box JSON (kept for ReloadTUN)
	overrideAutoRoute bool
	overrideInclude   []string
	overrideExclude   []string
}

// NewService creates a Service in StateCreated.
func NewService() *Service {
	return &Service{platformIO: NewPlatformIO()}
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
func (s *Service) Init(optionsJSON string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateCreated {
		return fmt.Errorf("init: invalid state %s", s.state)
	}

	var opts InitOptions
	if err := json.Unmarshal([]byte(optionsJSON), &opts); err != nil {
		return fmt.Errorf("parse init options: %w", err)
	}
	if opts.HomeDir == "" {
		return fmt.Errorf("init: home_dir is required")
	}

	if opts.Debug {
		SetLogLevel(LogLevelDebug)
	}
	slog.Debug("[init] begin", "homeDir", opts.HomeDir, "logMaxLines", opts.LogMaxLines, "debug", opts.Debug)

	tempDir := filepath.Join(opts.HomeDir, "temp")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}

if !libboxReady {
		slog.Debug("[init] calling libbox.Setup")
		if err := libbox.Setup(&libbox.SetupOptions{
			BasePath:        opts.HomeDir,
			WorkingPath:     opts.HomeDir,
			TempPath:        tempDir,
			LogMaxLines:     opts.LogMaxLines,
			Debug:           opts.Debug,
			FixAndroidStack: opts.FixAndroidStack,
		}); err != nil {
			return fmt.Errorf("libbox setup: %w", err)
		}
		libboxReady = true
		slog.Debug("[init] libbox.Setup done")
	} else {
		slog.Debug("[init] libbox already set up, skipping")
	}

	slog.Debug("[init] creating CommandServer")
	handler := NewClientHandler()
	commandServer, err := libbox.NewCommandServer(defaultServerHandler, s.platformIO)
	if err != nil {
		return fmt.Errorf("create command server: %w", err)
	}
	slog.Debug("[init] starting CommandServer")
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
	slog.Debug("[StartWithContent] begin", "bytes", len(content), "state", s.state)
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
	slog.Debug("[startWithContent] begin", "bytes", len(content), "proxy", ruleSetProxy, "state", s.state)

	data := []byte(content)
	format := translator.DetectFormat(data)
	slog.Debug("[startWithContent] detected format", "format", format)

	jsonContent, err := s.translateConfig(data, format, ruleSetProxy)
	if err != nil {
		slog.Error("[startWithContent] config translation failed", "format", format, "error", err)
		return err
	}
	slog.Debug("[startWithContent] config translated", "jsonBytes", len(jsonContent))

	return s.startWithJSON(jsonContent, &libbox.OverrideOptions{}, false, nil, nil)
}

func parseOverrideOptions(jsonStr string) (*libbox.OverrideOptions, OverrideConfig, error) {
	if jsonStr == "" {
		return &libbox.OverrideOptions{}, OverrideConfig{}, nil
	}
	var cfg OverrideConfig
	if err := json.Unmarshal([]byte(jsonStr), &cfg); err != nil {
		return nil, OverrideConfig{}, fmt.Errorf("parse override JSON: %w", err)
	}
	// cfg slices are fresh from json.Unmarshal — safe to share with stringIterator
	// and store as snapshot without Clone.
	return &libbox.OverrideOptions{
		AutoRedirect:   cfg.AutoRedirect,
		IncludePackage: &stringIterator{items: cfg.IncludePackages},
		ExcludePackage: &stringIterator{items: cfg.ExcludePackages},
	}, cfg, nil
}

// buildOverrideFromSnapshot constructs a fresh OverrideOptions with cloned slices
// so each call produces an independent iterator.
func buildOverrideFromSnapshot(autoRoute bool, include, exclude []string) *libbox.OverrideOptions {
	return &libbox.OverrideOptions{
		AutoRedirect:   autoRoute,
		IncludePackage: &stringIterator{items: slices.Clone(include)},
		ExcludePackage: &stringIterator{items: slices.Clone(exclude)},
	}
}

func (s *Service) translateConfig(data []byte, format translator.Format, ruleSetProxy string) (string, error) {
	if format == translator.FormatYAML {
		slog.Debug("[translate] YAML detected, translating to JSON", "proxy", ruleSetProxy)
		opts := &translator.Options{RuleSetURLPrefix: ruleSetProxy}
		result, _, err := translator.TranslateWithOptions(data, opts)
		if err != nil {
			return "", fmt.Errorf("translate config: %w", err)
		}
		slog.Debug("[translate] done", "jsonBytes", len(result))
		return result, nil
	}
	slog.Debug("[translate] already JSON, skipping", "bytes", len(data))
	return string(data), nil
}

// startWithJSON feeds translated JSON config to libbox and reconnects
// the command client. Caller must hold s.mu.
// autoRoute/include/exclude are stored as the current override snapshot on success.
func (s *Service) startWithJSON(jsonContent string, override *libbox.OverrideOptions, autoRoute bool, include, exclude []string) error {
	slog.Debug("[startWithJSON] begin", "bytes", len(jsonContent), "state", s.state)

	// 从 config 的 log.level 同步到 coreLogLevel，让 sing-box 日志和内核日志联动。
	syncLogLevelFromConfig(jsonContent)

	oldConfig := s.currentConfig
	s.currentConfig = jsonContent

	// Disconnect old client BEFORE starting new service to avoid stale callbacks.
	if s.commandClient != nil {
		slog.Debug("[startWithJSON] disconnecting old command client")
		s.commandClient.Disconnect()
		s.commandClient = nil
	}

	slog.Debug("[startWithJSON] calling StartOrReloadService")
	if err := s.commandServer.StartOrReloadService(jsonContent, override); err != nil {
		slog.Error("StartOrReloadService failed", "error", err)
		s.currentConfig = oldConfig
		s.state = StateInitialized
		return fmt.Errorf("start service: %w", err)
	}
	slog.Debug("[startWithJSON] StartOrReloadService OK")

	slog.Debug("[startWithJSON] creating CommandClient")
	opts := &libbox.CommandClientOptions{StatusInterval: int64(time.Second)}
	opts.AddCommand(libbox.CommandLog)
	opts.AddCommand(libbox.CommandStatus)
	opts.AddCommand(libbox.CommandGroup)
	opts.AddCommand(libbox.CommandConnections)

	newClient := libbox.NewCommandClient(s.handler, opts)
	slog.Debug("[startWithJSON] calling CommandClient.Connect")
	if err := newClient.Connect(); err != nil {
		slog.Error("command client Connect failed", "error", err)
		s.commandServer.CloseService()
		s.currentConfig = oldConfig
		s.state = StateInitialized
		return fmt.Errorf("connect command client: %w", err)
	}
	slog.Debug("[startWithJSON] CommandClient.Connect OK")

	s.commandClient = newClient
	s.handler.SetStartedAt(time.Now().Unix())
	s.overrideAutoRoute = autoRoute
	s.overrideInclude = include
	s.overrideExclude = exclude
	s.state = StateRunning
	slog.Info("service running")
	return nil
}

// Stop stops the running service. No-op if not running.
func (s *Service) Stop() error {
	slog.Debug("[Stop] begin", "state", s.state)
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateRunning {
		slog.Debug("[Stop] not running, no-op")
		return nil
	}

	s.platformIO.ResetTunFd()

	if s.commandClient != nil {
		slog.Debug("[Stop] disconnecting command client")
		s.commandClient.Disconnect()
		s.commandClient = nil
	}

	slog.Debug("[Stop] closing service")
	err := s.commandServer.CloseService()
	if err != nil {
		slog.Error("stop: CloseService error", "error", err)
	} else {
		slog.Info("service stopped")
	}

	s.overrideAutoRoute = false
	s.overrideInclude = nil
	s.overrideExclude = nil
	s.state = StateInitialized
	return err
}

// Destroy releases all resources. The instance cannot be reused.
func (s *Service) Destroy() {
	slog.Debug("[Destroy] begin", "state", s.state)
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateDestroyed {
		return
	}

	if s.state == StateRunning {
		slog.Debug("[Destroy] closing running service")
		s.platformIO.ResetTunFd()
		s.commandServer.CloseService()
	}

	if s.commandClient != nil {
		slog.Debug("[Destroy] disconnecting command client")
		s.commandClient.Disconnect()
		s.commandClient = nil
	}

	if s.commandServer != nil {
		slog.Debug("[Destroy] closing command server")
		s.commandServer.Close()
		s.commandServer = nil
	}

	s.state = StateDestroyed
	slog.Info("service destroyed")
}

// ReloadConfig reloads the service with new configuration content.
// Also works from StateInitialized (first profile activation).
func (s *Service) ReloadConfig(content, ruleSetProxy string) error {
	slog.Debug("[ReloadConfig] begin", "bytes", len(content), "state", s.state)
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateInitialized && s.state != StateRunning {
		return fmt.Errorf("reload config: invalid state %s", s.state)
	}
	return s.startWithContent(content, ruleSetProxy)
}

// ReloadTUN restarts the TUN interface without changing configuration.
func (s *Service) ReloadTUN() error {
	slog.Debug("[ReloadTUN] begin", "state", s.state)
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateRunning {
		return fmt.Errorf("reload TUN: invalid state %s", s.state)
	}
	if s.currentConfig == "" {
		return fmt.Errorf("reload TUN: no configuration stored")
	}

	slog.Debug("[ReloadTUN] reloading with existing config", "configBytes", len(s.currentConfig))
	override := buildOverrideFromSnapshot(s.overrideAutoRoute, s.overrideInclude, s.overrideExclude)
	return s.startWithJSON(s.currentConfig, override, s.overrideAutoRoute, s.overrideInclude, s.overrideExclude)
}

// SetOverridePackages updates the include/exclude package lists for VPN split tunneling
// and restarts the service with the current config. Requires StateRunning.
// Pass empty string to clear all overrides.
// Note: this triggers a full service reload (including TUN rebuild on mobile).
// Mobile callers must ensure a fresh TUN fd is set via SetTunFd before calling this.
func (s *Service) SetOverridePackages(overrideJSON string) error {
	slog.Debug("[SetOverridePackages] begin", "state", s.state)
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateRunning {
		return fmt.Errorf("set override packages: invalid state %s", s.state)
	}
	if s.currentConfig == "" {
		return fmt.Errorf("set override packages: no configuration stored")
	}

	override, cfg, err := parseOverrideOptions(overrideJSON)
	if err != nil {
		slog.Error("[SetOverridePackages] parse override failed", "error", err)
		return err
	}
	slog.Debug("[SetOverridePackages] parsed", "include", len(cfg.IncludePackages), "exclude", len(cfg.ExcludePackages))
	return s.startWithJSON(s.currentConfig, override, cfg.AutoRedirect, cfg.IncludePackages, cfg.ExcludePackages)
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

// SetLogLevel sets the minimum log level (2=Error .. 6=Trace).
func (s *Service) SetLogLevel(level int32) { SetLogLevel(level) }

// SetError pushes an error message to the command server.
func (s *Service) SetError(message string) {
	s.withServer(func(srv *libbox.CommandServer) { srv.SetError(message) })
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
		entries = append(entries, handler.GetCachedLogs()...)
	}
	entries = append(entries, queryCoreLogEntries()...)

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
