package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/include"
	singboxlog "github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/service"

	"github.com/mapleafgo/singcast/translator"
)

var Version = "dev"

type platformLogWriter struct {
	svc *Service
}

func (w *platformLogWriter) WriteMessage(level singboxlog.Level, message string) {
	var coreLevel int32
	switch {
	case level <= singboxlog.LevelError:
		coreLevel = LogLevelError
	case level <= singboxlog.LevelWarn:
		coreLevel = LogLevelWarn
	case level <= singboxlog.LevelInfo:
		coreLevel = LogLevelInfo
	case level <= singboxlog.LevelDebug:
		coreLevel = LogLevelDebug
	default:
		coreLevel = LogLevelTrace
	}
	data, _ := json.Marshal(LogEntry{
		Level:     coreLevel,
		Message:   message,
		Timestamp: time.Now().UnixMilli(),
	})
	w.svc.emitEvent(EventLog, string(data))
}

func VersionJSON() string {
	data, _ := json.Marshal(VersionInfo{Version: Version, Core: "singcast"})
	return string(data)
}

func CheckConfig(ctx context.Context, content string) error {
	data := []byte(content)
	if translator.DetectFormat(data) == translator.FormatYAML {
		result, _, err := translator.TranslateWithOptions(data, nil)
		if err != nil {
			return fmt.Errorf("translate config: %w", err)
		}
		data = []byte(result)
	}
	ctx = include.Context(ctx)
	_, err := singjson.UnmarshalExtendedContext[option.Options](ctx, data)
	return err
}

type State int32

const (
	StateCreated State = iota
	StateInitialized
	StateStarting
	StateRunning
	StateDestroyed
)

func (s State) String() string {
	switch s {
	case StateCreated:
		return "created"
	case StateInitialized:
		return "initialized"
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateDestroyed:
		return "destroyed"
	default:
		return "unknown"
	}
}

// atomicState wraps atomic.Int32 with typed State access, eliminating int32/State casts.
type atomicState struct{ v atomic.Int32 }

func (a *atomicState) Load() State   { return State(a.v.Load()) }
func (a *atomicState) Store(s State) { a.v.Store(int32(s)) }
func (a *atomicState) CompareAndSwap(old, newState State) bool {
	return a.v.CompareAndSwap(int32(old), int32(newState))
}

type runningState struct {
	instance      *box.Box
	boxCtx        context.Context
	currentConfig string
	startedAt     int64
	// stubTags maps outbound tag → original protocol for proxies converted to
	// socks stubs (unsupported protocols). Used by QueryProxies to report the
	// original type so the UI can distinguish unsupported nodes.
	stubTags map[string]string
}

type Service struct {
	state     atomicState
	running   atomic.Pointer[runningState]
	platform  *PlatformIO
	onEvent   atomic.Pointer[func(int32, string)]
	subCancel atomic.Pointer[func()]

	// startMu serializes StartWithContent calls. A concurrent caller waits
	// for the current startup to finish, then stops and restarts.
	startMu  sync.Mutex
	startSeq atomic.Int64 // coalesce: only the latest caller proceeds
}

func NewService() *Service { return &Service{platform: NewPlatformIO()} }

func (s *Service) State() State {
	return s.state.Load()
}

func (s *Service) PlatformIO() *PlatformIO { return s.platform }

func (s *Service) Init(optionsJSON string) error {
	if !s.state.CompareAndSwap(StateCreated, StateInitialized) {
		return fmt.Errorf("init: invalid state %s", s.State())
	}

	var opts InitOptions
	if err := json.Unmarshal([]byte(optionsJSON), &opts); err != nil {
		s.state.Store(StateCreated)
		return fmt.Errorf("parse init options: %w", err)
	}
	if opts.HomeDir == "" {
		s.state.Store(StateCreated)
		return fmt.Errorf("init: home_dir is required")
	}

	if err := os.MkdirAll(opts.HomeDir, 0o755); err != nil {
		s.state.Store(StateCreated)
		return fmt.Errorf("create home dir: %w", err)
	}
	if err := os.Chdir(opts.HomeDir); err != nil {
		s.state.Store(StateCreated)
		return fmt.Errorf("chdir: %w", err)
	}

	if opts.Debug {
		SetLogLevel(LogLevelDebug)
	}

	tempDir := filepath.Join(opts.HomeDir, "temp")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		s.state.Store(StateCreated)
		return fmt.Errorf("create temp dir: %w", err)
	}

	s.emitState(StateInitialized)
	slog.Info("service init done")
	return nil
}

func (s *Service) StartWithContent(content, ruleSetProxy string) error {
	// Translate config outside the lock — pure computation, no shared state.
	data := []byte(content)
	jsonContent, stubTags, err := s.translateConfig(data, translator.DetectFormat(data), ruleSetProxy)
	if err != nil {
		return err
	}

	mySeq := s.startSeq.Add(1)

	s.startMu.Lock()
	defer s.startMu.Unlock()

	// A newer request arrived while we waited — let it win.
	if s.startSeq.Load() != mySeq {
		return nil
	}

	// Stop any running/starting instance first.
	if err := s.Stop(); err != nil {
		slog.Warn("stop previous instance", "error", err)
	}

	if !s.casState(StateInitialized, StateStarting) {
		return fmt.Errorf("start: invalid state %s", s.State())
	}

	if err := s.startWithJSON(jsonContent, stubTags); err != nil {
		s.casState(StateStarting, StateInitialized)
		return err
	}
	return nil
}

