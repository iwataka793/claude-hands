//go:build !windows

package main

import (
	"io/fs"
	"time"
)

func isCloudOnly(fi fs.FileInfo) bool { return false }

func isReparse(d fs.DirEntry) bool { return d.Type()&fs.ModeSymlink != 0 }

// createdAt：作成日時（Windows 以外は更新日時で代わりにする。試験用）
func createdAt(fi fs.FileInfo) time.Time { return fi.ModTime() }
