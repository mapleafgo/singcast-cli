package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"unsafe"
)

//export CoreFreeString
func CoreFreeString(s *C.char) {
	C.free(unsafe.Pointer(s))
}

// goString converts a C string to a Go string.
func goString(s *C.char) string {
	return C.GoString(s)
}

// cString converts a Go string to a C string (caller must free via CoreFreeString).
func cString(s string) *C.char {
	return C.CString(s)
}

func mustMarshal(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}
