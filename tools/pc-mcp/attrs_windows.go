//go:build windows

package main

import (
	"io/fs"
	"syscall"
)

const (
	attrReparse        = 0x400
	attrOffline        = 0x1000
	attrRecallOnOpen   = 0x40000
	attrRecallOnAccess = 0x400000
)

func attrs(fi fs.FileInfo) uint32 {
	if d, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		return d.FileAttributes
	}
	return 0
}

// isCloudOnly：OneDrive などで、中身がクラウドにだけあるファイル
func isCloudOnly(fi fs.FileInfo) bool {
	return attrs(fi)&(attrOffline|attrRecallOnOpen|attrRecallOnAccess) != 0
}

// isReparse：ジャンクション・シンボリックリンクなどのつなぎ目
func isReparse(d fs.DirEntry) bool {
	if d.Type()&fs.ModeSymlink != 0 || d.Type()&fs.ModeIrregular != 0 {
		return true
	}
	fi, err := d.Info()
	if err != nil {
		return false
	}
	return attrs(fi)&attrReparse != 0
}
