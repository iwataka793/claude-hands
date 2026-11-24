//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// hideWindow：常駐役は画面を持たないので、PowerShell の黒い窓を出さない。
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
