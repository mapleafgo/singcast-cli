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

	"github.com/mapleafgo/singcast/core"
)

var (
	globalCallback C.CoreCallback
	callbackMu     sync.RWMutex
)

// setCallback stores the C function pointer and wires it into the core event system.
func setCallback(cb unsafe.Pointer) {
	callbackMu.Lock()
	globalCallback = C.CoreCallback(cb)
	callbackMu.Unlock()

	core.SetOnEvent(func(eventType int, jsonPayload string) {
		callbackMu.RLock()
		cb := globalCallback
		callbackMu.RUnlock()
		if cb != nil {
			cs := C.CString(jsonPayload)
			C.callCallback(cb, C.int(eventType), cs)
			C.free(unsafe.Pointer(cs))
		}
	})
}
