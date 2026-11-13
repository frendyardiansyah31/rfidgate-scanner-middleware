// Package web holds the built dashboard (web/dist) embedded into the binary.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the dashboard files rooted at dist/, so index.html is at "index.html".
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // dist is embedded at compile time; Sub only fails on a bad path
	}
	return sub
}
