package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

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
	configPath    string
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

	// Create temp directory under homeDir
	tempDir := filepath.Join(homeDir, "temp")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}

	// Setup libbox runtime
	err := libbox.Setup(&libbox.SetupOptions{
		BasePath:    homeDir,
		WorkingPath: homeDir,
		TempPath:    tempDir,
	})
	if err != nil {
		return fmt.Errorf("libbox setup: %w", err)
	}

	handler := NewClientHandler(nil)
	platformIO := &PlatformIO{}

	// Create and start the CommandServer
	commandServer, err := libbox.NewCommandServer(&serverHandler{}, platformIO)
	if err != nil {
		return fmt.Errorf("create command server: %w", err)
	}
	if err := commandServer.Start(); err != nil {
		return fmt.Errorf("start command server: %w", err)
	}

	instance = &Service{
		commandServer: commandServer,
		handler:       handler,
		homeDir:       homeDir,
		platformIO:    platformIO,
	}

	return nil
}

// Start starts the singleton service with the given config path.
// ruleSetProxy is an optional URL prefix for rule_set downloads (empty = direct).
func Start(configPath string, ruleSetProxy ...string) error {
	mu.Lock()
	svc := instance
	mu.Unlock()
	if svc == nil {
		return fmt.Errorf("core not initialized")
	}
	var proxy string
	if len(ruleSetProxy) > 0 {
		proxy = ruleSetProxy[0]
	}
	return svc.start(configPath, proxy)
}

func (s *Service) start(configPath string, ruleSetProxy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.commandServer == nil {
		return fmt.Errorf("service closed")
	}

	s.configPath = configPath
	s.ruleSetProxy = ruleSetProxy

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	jsonContent, err := s.translateConfig(data, ruleSetProxy)
	if err != nil {
		return err
	}

	return s.startWithJSON(jsonContent)
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
		return nil
	}
	s.started = false
	// The service closes the TUN interface on shutdown; clear the
	// stored fd so a subsequent start cannot reuse a stale descriptor.
	if s.platformIO != nil {
		s.platformIO.ResetTunFd()
	}
	return s.commandServer.CloseService()
}

// Close tears down the singleton and releases all resources.
func Close() error {
	mu.Lock()
	svc := instance
	instance = nil
	mu.Unlock()
	if svc == nil {
		return nil
	}
	return svc.close()
}

func (s *Service) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	return nil
}

// ReloadConfig reloads the singleton service with the last used config path.
func ReloadConfig() error {
	mu.Lock()
	svc := instance
	mu.Unlock()
	if svc == nil {
		return fmt.Errorf("core not initialized")
	}
	return svc.ReloadConfig()
}

// ReloadConfig re-reads and re-translates the config, then reloads.
func (s *Service) ReloadConfig() error {
	s.mu.Lock()
	path := s.configPath
	proxy := s.ruleSetProxy
	s.mu.Unlock()
	if path == "" {
		return fmt.Errorf("no config path set")
	}
	return s.start(path, proxy)
}

// CheckConfig validates a sing-box JSON config string.
// CheckConfig validates a config string (Clash YAML or sing-box JSON).
// YAML content is translated to sing-box JSON before validation.
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

// serverHandler implements libbox.CommandServerHandler with no-op methods.
type serverHandler struct{}

func (h *serverHandler) ServiceStop() error                { return nil }
func (h *serverHandler) ServiceReload() error              { return nil }
func (h *serverHandler) GetSystemProxyStatus() (*libbox.SystemProxyStatus, error) {
	return &libbox.SystemProxyStatus{}, nil
}
func (h *serverHandler) SetSystemProxyEnabled(enabled bool) error { return nil }
func (h *serverHandler) WriteDebugMessage(message string)         {}

// SetTunFd stores a TUN file descriptor for mobile platforms.
// Call this after creating the TUN interface (VpnService/NetworkExtension)
// and before Start/StartWithContent.
func SetTunFd(fd int32) {
	mu.Lock()
	defer mu.Unlock()
	if instance != nil && instance.platformIO != nil {
		instance.platformIO.SetTunFd(fd)
	}
}

// StartWithContent starts the service with raw YAML or JSON content.
// No file is involved; the content is translated and used directly.
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

	s.ruleSetProxy = ruleSetProxy

	jsonContent, err := s.translateConfig([]byte(content), ruleSetProxy)
	if err != nil {
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
	err := s.commandServer.StartOrReloadService(jsonContent, &libbox.OverrideOptions{})
	if err != nil {
		return fmt.Errorf("start service: %w", err)
	}

	opts := &libbox.CommandClientOptions{
		StatusInterval: 1000,
	}
	opts.AddCommand(libbox.CommandLog)
	opts.AddCommand(libbox.CommandStatus)
	opts.AddCommand(libbox.CommandGroup)
	opts.AddCommand(libbox.CommandConnections)

	newClient := libbox.NewCommandClient(s.handler, opts)
	if err := newClient.Connect(); err != nil {
		return fmt.Errorf("connect command client: %w", err)
	}

	if s.commandClient != nil {
		s.commandClient.Disconnect()
	}
	s.commandClient = newClient

	s.started = true
	return nil
}
