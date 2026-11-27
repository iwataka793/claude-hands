//go:build !windows

package main

import "os/exec"

func startQuiet(exe string, args []string) (int, error) {
	c := exec.Command(exe, args...)
	if err := c.Start(); err != nil {
		return 0, err
	}
	go func() { _ = c.Wait() }()
	return c.Process.Pid, nil
}

func sendBack(pid int) {}
