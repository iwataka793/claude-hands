package main

import (
	"embed"
	"io/fs"
)

func sub(f embed.FS) fs.FS {
	s, err := fs.Sub(f, "frontend")
	if err != nil {
		panic(err)
	}
	return s
}
