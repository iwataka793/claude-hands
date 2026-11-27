//go:build windows

package main

// 通知領域（右下）の丸。色はいちばん強いスイッチ（OFF 赤・VIEW 灰・OPERATE 緑・ALLOW オレンジ）。
// 左で押すとスイッチの窓、右で押すとメニュー（開く・すべてオフ・終わる）。
// Wails には通知領域が無いので、自分の糸（スレッド）に見えない窓を作って Shell_NotifyIcon で出す。

import (
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

var (
	gdi32               = syscall.NewLazyDLL("gdi32.dll")
	pRegisterClassEx    = user32.NewProc("RegisterClassExW")
	pCreateWindowEx     = user32.NewProc("CreateWindowExW")
	pDefWindowProc      = user32.NewProc("DefWindowProcW")
	pGetMessage         = user32.NewProc("GetMessageW")
	pTranslateMessage   = user32.NewProc("TranslateMessage")
	pDispatchMessage    = user32.NewProc("DispatchMessageW")
	pPostMessage        = user32.NewProc("PostMessageW")
	pRegisterWindowMsg  = user32.NewProc("RegisterWindowMessageW")
	pCreatePopupMenu    = user32.NewProc("CreatePopupMenu")
	pAppendMenu         = user32.NewProc("AppendMenuW")
	pTrackPopupMenu     = user32.NewProc("TrackPopupMenu")
	pDestroyMenu        = user32.NewProc("DestroyMenu")
	pGetCursorPos       = user32.NewProc("GetCursorPos")
	pCreateIconIndirect = user32.NewProc("CreateIconIndirect")
	pDestroyIcon        = user32.NewProc("DestroyIcon")
	pShellNotifyIcon    = shell32.NewProc("Shell_NotifyIconW")
	pCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	pCreateBitmap       = gdi32.NewProc("CreateBitmap")
	pDeleteObject       = gdi32.NewProc("DeleteObject")
)

const (
	wmApp          = 0x8000
	wmTray         = wmApp + 1
	wmQuitTray     = wmApp + 2
	wmLButtonUp    = 0x0202
	wmRButtonUp    = 0x0205
	wmNull         = 0x0000
	nimAdd         = 0
	nimModify      = 1
	nimDelete      = 2
	nifMessage     = 0x1
	nifIcon        = 0x2
	nifTip         = 0x4
	mfString       = 0x0
	mfSeparator    = 0x800
	tpmReturnCmd   = 0x0100
	tpmRightButton = 0x0002
)

type notifyIconData struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         [16]byte
	hBalloonIcon     uintptr
}

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      [2]int32
	private uint32
}

type trayHandlers struct {
	open   func()
	allOff func()
	quit   func()
}

var (
	trayMu       sync.Mutex
	trayHwnd     uintptr
	trayIcon     uintptr
	trayTipNow   string
	trayColorNow = "off"
	trayAdded    bool
	trayH        trayHandlers
	taskbarMsg   uintptr
)

var trayColors = map[string][3]byte{
	"off": {0xF2, 0x55, 0x5A}, "view": {0xC7, 0xC7, 0xD1}, "operate": {0x3D, 0xD6, 0x8C}, "allow": {0xFF, 0x8A, 0x3D},
}

// trayStart：通知領域に丸を出す（自分の糸で、窓のメッセージを回し続ける）
func trayStart(h trayHandlers) {
	trayH = h
	ready := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		cls, _ := syscall.UTF16PtrFromString("claude-hands-switch-tray")
		wc := wndClassEx{lpfnWndProc: syscall.NewCallback(trayProc), lpszClassName: cls}
		wc.cbSize = uint32(unsafe.Sizeof(wc))
		pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
		hw, _, _ := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(cls)), 0, 0, 0, 0, 0, 0, 0, 0, 0)
		tb, _ := syscall.UTF16PtrFromString("TaskbarCreated")
		taskbarMsg, _, _ = pRegisterWindowMsg.Call(uintptr(unsafe.Pointer(tb)))
		trayMu.Lock()
		trayHwnd = hw
		trayMu.Unlock()
		trayNotify(nimAdd)
		close(ready)
		var m msg
		for {
			r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				return
			}
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			pDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
		}
	}()
	<-ready
}

func trayProc(hw, m, wp, lp uintptr) uintptr {
	switch {
	case m == wmTray:
		switch lp & 0xFFFF {
		case wmLButtonUp:
			if trayH.open != nil {
				go trayH.open()
			}
		case wmRButtonUp:
			trayMenu(hw)
		}
		return 0
	case m == wmQuitTray:
		trayNotify(nimDelete)
		return 0
	case taskbarMsg != 0 && m == taskbarMsg:
		trayNotify(nimAdd) // エクスプローラーが立ち直ったら出し直す
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hw, m, wp, lp)
	return r
}

