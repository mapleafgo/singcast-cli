package core

import (
	"context"
	"encoding/json"
	"time"

	"github.com/sagernet/sing-box/experimental/clashapi/trafficontrol"
	"github.com/sagernet/sing/common/observable"
)

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
