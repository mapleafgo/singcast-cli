package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"strconv"
	"unsafe"

	"github.com/mapleafgo/singcast/ffi"
)

var api = ffi.New()

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

// --- Config ---

//export CoreCheckConfig
func CoreCheckConfig(content *C.char) *C.char {
	return resultJSON(api.CheckConfig(goString(content)))
}

//export CoreReloadConfig
func CoreReloadConfig(content, ruleSetProxy *C.char) *C.char {
	return resultJSON(api.ReloadConfig(goString(content), goString(ruleSetProxy)))
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

//export CoreSetError
func CoreSetError(message *C.char) { api.SetError(goString(message)) }

// --- Pause / Wake / Network ---

//export CorePause
func CorePause() { api.Pause() }

//export CoreWake
func CoreWake() { api.Wake() }

//export CoreResetNetwork
func CoreResetNetwork() { api.ResetNetwork() }

// --- Proxy Control ---

//export CoreSelectProxy
func CoreSelectProxy(group, tag *C.char) *C.char {
	return resultJSON(api.SelectProxy(goString(group), goString(tag)))
}

//export CoreTestDelay
func CoreTestDelay(name *C.char) *C.char {
	return resultJSON(api.TestDelay(goString(name)))
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

//export CoreClearLogs
func CoreClearLogs() *C.char { return resultJSON(api.ClearLogs()) }

//export CoreGetStartedAt
func CoreGetStartedAt() *C.char {
	ts, err := api.GetStartedAt()
	if err != nil {
		return resultJSON(err)
	}
	data, _ := json.Marshal(map[string]any{"ok": true, "started_at": ts})
	return cString(string(data))
}

// --- Connection Management ---

//export CoreCloseConnection
func CoreCloseConnection(id *C.char) *C.char {
	return resultJSON(api.CloseConnection(goString(id)))
}

//export CoreCloseAllConnections
func CoreCloseAllConnections() *C.char {
	return resultJSON(api.CloseAllConnections())
}

// --- Platform Queries ---

//export CoreNeedWIFIState
func CoreNeedWIFIState() C.int {
	if api.NeedWIFIState() {
		return 1
	}
	return 0
}

//export CoreNeedFindProcess
func CoreNeedFindProcess() C.int {
	if api.NeedFindProcess() {
		return 1
	}
	return 0
}

//export CoreUpdateWIFIState
func CoreUpdateWIFIState() { api.UpdateWIFIState() }

//export CoreSetIncludeAllNetworks
func CoreSetIncludeAllNetworks(v C.int) { api.SetIncludeAllNetworks(v != 0) }

//export CoreSetWIFIState
func CoreSetWIFIState(ssid, bssid *C.char) { api.SetWIFIState(goString(ssid), goString(bssid)) }

//export CoreQueryTunOptions
func CoreQueryTunOptions() *C.char { return cString(api.QueryTunOptions()) }

//export CoreWriteMessage
func CoreWriteMessage(level C.int, message *C.char) {
	api.WriteMessage(int32(level), goString(message))
}

//export CoreFlushSystemDNS
func CoreFlushSystemDNS() { api.FlushSystemDNS() }

//export CoreQueryMemoryStats
func CoreQueryMemoryStats() *C.char { return cString(api.QueryMemoryStats()) }

// --- Memory ---

//export CoreForceGC
func CoreForceGC() { api.ForceGC() }

//export CoreSetMemoryLimit
func CoreSetMemoryLimit(bytes *C.char) *C.char {
	n, err := strconv.ParseInt(goString(bytes), 10, 64)
	if err != nil {
		return resultJSON(err)
	}
	api.SetMemoryLimit(n)
	return resultJSON(nil)
}

// --- Version ---

//export CoreGetVersion
func CoreGetVersion() *C.char { return cString(api.Version()) }

// --- Events ---

//export CoreSetCallback
func CoreSetCallback(cb unsafe.Pointer) { setCallback(cb) }

// --- Package-level Utilities ---

//export CoreFormatBytes
func CoreFormatBytes(length C.longlong) *C.char {
	return cString(ffi.FormatBytes(int64(length)))
}

//export CoreFormatDuration
func CoreFormatDuration(duration C.longlong) *C.char {
	return cString(ffi.FormatDuration(int64(duration)))
}

//export CoreAvailablePort
func CoreAvailablePort(startPort C.int) *C.char {
	port, err := ffi.AvailablePort(int32(startPort))
	if err != nil {
		return resultJSON(err)
	}
	data, _ := json.Marshal(map[string]any{"ok": true, "port": int64(port)})
	return cString(string(data))
}

// --- System Proxy ---
// System proxy management is handled by the GUI, not the core.
