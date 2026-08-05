package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/include"
	singboxlog "github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/service"

	"github.com/mapleafgo/singcast/translator"
)

var Version = "dev"

type platformLogWriter struct {
	svc *Service
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
	data, _ := json.Marshal(LogEntry{
		Level:     coreLevel,
		Message:   message,
		Timestamp: time.Now().UnixMilli(),
	})
	w.svc.emitEvent(EventLog, string(data))
}

func VersionJSON() string {
	data, _ := json.Marshal(VersionInfo{Version: Version, Core: "singcast"})
	return string(data)
}

// CheckConfig 校验配置内容能否被 sing-box 接受。接受 Clash YAML 与 sing-box JSON，
// YAML 会先翻译再校验。返回 nil 表示配置可用；翻译过程产生的非致命 warning
// 通过 slog 输出（进而转为 EventLog），不影响返回值。
func CheckConfig(ctx context.Context, content string) error {
	data := []byte(content)
	jsonStr, warnings, err := translator.Convert(data)
	if err != nil {
		return fmt.Errorf("convert config: %w", err)
	}
	for _, w := range warnings {
		slog.Warn("check config", "warning", w)
	}
	data = []byte(jsonStr)
	ctx = include.Context(ctx)
	_, err = singjson.UnmarshalExtendedContext[option.Options](ctx, data)
	return err
}

// Convert 统一处理订阅输入并返回 sing-box JSON，不启动内核。
func Convert(content string) (string, error) {
	jsonStr, warnings, err := translator.Convert([]byte(content))
	if err != nil {
		return "", err
	}
	for _, w := range warnings {
		slog.Warn("convert config", "warning", w)
	}
	return jsonStr, nil
}

type State int32

const (
	StateCreated State = iota
	StateInitialized
	StateStarting
	StateRunning
	StateDestroyed
)

func (s State) String() string {
	switch s {
	case StateCreated:
		return "created"
	case StateInitialized:
		return "initialized"
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateDestroyed:
		return "destroyed"
	default:
		return "unknown"
	}
}

// atomicState wraps atomic.Int32 with typed State access, eliminating int32/State casts.
type atomicState struct{ v atomic.Int32 }

func (a *atomicState) Load() State   { return State(a.v.Load()) }
func (a *atomicState) Store(s State) { a.v.Store(int32(s)) }
func (a *atomicState) CompareAndSwap(old, newState State) bool {
	return a.v.CompareAndSwap(int32(old), int32(newState))
}

type runningState struct {
	instance      *box.Box
	boxCtx        context.Context
	currentConfig string
	startedAt     int64
	// stopCtx 在本实例被关闭前取消，供在途查询（URLTest/TestGroupDelay）及时退出。
	// 没有它，一次全组测速会在最长 timeoutMs 的窗口里继续使用已 Close 的实例。
	stopCtx    context.Context
	stopCancel context.CancelFunc
	// stubTags maps outbound tag → original protocol for proxies converted to
	// socks stubs (unsupported protocols). Used by QueryProxies to report the
	// original type so the UI can distinguish unsupported nodes.
	stubTags map[string]string
}

type Service struct {
	state     atomicState
	running   atomic.Pointer[runningState]
	platform  *PlatformIO
	onEvent   atomic.Pointer[func(int32, string)]
	subCancel atomic.Pointer[func()]

	// healthCancel 取消自愈看门狗 goroutine（Init 创建，Destroy 取消）。
	healthCancel atomic.Pointer[func()]
	healthCfg    healthConfig

	// startMu serializes StartWithContent calls. A concurrent caller waits
	// for the current startup to finish, then stops and restarts.
	//
	// 注意：Stop() 刻意不持有此锁。状态变更经 casState → emitState 同步调用宿主
	// 回调，而 start 路径是持锁的；若 Stop 也持锁，宿主在状态回调里调 Stop()
	// 就会因 sync.Mutex 不可重入而自锁（gomobile 的回调是同步调用）。
	// start/stop 的三份状态一致性改由各自的原子操作保证：
	// 实例关闭责任由 running.Swap 的返回值决定，在途查询由 runningState.stopCtx
	// 中止，hooks 订阅由 hooksMu + 代次校验（见 subscribeHooks）收口。
	startMu  sync.Mutex
	startSeq atomic.Int64 // coalesce: only the latest caller proceeds

	// hooksMu 保护 subCancel 的读改写。绝不在持有它时调用宿主回调，
	// 因此不会与 startMu 形成锁序问题。
	hooksMu sync.Mutex
}

