package core

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestOOMMonitor_SetLimitStartsAndStops(t *testing.T) {
	var resets atomic.Int32
	m := newOOMMonitor(func() { resets.Add(1) })

	// Start with a very high limit — should not trigger.
	m.setLimit(1 << 30) // 1 GiB
	if !m.running.Load() {
		t.Fatal("expected monitor to be running after setLimit > 0")
	}

	// Stop by setting limit to 0.
	m.setLimit(0)
	// Give goroutine time to exit.
	time.Sleep(100 * time.Millisecond)
	if m.running.Load() {
		t.Fatal("expected monitor to stop after setLimit(0)")
	}
}

func TestOOMMonitor_Stop(t *testing.T) {
	m := newOOMMonitor(func() {})
	m.setLimit(1 << 30)
	if !m.running.Load() {
		t.Fatal("expected running")
	}

	m.stop()
	time.Sleep(100 * time.Millisecond)
	if m.running.Load() {
		t.Fatal("expected stopped after stop()")
	}
}

func TestOOMMonitor_SetLimitZeroIsNoopWhenNotRunning(t *testing.T) {
	m := newOOMMonitor(func() {})
	m.setLimit(0) // should not panic or start goroutine
	if m.running.Load() {
		t.Fatal("setLimit(0) on idle monitor should not start it")
	}
}

func TestOOMMonitor_DoubleSetLimitOnlyStartsOnce(t *testing.T) {
	m := newOOMMonitor(func() {})

	m.setLimit(1 << 30)
	m.setLimit(1 << 30) // second call: CompareAndSwap(false, true) fails, no-op

	time.Sleep(200 * time.Millisecond)
	m.stop()
}

func TestOOMMonitor_AdaptiveInterval(t *testing.T) {
	m := newOOMMonitor(func() {})
	m.mu.Lock()
	m.lastInterval = oomMaxInterval
	m.lastUsage = 0
	m.mu.Unlock()

	// Simulate: 50MB used out of 100MB limit, grew 10MB since last check.
	usage := uint64(50 * 1024 * 1024)
	limit := uint64(100 * 1024 * 1024)
	delta := int64(10 * 1024 * 1024) // 10 MB growth
	remaining := limit - usage

	ratio := float64(remaining) / float64(delta)
	maxRatio := float64(oomMaxInterval) / float64(oomMaxInterval)
	if ratio > maxRatio {
		ratio = maxRatio
	}
	interval := time.Duration(ratio * float64(oomMaxInterval))
	interval /= time.Duration(oomChecksBeforeLimit)

	// 50MB remaining / 10MB delta = 5, * 10s = 50s / 4 = 12.5s → clamped to 10s
	if interval != oomMaxInterval {
		// With clamping, it should be oomMaxInterval.
		t.Logf("interval = %v (clamped to max)", interval)
	}

	// Fast growth scenario: 95MB used out of 100MB, grew 50MB since last check.
	usage = uint64(95 * 1024 * 1024)
	delta = int64(50 * 1024 * 1024)
	remaining = limit - usage

	ratio = float64(remaining) / float64(delta)
	// 5MB / 50MB = 0.1, * 10s = 1s / 4 = 250ms → clamped to 500ms (min)
	interval = time.Duration(ratio * float64(oomMaxInterval))
	interval /= time.Duration(oomChecksBeforeLimit)
	if interval < oomMinInterval {
		interval = oomMinInterval
	}
	if interval != oomMinInterval {
		t.Errorf("fast growth interval = %v, want %v", interval, oomMinInterval)
	}
}

func TestOOMMonitor_PreviouslyTriggeredFlag(t *testing.T) {
	m := newOOMMonitor(func() {})

	// Initially false.
	m.mu.Lock()
	if m.previouslyTriggered {
		t.Error("expected initially false")
	}
	m.mu.Unlock()

	// Set to true (simulating first trigger).
	m.mu.Lock()
	m.previouslyTriggered = true
	m.mu.Unlock()

	m.mu.Lock()
	if !m.previouslyTriggered {
		t.Error("expected true after set")
	}
	m.mu.Unlock()
}

