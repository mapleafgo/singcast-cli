package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofrs/uuid/v5"
	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/urltest"
	"github.com/sagernet/sing-box/experimental/clashapi"
	"github.com/sagernet/sing-box/experimental/clashapi/trafficontrol"
	"github.com/sagernet/sing-box/include"
	singboxlog "github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/group"
	singjson "github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/common/observable"
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
	data, _ := json.Marshal(map[string]string{"version": Version, "core": "singcast"})
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
func (a *atomicState) CompareAndSwap(old, new State) bool {
	return a.v.CompareAndSwap(int32(old), int32(new))
}

type runningState struct {
	instance      *box.Box
	boxCtx        context.Context
	currentConfig string
	startedAt     int64
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
	jsonContent, err := s.translateConfig(data, translator.DetectFormat(data), ruleSetProxy)
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

	if err := s.startWithJSON(jsonContent); err != nil {
		s.casState(StateStarting, StateInitialized)
		return err
	}
	return nil
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
	if ruleSetProxy != "" {
		return applyRuleSetProxy(data, ruleSetProxy)
	}
	return string(data), nil
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

func (s *Service) startWithJSON(jsonContent string) error {
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

// clashServer returns the *clashapi.Server from the running instance, or nil if
// clash_api is not enabled or the service is not running.
func (s *Service) clashServer() *clashapi.Server {
	rs := s.running.Load()
	if rs == nil {
		return nil
	}
	cs := service.FromContext[adapter.ClashServer](rs.boxCtx)
	if cs == nil {
		return nil
	}
	srv, ok := cs.(*clashapi.Server)
	if !ok {
		return nil
	}
	return srv
}

func (s *Service) QueryProxies() string {
	rs := s.running.Load()
	if rs == nil {
		return "[]"
	}
	inst := rs.instance

	srv := s.clashServer()

	var history adapter.URLTestHistoryStorage
	var cache adapter.CacheFile
	if srv != nil {
		history = srv.HistoryStorage()
	}
	cache = service.FromContext[adapter.CacheFile](rs.boxCtx)

	var groups []ProxyGroup
	for _, out := range inst.Outbound().Outbounds() {
		g, ok := out.(adapter.OutboundGroup)
		if !ok {
			continue
		}
		tag := out.Tag()
		pg := ProxyGroup{
			Tag:        tag,
			Type:       out.Type(),
			Selected:   g.Now(),
			Selectable: out.Type() == "selector",
		}
		if cache != nil {
			if expand, loaded := cache.LoadGroupExpand(tag); loaded {
				pg.Expand = expand
			}
		}
		for _, tag := range g.All() {
			item := ProxyGroupItem{Tag: tag}
			if ob, ok := inst.Outbound().Outbound(tag); ok {
				item.Type = ob.Type()
			}
			if history != nil {
				if h := history.LoadURLTestHistory(tag); h != nil {
					item.Delay = int32(h.Delay)
				}
			}
			pg.Items = append(pg.Items, item)
		}
		groups = append(groups, pg)
	}
	data, _ := json.Marshal(groups)
	return string(data)
}

func (s *Service) QueryStats() string {
	srv := s.clashServer()
	if srv == nil {
		return `{"up":0,"down":0,"connections":0,"memory":0}`
	}
	snap := srv.TrafficManager().Snapshot()
	var startedAt int64
	if rs := s.running.Load(); rs != nil {
		startedAt = rs.startedAt
	}
	data, _ := json.Marshal(map[string]any{
		"up":          snap.Upload,
		"down":        snap.Download,
		"connections": len(snap.Connections),
		"memory":      snap.Memory,
		"started_at":  startedAt,
	})
	return string(data)
}

func (s *Service) QueryConnections() string {
	srv := s.clashServer()
	if srv == nil {
		return "[]"
	}
	conns := srv.TrafficManager().Connections()
	if len(conns) == 0 {
		return "[]"
	}

	entries := make([]connEntry, 0, len(conns))
	for _, meta := range conns {
		entries = append(entries, trackerToEntry(meta))
	}
	data, _ := json.Marshal(entries)
	return string(data)
}

func (s *Service) QueryMode() string {
	srv := s.clashServer()
	if srv == nil {
		return `{"modes":["Rule","Global","Direct"],"current_mode":"Rule"}`
	}
	data, _ := json.Marshal(map[string]any{
		"modes":        srv.ModeList(),
		"current_mode": srv.Mode(),
	})
	return string(data)
}

func (s *Service) URLTest(outboundTag string, timeoutMs int32) int32 {
	rs := s.running.Load()
	if rs == nil {
		return -1
	}

	out, ok := rs.instance.Outbound().Outbound(outboundTag)
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
	rs := s.running.Load()
	if rs == nil {
		return fmt.Errorf("service not running")
	}
	inst := rs.instance

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

func (s *Service) SetMode(mode string) error {
	srv := s.clashServer()
	if srv == nil {
		return fmt.Errorf("clash API not available")
	}
	srv.SetMode(mode)
	return nil
}

func (s *Service) CloseConnection(connID string) error {
	srv := s.clashServer()
	if srv == nil {
		return fmt.Errorf("clash API not available")
	}
	id := uuid.FromStringOrNil(connID)
	if id == uuid.Nil {
		return fmt.Errorf("invalid connection ID: %s", connID)
	}
	tracker := srv.TrafficManager().Connection(id)
	if tracker == nil {
		return fmt.Errorf("connection %s not found", connID)
	}
	return tracker.Close()
}

func (s *Service) CloseConnections() error {
	srv := s.clashServer()
	if srv == nil {
		return fmt.Errorf("clash API not available")
	}
	srv.TrafficManager().ResetStatistic()
	s.ResetNetwork()
	return nil
}

func (s *Service) SetGroupExpand(groupTag string, isExpand bool) error {
	rs := s.running.Load()
	if rs == nil {
		return fmt.Errorf("service not running")
	}
	cf := service.FromContext[adapter.CacheFile](rs.boxCtx)
	if cf == nil {
		return fmt.Errorf("cache file not available")
	}
	return cf.StoreGroupExpand(groupTag, isExpand)
}

func (s *Service) ResetNetwork() {
	if rs := s.running.Load(); rs != nil {
		rs.instance.Router().ResetNetwork()
	}
}

func (s *Service) FlushSystemDNS()         { flushSystemDNS() }
func (s *Service) SetLogLevel(level int32) { SetLogLevel(level) }

func (s *Service) QueryRules() string {
	rs := s.running.Load()
	if rs == nil {
		return `{"rules":[]}`
	}
	inst := rs.instance
	rules := inst.Router().Rules()
	entries := make([]ruleEntry, len(rules))
	for i, r := range rules {
		entries[i] = ruleEntry{
			Type:    r.Type(),
			Payload: r.String(),
			Proxy:   r.Action().String(),
		}
	}
	data, _ := json.Marshal(map[string]any{"rules": entries})
	return string(data)
}

func (s *Service) FlushFakeIP() error {
	rs := s.running.Load()
	if rs == nil {
		return fmt.Errorf("service not running")
	}
	cf := service.FromContext[adapter.CacheFile](rs.boxCtx)
	if cf == nil {
		return fmt.Errorf("cache file not available")
	}
	return cf.FakeIPReset()
}

type connEntry struct {
	Event       int32  `json:"event"`
	ID          string `json:"id"`
	Network     string `json:"network"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Domain      string `json:"domain,omitempty"`
	Outbound    string `json:"outbound"`
	Rule        string `json:"rule,omitempty"`
	Upload      int64  `json:"upload"`
	Download    int64  `json:"download"`
	Start       string `json:"start"`
}

type ruleEntry struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
	Proxy   string `json:"proxy"`
}

// FlushDNSCache clears the internal DNS query cache.
func (s *Service) FlushDNSCache() error {
	rs := s.running.Load()
	if rs == nil {
		return fmt.Errorf("service not running")
	}
	dnsRouter := service.FromContext[adapter.DNSRouter](rs.boxCtx)
	if dnsRouter == nil {
		return fmt.Errorf("DNS router not available")
	}
	dnsRouter.ClearCache()
	return nil
}

// TestGroupDelay runs URL tests for all outbounds in a group.
// Returns a JSON map of {tag: delay_ms}. -1 means failure/timeout.
func (s *Service) TestGroupDelay(groupTag string, timeoutMs int32) string {
	rs := s.running.Load()
	if rs == nil {
		return "{}"
	}
	inst := rs.instance

	out, ok := inst.Outbound().Outbound(groupTag)
	if !ok {
		return "{}"
	}
	g, ok := out.(adapter.OutboundGroup)
	if !ok {
		return "{}"
	}

	tags := g.All()
	results := make(map[string]int32, len(tags))

	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(len(tags))

	for _, tag := range tags {
		go func(tag string) {
			defer wg.Done()
			ob, ok := inst.Outbound().Outbound(tag)
			if !ok {
				mu.Lock()
				results[tag] = -1
				mu.Unlock()
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
			defer cancel()
			delay, err := urltest.URLTest(ctx, "", ob)
			mu.Lock()
			if err != nil {
				results[tag] = -1
			} else {
				results[tag] = int32(delay)
			}
			mu.Unlock()
		}(tag)
	}
	wg.Wait()

	data, _ := json.Marshal(results)
	return string(data)
}

// TriggerGC forces a garbage collection.
func (s *Service) TriggerGC() {
	runtime.GC()
}

// --- Callbacks ---

func (s *Service) SetOnEvent(fn func(int32, string)) {
	if fn == nil {
		s.onEvent.Store(nil)
	} else {
		s.onEvent.Store(&fn)
	}
}

func (s *Service) getOnEvent() func(int32, string) {
	if p := s.onEvent.Load(); p != nil {
		return *p
	}
	return nil
}

func (s *Service) emitEvent(event int32, data string) {
	if fn := s.getOnEvent(); fn != nil {
		fn(event, data)
	}
}

func (s *Service) emitState(state State) {
	s.emitEvent(EventStateChange, state.String())
}

func (s *Service) casState(oldState, newState State) bool {
	if s.state.CompareAndSwap(oldState, newState) {
		s.emitState(newState)
		return true
	}
	return false
}

// subscribeHooks registers observable hooks on the ClashServer.
func (s *Service) subscribeHooks() {
	s.unsubscribeHooks()

	srv := s.clashServer()
	if srv == nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancelFn := func() { cancel() }
	s.subCancel.Store(&cancelFn)

	if s.getOnEvent() != nil {
		clashSrv := srv

		sub := observable.NewSubscriber[struct{}](8)
		clashSrv.HistoryStorage().SetHook(sub)
		go observe(ctx, sub, func(struct{}) {
			s.emitEvent(EventURLTest, "")
		})

		sub2 := observable.NewSubscriber[struct{}](8)
		clashSrv.SetModeUpdateHook(sub2)
		go observe(ctx, sub2, func(struct{}) {
			s.emitEvent(EventModeUpdate, clashSrv.Mode())
		})

		sub3 := observable.NewSubscriber[trafficontrol.ConnectionEvent](64)
		clashSrv.TrafficManager().SetEventHook(sub3)
		go observe(ctx, sub3, func(evt trafficontrol.ConnectionEvent) {
			meta := evt.Metadata
			if meta == nil {
				return
			}
			entry := trackerToEntry(meta)
			entry.Event = int32(evt.Type)
			data, _ := json.Marshal(entry)
			s.emitEvent(EventConnEvent, string(data))
		})

		go s.observeStats(ctx)
	}
}

func (s *Service) observeStats(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.emitEvent(EventStats, s.QueryStats())
		case <-ctx.Done():
			stats, _ := json.Marshal(map[string]any{
				"up": 0, "down": 0, "connections": 0, "memory": 0, "started_at": int64(0),
			})
			s.emitEvent(EventStats, string(stats))
			return
		}
	}
}

// observe listens on a subscriber until ctx is cancelled or the subscription closes.
func observe[T any](ctx context.Context, sub *observable.Subscriber[T], fn func(T)) {
	ch, done := sub.Subscription()
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			sub.Close()
			return
		case v, ok := <-ch:
			if !ok {
				return
			}
			fn(v)
		}
	}
}

// unsubscribeHooks cancels all subscription goroutines.
func (s *Service) unsubscribeHooks() {
	if cancelPtr := s.subCancel.Swap(nil); cancelPtr != nil {
		(*cancelPtr)()
	}
}

func trackerToEntry(meta *trafficontrol.TrackerMetadata) connEntry {
	domain := meta.Metadata.Domain
	if domain == "" {
		domain = meta.Metadata.Destination.Fqdn
	}
	var ruleStr string
	if meta.Rule != nil {
		ruleStr = meta.Rule.String()
	}
	return connEntry{
		ID:          meta.ID.String(),
		Network:     meta.Metadata.Network,
		Source:      meta.Metadata.Source.String(),
		Destination: meta.Metadata.Destination.String(),
		Domain:      domain,
		Outbound:    meta.Outbound,
		Rule:        ruleStr,
		Upload:      meta.Upload.Load(),
		Download:    meta.Download.Load(),
		Start:       meta.CreatedAt.Format(time.RFC3339),
	}
}
