//go:build !windows

package main

func winRemember()                               {}
func winPrepare()                                {}
func winPlace(w, h int)                          {}
func winShow(on bool)                            {}
func winReport(path string, shown, compact bool) {}
