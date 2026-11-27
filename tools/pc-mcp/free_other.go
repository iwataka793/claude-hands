//go:build !windows

package main

import "syscall"

// diskFree：ドライブの空き（バイト）。分からなければ -1
func diskFree(root string) int64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(root, &st); err != nil {
		return -1
	}
	return int64(st.Bavail) * int64(st.Bsize)
}
