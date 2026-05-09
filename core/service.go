package core

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/urltest"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/group"
	"github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/service"

	"github.com/mapleafgo/singcast/translator"
)

var Version = "dev"

func VersionJSON() string {
	data, _ := json.Marshal(map[string]string{"version": Version, "core": "sing-box"})
	return string(data)
}

func CheckConfig(content string) error {
	data := []byte(content)
	if translator.DetectFormat(data) == translator.FormatYAML {
		result, _, err := translator.TranslateWithOptions(data, nil)
		if err != nil {
			return fmt.Errorf("translate config: %w", err)
		}
		data = []byte(result)
	}
	ctx := include.Context(context.Background())
	_, err := json.UnmarshalExtendedContext[option.Options](ctx, data)
	return err
}

type State int32

const (
	StateCreated     State = iota
	StateInitialized
	StateRunning
	StateDestroyed
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

type Service struct {
	mu            sync.Mutex
	state         State
	instance      *box.Box
	platform      *PlatformIO
	currentConfig string

	overrideAutoRoute bool
	overrideInclude   []string
	overrideExclude   []string
}

func NewService() *Service { return &Service{platform: NewPlatformIO()} }

func (s *Service) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *Service) PlatformIO() *PlatformIO { return s.platform }

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

	tempDir := filepath.Join(opts.HomeDir, "temp")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}

	s.state = StateInitialized
	slog.Info("service init done")
	return nil
}

func (s *Service) StartWithContent(content, ruleSetProxy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateInitialized && s.state != StateRunning {
		return fmt.Errorf("start: invalid state %s", s.state)
	}

	data := []byte(content)
	format := translator.DetectFormat(data)

	jsonContent, err := s.translateConfig(data, format, ruleSetProxy)
	if err != nil {
		return err
	}
	jsonContent = ensureClashModes(jsonContent)
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

func (s *Service) startWithJSON(jsonContent string) error {
	syncLogLevelFromConfig(jsonContent)

	if s.instance != nil {
		s.instance.Close()
		s.instance = nil
	}
	s.platform.ResetTunFd()

	ctx := include.Context(context.Background())
	if s.platform.UsePlatformInterface() {
		ctx = service.ContextWith[adapter.PlatformInterface](ctx, s.platform)
	}

	options, err := json.UnmarshalExtendedContext[option.Options](ctx, []byte(jsonContent))
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	inst, err := box.New(box.Options{Options: options, Context: ctx})
	if err != nil {
		return fmt.Errorf("create instance: %w", err)
	}

	if err := inst.Start(); err != nil {
		inst.Close()
		return fmt.Errorf("start instance: %w", err)
	}

	s.instance = inst
	s.currentConfig = jsonContent
	s.state = StateRunning
	slog.Info("service running")
	return nil
}

func (s *Service) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateRunning {
		return nil
	}

	s.platform.ResetTunFd()
	if s.instance != nil {
		err := s.instance.Close()
		s.instance = nil
		s.state = StateInitialized
		slog.Info("service stopped")
		return err
	}
	s.state = StateInitialized
	return nil
}

func (s *Service) Destroy() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateDestroyed {
		return
	}

	if s.state == StateRunning {
		s.platform.ResetTunFd()
	}
	if s.instance != nil {
		s.instance.Close()
		s.instance = nil
	}
	s.state = StateDestroyed
	slog.Info("service destroyed")
}

func (s *Service) ReloadTUN() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateRunning {
		return fmt.Errorf("reload TUN: invalid state %s", s.state)
	}
	if s.currentConfig == "" {
		return fmt.Errorf("reload TUN: no configuration stored")
	}
	return s.startWithJSON(s.currentConfig)
}

func (s *Service) SetOverridePackages(overrideJSON string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateRunning {
		return fmt.Errorf("set override packages: invalid state %s", s.state)
	}
	if s.currentConfig == "" {
		return fmt.Errorf("set override packages: no configuration stored")
	}

	var cfg OverrideConfig
	if overrideJSON != "" {
		if err := json.Unmarshal([]byte(overrideJSON), &cfg); err != nil {
			return fmt.Errorf("parse override JSON: %w", err)
		}
	}

	modified, err := injectTunOverride(s.currentConfig, cfg)
	if err != nil {
		return err
	}

	s.overrideAutoRoute = cfg.AutoRedirect
	s.overrideInclude = cfg.IncludePackages
	s.overrideExclude = cfg.ExcludePackages
	return s.startWithJSON(modified)
}

func injectTunOverride(configJSON string, cfg OverrideConfig) (string, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(configJSON), &top); err != nil {
		return "", err
	}

	tunRaw, ok := top["tun"]
	if !ok {
		return configJSON, nil
	}

	var tun map[string]json.RawMessage
	if err := json.Unmarshal(tunRaw, &tun); err != nil {
		return "", err
	}

	if cfg.AutoRedirect {
		tun["auto_redirect"] = json.RawMessage(`true`)
	}
	if len(cfg.IncludePackages) > 0 {
		b, _ := json.Marshal(cfg.IncludePackages)
		tun["include_package"] = json.RawMessage(b)
	}
	if len(cfg.ExcludePackages) > 0 {
		b, _ := json.Marshal(cfg.ExcludePackages)
		tun["exclude_package"] = json.RawMessage(b)
	}

	tunJSON, _ := json.Marshal(tun)
	top["tun"] = json.RawMessage(tunJSON)
	out, _ := json.Marshal(top)
	return string(out), nil
}

