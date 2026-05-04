// Package ui owns the server-side HTML rendering pipeline for the
// Wails app. Templates and static assets are embedded into the binary
// at build time so the running app has zero filesystem dependency on
// the source tree.
//
// Layout:
//   internal/ui/templates/   — Go html/template files (layout + pages)
//   internal/ui/assets/      — CSS / JS / fonts served at /assets/...
//   internal/ui/render/      — template parsing + render helpers
//   internal/ui/handlers/    — http.Handler with route table
//   internal/ui/viewmodels/  — typed data shapes passed to templates
//
// The pattern mirrors goop2's wiki renderer: each page composes the
// shared `layout` template with a body picked by `.ContentTmpl`. New
// pages = new template + new handler + viewmodel struct.
package ui

import "embed"

//go:embed templates/*.html
var TemplatesFS embed.FS

//go:embed all:assets
var AssetsFS embed.FS