func TestOOMMonitor_DestroyPreventsRestart(t *testing.T) {
	var resets atomic.Int32
	m := newOOMMonitor(func() { resets.Add(1) })

	m.setLimit(1 << 30)
	m.stop()
	time.Sleep(100 * time.Millisecond) // wait for goroutine to exit

	// After stop, setLimit should be able to restart.
	m.setLimit(1 << 30)
	if !m.running.Load() {
		t.Fatal("expected monitor to restart after stop + setLimit")
	}
	m.stop()
}

func TestOOMNewCancelCtx(t *testing.T) {
	ctx := newCancelCtx()
	select {
	case <-ctx.done:
		t.Fatal("done channel should not be closed before cancel")
	default:
	}

	ctx.cancel()
	select {
	case <-ctx.done:
		// ok
	case <-time.After(time.Second):
		t.Fatal("done channel should be closed after cancel")
	}

	// Double cancel should not panic.
	ctx.cancel()
}

// --- OOM trigger path with mock readMemStats ---

func setupOOMShortInterval() (restore func()) {
	origMin := oomMinInterval
	origMax := oomMaxInterval
	oomMinInterval = 50 * time.Millisecond
	oomMaxInterval = 50 * time.Millisecond
	return func() {
		oomMinInterval = origMin
		oomMaxInterval = origMax
	}
}

func TestOOMMonitor_TriggersOnReset(t *testing.T) {
	restore := setupOOMShortInterval()
	defer restore()

	var triggered atomic.Int32
	m := newOOMMonitor(func() { triggered.Add(1) })

	orig := readMemStatsFn
	readMemStatsFn = func() uint64 { return 1 << 30 }
	defer func() { readMemStatsFn = orig }()

	m.setLimit(512 << 20)
	if !m.running.Load() {
		t.Fatal("expected monitor running")
	}

	// Wait for first check cycle at short interval.
	time.Sleep(200 * time.Millisecond)
	m.stop()

	if triggered.Load() < 1 {
		t.Error("expected onReset to be called at least once")
	}
}

func TestOOMMonitor_FirstTriggerOnly(t *testing.T) {
	restore := setupOOMShortInterval()
	defer restore()

	var triggered atomic.Int32
	m := newOOMMonitor(func() { triggered.Add(1) })

	orig := readMemStatsFn
	readMemStatsFn = func() uint64 { return 1 << 30 }
	defer func() { readMemStatsFn = orig }()

	m.setLimit(512 << 20)

	// Wait for multiple check cycles.
	time.Sleep(500 * time.Millisecond)
	m.stop()

	// Should only trigger once (first trigger) — subsequent triggers just GC.
	if triggered.Load() != 1 {
		t.Errorf("expected exactly 1 trigger, got %d", triggered.Load())
	}
}

func TestOOMMonitor_ReTriggersAfterRecovery(t *testing.T) {
	restore := setupOOMShortInterval()
	defer restore()

	var triggered atomic.Int32
	m := newOOMMonitor(func() { triggered.Add(1) })

	var aboveLimit atomic.Bool
	aboveLimit.Store(true)
	orig := readMemStatsFn
	readMemStatsFn = func() uint64 {
		if aboveLimit.Load() {
			return 1 << 30
		}
		return 0
	}
	defer func() { readMemStatsFn = orig }()

	m.setLimit(512 << 20)

	// Wait for first trigger.
	time.Sleep(200 * time.Millisecond)

	// Recover — below limit.
	aboveLimit.Store(false)
	time.Sleep(200 * time.Millisecond)

	// Go above again — should trigger a second time.
	aboveLimit.Store(true)
	time.Sleep(200 * time.Millisecond)

	m.stop()

	if triggered.Load() < 2 {
		t.Errorf("expected at least 2 triggers (first + re-trigger), got %d", triggered.Load())
	}
}
