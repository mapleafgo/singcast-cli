package main

/*
#include <stdlib.h>
*/
import "C"

import "unsafe"

// CoreFreeString 释放本库返回的字符串。所有返回 char* 的 Core* 函数都把所有权
// 交给调用方，必须由此函数释放。传 NULL 是安全的空操作。
// 同一指针不可重复释放。
//
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