func NewService() *Service { return &Service{platform: NewPlatformIO()} }

func (s *Service) State() State {
	return s.state.Load()
}

func (s *Service) PlatformIO() *PlatformIO { return s.platform }

// Init 以 context.Background() 初始化服务，等价于 InitContext。
// 供 FFI/gomobile 宿主使用：那里没有可用的 context，生命周期由 Destroy 控制。
func (s *Service) Init(optionsJSON string) error {
	return s.InitContext(context.Background(), optionsJSON)
}

// InitContext 解析 InitOptions 并把服务从 Created 推进到 Initialized：
// 建立 home/temp 目录、切换工作目录、按需启动自愈看门狗。
// ctx 仅用于派生看门狗的生命周期——ctx 取消或 Destroy 调用都会停止看门狗；
// 初始化本身是同步的，不会因 ctx 取消而中断。
// 重复调用或状态不为 Created 时返回错误，且状态保持不变。
func (s *Service) InitContext(ctx context.Context, optionsJSON string) error {
	if !s.state.CompareAndSwap(StateCreated, StateInitialized) {
		return fmt.Errorf("init: invalid state %s", s.State())
	}

	var opts InitOptions
	if err := json.Unmarshal([]byte(optionsJSON), &opts); err != nil {
		s.state.Store(StateCreated)
		return fmt.Errorf("parse init options: %w", err)
	}
	if opts.HomeDir == "" {
		s.state.Store(StateCreated)
		return fmt.Errorf("init: home_dir is required")
	}

	if err := os.MkdirAll(opts.HomeDir, 0o755); err != nil {
		s.state.Store(StateCreated)
		return fmt.Errorf("create home dir: %w", err)
	}
	if err := os.Chdir(opts.HomeDir); err != nil {
		s.state.Store(StateCreated)
		return fmt.Errorf("chdir: %w", err)
	}

	if opts.Debug {
		SetLogLevel(LogLevelDebug)
	}

	tempDir := filepath.Join(opts.HomeDir, "temp")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		s.state.Store(StateCreated)
		return fmt.Errorf("create temp dir: %w", err)
	}

	// 自愈看门狗：默认关闭，需显式设置 health_check.enabled=true 启用。
	hc := normalizeHealthConfig(opts.HealthCheck)
	if opts.HealthCheck != nil && opts.HealthCheck.Enabled {
		s.healthCfg = hc
		wdCtx, wdCancel := context.WithCancel(ctx)
		cancelFn := func() { wdCancel() }
		s.healthCancel.Store(&cancelFn)
		go s.newWatchdog().run(wdCtx)
		slog.Info("health watchdog enabled",
			"interval", hc.interval.String(), "timeout", hc.timeout.String(),
			"fail_threshold", hc.failThreshold, "cooldown", hc.cooldown.String())
	}

	// 提前触发国家检测：Init 阶段代理尚未启动，IP 地理位置服务拿到的是真实出口 IP。
	// sync.Once 缓存后，后续 StartWithContent 翻译时即使旧代理还在运行也读缓存值。
	cc, fallback := translator.DetectCountryWithFallback("")
	if fallback {
		slog.Warn("country detection failed, using CN fallback", "country", cc)
	} else {
		slog.Debug("country detected", "country", cc)
	}

	s.emitState(StateInitialized)
	slog.Info("service init done")
	return nil
}

func (s *Service) StartWithContent(content, ruleSetProxy string) error {
	data := []byte(content)
	// 统一走 ConvertWithMeta：base64 解码、URI 列表、YAML 翻译、JSON 透传
	jsonContent, stubTags, err := s.translateConfig(data, ruleSetProxy)
	if err != nil {
		return err
	}

	mySeq := s.startSeq.Add(1)

	s.startMu.Lock()
	defer s.startMu.Unlock()

	// A newer request arrived while we waited — let it win.
	if s.startSeq.Load() != mySeq {
		return nil
	}

	// Stop any running/starting instance first.
	if err := s.Stop(); err != nil {
		slog.Warn("stop previous instance", "error", err)
	}

	if !s.casState(StateInitialized, StateStarting) {
		return fmt.Errorf("start: invalid state %s", s.State())
	}

	if err := s.startWithJSON(jsonContent, stubTags); err != nil {
		s.casState(StateStarting, StateInitialized)
		return err
	}
	return nil
}

