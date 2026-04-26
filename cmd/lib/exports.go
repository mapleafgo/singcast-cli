package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
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

//export CoreInit
func CoreInit(homeDir *C.char) *C.char {
	return resultJSON(api.Init(goString(homeDir)))
}

//export CoreStart
func CoreStart(configPath *C.char, ruleSetProxy *C.char) *C.char {
	return resultJSON(api.Start(goString(configPath), goString(ruleSetProxy)))
}

//export CoreStop
func CoreStop() *C.char {
	return resultJSON(api.Stop())
}

//export CoreClose
func CoreClose() {
	api.Close()
}

//export CoreCheckConfig
func CoreCheckConfig(content *C.char) *C.char {
	return resultJSON(api.CheckConfig(goString(content)))
}

//export CoreReloadConfig
func CoreReloadConfig() *C.char {
	return resultJSON(api.ReloadConfig())
}

//export CoreQueryProxies
func CoreQueryProxies() *C.char {
	return cString(api.QueryProxies())
}

//export CoreQueryTraffic
func CoreQueryTraffic() *C.char {
	return cString(api.QueryTraffic())
}

//export CoreQueryLogs
func CoreQueryLogs() *C.char {
	return cString(api.QueryLogs())
}

//export CoreQueryConnections
func CoreQueryConnections() *C.char {
	return cString(api.QueryConnections())
}

//export CoreSelectProxy
func CoreSelectProxy(group *C.char, tag *C.char) *C.char {
	return resultJSON(api.SelectProxy(goString(group), goString(tag)))
}

//export CoreSetMode
func CoreSetMode(mode *C.char) *C.char {
	return resultJSON(api.SetMode(goString(mode)))
}

//export CoreCloseConnection
func CoreCloseConnection(id *C.char) *C.char {
	return resultJSON(api.CloseConnection(goString(id)))
}

//export CoreCloseAllConnections
func CoreCloseAllConnections() *C.char {
	return resultJSON(api.CloseAllConnections())
}

//export CoreTestDelay
func CoreTestDelay(name *C.char, _ *C.char) *C.char {
	return resultJSON(api.TestDelay(goString(name)))
}

//export CoreGetVersion
func CoreGetVersion() *C.char {
	return cString(api.Version())
}

//export CoreSetCallback
func CoreSetCallback(cb unsafe.Pointer) {
	setCallback(cb)
}

//export CoreStartWithContent
func CoreStartWithContent(content *C.char, ruleSetProxy *C.char) *C.char {
	return resultJSON(api.StartWithContent(goString(content), goString(ruleSetProxy)))
}

