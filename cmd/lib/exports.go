package main

/*
#include <stdlib.h>

typedef void (*EventCallback)(int, const char*);
static void invokeEventCB(EventCallback cb, int t, const char* s) { cb(t, s); }
*/
import "C"

import (
	runtimeDebug "runtime/debug"
	"unsafe"

	"github.com/mapleafgo/singcast/core"
)

var svc = core.NewService()

var cEventCB unsafe.Pointer

func init() {
	emit := func(eventType int32, json string) {
		if cEventCB != nil {
			// Ownership transfer: Dart side must call CoreFreeString after copying.
			// Do NOT free here — NativeCallable.listener is async (dart-lang/sdk#54554).
			cs := C.CString(json)
			C.invokeEventCB(C.EventCallback(cEventCB), C.int(eventType), cs)
		}
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
	return errorString(core.CheckConfig(goString(content)))
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

//export CoreSetEventCallback
func CoreSetEventCallback(cb unsafe.Pointer) { cEventCB = cb }
