// Package render is Formidable's two-stage Handlebars->Markdown->HTML
// pipeline, shared by the Storage workspace's "Render" button and the
// internal HTTP server. The Manager wraps the stage functions for Wails.
package render

import "github.com/petervdpas/formidable2/internal/modules/template"

// Options carries per-render configuration. URL strategies are funcs so
// each consumer (slideout, wiki HTTP server, MD export) plugs its own
// scheme without leaking transport details into this package.
type Options struct {
	// ImageURL resolves an image filename to a URL; nil returns "images/<name>".
	ImageURL func(name string) string

	// ImageBase64URL resolves an image to a data: URL (generator inline mode,
	// self-contained exports); independent of ImageURL. Nil returns "".
	ImageBase64URL func(name string) string

	// LinkURL resolves a relative href; absolute and file:/mailto:/tel: pass
	// through before this is called. Nil returns the href unchanged.
	LinkURL func(href string) string

	// FormidableLinkURL rewrites a parsed formidable://<template>:<datafile>
	// href. Nil keeps the formidable:// URL (the slideout's Vue interceptor
	// handles the click); empty-string return falls back to the original URL.
	FormidableLinkURL func(templateFilename, datafile string) string

	// LoadTemplate resolves a template for api-field helpers (column types,
	// option-label headers). Nil, or a nil return, makes the helpers degrade
	// to JSON fallbacks; safe on targets that don't render api fields.
	LoadTemplate func(name string) *template.Template

	// TemplateFilename and Datafile drive the meta-category helpers
	// ({{templateName}}, {{datafile}}, …); empty strings expand to "".
	TemplateFilename string
	Datafile         string

	// Facets is the per-record facetKey->selectedLabel projection for the
	// {{virtual-field}} helper. Plain string map (not storage.FacetState) to
	// keep render decoupled from storage; the Manager flattens it.
	Facets map[string]string
}

// Result is the dual-stage output of RenderForm.
type Result struct {
	Markdown string `json:"markdown"`
	HTML     string `json:"html"`
}
