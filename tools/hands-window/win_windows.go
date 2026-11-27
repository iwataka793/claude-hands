//go:build windows

package main

import (
	"encoding/json"
	"os"
	"syscall"
	"unsafe"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	pFindWindow      = user32.NewProc("FindWindowW")
	pGetWindowLong   = user32.NewProc("GetWindowLongPtrW")
	pSetWindowLong   = user32.NewProc("SetWindowLongPtrW")
	pSetWindowPos    = user32.NewProc("SetWindowPos")
	pShowWindow      = user32.NewProc("ShowWindow")
	pSystemParams    = user32.NewProc("SystemParametersInfoW")
	pGetDpiForWindow = user32.NewProc("GetDpiForWindow")
	pGetForeground   = user32.NewProc("GetForegroundWindow")
	pIsWindowVisible = user32.NewProc("IsWindowVisible")
	pGetWindowRect   = user32.NewProc("GetWindowRect")
)

const (
	gwlExStyle       = ^uintptr(19) // -20
	wsExTopmost      = 0x00000008
	wsExToolWindow   = 0x00000080
	wsExNoActivate   = 0x08000000
	swHide           = 0
	swShowNoActivate = 4
	swpNoActivate    = 0x0010
	swpShowWindow    = 0x0040
	spiGetWorkArea   = 0x0030
	hwndTopmost      = ^uintptr(0) // -1
)

type rect struct{ L, T, R, B int32 }

var hwnd uintptr

// prevFG：LIVE の窓を最初に出す前に、いちばん前だった窓（試験で、前の窓を取っていないかを見る）
var prevFG uintptr

var pGetWindowText = user32.NewProc("GetWindowTextW")
var pSetForeground = user32.NewProc("SetForegroundWindow")

// initFG：起動する前にいちばん前だった窓。Wails は窓を作ったときに（見えないままでも）いちばん前を取るので、準備ができたら返す
var initFG uintptr

func winRemember() { initFG, _, _ = pGetForeground.Call() }

func winTitle(h uintptr) string {
	if h == 0 {
		return ""
	}
	buf := make([]uint16, 256)
	pGetWindowText.Call(h, uintptr(unsafe.Pointer(&buf[0])), 256)
	return syscall.UTF16ToString(buf)
}

func findWin() uintptr {
	if hwnd != 0 {
		return hwnd
	}
	t, _ := syscall.UTF16PtrFromString(title)
	h, _, _ := pFindWindow.Call(0, uintptr(unsafe.Pointer(t)))
	hwnd = h
	return h
}

// winPrepare：フォーカスを取らない・タスクバーに出さない・いちばん前
func winPrepare() {
	h := findWin()
	if h == 0 {
		return
	}
	ex, _, _ := pGetWindowLong.Call(h, gwlExStyle)
	pSetWindowLong.Call(h, gwlExStyle, ex|wsExNoActivate|wsExToolWindow|wsExTopmost)
	if fg, _, _ := pGetForeground.Call(); fg == h && initFG != 0 && initFG != h {
		pSetForeground.Call(initFG)
	}
}

// winPlace：画面（作業できる範囲）の右下に置く。w・h は拡大前の大きさ
func winPlace(w, h int) {
	hw := findWin()
	if hw == 0 {
		return
	}
	var wa rect
	pSystemParams.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&wa)), 0)
	dpi, _, _ := pGetDpiForWindow.Call(hw)
	if dpi == 0 {
		dpi = 96
	}
	pw, ph := int32(w)*int32(dpi)/96, int32(h)*int32(dpi)/96
	m := int32(16) * int32(dpi) / 96
	x, y := wa.R-pw-m, wa.B-ph-m
	pSetWindowPos.Call(hw, hwndTopmost, uintptr(x), uintptr(y), uintptr(pw), uintptr(ph), swpNoActivate)
}

func winShow(on bool) {
	h := findWin()
	if h == 0 {
		return
	}
	if on && prevFG == 0 {
		prevFG, _, _ = pGetForeground.Call()
	}
	if on {
		pShowWindow.Call(h, swShowNoActivate)
	} else {
		pShowWindow.Call(h, swHide)
	}
}

// winReport：試験のとき、窓の様子をファイルに書く
func winReport(path string, shown, compact bool) {
	h := findWin()
	var r rect
	vis, _, _ := pIsWindowVisible.Call(h)
	pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&r)))
	ex, _, _ := pGetWindowLong.Call(h, gwlExStyle)
	fg, _, _ := pGetForeground.Call()
	b, _ := json.Marshal(map[string]any{
		"found": h != 0, "visible": vis != 0, "shown": shown, "compact": compact,
		"noActivate": ex&wsExNoActivate != 0, "toolWindow": ex&wsExToolWindow != 0, "topmost": ex&wsExTopmost != 0,
		"foregroundIsLive": fg == h && h != 0, "rect": []int32{r.L, r.T, r.R, r.B},
		"initForeground": winTitle(initFG), "prevForeground": winTitle(prevFG), "prevWasLive": prevFG == h && h != 0, "foreground": winTitle(fg),
	})
	_ = os.WriteFile(path, b, 0o644)
}
