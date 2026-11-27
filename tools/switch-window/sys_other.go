//go:build !windows

package main

func winRemember()                 {}
func winPrepare()                  {}
func winPlace(w, h int, c bool)    {}
func winTopmost(on bool)           {}
func winShow(activate bool)        {}
func winHide()                     {}
func playSound()                   {}
func agentRunning() bool           { return false }
func startAgent(exe string) error  { return nil }
func grantAllow() (bool, string)   { return false, "Windows でない" }
func winReport(path, shown string) {}

type trayHandlers struct {
	open   func()
	allOff func()
	quit   func()
}

func trayStart(h trayHandlers)  {}
func traySet(tip, color string) {}
func trayStop()                 {}