func (s *Service) translateConfig(data []byte, ruleSetProxy string) (string, map[string]string, error) {
	start := time.Now()
	result, warnings, meta, err := translator.ConvertWithMeta(data, nil)
	if err != nil {
		return "", nil, fmt.Errorf("translate config: %w", err)
	}
	slog.Debug("translate config", "elapsed", time.Since(start), "warnings", len(warnings))
	// warnings 是非致命但用户需要知道的降级项（跳过的节点、未翻译的规则）。
	// 走 slog 而非丢弃：coreLogHandler 会转成 EventLog 送到前端日志面板。
	for _, w := range warnings {
		slog.Warn("translate config", "warning", w)
	}
	// rule_set URL 前缀改写是启动时参数，在翻译完成后做后处理
	if ruleSetProxy != "" {
		result, err = translator.ApplyRuleSetProxy(result, ruleSetProxy)
		if err != nil {
			return "", nil, fmt.Errorf("apply rule-set proxy: %w", err)
		}
	}
	if len(meta.StubTags) == 0 {
		// JSON 透传路径不会重新计算 stubTags；从转换产物里恢复内嵌标记，
		// 这样保存后的配置再次启动时还能继续报告 unsupported。
		meta.StubTags = extractStubTagsFromJSON(result)
	}
	return result, meta.StubTags, nil
}

type rawOutbound struct {
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
	Username   string `json:"username"`
}

// extractStubTagsFromJSON 扫描转换后的 JSON，找回 socks stub 中内嵌的
// "unsupported:<original>" 标记。除 JSON 透传路径外，也兼容用户手动改过
// 订阅但保留 stub 的已保存配置。
func extractStubTagsFromJSON(content string) map[string]string {
	var cfg struct {
		Outbounds []rawOutbound `json:"outbounds"`
	}
	if err := json.Unmarshal([]byte(content), &cfg); err != nil {
		return nil
	}
	tags := make(map[string]string)
	for _, ob := range cfg.Outbounds {
		if ob.Type != "socks" || ob.Server != "127.0.0.1" || ob.ServerPort != 1 {
			continue
		}
		if !strings.HasPrefix(ob.Username, "unsupported:") {
			continue
		}
		tags[ob.Tag] = strings.TrimPrefix(ob.Username, "unsupported:")
	}
	return tags
}

func (s *Service) startWithJSON(jsonContent string, stubTags map[string]string) error {
	syncLogLevelFromConfig(jsonContent)
	s.platform.protectCount.Store(0)

	if s.platform.IsMobile() {
		hasProtect := s.platform.protectFn.Load() != nil
		fd := s.platform.tunFd.Load()
		monitor := s.platform.ifaceMonitor
		var defaultIface string
		var myIface []string
		if monitor != nil {
			if di := monitor.DefaultInterface(); di != nil {
				defaultIface = di.Name
			}
			myIface = monitor.MyInterfaces()
		}
		slog.Debug("mobile startup", "hasProtect", hasProtect, "tunFd", fd, "defaultIface", defaultIface, "myIface", myIface)
	}

	ctx := include.Context(context.Background())
	if s.platform.IsMobile() {
		ctx = service.ContextWith[adapter.PlatformInterface](ctx, s.platform)
	}

	options, err := singjson.UnmarshalExtendedContext[option.Options](ctx, []byte(jsonContent))
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	// Mobile platforms require auto_detect_interface for VpnService.protect()
	// to bypass VPN routing on outbound sockets.
	if s.platform.IsMobile() && options.Route != nil {
		options.Route.AutoDetectInterface = true
	}

	logWriter := &platformLogWriter{svc: s}

	inst, err := box.New(box.Options{Options: options, Context: ctx, PlatformLogWriter: logWriter})
	if err != nil {
		return fmt.Errorf("create instance: %w", err)
	}
	s.updateMobileInterfaces(ctx)
	if err := inst.Start(); err != nil {
		return fmt.Errorf("start instance: %w", err)
	}

	s.platform.SetRouter(inst.Router())

	stopCtx, stopCancel := context.WithCancel(context.Background())
	rs := &runningState{
		instance:      inst,
		boxCtx:        ctx,
		currentConfig: jsonContent,
		startedAt:     time.Now().UnixMilli(),
		stubTags:      stubTags,
		stopCtx:       stopCtx,
		stopCancel:    stopCancel,
	}
	s.running.Store(rs)
	// CAS 失败说明有并发方（Stop/Destroy）已改走状态；由 Swap 的原子性决定谁负责
	// 关闭，无条件 Close 会与对方的 Close 重入。
	if !s.casState(StateStarting, StateRunning) {
		if old := s.running.Swap(nil); old != nil {
			old.close()
		}
		return nil
	}
	s.subscribeHooks(rs)

	slog.Info("service running")
	return nil
}

