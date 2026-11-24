//go:build !windows

package main

import "io/fs"

func isCloudOnly(fi fs.FileInfo) bool { return false }

func isReparse(d fs.DirEntry) bool { return d.Type()&fs.ModeSymlink != 0 }
