package main

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embedded embed.FS

func webAssets() (fs.FS, bool) {
	assets, err := fs.Sub(embedded, "dist")
	if err != nil {
		return nil, false
	}
	if _, statErr := fs.Stat(assets, "index.html"); statErr != nil {
		return nil, false
	}
	return assets, true
}
