package core

import (
	"context"
	"log/slog"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/urltest"
	C "github.com/sagernet/sing-box/constant"
)

// 健康看门狗默认参数。选型理由：
//   - interval 60s：探测开销低，又能在一分钟内感知到断网
//   - timeout 10s：单次探测上限，避免长尾拖慢检测周期
//   - failThreshold 3：连续 3 次失败（约 3 分钟）才重启，规避瞬时抖动误判
//   - cooldown 5min：重启后冷却，防止链路仍未恢复时反复重启形成风暴
const (
	defaultHealthInterval  = 60 * time.Second
	defaultHealthTimeout   = 10 * time.Second
	defaultHealthFailCount = 3
	defaultHealthCooldown  = 5 * time.Minute

	// maxProxyDepth 限制沿代理分组 Now() 解析叶子节点的最大层数，防止配置成环时无限递归。
	maxProxyDepth = 16
)

// healthConfig 是 HealthCheckConfig 经 normalizeHealthConfig 填充默认值后的运行时形态。
type healthConfig struct {
	interval      time.Duration
	timeout       time.Duration
	failThreshold int
	cooldown      time.Duration
}

func normalizeHealthConfig(cfg *HealthCheckConfig) healthConfig {
	c := healthConfig{
		interval:      defaultHealthInterval,
		timeout:       defaultHealthTimeout,
		failThreshold: defaultHealthFailCount,
		cooldown:      defaultHealthCooldown,
	}
	if cfg == nil {
		return c
	}
	if cfg.Interval > 0 {
		c.interval = time.Duration(cfg.Interval) * time.Millisecond
	}
	if cfg.Timeout > 0 {
		c.timeout = time.Duration(cfg.Timeout) * time.Millisecond
	}
	if cfg.FailThreshold > 0 {
		c.failThreshold = cfg.FailThreshold
	}
	if cfg.Cooldown > 0 {
		c.cooldown = time.Duration(cfg.Cooldown) * time.Millisecond
	}
	return c
}

// healthWatchdog 周期性探测代理连通性，持续失败时触发一次 core 自愈重启。
// probe/restart 为注入回调，使循环逻辑可脱离 *Service 独立测试。
type healthWatchdog struct {
	interval  time.Duration
	threshold int
	cooldown  time.Duration
	probe     func(context.Context) bool // false 表示本次探测不健康
	restart   func()                     // 触发恢复（如重启 core）
	onFail    func(fails int)
}

// run 执行探测循环，直到 ctx 被取消。
// 失败计数与冷却期均在此处维护：达到阈值后调用 restart 并进入冷却，
// 冷却期内跳过探测，避免链路未恢复时频繁重启。
func (w *healthWatchdog) run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	var fails int
	var cooldownUntil time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if now.Before(cooldownUntil) {
				continue
			}
			if w.probe(ctx) {
				fails = 0
				continue
			}
			fails++
			if w.onFail != nil {
				w.onFail(fails)
			}
			if fails >= w.threshold {
				slog.Warn("health watchdog: proxy unreachable, restarting core to recover",
					"fails", fails, "threshold", w.threshold)
				w.restart()
				fails = 0
				cooldownUntil = time.Now().Add(w.cooldown)
			}
		}
	}
}

// newWatchdog 用当前配置构造一个自愈看门狗。
func (s *Service) newWatchdog() *healthWatchdog {
	hc := s.healthCfg
	return &healthWatchdog{
		interval:  hc.interval,
		threshold: hc.failThreshold,
		cooldown:  hc.cooldown,
		probe:     func(ctx context.Context) bool { return s.healthProbe(ctx, hc.timeout) },
		restart:   s.restartForHealth,
		onFail: func(fails int) {
			slog.Warn("health watchdog: proxy probe failed", "fails", fails, "threshold", hc.failThreshold)
		},
	}
}

// healthProbe 通过当前生效的代理节点做一次 URL 探测判断代理是否可用。
// 服务未运行、或当前处于直连模式（无代理节点）时返回 true，避免误触发自愈。
// ctx 取消（服务销毁）时探测随之中止。
func (s *Service) healthProbe(ctx context.Context, timeout time.Duration) bool {
	if s.State() != StateRunning {
		return true
	}
	rs := s.running.Load()
	if rs == nil {
		return true
	}
	node := s.currentProxyNode(rs)
	if node == nil {
		return true
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	_, err := urltest.URLTest(probeCtx, "", node)
	cancel()
	return err == nil
}

// currentProxyNode 返回路由当前实际使用的代理节点：从默认出站（route.final）
// 出发，沿代理分组的 Now() 一路解析到叶子节点（用户当下真正在用的节点）。
// 当前是直连/阻断/无代理节点时返回 nil，使看门狗不触发自愈。
func (s *Service) currentProxyNode(rs *runningState) adapter.Outbound {
	out := rs.instance.Outbound().Default()
	seen := make(map[string]bool)
	for range maxProxyDepth {
		if out == nil {
			return nil
		}
		switch out.Type() {
		case C.TypeDirect, C.TypeBlock, C.TypeDNS:
			return nil
		}
		group, ok := out.(adapter.OutboundGroup)
		if !ok {
			return out
		}
		tag := out.Tag()
		if seen[tag] {
			return nil
		}
		seen[tag] = true
		next := group.Now()
		if next == "" || next == tag {
			return nil
		}
		ob, found := rs.instance.Outbound().Outbound(next)
		if !found {
			return nil
		}
		out = ob
	}
	return nil
}

// restartForHealth 用当前生效配置重启 core，重新解析代理入口域名并重建连接。
func (s *Service) restartForHealth() {
	rs := s.running.Load()
	if rs == nil {
		return
	}
	if err := s.StartWithContent(rs.currentConfig, ""); err != nil {
		slog.Error("health watchdog: restart failed", "error", err)
	}
}