// updateMobileInterfaces 在移动端启动实例前上报网络接口并输出 socket 保护诊断日志。
// 桌面端为空操作。需在 box.New 之后调用（NetworkManager 由其注册进 ctx）。
func (s *Service) updateMobileInterfaces(ctx context.Context) {
	if !s.platform.IsMobile() {
		return
	}
	nm := service.FromContext[adapter.NetworkManager](ctx)
	if nm == nil {
		slog.Warn("update interfaces: NetworkManager is nil")
		return
	}
	pi := service.FromContext[adapter.PlatformInterface](ctx)
	protectFn := nm.ProtectFunc()
	slog.Debug("mobile protect diagnostic",
		"autoDetect", nm.AutoDetectInterface(),
		"protectFuncNil", protectFn == nil,
		"platformNil", pi == nil,
		"usePlatformCtrl", pi != nil && pi.UsePlatformAutoDetectInterfaceControl(),
	)
	if err := nm.UpdateInterfaces(); err != nil {
		slog.Warn("update interfaces failed", "error", err)
	} else {
		slog.Debug("update interfaces", "count", len(nm.NetworkInterfaces()))
	}
}

// Stop 停止当前实例并回到 Initialized 态。未在运行时是空操作，返回 nil。
// 可并发调用，也可在事件回调内调用（不持有 startMu，见该字段注释）。
// 实例的 Close 错误原样返回。
func (s *Service) Stop() error {
	for {
		st := s.state.Load()
		if st != StateRunning && st != StateStarting {
			return nil
		}
		if s.casState(st, StateInitialized) {
			break
		}
	}

	// 先摘掉 running 再退订：顺序反了会留下泄漏窗口——并发的 start 可能在
	// 退订之后完成订阅，随后被这里的 Swap 关掉实例，4 个订阅 goroutine
	// 就对着已关闭的实例长期空转。摘除在前，start 的代次校验才能发现自己已过期。
	old := s.running.Swap(nil)
	s.unsubscribeHooks()

	if old != nil {
		slog.Info("service stopped")
		return old.close()
	}
	return nil
}

// close 先取消在途查询再关闭 sing-box 实例，使查询 goroutine 不会继续
// 使用已关闭的实例。可安全重复调用（Box.Close 二次调用返回 os.ErrClosed）。
func (rs *runningState) close() error {
	rs.stopCancel()
	return rs.instance.Close()
}

func (s *Service) Destroy() {
	for {
		st := s.state.Load()
		if st == StateDestroyed {
			return
		}
		if s.casState(st, StateDestroyed) {
			break
		}
	}

	if cancelPtr := s.healthCancel.Swap(nil); cancelPtr != nil {
		(*cancelPtr)()
	}

	// 与 Stop 同序：先摘 running 再退订，使并发 start 的代次校验能发现自己已过期。
	old := s.running.Swap(nil)
	s.unsubscribeHooks()

	if old != nil {
		old.close()
	}
	slog.Info("service destroyed")
}

func (s *Service) casState(oldState, newState State) bool {
	if s.state.CompareAndSwap(oldState, newState) {
		s.emitState(newState)
		return true
	}
	return false
}
