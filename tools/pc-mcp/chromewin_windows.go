//go:build windows

package main

// Claude 用の Chrome を、カオルが見ている窓の前に出さずに（フォーカスを取らずに）起動し、窓をいちばん後ろに置く。
// カオルの PC で確かめた（2026-11-27）：最小化の指定（STARTF_USESHOWWINDOW）で起動するとフォーカスを取らない。
// そのあと SW_SHOWNOACTIVATE と HWND_BOTTOM で、ほかの窓の後ろに置く。

import (
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	cwUser32          = syscall.NewLazyDLL("user32.dll")
	pEnumWindows      = cwUser32.NewProc("EnumWindows")
	pGetWindowThreadP = cwUser32.NewProc("GetWindowThreadProcessId")
	pIsWindowVisibleC = cwUser32.NewProc("IsWindowVisible")
	pGetClassNameW    = cwUser32.NewProc("GetClassNameW")
	pShowWindowC      = cwUser32.NewProc("ShowWindow")
	pSetWindowPosC    = cwUser32.NewProc("SetWindowPos")
	pGetForegroundC   = cwUser32.NewProc("GetForegroundWindow")
)

// startQuiet：最小化（フォーカスを取らない）の指定で起動する。返り値はプロセスの番号
func startQuiet(exe string, args []string) (int, error) {
	parts := []string{syscall.EscapeArg(exe)}
	for _, a := range args {
		parts = append(parts, syscall.EscapeArg(a))
	}
	cmd, err := syscall.UTF16PtrFromString(strings.Join(parts, " "))
	if err != nil {
		return 0, err
	}
	var si syscall.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = syscall.STARTF_USESHOWWINDOW
	si.ShowWindow = 7 // SW_SHOWMINNOACTIVE
	var pi syscall.ProcessInformation
	if err := syscall.CreateProcess(nil, cmd, nil, nil, false, 0, nil, nil, &si, &pi); err != nil {
		return 0, err
	}
	syscall.CloseHandle(pi.Thread)
	syscall.CloseHandle(pi.Process)
	return int(pi.ProcessId), nil
}

// chromeWindows：その Chrome（pid）の見えている窓
func chromeWindows(pid int) []uintptr {
	var out []uintptr
	cb := syscall.NewCallback(func(h, _ uintptr) uintptr {
		var p uint32
		pGetWindowThreadP.Call(h, uintptr(unsafe.Pointer(&p)))
		if int(p) != pid {
			return 1
		}
		if v, _, _ := pIsWindowVisibleC.Call(h); v == 0 {
			return 1
		}
		buf := make([]uint16, 64)
		pGetClassNameW.Call(h, uintptr(unsafe.Pointer(&buf[0])), 64)
		if syscall.UTF16ToString(buf) == "Chrome_WidgetWin_1" {
			out = append(out, h)
		}
		return 1
	})
	pEnumWindows.Call(cb, 0)
	return out
}

// sendBack：Claude 用の Chrome の窓を、フォーカスを取らずにいちばん後ろに置く（いちばん前のときは触らない：カオルが見ている）
func sendBack(pid int) {
	if pid == 0 {
		return
	}
	for i := 0; i < 20; i++ {
		ws := chromeWindows(pid)
		if len(ws) > 0 {
			fg, _, _ := pGetForegroundC.Call()
			for _, h := range ws {
				if h == fg {
					continue
				}
				pShowWindowC.Call(h, 4)                       // SW_SHOWNOACTIVATE（最小化から戻す）
				pSetWindowPosC.Call(h, 1, 0, 0, 0, 0, 0x0013) // HWND_BOTTOM・NOSIZE|NOMOVE|NOACTIVATE
			}
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
}
