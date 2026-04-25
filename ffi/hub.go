package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"unsafe"

	"github.com/mapleafgo/singcast/core"
	"github.com/mapleafgo/singcast/translator"
)

func resultJSON(err error) *C.char {
	if err != nil {
		return cString(mustMarshal(map[string]string{"error": err.Error()}))
	}
	return cString(`{"ok":true}`)
}

func svcResultJSON(fn func(*core.Service) error) *C.char {
	svc := core.GetService()
	if svc == nil {
		return cString(`{"error":"core not initialized"}`)
	}
	return resultJSON(fn(svc))
}

func queryCached(getter func(*core.Service) string, fallback string) *C.char {
	svc := core.GetService()
	if svc == nil || svc.Handler() == nil {
		return cString(fallback)
	}
	return cString(getter(svc))
}

//export CoreInit
func CoreInit(homeDir *C.char) *C.char {
	return resultJSON(core.Init(goString(homeDir)))
}

//export CoreStart
func CoreStart(configPath *C.char) *C.char {
	return resultJSON(core.Start(goString(configPath)))
}

//export CoreStop
func CoreStop() *C.char {
	return resultJSON(core.Stop())
}

//export CoreClose
func CoreClose() {
	core.Close()
}

//export CoreCheckConfig
func CoreCheckConfig(jsonContent *C.char) *C.char {
	return resultJSON(core.CheckConfig(goString(jsonContent)))
}

//export CoreReloadConfig
func CoreReloadConfig() *C.char {
	return resultJSON(core.ReloadConfig())
}

//export CoreQueryProxies
func CoreQueryProxies() *C.char {
	return queryCached(func(svc *core.Service) string {
		return svc.Handler().GetCachedGroupsJSON()
	}, "[]")
}

//export CoreQueryTraffic
func CoreQueryTraffic() *C.char {
	return queryCached(func(svc *core.Service) string {
		return svc.Handler().GetCachedStatusJSON()
	}, "{}")
}

//export CoreQueryLogs
func CoreQueryLogs() *C.char {
	return queryCached(func(svc *core.Service) string {
		return svc.Handler().GetCachedLogsJSON()
	}, "[]")
}

//export CoreQueryConnections
func CoreQueryConnections() *C.char {
	return queryCached(func(svc *core.Service) string {
		return svc.Handler().GetCachedConnectionsJSON()
	}, "[]")
}

//export CoreSelectProxy
func CoreSelectProxy(group *C.char, tag *C.char) *C.char {
	return svcResultJSON(func(svc *core.Service) error {
		return svc.SelectOutbound(goString(group), goString(tag))
	})
}

//export CoreSetMode
func CoreSetMode(mode *C.char) *C.char {
	return svcResultJSON(func(svc *core.Service) error {
		return svc.SetClashMode(goString(mode))
	})
}

//export CoreCloseConnection
func CoreCloseConnection(id *C.char) *C.char {
	return svcResultJSON(func(svc *core.Service) error {
		return svc.CloseConnection(goString(id))
	})
}

//export CoreCloseAllConnections
func CoreCloseAllConnections() *C.char {
	return svcResultJSON(func(svc *core.Service) error {
		return svc.CloseConnections()
	})
}

//export CoreTestDelay
func CoreTestDelay(name *C.char, _ *C.char) *C.char {
	return svcResultJSON(func(svc *core.Service) error {
		return svc.URLTest(goString(name))
	})
}

//export CoreGetVersion
func CoreGetVersion() *C.char {
	return cString(core.VersionJSON())
}

//export CoreSetCallback
func CoreSetCallback(cb unsafe.Pointer) {
	setCallback(cb)
}

//export CoreTranslateConfig
func CoreTranslateConfig(yamlContent *C.char) *C.char {
	jsonStr, warnings, err := translator.Translate([]byte(goString(yamlContent)))
	if err != nil {
		return cString(mustMarshal(map[string]string{"error": err.Error()}))
	}
	return cString(mustMarshal(map[string]any{
		"config":   jsonStr,
		"warnings": warnings,
	}))
}
