package main

/*
#include <stdlib.h>

typedef void (*VoidCallback)();
typedef void (*StringCallback)(const char*);
typedef void (*IntStringCallback)(int, const char*);

static void invokeVoidCB(VoidCallback cb) { cb(); }
static void invokeStringCB(StringCallback cb, const char* s) { cb(s); }
static void invokeIntStringCB(IntStringCallback cb, int i, const char* s) { cb(i, s); }
*/
import "C"

import (
	"encoding/json"
	"unsafe"

	"github.com/mapleafgo/singcast/ffi"
)

var api = ffi.Create()

var (
	cURLTestCB   unsafe.Pointer
	cModeUpdateCB unsafe.Pointer
	cConnEventCB  unsafe.Pointer
)

func init() {
	api.SetCallbackFuncs(
		func() {
			if cURLTestCB != nil {
				C.invokeVoidCB(C.VoidCallback(cURLTestCB))
			}
		},
		func(mode string) {
			if cModeUpdateCB != nil {
				cs := C.CString(mode)
				C.invokeStringCB(C.StringCallback(cModeUpdateCB), cs)
			}
		},
		func(eventType int32, connJSON string) {
			if cConnEventCB != nil {
				cs := C.CString(connJSON)
				C.invokeIntStringCB(C.IntStringCallback(cConnEventCB), C.int(eventType), cs)
			}
		},
	)
}

func resultJSON(err error) *C.char {
	if err != nil {
		data, _ := json.Marshal(map[string]string{"error": err.Error()})
		return cString(string(data))
	}
	return cString(`{"ok":true}`)
}

// --- Lifecycle ---

//export CoreInit
func CoreInit(optionsJSON *C.char) *C.char {
	return resultJSON(api.Init(goString(optionsJSON)))
}

//export CoreStartWithContent
func CoreStartWithContent(content, ruleSetProxy *C.char) *C.char {
	return resultJSON(api.StartWithContent(goString(content), goString(ruleSetProxy)))
}

//export CoreStop
func CoreStop() *C.char { return resultJSON(api.Stop()) }

//export CoreDestroy
func CoreDestroy() { api.Destroy() }

//export CoreQueryState
func CoreQueryState() C.int { return C.int(api.State()) }

// --- Config ---

//export CoreCheckConfig
func CoreCheckConfig(content *C.char) *C.char {
	return resultJSON(api.CheckConfig(goString(content)))
}

//export CoreReloadTUN
func CoreReloadTUN() *C.char { return resultJSON(api.ReloadTUN()) }

//export CoreSetOverridePackages
func CoreSetOverridePackages(overrideJSON *C.char) *C.char {
	return resultJSON(api.SetOverridePackages(goString(overrideJSON)))
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
	return resultJSON(api.SelectProxy(goString(group), goString(tag)))
}

//export CoreTestDelay
func CoreTestDelay(name *C.char, timeoutMs C.int) C.int {
	return C.int(api.TestDelay(goString(name), int32(timeoutMs)))
}

//export CoreSetMode
func CoreSetMode(mode *C.char) *C.char {
	return resultJSON(api.SetMode(goString(mode)))
}

//export CoreSetGroupExpand
func CoreSetGroupExpand(group *C.char, expand C.int) *C.char {
	return resultJSON(api.SetGroupExpand(goString(group), expand != 0))
}

// --- Queries ---

//export CoreQueryProxies
func CoreQueryProxies() *C.char { return cString(api.QueryProxies()) }

//export CoreQueryTraffic
func CoreQueryTraffic() *C.char { return cString(api.QueryTraffic()) }

//export CoreQueryLogs
func CoreQueryLogs() *C.char { return cString(api.QueryLogs()) }

//export CoreQueryConnections
func CoreQueryConnections() *C.char { return cString(api.QueryConnections()) }

//export CoreQueryMode
func CoreQueryMode() *C.char { return cString(api.QueryMode()) }

// --- Connection Management ---

//export CoreCloseConnection
func CoreCloseConnection(id *C.char) *C.char {
	return resultJSON(api.CloseConnection(goString(id)))
}

//export CoreCloseAllConnections
func CoreCloseAllConnections() *C.char {
	return resultJSON(api.CloseAllConnections())
}

// --- System ---

//export CoreFlushSystemDNS
func CoreFlushSystemDNS() { api.FlushSystemDNS() }

// --- Rules / DNS / Cache ---

//export CoreQueryRules
func CoreQueryRules() *C.char { return cString(api.QueryRules()) }

//export CoreFlushFakeIP
func CoreFlushFakeIP() *C.char { return resultJSON(api.FlushFakeIP()) }

//export CoreQueryDNS
func CoreQueryDNS(name *C.char, qType C.int) *C.char {
	return cString(api.QueryDNS(goString(name), uint16(qType)))
}

//export CoreFlushDNSCache
func CoreFlushDNSCache() *C.char { return resultJSON(api.FlushDNSCache()) }

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

// --- Event Callbacks ---

//export CoreSetURLTestCallback
func CoreSetURLTestCallback(cb unsafe.Pointer) { cURLTestCB = cb }

//export CoreSetModeUpdateCallback
func CoreSetModeUpdateCallback(cb unsafe.Pointer) { cModeUpdateCB = cb }

//export CoreSetConnEventCallback
func CoreSetConnEventCallback(cb unsafe.Pointer) { cConnEventCB = cb }
