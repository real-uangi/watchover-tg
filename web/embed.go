// Package web provides the compiled management application.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var compiled embed.FS

// Assets returns the production assets with index.html at the filesystem root.
func Assets() fs.FS {
	assets, err := fs.Sub(compiled, "dist")
	if err != nil {
		panic(err)
	}
	return assets
}
