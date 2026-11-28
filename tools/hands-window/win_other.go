//go:build !windows

package main

import "os"

func winRemember()                               {}
func winPrepare()                                {}
func winPlace(w, h int)                          {}
func winShow(on bool)                            {}
func winReport(path string, shown, compact bool) {}
func winSwitchOpen() bool                        { return false }
func winBeep()                                   {}

func startProc(exe string, args ...string) error { return nil }

func regQuery(key, value string) (string, error) { return "", os.ErrNotExist }
