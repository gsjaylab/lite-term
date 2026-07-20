package web

import (
	"embed"
	"io/fs"
)

// all: is required so a clean checkout can embed the otherwise hidden .gitkeep.
//
//go:embed all:dist
var embedded embed.FS

func Assets() fs.FS {
	assets, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic(err)
	}
	return assets
}
