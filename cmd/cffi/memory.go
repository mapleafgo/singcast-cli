package main

/*
#include <stdlib.h>
*/
import "C"

import "unsafe"

//export CoreFreeString
func CoreFreeString(s *C.char) {
	C.free(unsafe.Pointer(s))
}

func goString(s *C.char) string {
	return C.GoString(s)
}

func cString(s string) *C.char {
	return C.CString(s)
}
