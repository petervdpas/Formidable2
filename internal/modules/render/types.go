// Package render is Formidable2's two-stage Handlebars→Markdown→HTML
// pipeline. It mirrors the original `controls/markdownRenderer.js` +
// `controls/htmlRenderer.js` and is shared by the Storage workspace's
// "Render" button and the future internal HTTP server.
//
// Public surface:
//   - RenderMarkdown(values, tpl, opts) → markdown
//   - RenderHTML(md) → sanitized HTML
//   - RenderForm(values, tpl, opts) → both, in one call
//   - ParseFrontmatter / BuildFrontmatter / FilterFrontmatter
//
// The Manager wraps these for Wails consumption: it loads the template
// + datafile through narrow interfaces and returns both stages.
package render

// Options carries per-render configuration. URL strategies are funcs so
// the desktop and HTTP-server consumers can plug different schemes
// without leaking storage paths into this package.
type Options struct {
	// ImageURL resolves an image filename (stored under the template's
	// images/ folder) to a URL. Desktop returns "file:///abs/path";
	// HTTP server returns "/storage/<tpl>/images/<file>".
	// If nil, the emitter returns "images/<name>".
	ImageURL func(name string) string

	// LinkURL resolves a relative link href against the template
	// storage. Absolute URLs and `file:`/`mailto:`/`tel:` schemes are
	// passed through unchanged before this is called. If nil, the
	// emitter returns the href unchanged.
	LinkURL func(href string) string
}

// Result is the dual-stage output of RenderForm.
type Result struct {
	Markdown string `json:"markdown"`
	HTML     string `json:"html"`
}
