//go:build windows

package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	shell32          = syscall.NewLazyDLL("shell32.dll")
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	winmm            = syscall.NewLazyDLL("winmm.dll")
	pFindWindow      = user32.NewProc("FindWindowW")
	pGetWindowLong   = user32.NewProc("GetWindowLongPtrW")
	pSetWindowPos    = user32.NewProc("SetWindowPos")
	pShowWindow      = user32.NewProc("ShowWindow")
	pSystemParams    = user32.NewProc("SystemParametersInfoW")
	pGetDpiForWindow = user32.NewProc("GetDpiForWindow")
	pGetForeground   = user32.NewProc("GetForegroundWindow")
	pSetForeground   = user32.NewProc("SetForegroundWindow")
	pIsWindowVisible = user32.NewProc("IsWindowVisible")
	pGetWindowRect   = user32.NewProc("GetWindowRect")
	pGetWindowText   = user32.NewProc("GetWindowTextW")
	pMessageBeep     = user32.NewProc("MessageBeep")
	pPlaySound       = winmm.NewProc("PlaySoundW")
	pShellExecuteEx  = shell32.NewProc("ShellExecuteExW")
	pWaitForSingle   = kernel32.NewProc("WaitForSingleObject")
	pGetExitCode     = kernel32.NewProc("GetExitCodeProcess")
	pCloseHandle     = kernel32.NewProc("CloseHandle")
)

const (
	gwlExStyle       = ^uintptr(19) // -20
	wsExTopmost      = 0x00000008
	swHide           = 0
	swShow           = 5
	swShowNoActivate = 4
	swRestore        = 9
	swpNoActivate    = 0x0010
	swpNoMove        = 0x0002
	swpNoSize        = 0x0001
	spiGetWorkArea   = 0x0030
	hwndTopmost      = ^uintptr(0) // -1
	hwndNoTopmost    = ^uintptr(1) // -2
)

type rect struct{ L, T, R, B int32 }

var hwnd uintptr

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

func winPrepare() {
	h := findWin()
	if h == 0 {
		return
	}
	if fg, _, _ := pGetForeground.Call(); fg == h && initFG != 0 && initFG != h {
		pSetForeground.Call(initFG)
	}
}

// winPlace：center なら画面（作業できる範囲）の真ん中、でなければ右下。w・h は拡大前の大きさ
func winPlace(w, h int, center bool) {
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
	if max := (wa.B - wa.T) - 16; ph > max { // 画面より高くしない
		ph = max
	}
	m := int32(16) * int32(dpi) / 96
	x, y := wa.R-pw-m, wa.B-ph-m // 右下（スイッチの窓を出している間、LIVE の窓は隠れて、この窓の下の帯が LIVE の役をする）
	if center {
		x, y = wa.L+(wa.R-wa.L-pw)/2, wa.T+(wa.B-wa.T-ph)/2
	}
	pSetWindowPos.Call(hw, 0, uintptr(x), uintptr(y), uintptr(pw), uintptr(ph), swpNoActivate)
}

