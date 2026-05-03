package core

import (
	"log/slog"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

var (
	oomMinInterval       = 500 * time.Millisecond
	oomMaxInterval       = 10 * time.Second
	oomChecksBeforeLimit = 4
)

// oomMonitor adaptively polls memory usage and takes corrective action
// when the soft memory limit is approached. The algorithm follows sing-box's
// adaptiveTimer: interval adjusts based on memory growth rate.
//
// Constraint: onReset is called from the monitor goroutine without holding
// any Service lock. It MUST NOT acquire Service.mu (deadlock risk).
type oomMonitor struct {
	limit   atomic.Int64 // soft memory limit in bytes (0 = disabled)
	running atomic.Bool
	onReset func() // called asynchronously when memory limit is breached

	mu                  sync.Mutex
	cancel              func()
	lastUsage           uint64
	lastInterval        time.Duration
	previouslyTriggered bool
}

// newOOMMonitor creates a monitor. It does not start until setLimit is called.
func newOOMMonitor(onReset func()) *oomMonitor {
	return &oomMonitor{onReset: onReset}
}

// setLimit updates the soft memory limit and starts/stops the monitor goroutine.
func (m *oomMonitor) setLimit(bytes int64) {
	m.limit.Store(bytes)
	if bytes > 0 && m.running.CompareAndSwap(false, true) {
		m.mu.Lock()
		ctx := newCancelCtx()
		m.cancel = ctx.cancel
		m.lastInterval = oomMaxInterval
		m.lastUsage = readMemStats()
		m.previouslyTriggered = false
		m.mu.Unlock()
		go m.run(ctx.done)
	}
	if bytes <= 0 && m.running.CompareAndSwap(true, false) {
		m.mu.Lock()
		if m.cancel != nil {
			m.cancel()
			m.cancel = nil
		}
		m.mu.Unlock()
	}
}

// stop terminates the monitor goroutine.
func (m *oomMonitor) stop() {
	m.running.Store(false)
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.mu.Unlock()
}

type cancelCtx struct {
	done   <-chan struct{}
	cancel func()
}

func newCancelCtx() cancelCtx {
	ch := make(chan struct{})
	var once sync.Once
	return cancelCtx{
		done:   ch,
		cancel: func() { once.Do(func() { close(ch) }) },
	}
}

func (m *oomMonitor) run(done <-chan struct{}) {
	m.mu.Lock()
	initialInterval := m.lastInterval
	m.mu.Unlock()

	timer := time.NewTimer(initialInterval)
	defer timer.Stop()

	for {
		select {
		case <-done:
			return
		case <-timer.C:
		}

		if !m.running.Load() {
			return
		}

		usage := readMemStats()
		limit := uint64(m.limit.Load())
		if limit == 0 {
			m.running.Store(false)
			return
		}

		triggered := usage >= limit
		remaining := limit - usage
		delta := int64(usage) - int64(m.lastUsage)

		// Single lock scope: update all state, compute interval, decide action.
		m.mu.Lock()
		m.lastUsage = usage

		// Determine action under lock.
		firstTrigger := triggered && !m.previouslyTriggered
		if triggered {
			m.previouslyTriggered = true
		} else {
			m.previouslyTriggered = false
		}

		// Compute adaptive interval.
		var interval time.Duration
		if triggered || delta <= 0 {
			interval = oomMaxInterval
		} else {
			ratio := float64(remaining) / float64(delta)
			maxRatio := float64(oomMaxInterval) / float64(m.lastInterval)
			if ratio > maxRatio {
				ratio = maxRatio
			}
			interval = time.Duration(ratio * float64(m.lastInterval))
			interval /= time.Duration(oomChecksBeforeLimit)
			if interval < oomMinInterval {
				interval = oomMinInterval
			}
			if interval > oomMaxInterval {
				interval = oomMaxInterval
			}
		}
		m.lastInterval = interval
		m.mu.Unlock()

		// Execute actions outside lock.
		if firstTrigger {
			slog.Error("memory limit reached, resetting network", "usage_mb", usage/(1024*1024), "limit_mb", limit/(1024*1024))
			if m.onReset != nil {
				go m.onReset()
			}
			runtime.GC()
			debug.FreeOSMemory()
		} else if triggered {
			runtime.GC()
			debug.FreeOSMemory()
		}

		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(interval)
	}
}

// readMemStatsFn is the function used to read memory stats. Overridable for testing.
var readMemStatsFn = realReadMemStats

// readMemStats returns the effective memory usage for OOM decisions.
func readMemStats() uint64 { return readMemStatsFn() }

// realReadMemStats returns HeapInuse + StackInuse.
func realReadMemStats() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapInuse + m.StackInuse
}
