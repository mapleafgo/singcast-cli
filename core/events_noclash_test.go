package core

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// 未配置 clash_api 时，运行时统计、模式控制等应用层功能仍必须可用。
// 仪表盘的流量/连接数/内存/启动时间，以及 Rule/Global/Direct 模式切换，
// 都不应要求开启对外 API（1.14 起这些 manager 独立于 clash_api 注册）。
func TestService_NoClashAPI_StillFunctional(t *testing.T) {
	// syncLogLevelFromConfig 会把全局日志级别改成配置里的 error，
	// 必须在测试结束后复位，否则会污染同包后续的日志测试。
	t.Cleanup(func() { SetLogLevel(LogLevelInfo) })

	mixedPort := freeTCPPort(t)
	config := fmt.Sprintf(`{
		"log": {"level": "error"},
		"inbounds": [{"type": "mixed", "tag": "mixed-in", "listen": "127.0.0.1", "listen_port": %d}],
		"outbounds": [{"type": "direct", "tag": "DIRECT"}],
		"route": {
			"rules": [
				{"clash_mode": "Global", "outbound": "DIRECT"},
				{"clash_mode": "Direct", "outbound": "DIRECT"}
			],
			"final": "DIRECT"
		}
	}`, mixedPort)

	svc := NewService()
	if err := svc.Init(initJSON(filepath.Join(t.TempDir(), "home"))); err != nil {
		t.Fatalf("init: %v", err)
	}
	defer svc.Destroy()

	var mu sync.Mutex
	statsCount := 0
	var last StatsSnapshot
	svc.SetOnEvent(func(event int32, payload string) {
		if event == EventStats {
			mu.Lock()
			statsCount++
			_ = json.Unmarshal([]byte(payload), &last)
			mu.Unlock()
		}
	})
	if err := svc.StartWithContent(config, ""); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer svc.Stop()

	time.Sleep(3 * time.Second)
	mu.Lock()
	defer mu.Unlock()
	if statsCount == 0 {
		t.Fatal("未配置 clash_api 时 3s 内没有收到任何 EventStats，仪表盘统计会为空")
	}
	if last.StartedAt <= 0 {
		t.Errorf("started_at = %d，启动时间会显示为空", last.StartedAt)
	}
	if last.Memory == 0 {
		t.Error("memory = 0，内存显示会为空")
	}

	// 模式控制：应用层要能查询并可切换到 Global。
	if mode := svc.QueryMode(); !strings.Contains(mode, `"current_mode":"Rule"`) || !strings.Contains(mode, `"Global"`) {
		t.Errorf("QueryMode = %s，应能读到当前模式并包含 Global", mode)
	}
	if err := svc.SetMode("Global"); err != nil {
		t.Fatalf("未配置 clash_api 时切换模式失败: %v", err)
	}
	if mode := svc.QueryMode(); !strings.Contains(mode, `"current_mode":"Global"`) {
		t.Errorf("SetMode(Global) 后 QueryMode = %s，期望 current_mode=Global", mode)
	}
}
