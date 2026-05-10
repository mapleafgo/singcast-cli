package main

/*
#include <stdlib.h>

typedef void (*EventCallback)(int, const char*);
static void invokeEventCB(EventCallback cb, int t, const char* s) { cb(t, s); }
*/
import "C"

import (
	"unsafe"

	"github.com/mapleafgo/singcast/mobile"
)

var api = mobile.Create()

var cEventCB unsafe.Pointer

func init() {
	api.SetCallbackFuncs(func(eventType int32, json string) {
		if cEventCB != nil {
			// Ownership transfer: Dart side must call CoreFreeString after copying.
			// Do NOT free here — NativeCallable.listener is async (dart-lang/sdk#54554).
			cs := C.CString(json)
			C.invokeEventCB(C.EventCallback(cEventCB), C.int(eventType), cs)
		}
	})
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
	return errorString(api.Init(goString(optionsJSON)))
}

//export CoreStartWithContent
func CoreStartWithContent(content, ruleSetProxy *C.char) *C.char {
	return errorString(api.StartWithContent(goString(content), goString(ruleSetProxy)))
}

//export CoreStop
func CoreStop() *C.char { return errorString(api.Stop()) }

//export CoreDestroy
func CoreDestroy() { api.Destroy() }

//export CoreQueryState
func CoreQueryState() C.int { return C.int(api.State()) }

// --- Config ---

//export CoreCheckConfig
func CoreCheckConfig(content *C.char) *C.char {
	return errorString(api.CheckConfig(goString(content)))
}

//export CoreReloadTUN
func CoreReloadTUN() *C.char { return errorString(api.ReloadTUN()) }

//export CoreSetOverridePackages
func CoreSetOverridePackages(overrideJSON *C.char) *C.char {
	return errorString(api.SetOverridePackages(goString(overrideJSON)))
}

// --- Logging ---

//export CoreSetLogLevel
func CoreSetLogLevel(level C.int) { api.SetLogLevel(int32(level)) }

// --- Network ---

//export CoreResetNetwork
func CoreResetNetwork() { api.ResetNetwork() }

// --- Proxy Control ---

//export CoreSelectProxy
func CoreSelectProxy(group, tag *C.char) *C.char {
	return errorString(api.SelectProxy(goString(group), goString(tag)))
}

//export CoreTestDelay
func CoreTestDelay(name *C.char, timeoutMs C.int) C.int {
	return C.int(api.TestDelay(goString(name), int32(timeoutMs)))
}

//export CoreSetMode
func CoreSetMode(mode *C.char) *C.char {
	return errorString(api.SetMode(goString(mode)))
}

//export CoreSetGroupExpand
func CoreSetGroupExpand(group *C.char, expand C.int) *C.char {
	return errorString(api.SetGroupExpand(goString(group), expand != 0))
}

// --- Queries ---

//export CoreQueryProxies
func CoreQueryProxies() *C.char { return cString(api.QueryProxies()) }

//export CoreQueryStats
func CoreQueryStats() *C.char { return cString(api.QueryStats()) }

//export CoreQueryConnections
func CoreQueryConnections() *C.char { return cString(api.QueryConnections()) }

//export CoreQueryMode
func CoreQueryMode() *C.char { return cString(api.QueryMode()) }

// --- Connection Management ---

//export CoreCloseConnection
func CoreCloseConnection(id *C.char) *C.char {
	return errorString(api.CloseConnection(goString(id)))
}

//export CoreCloseAllConnections
func CoreCloseAllConnections() *C.char {
	return errorString(api.CloseAllConnections())
}

// --- System ---

//export CoreFlushSystemDNS
func CoreFlushSystemDNS() { api.FlushSystemDNS() }

// --- Rules / DNS / Cache ---

//export CoreQueryRules
func CoreQueryRules() *C.char { return cString(api.QueryRules()) }

//export CoreFlushFakeIP
func CoreFlushFakeIP() *C.char { return errorString(api.FlushFakeIP()) }

//export CoreQueryDNS
func CoreQueryDNS(name *C.char, qType C.int) *C.char {
	return cString(api.QueryDNS(goString(name), uint16(qType)))
}

//export CoreFlushDNSCache
func CoreFlushDNSCache() *C.char { return errorString(api.FlushDNSCache()) }

//export CoreTestGroupDelay
func CoreTestGroupDelay(group *C.char, timeoutMs C.int) *C.char {
	return cString(api.TestGroupDelay(goString(group), int32(timeoutMs)))
}

//export CoreTriggerGC
func CoreTriggerGC() { api.TriggerGC() }

// --- Memory ---

//export CoreSetMemoryLimit
func CoreSetMemoryLimit(bytes C.longlong) { api.SetMemoryLimit(int64(bytes)) }

// --- Version ---

//export CoreGetVersion
func CoreGetVersion() *C.char { return cString(api.Version()) }

// --- Event Callback ---

//export CoreSetEventCallback
func CoreSetEventCallback(cb unsafe.Pointer) { cEventCB = cb }
