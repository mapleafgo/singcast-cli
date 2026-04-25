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
	started       bool
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

	// Create the CommandServer
	commandServer, err := libbox.NewCommandServer((*serverHandler)(nil), platformIO)
	if err != nil {
		return fmt.Errorf("create command server: %w", err)
	}

	instance = &Service{
		commandServer: commandServer,
		handler:       handler,
		homeDir:       homeDir,
	}

	return nil
}

// Start starts the singleton service with the given config path.
func Start(configPath string) error {
	mu.Lock()
	svc := instance
	mu.Unlock()
	if svc == nil {
		return fmt.Errorf("core not initialized")
	}
	return svc.start(configPath)
}

func (s *Service) start(configPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.configPath = configPath

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	// Translate YAML to sing-box JSON if needed
	var jsonContent string
	if translator.DetectFormat(data) == translator.FormatYAML {
		result, _, err := translator.Translate(data)
		if err != nil {
			return fmt.Errorf("translate config: %w", err)
		}
		jsonContent = result
	} else {
		jsonContent = string(data)
	}

	// Start or reload the service
	err = s.commandServer.StartOrReloadService(jsonContent, nil)
	if err != nil {
		return fmt.Errorf("start service: %w", err)
	}

	// Disconnect previous client before creating a new one (reload case)
	if s.commandClient != nil {
		s.commandClient.Disconnect()
	}

	// Create and connect the command client
	opts := &libbox.CommandClientOptions{
		StatusInterval: 1000,
	}
	opts.AddCommand(libbox.CommandLog)
	opts.AddCommand(libbox.CommandStatus)
	opts.AddCommand(libbox.CommandGroup)
	opts.AddCommand(libbox.CommandConnections)

	s.commandClient = libbox.NewCommandClient(s.handler, opts)
	if err := s.commandClient.Connect(); err != nil {
		return fmt.Errorf("connect command client: %w", err)
	}

	s.started = true
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
		return nil
	}
	s.started = false
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
	if s.commandClient != nil {
		s.commandClient.Disconnect()
		s.commandClient = nil
	}
	if s.commandServer != nil {
		s.commandServer.Close()
		s.commandServer = nil
	}
	s.started = false
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
	if s.configPath == "" {
		return fmt.Errorf("no config path set")
	}
	return s.start(s.configPath)
}

// CheckConfig validates a sing-box JSON config string.
func CheckConfig(jsonContent string) error {
	return libbox.CheckConfig(jsonContent)
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
