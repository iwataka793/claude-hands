//go:build windows

package main

import (
	"runtime"
	"syscall"
	"unsafe"
)

var (
	hwUser32          = syscall.NewLazyDLL("user32.dll")
	pWindowFromPoint  = hwUser32.NewProc("WindowFromPoint")
	pGetAncestor      = hwUser32.NewProc("GetAncestor")
	pGetWindowTextW   = hwUser32.NewProc("GetWindowTextW")
	pSetThreadDpiAwar = hwUser32.NewProc("SetThreadDpiAwarenessContext")
)

// windowTitleAt：画面の (x, y)（画素）にある窓の、いちばん上の親の題名
func windowTitleAt(x, y int) string {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if pSetThreadDpiAwar.Find() == nil {
		old, _, _ := pSetThreadDpiAwar.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2（-4）：画素のまま
		defer pSetThreadDpiAwar.Call(old)
	}
	pt := uintptr(uint32(int32(x))) | uintptr(uint32(int32(y)))<<32
	h, _, _ := pWindowFromPoint.Call(pt)
	if h == 0 {
		return ""
	}
	if r, _, _ := pGetAncestor.Call(h, 2); r != 0 { // GA_ROOT
		h = r
	}
	buf := make([]uint16, 256)
	pGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), 256)
	return syscall.UTF16ToString(buf)
}
