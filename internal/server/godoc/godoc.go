// Package godoc serves the embedded `go doc` output of the internal API on the
// internal HTTP server. The text is generated at build time (see gen/), so the
// shipped binary carries it without source or the go toolchain at runtime.
package godoc

import (
	_ "embed"
	"html"
	"net/http"
)

//go:generate go run ./gen

//go:embed static/godoc.txt
var docText string

const page = `<!doctype html><meta charset="utf-8">
<title>Formidable API (go doc)</title>
<style>body{font:13px/1.5 ui-monospace,monospace;margin:2rem;max-width:64rem;color:#d7dce5;background:#1c2230}pre{white-space:pre-wrap}h1{font-size:1.1rem}</style>
<h1>Formidable internal API</h1>
<pre>`

// Handler serves the embedded docs at any path. Mount it under a prefix on the
// internal server.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
		_, _ = w.Write([]byte(html.EscapeString(docText)))
		_, _ = w.Write([]byte("</pre>"))
	})
}