func trayMenu(hw uintptr) {
	menu, _, _ := pCreatePopupMenu.Call()
	add := func(id uintptr, s string) {
		p, _ := syscall.UTF16PtrFromString(s)
		pAppendMenu.Call(menu, mfString, id, uintptr(unsafe.Pointer(p)))
	}
	add(1, "開く")
	add(2, "すべてオフ")
	pAppendMenu.Call(menu, mfSeparator, 0, 0)
	add(3, "終わる")
	var pt [2]int32
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForeground.Call(hw)
	id, _, _ := pTrackPopupMenu.Call(menu, tpmReturnCmd|tpmRightButton, uintptr(pt[0]), uintptr(pt[1]), 0, hw, 0)
	pPostMessage.Call(hw, wmNull, 0, 0)
	pDestroyMenu.Call(menu)
	switch id {
	case 1:
		if trayH.open != nil {
			go trayH.open()
		}
	case 2:
		if trayH.allOff != nil {
			go trayH.allOff()
		}
	case 3:
		if trayH.quit != nil {
			go trayH.quit()
		}
	}
}

func trayNotify(op uintptr) {
	trayMu.Lock()
	defer trayMu.Unlock()
	if trayHwnd == 0 {
		return
	}
	if trayIcon == 0 {
		trayIcon = makeDotIcon(trayColors[trayColorNow])
	}
	var d notifyIconData
	d.cbSize = uint32(unsafe.Sizeof(d))
	d.hWnd = trayHwnd
	d.uID = 1
	d.uFlags = nifMessage | nifIcon | nifTip
	d.uCallbackMessage = wmTray
	d.hIcon = trayIcon
	tip := trayTipNow
	if tip == "" {
		tip = "claude-hands SWITCH"
	}
	copy(d.szTip[:127], syscall.StringToUTF16(tip))
	r, _, _ := pShellNotifyIcon.Call(op, uintptr(unsafe.Pointer(&d)))
	switch op {
	case nimAdd:
		trayAdded = r != 0
	case nimDelete:
		trayAdded = false
	}
}

// traySet：丸の色と文字を変える
func traySet(tip, color string) {
	trayMu.Lock()
	changed := color != trayColorNow
	trayTipNow, trayColorNow = tip, color
	if changed && trayIcon != 0 {
		pDestroyIcon.Call(trayIcon)
		trayIcon = 0
	}
	trayMu.Unlock()
	trayNotify(nimModify)
}

func trayStop() {
	trayMu.Lock()
	hw := trayHwnd
	trayMu.Unlock()
	if hw != 0 {
		trayNotify(nimDelete)
	}
}

// makeDotIcon：32×32 の丸（ふちを少し暗く）。色つきの絵は 32 ビットで、透けるところはアルファ 0
func makeDotIcon(c [3]byte) uintptr {
	const n = 32
	type bmih struct {
		biSize          uint32
		biWidth         int32
		biHeight        int32
		biPlanes        uint16
		biBitCount      uint16
		biCompression   uint32
		biSizeImage     uint32
		biXPelsPerMeter int32
		biYPelsPerMeter int32
		biClrUsed       uint32
		biClrImportant  uint32
	}
	h := bmih{biWidth: n, biHeight: -n, biPlanes: 1, biBitCount: 32}
	h.biSize = uint32(unsafe.Sizeof(h))
	var bits unsafe.Pointer
	color, _, _ := pCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&h)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if color == 0 || bits == nil {
		return 0
	}
	px := unsafe.Slice((*byte)(bits), n*n*4)
	const cx, cy, r, ring = 15.5, 15.5, 13.0, 2.2
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			// 4×4 で数えて、ふちをなめらかに
			in, edge := 0, 0
			for sy := 0; sy < 4; sy++ {
				for sx := 0; sx < 4; sx++ {
					dx, dy := float64(x)+(float64(sx)+0.5)/4-cx-0.5, float64(y)+(float64(sy)+0.5)/4-cy-0.5
					d2 := dx*dx + dy*dy
					if d2 <= r*r {
						in++
						if d2 > (r-ring)*(r-ring) {
							edge++
						}
					}
				}
			}
			a := in * 255 / 16
			k := 1.0 - 0.45*float64(edge)/float64(max(in, 1)) // ふちは暗く
			i := (y*n + x) * 4
			// 先にアルファを掛けた形（BGRA）
			px[i+0] = byte(float64(c[2]) * k * float64(a) / 255)
			px[i+1] = byte(float64(c[1]) * k * float64(a) / 255)
			px[i+2] = byte(float64(c[0]) * k * float64(a) / 255)
			px[i+3] = byte(a)
		}
	}
	mask, _, _ := pCreateBitmap.Call(n, n, 1, 1, 0)
	type iconInfo struct {
		fIcon    int32
		xHotspot uint32
		yHotspot uint32
		hbmMask  uintptr
		hbmColor uintptr
	}
	ii := iconInfo{fIcon: 1, hbmMask: mask, hbmColor: color}
	icon, _, _ := pCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	pDeleteObject.Call(color)
	pDeleteObject.Call(mask)
	return icon
}
