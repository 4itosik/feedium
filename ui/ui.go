package ui

import (
	"io/fs"
	"net/http"

	"embed"
)

//go:embed all:dist
var files embed.FS

// Assets returns an http.FileSystem rooted at the dist/ directory.
func Assets() http.FileSystem {
	dist, err := fs.Sub(files, "dist")
	if err != nil {
		// dist/ must exist at build time (run make ui-build first)
		panic("ui: dist/ not embedded: " + err.Error())
	}
	return http.FS(dist)
}
