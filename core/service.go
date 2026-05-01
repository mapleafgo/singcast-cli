package core

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sagernet/sing-box/experimental/libbox"
	_ "github.com/sagernet/sing-box/include" // register all protocols

	"github.com/mapleafgo/singcast/translator"
)

var (
	instance *Service
	mu       sync.Mutex
)

// Version is set via -ldflags at build time.
var Version = "dev"

// VersionJSON returns the version info as a JSON string.
func VersionJSON() string {
	data, _ := json.Marshal(map[string]string{
		"version": Version,
		"core":    "sing-box",
	})
	return string(data)
}

// Service wraps a libbox.CommandServer to manage the sing-box service lifecycle.
type Service struct {
	mu            sync.Mutex
	commandServer *libbox.CommandServer
	commandClient *libbox.CommandClient
	handler       *ClientHandler
	homeDir       string
	ruleSetProxy  string
	started       bool
	platformIO    *PlatformIO
}

// GetService returns the singleton service instance.
func GetService() *Service {
	mu.Lock()
	defer mu.Unlock()
	return instance
}

// Init initializes the libbox runtime environment and creates the command server.
func Init(homeDir string) error {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil {
		return fmt.Errorf("core already initialized")
	}

	slog.Info("core init", "homeDir", homeDir)

	tempDir := filepath.Join(homeDir, "temp")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}

	err := libbox.Setup(&libbox.SetupOptions{
		BasePath:    homeDir,
		WorkingPath: homeDir,
		TempPath:    tempDir,
	})
	if err != nil {
		slog.Error("libbox setup failed", "error", err)
		return fmt.Errorf("libbox setup: %w", err)
	}

	handler := NewClientHandler(nil)
	platformIO := &PlatformIO{}

	commandServer, err := libbox.NewCommandServer(&serverHandler{}, platformIO)
	if err != nil {
		slog.Error("create command server failed", "error", err)
		return fmt.Errorf("create command server: %w", err)
	}
	if err := commandServer.Start(); err != nil {
		slog.Error("start command server failed", "error", err)
		return fmt.Errorf("start command server: %w", err)
	}

	instance = &Service{
		commandServer: commandServer,
		handler:       handler,
		homeDir:       homeDir,
		platformIO:    platformIO,
	}

	slog.Info("core init done")
	return nil
}

// Stop shuts down the running singleton service.
func Stop() error {
	mu.Lock()
	svc := instance
	mu.Unlock()
	if svc == nil {
		return fmt.Errorf("core not initialized")
	}
	return svc.stop()
}

func (s *Service) stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started {
		slog.Debug("stop: not started, skipping")
		return nil
	}
	slog.Info("stopping service")
	s.started = false
	if s.platformIO != nil {
		s.platformIO.ResetTunFd()
	}
	err := s.commandServer.CloseService()
	if err != nil {
		slog.Error("stop: CloseService error", "error", err)
	} else {
		slog.Info("service stopped")
	}
	return err
}

// Destroy tears down the singleton and releases all resources.
func Destroy() error {
	mu.Lock()
	svc := instance
	instance = nil
	mu.Unlock()
	if svc == nil {
		return nil
	}
	return svc.destroy()
}

func (s *Service) destroy() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	slog.Info("releasing all resources", "started", s.started)
	if s.started {
		s.commandServer.CloseService()
		s.started = false
	}
	if s.platformIO != nil {
		s.platformIO.ResetTunFd()
	}
	if s.commandClient != nil {
		s.commandClient.Disconnect()
		s.commandClient = nil
	}
	if s.commandServer != nil {
		s.commandServer.Close()
		s.commandServer = nil
	}
	slog.Info("all resources released")
	return nil
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

// SetOnEvent sets the event callback on the singleton service.
func SetOnEvent(fn func(eventType int, jsonPayload string)) {
	mu.Lock()
	svc := instance
	mu.Unlock()
	if svc != nil && svc.handler != nil {
		svc.handler.SetOnEvent(fn)
	}
	setLogCallback(fn)
}

func (s *Service) client() (*libbox.CommandClient, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.commandClient == nil {
		return nil, fmt.Errorf("command client not connected")
	}
	return s.commandClient, nil
}

// SelectOutbound selects a proxy in the given group.
func (s *Service) SelectOutbound(groupTag, outboundTag string) error {
	c, err := s.client()
	if err != nil {
		return err
	}
	return c.SelectOutbound(groupTag, outboundTag)
}

// URLTest runs a URL test for the given outbound tag.
func (s *Service) URLTest(outboundTag string) error {
	c, err := s.client()
	if err != nil {
		return err
	}
	return c.URLTest(outboundTag)
}

// SetClashMode sets the clash routing mode.
func (s *Service) SetClashMode(mode string) error {
	c, err := s.client()
	if err != nil {
		return err
	}
	return c.SetClashMode(mode)
}

