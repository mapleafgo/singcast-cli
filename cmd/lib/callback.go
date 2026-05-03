package main

/*
#include <stdlib.h>

typedef void (*CoreCallback)(int eventType, const char* data);

static void callCallback(CoreCallback cb, int eventType, const char* data) {
	if (cb != NULL) {
		cb(eventType, data);
	}
}
*/
import "C"

import (
	"sync"
	"unsafe"
)

var (
	globalCallback C.CoreCallback
	callbackMu     sync.RWMutex
)

// cCallbackHandler adapts a C function pointer to ffi.EventHandler.
type cCallbackHandler struct{}

func (cCallbackHandler) OnEvent(eventType int32, jsonPayload string) {
	callbackMu.RLock()
	cb := globalCallback
	callbackMu.RUnlock()
	if cb != nil {
		cs := C.CString(jsonPayload)
		C.callCallback(cb, C.int(eventType), cs)
		C.free(unsafe.Pointer(cs))
	}
}

// setCallback stores the C function pointer and wires it into the event system.
func setCallback(cb unsafe.Pointer) {
	callbackMu.Lock()
	globalCallback = C.CoreCallback(cb)
	callbackMu.Unlock()

	api.SetOnEvent(cCallbackHandler{})
}
