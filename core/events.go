package core

import (
	"context"
	"encoding/json"
	"time"

	"github.com/sagernet/sing-box/experimental/clashapi/trafficontrol"
	"github.com/sagernet/sing/common/observable"
)

// --- Callbacks ---

// SetOnEvent 注册统一事件回调，传 nil 取消注册。回调会同时收到内核 slog 日志
// 与状态/连接/统计事件。可在任意时刻调用（含 Running 态）：
// 订阅始终建立，回调只在发事件时读取，因此 Start 之后注册也能立即收到后续事件。
// 回调会在 core 内部 goroutine 上被调用，实现方需自行保证线程安全且不可长时间阻塞。
func (s *Service) SetOnEvent(fn func(int32, string)) {
	if fn == nil {
		s.onEvent.Store(nil)
	} else {
		s.onEvent.Store(&fn)
	}
	SetOnLogEvent(fn)
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

// subscribeHooks 为 rs 这一代实例注册 Clash 事件订阅。
// rs 必须是刚存入 s.running 的那个实例：安装前会校验它是否仍是当前代，
// 若并发的 Stop 已把它摘除则直接放弃订阅，避免 goroutine 对着已关闭的实例空转。
func (s *Service) subscribeHooks(rs *runningState) {
	srv := s.clashServer()
	if srv == nil {
		return
	}

	s.hooksMu.Lock()
	defer s.hooksMu.Unlock()

	s.cancelHooksLocked()

	// 代次校验：Stop 先摘 running 再退订，因此这里读到的不是自己就说明已被取代。
	if s.running.Load() != rs {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancelFn := func() { cancel() }
	s.subCancel.Store(&cancelFn)

	// 无条件订阅：不能用"启动瞬间是否已注册回调"来决定，否则宿主在 Start 之后
	// 才 SetOnEvent 时，事件在下次重启前永远不会送达。emitEvent 内部已判空，
	// 未注册回调时这些 goroutine 只是空转。
	sub := observable.NewSubscriber[struct{}](8)
	srv.HistoryStorage().SetHook(sub)
	go observe(ctx, sub, func(struct{}) {
		s.emitEvent(EventURLTest, "")
	})

	sub2 := observable.NewSubscriber[struct{}](8)
	srv.SetModeUpdateHook(sub2)
	go observe(ctx, sub2, func(struct{}) {
		s.emitEvent(EventModeUpdate, srv.Mode())
	})

	sub3 := observable.NewSubscriber[trafficontrol.ConnectionEvent](64)
	srv.TrafficManager().SetEventHook(sub3)
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

func (s *Service) observeStats(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.emitEvent(EventStats, s.QueryStats())
		case <-ctx.Done():
			s.emitEvent(EventStats, zeroStatsJSON())
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
	s.hooksMu.Lock()
	defer s.hooksMu.Unlock()
	s.cancelHooksLocked()
}

// cancelHooksLocked 取消当前订阅，调用方必须持有 hooksMu。
func (s *Service) cancelHooksLocked() {
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