// CloseConnection closes a connection by ID.
func (s *Service) CloseConnection(connID string) error {
	c, err := s.client()
	if err != nil {
		return err
	}
	return c.CloseConnection(connID)
}

// CloseConnections closes all active connections.
func (s *Service) CloseConnections() error {
	c, err := s.client()
	if err != nil {
		return err
	}
	return c.CloseConnections()
}

// QueryLogs returns combined sing-box logs and core internal logs as JSON.
func QueryLogs() string {
	mu.Lock()
	svc := instance
	mu.Unlock()

	var singboxJSON string
	if svc != nil && svc.handler != nil {
		singboxJSON = svc.handler.GetCachedLogsJSON()
	}

	coreJSON := queryCoreLogs()

	if singboxJSON == "[]" || singboxJSON == "" {
		return coreJSON
	}
	if coreJSON == "[]" || coreJSON == "" {
		return singboxJSON
	}
	// Merge "[a,b]" + "[c,d]" → "[a,b,c,d]"
	return singboxJSON[:len(singboxJSON)-1] + "," + coreJSON[1:]
}

// serverHandler implements libbox.CommandServerHandler with no-op methods.
type serverHandler struct{}

func (h *serverHandler) ServiceStop() error {
	slog.Warn("core requested stop")
	return nil
}
func (h *serverHandler) ServiceReload() error {
	slog.Info("core requested reload")
	return nil
}
func (h *serverHandler) GetSystemProxyStatus() (*libbox.SystemProxyStatus, error) {
	return &libbox.SystemProxyStatus{}, nil
}
func (h *serverHandler) SetSystemProxyEnabled(enabled bool) error { return nil }
func (h *serverHandler) WriteDebugMessage(message string) {
	slog.Debug(message)
}

// SetTunFd stores a TUN file descriptor for mobile platforms.
func SetTunFd(fd int32) {
	mu.Lock()
	defer mu.Unlock()
	slog.Info("set TUN fd", "fd", fd)
	if instance != nil && instance.platformIO != nil {
		instance.platformIO.SetTunFd(fd)
	} else {
		slog.Warn("SetTunFd: instance or platformIO is nil", "instance", instance != nil)
	}
}

// StartWithContent starts the service with raw YAML or JSON content.
func StartWithContent(content, ruleSetProxy string) error {
	mu.Lock()
	svc := instance
	mu.Unlock()
	if svc == nil {
		return fmt.Errorf("core not initialized")
	}
	return svc.startWithContent(content, ruleSetProxy)
}

func (s *Service) startWithContent(content, ruleSetProxy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.commandServer == nil {
		return fmt.Errorf("service closed")
	}

	slog.Info("startWithContent", "bytes", len(content), "proxy", ruleSetProxy, "started", s.started)

	s.ruleSetProxy = ruleSetProxy

	format := translator.DetectFormat([]byte(content))
	slog.Debug("detected config format", "format", format)

	jsonContent, err := s.translateConfig([]byte(content), ruleSetProxy)
	if err != nil {
		slog.Error("config translation failed", "error", err)
		return err
	}

	return s.startWithJSON(jsonContent)
}

// translateConfig translates raw config bytes (YAML or JSON) to sing-box JSON.
func (s *Service) translateConfig(data []byte, ruleSetProxy string) (string, error) {
	if translator.DetectFormat(data) == translator.FormatYAML {
		opts := &translator.Options{RuleSetURLPrefix: ruleSetProxy}
		result, _, err := translator.TranslateWithOptions(data, opts)
		if err != nil {
			return "", fmt.Errorf("translate config: %w", err)
		}
		return result, nil
	}
	return string(data), nil
}

// startWithJSON feeds the already-translated JSON config to libbox and
// reconnects the command client. Caller must hold s.mu.
func (s *Service) startWithJSON(jsonContent string) error {
	slog.Info("starting/reloading service", "bytes", len(jsonContent))
	startTime := time.Now()

	err := s.commandServer.StartOrReloadService(jsonContent, &libbox.OverrideOptions{})
	elapsed := time.Since(startTime)
	if err != nil {
		slog.Error("StartOrReloadService failed", "elapsed", elapsed, "error", err)
		return fmt.Errorf("start service: %w", err)
	}
	slog.Info("StartOrReloadService succeeded", "elapsed", elapsed)

	opts := &libbox.CommandClientOptions{
		StatusInterval: int64(time.Second),
	}
	opts.AddCommand(libbox.CommandLog)
	opts.AddCommand(libbox.CommandStatus)
	opts.AddCommand(libbox.CommandGroup)
	opts.AddCommand(libbox.CommandConnections)

	newClient := libbox.NewCommandClient(s.handler, opts)
	if err := newClient.Connect(); err != nil {
		slog.Error("command client Connect failed", "error", err)
		return fmt.Errorf("connect command client: %w", err)
	}

	if s.commandClient != nil {
		s.commandClient.Disconnect()
	}
	s.commandClient = newClient

	s.started = true
	slog.Info("core fully initialized and running")
	return nil
}