// ensureClashModes injects harmless route rules that reference all three base
// clash modes so that sing-box always discovers them.
func ensureClashModes(jsonContent string) string {
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonContent), &top); err != nil {
		return jsonContent
	}
	routeRaw, ok := top["route"]
	if !ok {
		return jsonContent
	}
	var route map[string]json.RawMessage
	if err := json.Unmarshal(routeRaw, &route); err != nil {
		return jsonContent
	}

	type modeRule struct {
		Domain    []string `json:"domain"`
		ClashMode string   `json:"clash_mode"`
		Outbound  string   `json:"outbound"`
	}
	indicators := []modeRule{
		{Domain: []string{"mode-rule.invalid"}, ClashMode: "Rule", Outbound: "direct"},
		{Domain: []string{"mode-direct.invalid"}, ClashMode: "Direct", Outbound: "direct"},
		{Domain: []string{"mode-global.invalid"}, ClashMode: "Global", Outbound: "direct"},
	}

	var rules []json.RawMessage
	if raw, ok := route["rules"]; ok {
		_ = json.Unmarshal(raw, &rules)
	}
	for _, r := range indicators {
		b, _ := json.Marshal(r)
		rules = append(rules, b)
	}

	rulesJSON, _ := json.Marshal(rules)
	route["rules"] = json.RawMessage(rulesJSON)
	routeJSON, _ := json.Marshal(route)
	top["route"] = json.RawMessage(routeJSON)

	out, _ := json.Marshal(top)
	return string(out)
}

// --- Query methods ---

func (s *Service) QueryProxies() string {
	s.mu.Lock()
	inst := s.instance
	s.mu.Unlock()
	if inst == nil {
		return "[]"
	}

	var groups []ProxyGroup
	for _, out := range inst.Outbound().Outbounds() {
		g, ok := out.(adapter.OutboundGroup)
		if !ok {
			continue
		}
		pg := ProxyGroup{
			Tag:        out.Tag(),
			Type:       out.Type(),
			Selected:   g.Now(),
			Selectable: true,
		}
		for _, tag := range g.All() {
			pg.Items = append(pg.Items, ProxyGroupItem{Tag: tag})
		}
		groups = append(groups, pg)
	}
	data, _ := json.Marshal(groups)
	return string(data)
}

func (s *Service) QueryTraffic() string {
	// TODO: implement via NetworkManager statistics
	return "{}"
}

func (s *Service) QueryLogs() string {
	entries := queryCoreLogEntries()
	if len(entries) == 0 {
		return "[]"
	}
	data, _ := json.Marshal(entries)
	return string(data)
}

func (s *Service) QueryConnections() string {
	// TODO: implement via ClashServer ConnectionTracker
	return "[]"
}

func (s *Service) QueryMode() string {
	// TODO: implement via ClashServer Mode/ModeList
	return `{"modes":["Rule","Global","Direct"],"current_mode":"Rule"}`
}

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

// --- Control methods ---

func (s *Service) URLTest(outboundTag string, timeoutMs int32) int32 {
	s.mu.Lock()
	inst := s.instance
	s.mu.Unlock()
	if inst == nil {
		return -1
	}

	out, ok := inst.Outbound().Outbound(outboundTag)
	if !ok {
		return -1
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	delay, err := urltest.URLTest(ctx, "", out)
	if err != nil {
		return -1
	}
	return int32(delay)
}

func (s *Service) SelectOutbound(groupTag, outboundTag string) error {
	s.mu.Lock()
	inst := s.instance
	s.mu.Unlock()
	if inst == nil {
		return fmt.Errorf("service not running")
	}

	out, ok := inst.Outbound().Outbound(groupTag)
	if !ok {
		return fmt.Errorf("group %q not found", groupTag)
	}
	selector, ok := out.(*group.Selector)
	if !ok {
		return fmt.Errorf("%q is not a selectable group", groupTag)
	}
	if !selector.SelectOutbound(outboundTag) {
		return fmt.Errorf("outbound %q not found in group %q", outboundTag, groupTag)
	}
	return nil
}

func (s *Service) SetClashMode(mode string) error {
	// TODO: implement via ClashServer.SetMode
	return nil
}

func (s *Service) CloseConnection(connID string) error {
	// TODO: implement
	return nil
}

func (s *Service) CloseConnections() error {
	// TODO: implement
	return nil
}

func (s *Service) SetGroupExpand(groupTag string, isExpand bool) error {
	// TODO: implement via CacheFile
	return nil
}

func (s *Service) ResetNetwork() {
	s.mu.Lock()
	inst := s.instance
	s.mu.Unlock()
	if inst != nil {
		inst.Network().ResetNetwork()
	}
}

func (s *Service) FlushSystemDNS()    { flushSystemDNS() }
func (s *Service) SetLogLevel(level int32) { SetLogLevel(level) }
