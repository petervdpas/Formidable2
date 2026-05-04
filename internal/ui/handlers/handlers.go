// Package handlers wires the ui package to the Wails asset server.
// Each route renders a typed viewmodel through render.Layout, or
// serves a static asset from the embedded assets FS.
package handlers

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/config"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
	"github.com/petervdpas/formidable2/internal/ui"
	"github.com/petervdpas/formidable2/internal/ui/render"
	"github.com/petervdpas/formidable2/internal/ui/viewmodels"
)

// Deps is the bundle of services the handlers read from. Keeps the
// handler signature stable and makes it obvious from main.go what the
// UI layer depends on. Uses the Service surfaces — the same methods
// the Wails bindings expose, so handlers and bindings see identical
// behavior.
type Deps struct {
	Template *template.Service
	Storage  *storage.Service
	Config   *config.Service
	AppName  string
	Version  string
}

// New returns an http.Handler that serves the studio. Mounted as the
// Wails AssetOptions.Handler — every webview request flows through here.
func New(d Deps) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /assets/", assetsHandler())
	mux.HandleFunc("GET /{$}", index(d))
	mux.HandleFunc("GET /favicon.ico", favicon)

	return mux
}

// assetsHandler serves files from internal/ui/assets via the embedded FS.
// The leading "assets/" prefix in the embedded paths is stripped by
// fs.Sub so URL `/assets/app.css` maps to `assets/app.css` in the FS.
func assetsHandler() http.Handler {
	sub, err := fs.Sub(ui.AssetsFS, "assets")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "assets unavailable: "+err.Error(), http.StatusInternalServerError)
		})
	}
	return http.StripPrefix("/assets/", http.FileServer(http.FS(sub)))
}

// favicon — Wails' webview asks for /favicon.ico early. Return 204 so
// devtools doesn't whine about a missing favicon while we don't ship one yet.
func favicon(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// index renders the studio home page (template list).
func index(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filenames, err := d.Template.ListTemplates()
		if err != nil {
			http.Error(w, "list templates: "+err.Error(), http.StatusInternalServerError)
			return
		}

		rows := make([]viewmodels.TemplateRow, 0, len(filenames))
		for _, fn := range filenames {
			row := viewmodels.TemplateRow{Filename: fn}
			if t, err := d.Template.LoadTemplate(fn); err == nil && t != nil {
				row.Name = strings.TrimSpace(t.Name)
				if row.Name == "" {
					row.Name = strings.TrimSuffix(fn, ".yaml")
				}
			} else {
				row.Name = strings.TrimSuffix(fn, ".yaml")
			}
			if forms, err := d.Storage.ListForms(fn); err == nil {
				row.FormCount = len(forms)
			}
			rows = append(rows, row)
		}

		data := viewmodels.Index{
			Layout: viewmodels.Layout{
				Title:       d.AppName + " — Templates",
				Theme:       resolveTheme(d.Config),
				Active:      "templates",
				AppName:     d.AppName,
				AppVersion:  d.Version,
				ContentTmpl: "page.index",
			},
			Templates: rows,
		}
		render.Layout(w, data)
	}
}

// resolveTheme reads the active profile's theme. Defaults to "light"
// on any error (matches the renderer's early-theme fallback).
func resolveTheme(cfg *config.Service) string {
	if cfg == nil {
		return "light"
	}
	c, err := cfg.LoadUserConfig()
	if err != nil || c == nil {
		return "light"
	}
	switch c.Theme {
	case "dark", "purplish":
		return c.Theme
	default:
		return "light"
	}
}
