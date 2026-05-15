// Package web embeds the built single-page application so the compiled
// mongosync-ui binary is fully self-contained.
package web

import (
	"embed"
	"io/fs"
)

// dist holds the production build of the React UI. The placeholder
// index.html is committed so the package compiles before the UI is built;
// `vite build` overwrites the directory with real assets.
//
//go:embed all:dist
var dist embed.FS

// FS returns the embedded UI rooted at the dist directory.
func FS() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}
