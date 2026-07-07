// Package viewerui embeds the built Formidable Viewer shell (the Vue SPA). It
// lives beside the dist output because go:embed paths cannot reach across
// directories, so the cmd binary imports this package to get the assets.
package viewerui

import "embed"

//go:embed all:dist
var assets embed.FS

// Assets is the embedded dist tree, rooted so "index.html" is at the top.
var Assets = assets
