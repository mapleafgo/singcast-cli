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
	"github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/urltest"
	"github.com/sagernet/sing-box/experimental/clashapi"
	"github.com/sagernet/sing-box/experimental/clashapi/trafficontrol"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/group"
	"github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/common/observable"
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
	boxCtx        context.Context
	platform      *PlatformIO
	currentConfig string

	overrideAutoRoute bool
	overrideInclude   []string
	overrideExclude   []string

	onURLTestUpdate func()
	onModeUpdate    func(mode string)
	onConnEvent     func(eventType int32, connJSON string)
	subCancel       context.CancelFunc
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
	s.boxCtx = ctx
	s.currentConfig = jsonContent
	s.state = StateRunning
	s.subscribeHooks()
	slog.Info("service running")
	return nil
}

func (s *Service) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateRunning {
		return nil
	}

	s.unsubscribeHooks()
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

	s.unsubscribeHooks()
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

func (s *Service) QueryTraffic() string {
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

func (s *Service) QueryLogs() string {
	entries := queryCoreLogEntries()
	if len(entries) == 0 {
		return "[]"
	}
	data, _ := json.Marshal(entries)
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

func (s *Service) SetOnURLTestUpdate(fn func())             { s.onURLTestUpdate = fn }
func (s *Service) SetOnModeUpdate(fn func(mode string))     { s.onModeUpdate = fn }
func (s *Service) SetOnConnEvent(fn func(int32, string))    { s.onConnEvent = fn }

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

	if s.onURLTestUpdate != nil {
		sub := observable.NewSubscriber[struct{}](8)
		srv.HistoryStorage().SetHook(sub)
		fn := s.onURLTestUpdate
		go observe(ctx, sub, func(struct{}) { fn() })
	}

	if s.onModeUpdate != nil {
		sub := observable.NewSubscriber[struct{}](8)
		srv.SetModeUpdateHook(sub)
		srv := srv
		fn := s.onModeUpdate
		go observe(ctx, sub, func(struct{}) { fn(srv.Mode()) })
	}

	if s.onConnEvent != nil {
		sub := observable.NewSubscriber[trafficontrol.ConnectionEvent](64)
		srv.TrafficManager().SetEventHook(sub)
		fn := s.onConnEvent
		go observe(ctx, sub, func(evt trafficontrol.ConnectionEvent) {
			meta := evt.Metadata
			if meta == nil {
				return
			}
			data, _ := json.Marshal(trackerToEntry(meta))
			fn(int32(evt.Type), string(data))
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
