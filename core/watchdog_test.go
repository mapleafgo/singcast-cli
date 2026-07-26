package core

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeHealthConfig_Defaults(t *testing.T) {
	c := normalizeHealthConfig(nil)
	assert.Equal(t, defaultHealthInterval, c.interval)
	assert.Equal(t, defaultHealthTimeout, c.timeout)
	assert.Equal(t, defaultHealthFailCount, c.failThreshold)
	assert.Equal(t, defaultHealthCooldown, c.cooldown)
}

func TestNormalizeHealthConfig_Override(t *testing.T) {
	in := &HealthCheckConfig{
		Interval:      5000,
		Timeout:       2000,
		FailThreshold: 5,
		Cooldown:      60000,
	}
	c := normalizeHealthConfig(in)
	assert.Equal(t, 5*time.Second, c.interval)
	assert.Equal(t, 2*time.Second, c.timeout)
	assert.Equal(t, 5, c.failThreshold)
	assert.Equal(t, time.Minute, c.cooldown)
}

// 健康时看门狗永远不应触发 restart。
func TestHealthWatchdog_HealthyNeverRestarts(t *testing.T) {
	var restarts atomic.Int32
	wd := &healthWatchdog{
		interval:  3 * time.Millisecond,
		threshold: 2,
		cooldown:  time.Second,
		probe:     func(context.Context) bool { return true },
		restart:   func() { restarts.Add(1) },
	}
	ctx, cancel := context.WithCancel(context.Background())
	go wd.run(ctx)

	time.Sleep(60 * time.Millisecond) // 足够多次 tick
	cancel()

	assert.Equal(t, int32(0), restarts.Load(), "restart must not fire when healthy")
}

// 持续失败达到阈值后应触发 restart。
func TestHealthWatchdog_RestartsOnSustainedFailure(t *testing.T) {
	var restarts atomic.Int32
	wd := &healthWatchdog{
		interval:  3 * time.Millisecond,
		threshold: 3,
		cooldown:  time.Second,
		probe:     func(context.Context) bool { return false },
		restart:   func() { restarts.Add(1) },
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go wd.run(ctx)

	require.Eventually(t, func() bool { return restarts.Load() >= 1 },
		500*time.Millisecond, 2*time.Millisecond)
}

// 未达阈值前恢复健康，失败计数应归零，不触发 restart。
func TestHealthWatchdog_ResetOnRecovery(t *testing.T) {
	var restarts atomic.Int32
	var mu sync.Mutex
	failing := true
	wd := &healthWatchdog{
		interval:  5 * time.Millisecond,
		threshold: 20,
		cooldown:  time.Second,
		probe: func(context.Context) bool {
			mu.Lock()
			defer mu.Unlock()
			return !failing
		},
		restart: func() { restarts.Add(1) },
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go wd.run(ctx)

	time.Sleep(12 * time.Millisecond) // 失败约 2-3 次（远小于阈值 20）

	mu.Lock()
	failing = false // 恢复健康
	mu.Unlock()

	time.Sleep(60 * time.Millisecond) // 继续运行一段时间
	cancel()

	assert.Equal(t, int32(0), restarts.Load(), "no restart before threshold")
}

// 冷却期应限制两次 restart 的最小间隔不小于 cooldown。
func TestHealthWatchdog_CooldownDelaysNextRestart(t *testing.T) {
	var mu sync.Mutex
	var times []time.Time
	wd := &healthWatchdog{
		interval:  3 * time.Millisecond,
		threshold: 2,
		cooldown:  60 * time.Millisecond,
		probe:     func(context.Context) bool { return false },
		restart: func() {
			mu.Lock()
			times = append(times, time.Now())
			mu.Unlock()
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go wd.run(ctx)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(times) >= 2
	}, 500*time.Millisecond, 2*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	// 容差取一个 interval，吸收 ticker 抖动。
	gap := times[1].Sub(times[0])
	assert.GreaterOrEqual(t, gap, 60*time.Millisecond-3*time.Millisecond,
		"second restart must respect cooldown")
}
