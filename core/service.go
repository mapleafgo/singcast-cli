package core

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/miekg/dns"
	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/urltest"
	"github.com/sagernet/sing-box/experimental/clashapi"
	"github.com/sagernet/sing-box/experimental/clashapi/trafficontrol"
	"github.com/sagernet/sing-box/include"
	singboxlog "github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/group"
	"github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/common/observable"
	"github.com/sagernet/sing/service"

	"github.com/mapleafgo/singcast/translator"
)

var Version = "dev"

// platformLogWriter routes sing-box kernel logs through the unified event callback.
type platformLogWriter struct {
	emit func(eventType int32, json string)
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
	if w.emit != nil {
		data, _ := json.Marshal(LogEntry{
			Level:     coreLevel,
			Message:   message,
			Timestamp: time.Now().UnixMilli(),
		})
		w.emit(EventLog, string(data))
	}
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
	boxCtx        context.Context
	platform      *PlatformIO
	currentConfig string

	onEvent   func(eventType int32, json string)
	subCancel context.CancelFunc
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

	if err := os.MkdirAll(opts.HomeDir, 0o755); err != nil {
		return fmt.Errorf("create home dir: %w", err)
	}
	if err := os.Chdir(opts.HomeDir); err != nil {
		return fmt.Errorf("chdir: %w", err)
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

	if s.state != StateInitialized && s.state != StateRunning {
		s.mu.Unlock()
		return fmt.Errorf("start: invalid state %s", s.state)
	}

	data := []byte(content)
	format := translator.DetectFormat(data)

	jsonContent, err := s.translateConfig(data, format, ruleSetProxy)
	if err != nil {
		s.mu.Unlock()
		return err
	}

	// Close old instance outside lock to avoid deadlock risk.
	oldInst := s.instance
	s.instance = nil
	s.unsubscribeHooks()
	s.mu.Unlock()

	if oldInst != nil {
		oldInst.Close()
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

func (s *Service) startWithJSON(jsonContent string) error {
	syncLogLevelFromConfig(jsonContent)

	ctx := include.Context(context.Background())
	if s.platform.IsMobile() {
		ctx = service.ContextWith[adapter.PlatformInterface](ctx, s.platform)
	}

	options, err := json.UnmarshalExtendedContext[option.Options](ctx, []byte(jsonContent))
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	logWriter := &platformLogWriter{emit: s.getOnEvent()}
	inst, err := box.New(box.Options{Options: options, Context: ctx, PlatformLogWriter: logWriter})
	if err != nil {
		return fmt.Errorf("create instance: %w", err)
	}

	if err := inst.Start(); err != nil {
		inst.Close()
		return fmt.Errorf("start instance: %w", err)
	}

	s.mu.Lock()
	s.instance = inst
	s.boxCtx = ctx
	s.currentConfig = jsonContent
	s.state = StateRunning
	s.subscribeHooks()
	s.mu.Unlock()

	slog.Info("service running")
	return nil
}

func (s *Service) Stop() error {
	s.mu.Lock()
	if s.state != StateRunning {
		s.mu.Unlock()
		return nil
	}

	s.unsubscribeHooks()
	inst := s.instance
	s.instance = nil
	s.state = StateInitialized
	s.mu.Unlock()

	if inst != nil {
		slog.Info("service stopped")
		return inst.Close()
	}
	return nil
}

func (s *Service) Destroy() {
	s.mu.Lock()
	if s.state == StateDestroyed {
		s.mu.Unlock()
		return
	}

	s.unsubscribeHooks()
	inst := s.instance
	s.instance = nil
	s.state = StateDestroyed
	s.mu.Unlock()

	if inst != nil {
		inst.Close()
	}
	slog.Info("service destroyed")
}

// clashServer returns the *clashapi.Server from the running instance, or nil if
// clash_api is not enabled or the service is not running.
func (s *Service) clashServer() *clashapi.Server {
	inst := s.instance
	if inst == nil {
		return nil
	}
	cs := service.FromContext[adapter.ClashServer](s.boxCtx)
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
	s.mu.Lock()
	inst := s.instance
	s.mu.Unlock()
	if inst == nil {
		return "[]"
	}

	srv := s.clashServer()

	var history adapter.URLTestHistoryStorage
	var cache adapter.CacheFile
	if srv != nil {
		history = srv.HistoryStorage()
	}
	cache = service.FromContext[adapter.CacheFile](s.boxCtx)

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
			Selectable: true,
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
	data, _ := json.Marshal(map[string]any{
		"up":          snap.Upload,
		"down":        snap.Download,
		"connections": len(snap.Connections),
		"memory":      snap.Memory,
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
	cf := service.FromContext[adapter.CacheFile](s.boxCtx)
	if cf == nil {
		return fmt.Errorf("cache file not available")
	}
	return cf.StoreGroupExpand(groupTag, isExpand)
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

func (s *Service) QueryRules() string {
	inst := s.instance
	if inst == nil {
		return `{"rules":[]}`
	}
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
	cf := service.FromContext[adapter.CacheFile](s.boxCtx)
	if cf == nil {
		return fmt.Errorf("cache file not available")
	}
	return cf.FakeIPReset()
}

type DNSAnswer struct {
	Name string `json:"name"`
	Type uint16 `json:"type"`
	TTL  uint32 `json:"TTL"`
	Data string `json:"data"`
}

type DNSQueryResult struct {
	Status   uint16         `json:"Status"`
	Question []dns.Question `json:"Question"`
	Answer   []DNSAnswer    `json:"Answer"`
	Server   string         `json:"Server"`
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

func (s *Service) QueryDNS(name string, qType uint16) string {
	dnsRouter := service.FromContext[adapter.DNSRouter](s.boxCtx)
	if dnsRouter == nil {
		return `{"Status":2,"Server":"","Answer":null}`
	}

	msg := dns.Msg{}
	msg.SetQuestion(dns.Fqdn(name), qType)
	msg.RecursionDesired = true

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := dnsRouter.Exchange(ctx, &msg, adapter.DNSQueryOptions{})
	if err != nil {
		data, _ := json.Marshal(DNSQueryResult{Status: 2, Server: "internal"})
		return string(data)
	}

	answers := make([]DNSAnswer, len(resp.Answer))
	for i, rr := range resp.Answer {
		answers[i] = DNSAnswer{
			Name: rr.Header().Name,
			Type: rr.Header().Rrtype,
			TTL:  rr.Header().Ttl,
			Data: dnsFieldData(rr),
		}
	}

	data, _ := json.Marshal(DNSQueryResult{
		Status:   uint16(resp.Rcode),
		Question: msg.Question,
		Answer:   answers,
		Server:   "internal",
	})
	return string(data)
}

func dnsFieldData(rr dns.RR) string {
	switch v := rr.(type) {
	case *dns.A:
		return v.A.String()
	case *dns.AAAA:
		return v.AAAA.String()
	case *dns.CNAME:
		return v.Target
	case *dns.MX:
		return fmt.Sprintf("%d %s", v.Preference, v.Mx)
	case *dns.TXT:
		if len(v.Txt) > 0 {
			return v.Txt[0]
		}
		return ""
	case *dns.NS:
		return v.Ns
	case *dns.PTR:
		return v.Ptr
	case *dns.SRV:
		return fmt.Sprintf("%d %d %d %s", v.Priority, v.Weight, v.Port, v.Target)
	default:
		return ""
	}
}

// FlushDNSCache clears the internal DNS query cache.
func (s *Service) FlushDNSCache() error {
	dnsRouter := service.FromContext[adapter.DNSRouter](s.boxCtx)
	if dnsRouter == nil {
		return fmt.Errorf("DNS router not available")
	}
	dnsRouter.ClearCache()
	return nil
}

// TestGroupDelay runs URL tests for all outbounds in a group.
// Returns a JSON map of {tag: delay_ms}. -1 means failure/timeout.
func (s *Service) TestGroupDelay(groupTag string, timeoutMs int32) string {
	s.mu.Lock()
	inst := s.instance
	s.mu.Unlock()
	if inst == nil {
		return "{}"
	}

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
	s.onEvent = fn
}

func (s *Service) getOnEvent() func(int32, string) {
	return s.onEvent
}

// subscribeHooks registers observable hooks on the ClashServer.
// Must be called with s.mu held.
func (s *Service) subscribeHooks() {
	s.unsubscribeHooks()

	srv := s.clashServer()
	if srv == nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.subCancel = cancel

	if fn := s.getOnEvent(); fn != nil {
		clashSrv := srv

		sub := observable.NewSubscriber[struct{}](8)
		clashSrv.HistoryStorage().SetHook(sub)
		go observe(ctx, sub, func(struct{}) { fn(EventURLTest, "") })

		sub2 := observable.NewSubscriber[struct{}](8)
		clashSrv.SetModeUpdateHook(sub2)
		go observe(ctx, sub2, func(struct{}) { fn(EventModeUpdate, clashSrv.Mode()) })

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
			fn(EventConnEvent, string(data))
		})
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
// Must be called with s.mu held.
func (s *Service) unsubscribeHooks() {
	if s.subCancel != nil {
		s.subCancel()
		s.subCancel = nil
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
