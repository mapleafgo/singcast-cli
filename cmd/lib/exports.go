package main

/*
#include <stdlib.h>

typedef void (*EventCallback)(int, const char*);
static void invokeEventCB(EventCallback cb, int t, const char* s) { cb(t, s); }
*/
import "C"

import (
	"context"
	runtimeDebug "runtime/debug"
	"sync/atomic"
	"unsafe"

	"github.com/mapleafgo/singcast/core"
)

// 本文件所有 //export 函数遵循统一的内存所有权约定：
//   - 返回的 *C.char 由 native 侧分配，调用方复制内容后必须调用 CoreFreeString 释放；
//   - 返回 nil 表示成功（错误型返回值）或无数据；对 nil 调用 CoreFreeString 是安全的；
//   - 传入的 *C.char 由调用方持有，native 侧只读不释放。
var svc = core.NewService()

// cEventCB 保存 Dart 侧注册的事件回调。用原子指针而非裸 unsafe.Pointer：
// 注册发生在 Dart 线程，读取发生在 core 的事件 goroutine，裸指针的
// "判空后使用"两段式读取可能在热重载时对已失效的函数指针发起 C 调用，
// 而 FFI 层崩溃会带走整个宿主 App。
var cEventCB atomic.Pointer[C.EventCallback]

func init() {
	emit := func(eventType int32, json string) {
		cb := cEventCB.Load()
		if cb == nil {
			return
		}
		// 所有权转移：native 分配，Dart 侧复制后调 CoreFreeString。
		// 不能在此处释放——NativeCallable.listener 是异步的（dart-lang/sdk#54554）。
		cs := C.CString(json)
		C.invokeEventCB(*cb, C.int(eventType), cs)
	}
	svc.SetOnEvent(emit)
	core.SetOnLogEvent(emit)
}

// errorString returns nil on success, or the error message as a C string on failure.
// The caller must free non-nil returns with CoreFreeString.
func errorString(err error) *C.char {
	if err != nil {
		return cString(err.Error())
	}
	return nil
}

// --- Lifecycle ---

//export CoreInit
func CoreInit(optionsJSON *C.char) *C.char {
	return errorString(svc.Init(goString(optionsJSON)))
}

//export CoreStartWithContent
func CoreStartWithContent(content, ruleSetProxy *C.char) *C.char {
	return errorString(svc.StartWithContent(goString(content), goString(ruleSetProxy)))
}

//export CoreStop
func CoreStop() *C.char { return errorString(svc.Stop()) }

//export CoreDestroy
func CoreDestroy() { svc.Destroy() }

//export CoreQueryState
func CoreQueryState() *C.char { return cString(svc.State().String()) }

// --- Config ---

//export CoreCheckConfig
func CoreCheckConfig(content *C.char) *C.char {
	return errorString(core.CheckConfig(context.Background(), goString(content)))
}

// --- Logging ---

//export CoreSetLogLevel
func CoreSetLogLevel(level C.int) { svc.SetLogLevel(int32(level)) }

// --- Network ---

//export CoreResetNetwork
func CoreResetNetwork() { svc.ResetNetwork() }

// --- Proxy Control ---

//export CoreSelectProxy
func CoreSelectProxy(group, tag *C.char) *C.char {
	return errorString(svc.SelectOutbound(goString(group), goString(tag)))
}

//export CoreTestDelay
func CoreTestDelay(name *C.char, timeoutMs C.int) C.int {
	return C.int(svc.URLTest(goString(name), int32(timeoutMs)))
}

//export CoreSetMode
func CoreSetMode(mode *C.char) *C.char { return errorString(svc.SetMode(goString(mode))) }

//export CoreSetGroupExpand
func CoreSetGroupExpand(group *C.char, expand C.int) *C.char {
	return errorString(svc.SetGroupExpand(goString(group), expand != 0))
}

// --- Queries ---

//export CoreQueryProxies
func CoreQueryProxies() *C.char { return cString(svc.QueryProxies()) }

//export CoreQueryStats
func CoreQueryStats() *C.char { return cString(svc.QueryStats()) }

//export CoreQueryConnections
func CoreQueryConnections() *C.char { return cString(svc.QueryConnections()) }

//export CoreQueryMode
func CoreQueryMode() *C.char { return cString(svc.QueryMode()) }

// --- Connection Management ---

//export CoreCloseConnection
func CoreCloseConnection(id *C.char) *C.char {
	return errorString(svc.CloseConnection(goString(id)))
}

//export CoreCloseAllConnections
func CoreCloseAllConnections() *C.char { return errorString(svc.CloseConnections()) }

// --- System ---

//export CoreFlushSystemDNS
func CoreFlushSystemDNS() { svc.FlushSystemDNS() }

// --- Rules / DNS / Cache ---

//export CoreQueryRules
func CoreQueryRules() *C.char { return cString(svc.QueryRules()) }

//export CoreFlushFakeIP
func CoreFlushFakeIP() *C.char { return errorString(svc.FlushFakeIP()) }

//export CoreFlushDNSCache
func CoreFlushDNSCache() *C.char { return errorString(svc.FlushDNSCache()) }

//export CoreTestGroupDelay
func CoreTestGroupDelay(group *C.char, timeoutMs C.int) *C.char {
	return cString(svc.TestGroupDelay(goString(group), int32(timeoutMs)))
}

//export CoreTriggerGC
func CoreTriggerGC() { svc.TriggerGC() }

// --- Memory ---

//export CoreSetMemoryLimit
func CoreSetMemoryLimit(bytes C.longlong) { runtimeDebug.SetMemoryLimit(int64(bytes)) }

// --- Version ---

//export CoreGetVersion
func CoreGetVersion() *C.char { return cString(core.VersionJSON()) }

// --- Event Callback ---

// CoreSetEventCallback 注册事件回调，传 NULL 取消注册。可在任意时刻调用。
//
// 回调签名为 void(int eventType, const char *json)。json 参数的所有权转移给
// 调用方：消费完后必须调用 CoreFreeString 释放，否则每条事件泄漏一次。
// 回调会在 core 内部 goroutine 上被调用，实现方需自行保证线程安全。
//
//export CoreSetEventCallback
func CoreSetEventCallback(cb unsafe.Pointer) {
	if cb == nil {
		cEventCB.Store(nil)
		return
	}
	fn := C.EventCallback(cb)
	cEventCB.Store(&fn)
}
