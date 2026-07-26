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
	C "github.com/sagernet/sing-box/constant"
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
			Selectable: out.Type() == C.TypeSelector,
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

// URLTest 测量单个出站的延迟（毫秒）。服务未运行、tag 不存在或测试
// 失败/超时时返回 -1。服务在测试期间被停止时提前返回 -1。
func (s *Service) URLTest(outboundTag string, timeoutMs int32) int32 {
	rs := s.running.Load()
	if rs == nil {
		return -1
	}

	out, ok := rs.instance.Outbound().Outbound(outboundTag)
	if !ok {
		return -1
	}

	ctx, cancel := context.WithTimeout(rs.stopCtx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	delay, err := urltest.URLTest(ctx, "", out)
	if err != nil {
		return -1
	}
	return int32(delay)
}

// groupTestConcurrency 限制全组测速的并发数。机场订阅常有 300+ 节点，
// 不限并发会瞬间建立同等数量的 TLS 连接，在移动端足以耗尽 fd。
const groupTestConcurrency = 16

// TestGroupDelay runs URL tests for all outbounds in a group.
// Returns a JSON map of {tag: delay_ms}. -1 means failure/timeout.
//
// 并发上限为 groupTestConcurrency，但 timeoutMs 是**整批**的墙钟上限而非每个
// 节点各自的上限：调用方（IPC handler、移动端）是同步等待这个结果的，
// 若每个节点各自计时，300 个节点在 16 并发下最坏要 ceil(300/16)×timeoutMs，
// GUI 会像卡死一样等上一分多钟。没轮到或未完成的节点按超时记为 -1。
// 服务在测试期间被停止时同样立即收敛。
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

	// 整批共用一个 deadline，保证总耗时不超过 timeoutMs。
	ctx, cancel := context.WithTimeout(rs.stopCtx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, groupTestConcurrency)

	for _, tag := range tags {
		// 在派生 goroutine 前占槽：放到 goroutine 里占会先创建全部 goroutine，
		// 限流效果只作用于测速本身，白白付出 N 个 goroutine 的开销。
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			// 已超时/已停止，余下节点保持 -1
			mu.Lock()
			results[tag] = -1
			mu.Unlock()
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			delay := int32(-1)
			if ob, ok := inst.Outbound().Outbound(tag); ok {
				if d, err := urltest.URLTest(ctx, "", ob); err == nil {
					delay = int32(d)
				}
			}
			mu.Lock()
			results[tag] = delay
			mu.Unlock()
		}()
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
