// Package web embeds the built web app (copied into dist/ at build time).
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the web app files.
func FS() fs.FS {
	sub, _ := fs.Sub(dist, "dist")
	return sub
}
