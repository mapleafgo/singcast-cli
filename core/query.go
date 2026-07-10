package core

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/urltest"
	"github.com/sagernet/sing-box/experimental/clashapi"
	"github.com/sagernet/sing-box/protocol/group"
	"github.com/sagernet/sing/service"
)

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
			// Override type for stub nodes (unsupported protocols) so the UI
			// can distinguish them from real socks nodes.
			if origType, isStub := rs.stubTags[tag]; isStub {
				item.Type = "unsupported:" + origType
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
		return zeroStatsJSON()
	}
	snap := srv.TrafficManager().Snapshot()
	var startedAt int64
	if rs := s.running.Load(); rs != nil {
		startedAt = rs.startedAt
	}
	data, _ := json.Marshal(StatsSnapshot{
		Up:          snap.Upload,
		Down:        snap.Download,
		Connections: len(snap.Connections),
		Memory:      snap.Memory,
		StartedAt:   startedAt,
	})
	return string(data)
}

func zeroStatsJSON() string {
	data, _ := json.Marshal(StatsSnapshot{})
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
		data, _ := json.Marshal(ModeInfo{
			Modes:       []string{"Rule", "Global", "Direct"},
			CurrentMode: "Rule",
		})
		return string(data)
	}
	data, _ := json.Marshal(ModeInfo{
		Modes:       srv.ModeList(),
		CurrentMode: srv.Mode(),
	})
	return string(data)
}

func (s *Service) QueryRules() string {
	rs := s.running.Load()
	if rs == nil {
		data, _ := json.Marshal(RulesInfo{Rules: []ruleEntry{}})
		return string(data)
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
	data, _ := json.Marshal(RulesInfo{Rules: entries})
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

func (s *Service) ResetNetwork() {
	if rs := s.running.Load(); rs != nil {
		rs.instance.Router().ResetNetwork()
	}
}

func (s *Service) FlushSystemDNS()         { flushSystemDNS(context.Background()) }
func (s *Service) SetLogLevel(level int32) { SetLogLevel(level) }

// TriggerGC forces a garbage collection.
func (s *Service) TriggerGC() {
	runtime.GC()
}