func winTopmost(on bool) {
	h := findWin()
	if h == 0 {
		return
	}
	after := hwndNoTopmost
	if on {
		after = hwndTopmost
	}
	pSetWindowPos.Call(h, after, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
}

// winShow：activate なら前に出してフォーカスも渡す（スイッチの窓）。でなければフォーカスは取らない（頼みの窓）
func winShow(activate bool) {
	h := findWin()
	if h == 0 {
		return
	}
	if activate {
		pShowWindow.Call(h, swRestore)
		pShowWindow.Call(h, swShow)
		pSetForeground.Call(h)
	} else {
		pShowWindow.Call(h, swShowNoActivate)
	}
}

func winHide() {
	if h := findWin(); h != 0 {
		pShowWindow.Call(h, swHide)
	}
}

// playSound：カオルを待つときの音（Windows の通知音）
func playSound() {
	s, _ := syscall.UTF16PtrFromString("Notification.Default")
	const sndAlias, sndAsync, sndNoDefault = 0x00010000, 0x0001, 0x0002
	if r, _, _ := pPlaySound.Call(uintptr(unsafe.Pointer(s)), 0, sndAlias|sndAsync|sndNoDefault); r == 0 {
		pMessageBeep.Call(0x40) // MB_ICONINFORMATION
	}
}

// agentRunning：常駐役（pc-agent.exe）が動いているか
func agentRunning() bool {
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(snap)
	var e syscall.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = syscall.Process32First(snap, &e); err == nil; err = syscall.Process32Next(snap, &e) {
		if strings.EqualFold(syscall.UTF16ToString(e.ExeFile[:]), "pc-agent.exe") {
			return true
		}
	}
	return false
}

func startAgent(exe string) error {
	c := exec.Command(exe, "agent")
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	return c.Start()
}

type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         uintptr
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     uintptr
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    uintptr
	dwHotKey     uint32
	hIcon        uintptr
	hProcess     uintptr
}

// allowScript：今までの PC-ALLOW.bat と同じこと（管理者で allow.txt を書く。中の場所は管理者しか書けない）
const allowScript = `$A = Join-Path $env:ProgramData 'pc-agent'
New-Item -ItemType Directory -Force $A | Out-Null
icacls $A /inheritance:r /grant:r '*S-1-5-32-544:(OI)(CI)F' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-545:(OI)(CI)RX' | Out-Null
[IO.File]::WriteAllText((Join-Path $A 'allow.txt'), "allow` + "`r`n" + `")
`

// grantAllow：UAC（管理者の確認）を通って ALLOW を書く。終わるまで待つ。返り値：書けたか・書けなかった理由
func grantAllow() (bool, string) {
	u := utf16.Encode([]rune(allowScript))
	b := make([]byte, len(u)*2)
	for i, c := range u {
		b[2*i], b[2*i+1] = byte(c), byte(c>>8)
	}
	params := "-NoProfile -NonInteractive -WindowStyle Hidden -EncodedCommand " + base64.StdEncoding.EncodeToString(b)
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString("powershell.exe")
	par, _ := syscall.UTF16PtrFromString(params)
	info := shellExecuteInfo{fMask: 0x00000040, lpVerb: verb, lpFile: file, lpParameters: par, nShow: swHide} // SEE_MASK_NOCLOSEPROCESS
	info.cbSize = uint32(unsafe.Sizeof(info))
	if r, _, err := pShellExecuteEx.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		if errno, ok := err.(syscall.Errno); ok && errno == 1223 { // ERROR_CANCELLED
			return false, "UAC で「いいえ」"
		}
		return false, "UAC を出せなかった：" + err.Error()
	}
	if info.hProcess == 0 {
		return false, "管理者の処理が始まらなかった"
	}
	defer pCloseHandle.Call(info.hProcess)
	pWaitForSingle.Call(info.hProcess, 120000)
	var code uint32
	pGetExitCode.Call(info.hProcess, uintptr(unsafe.Pointer(&code)))
	if code != 0 {
		return false, "書けなかった（終わりの番号 " + itoa(int(code)) + "）"
	}
	return true, ""
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// winReport：試験のとき、窓の様子をファイルに書く
func winReport(path, shown string) {
	h := findWin()
	var r rect
	vis, _, _ := pIsWindowVisible.Call(h)
	pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&r)))
	ex, _, _ := pGetWindowLong.Call(h, gwlExStyle)
	fg, _, _ := pGetForeground.Call()
	b, _ := json.Marshal(map[string]any{
		"found": h != 0, "visible": vis != 0, "shown": shown, "topmost": ex&wsExTopmost != 0,
		"foregroundIsSwitch": fg == h && h != 0, "rect": []int32{r.L, r.T, r.R, r.B},
		"initForeground": winTitle(initFG), "foreground": winTitle(fg), "tray": trayAdded,
	})
	_ = os.WriteFile(path, b, 0o644)
}