func (s *Service) translateConfig(data []byte, format translator.Format, ruleSetProxy string) (string, map[string]string, error) {
	if format == translator.FormatYAML {
		opts := &translator.Options{RuleSetURLPrefix: ruleSetProxy}
		result, _, meta, err := translator.TranslateWithMeta(data, opts)
		if err != nil {
			return "", nil, fmt.Errorf("translate config: %w", err)
		}
		return result, meta.StubTags, nil
	}
	if ruleSetProxy != "" {
		applied, err := applyRuleSetProxy(data, ruleSetProxy)
		return applied, nil, err
	}
	return string(data), nil, nil
}

// applyRuleSetProxy prepends the proxy prefix to raw.githubusercontent.com URLs
// in sing-box JSON route.rule_set[].url, using translator.ProxyURL.
func applyRuleSetProxy(data []byte, proxy string) (string, error) {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return "", fmt.Errorf("parse json: %w", err)
	}
	route, _ := root["route"].(map[string]any)
	if route == nil {
		return string(data), nil
	}
	ruleSets, _ := route["rule_set"].([]any)
	if ruleSets == nil {
		return string(data), nil
	}
	for _, rs := range ruleSets {
		def, _ := rs.(map[string]any)
		if def == nil {
			continue
		}
		if url, _ := def["url"].(string); url != "" {
			def["url"] = translator.ProxyURL(url, proxy)
		}
	}
	out, err := json.Marshal(root)
	if err != nil {
		return "", fmt.Errorf("marshal json: %w", err)
	}
	return string(out), nil
}

func (s *Service) startWithJSON(jsonContent string, stubTags map[string]string) error {
	syncLogLevelFromConfig(jsonContent)
	s.platform.protectCount.Store(0)

	if s.platform.IsMobile() {
		hasProtect := s.platform.protectFn.Load() != nil
		fd := s.platform.tunFd.Load()
		monitor := s.platform.ifaceMonitor
		var defaultIface string
		var myIface []string
		if monitor != nil {
			if di := monitor.DefaultInterface(); di != nil {
				defaultIface = di.Name
			}
			myIface = monitor.MyInterfaces()
		}
		slog.Debug("mobile startup", "hasProtect", hasProtect, "tunFd", fd, "defaultIface", defaultIface, "myIface", myIface)
	}

	ctx := include.Context(context.Background())
	if s.platform.IsMobile() {
		ctx = service.ContextWith[adapter.PlatformInterface](ctx, s.platform)
	}

	options, err := singjson.UnmarshalExtendedContext[option.Options](ctx, []byte(jsonContent))
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	// Mobile platforms require auto_detect_interface for VpnService.protect()
	// to bypass VPN routing on outbound sockets.
	if s.platform.IsMobile() && options.Route != nil {
		options.Route.AutoDetectInterface = true
	}

	logWriter := &platformLogWriter{svc: s}
	inst, err := box.New(box.Options{Options: options, Context: ctx, PlatformLogWriter: logWriter})
	if err != nil {
		return fmt.Errorf("create instance: %w", err)
	}

	if s.platform.IsMobile() {
		nm := service.FromContext[adapter.NetworkManager](ctx)
		if nm != nil {
			pi := service.FromContext[adapter.PlatformInterface](ctx)
			protectFn := nm.ProtectFunc()
			slog.Debug("mobile protect diagnostic",
				"autoDetect", nm.AutoDetectInterface(),
				"protectFuncNil", protectFn == nil,
				"platformNil", pi == nil,
				"usePlatformCtrl", pi != nil && pi.UsePlatformAutoDetectInterfaceControl(),
			)
			if err := nm.UpdateInterfaces(); err != nil {
				slog.Warn("update interfaces failed", "error", err)
			} else {
				slog.Debug("update interfaces", "count", len(nm.NetworkInterfaces()))
			}
		} else {
			slog.Warn("update interfaces: NetworkManager is nil")
		}
	}

	if err := inst.Start(); err != nil {
		inst.Close()
		return fmt.Errorf("start instance: %w", err)
	}

	s.platform.SetRouter(inst.Router())

	s.running.Store(&runningState{
		instance:      inst,
		boxCtx:        ctx,
		currentConfig: jsonContent,
		startedAt:     time.Now().UnixMilli(),
		stubTags:      stubTags,
	})
	if !s.casState(StateStarting, StateRunning) {
		s.running.Swap(nil)
		inst.Close()
		return nil
	}
	s.subscribeHooks()

	slog.Info("service running")
	return nil
}

func (s *Service) Stop() error {
	for {
		st := s.state.Load()
		if st != StateRunning && st != StateStarting {
			return nil
		}
		if s.casState(st, StateInitialized) {
			break
		}
	}

	s.unsubscribeHooks()
	old := s.running.Swap(nil)

	if old != nil {
		slog.Info("service stopped")
		return old.instance.Close()
	}
	return nil
}

func (s *Service) Destroy() {
	for {
		st := s.state.Load()
		if st == StateDestroyed {
			return
		}
		if s.casState(st, StateDestroyed) {
			break
		}
	}

	s.unsubscribeHooks()
	old := s.running.Swap(nil)

	if old != nil {
		old.instance.Close()
	}
	slog.Info("service destroyed")
}

func (s *Service) casState(oldState, newState State) bool {
	if s.state.CompareAndSwap(oldState, newState) {
		s.emitState(newState)
		return true
	}
	return false
}
