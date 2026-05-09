package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"

	"github.com/mapleafgo/singcast/ffi"
)

var api = ffi.Create()

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

//export CoreQueryMemoryStats
func CoreQueryMemoryStats() *C.char { return cString(api.QueryMemoryStats()) }

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
