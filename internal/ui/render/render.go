// Package render parses the embedded HTML templates once and exposes
// thin Render helpers for handlers to use. Mirrors goop2's pattern:
// every page composes the shared `layout` template, which dispatches
// to a per-page body via the `include` template func + ContentTmpl
// field on the viewmodel.
package render

import (
	"fmt"
	"html"
	"html/template"
	"net/http"
	"strings"
	"sync"

	"github.com/petervdpas/formidable2/internal/ui"
)

var (
	tmpl    *template.Template
	once    sync.Once
	initErr error
)

// initTemplates parses every templates/*.html file once. Subsequent
// calls are no-ops. Errors are sticky so repeated calls don't retry.
func initTemplates() error {
	once.Do(func() {
		funcs := template.FuncMap{
			// include lets the layout dispatch to a body template named
			// at runtime (.ContentTmpl). html/template can't `{{template
			// .Var .}}`, so we route through a helper that ExecuteTemplates
			// the named template and returns the result as template.HTML.
			"include": func(name string, data any) template.HTML {
				if tmpl == nil {
					return template.HTML(`<pre class="err">templates not initialized</pre>`)
				}
				if name == "" {
					return template.HTML(`<pre class="err">empty template name</pre>`)
				}
				var b strings.Builder
				if err := tmpl.ExecuteTemplate(&b, name, data); err != nil {
					return template.HTML(`<pre class="err">` + html.EscapeString(err.Error()) + `</pre>`)
				}
				return template.HTML(b.String())
			},
		}

		t, err := template.New("root").Funcs(funcs).ParseFS(ui.TemplatesFS, "templates/*.html")
		if err != nil {
			initErr = err
			return
		}
		tmpl = t
	})
	return initErr
}

// Layout renders the shared `layout` template, which then includes the
// per-page body via the viewmodel's ContentTmpl field. data MUST embed
// or set the layout fields (Title, Theme, ContentTmpl, …) — typed
// viewmodels in `internal/ui/viewmodels` make that easy.
func Layout(w http.ResponseWriter, data any) {
	if err := initTemplates(); err != nil {
		http.Error(w, fmt.Sprintf("template init error: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, fmt.Sprintf("template error: %v", err), http.StatusInternalServerError)
	}
}

// Standalone renders a named template directly, without the layout
// wrapper. Used for error pages and partial-fragment endpoints.
func Standalone(w http.ResponseWriter, name string, data any) {
	if err := initTemplates(); err != nil {
		http.Error(w, fmt.Sprintf("template init error: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, fmt.Sprintf("template error: %v", err), http.StatusInternalServerError)
	}
}

// MustInit forces an early parse + returns any error. Useful from main.go
// or tests so failures surface at startup rather than first-page-load.
func MustInit() error { return initTemplates() }
