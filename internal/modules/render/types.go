// Package render is Formidable's two-stage Handlebars→Markdown→HTML
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
// each consumer (in-app slideout, wiki HTTP server, future Azure/GitHub
// wiki exporters, …) can plug a different scheme without leaking
// transport details into this package.
type Options struct {
	// ImageURL resolves an image filename (stored under the template's
	// images/ folder) to a URL. Desktop returns "file:///abs/path";
	// HTTP server returns "/storage/<tpl>/images/<file>".
	// If nil, the emitter returns "images/<name>".
	ImageURL func(name string) string

	// ImageBase64URL resolves an image filename to a `data:<mime>;
	// base64,<bytes>` URL. Used by the generator's "inline" mode and
	// by self-contained-export targets. Independent of ImageURL so a
	// single Manager can serve both `<img src="/api/images/…">` (via
	// ImageURL) and inlined data URLs (via ImageBase64URL).
	// If nil, the {{imageBase64}} helper returns "".
	ImageBase64URL func(name string) string

	// LinkURL resolves a relative link href against the template
	// storage. Absolute URLs and `file:`/`mailto:`/`tel:` schemes are
	// passed through unchanged before this is called. If nil, the
	// emitter returns the href unchanged.
	LinkURL func(href string) string

	// FormidableLinkURL rewrites `formidable://<template>:<datafile>`
	// hrefs into transport-specific URLs. The renderer parses the URL
	// into its (template, datafile) pair before calling this; nil =
	// keep the formidable:// URL as-is (slideout uses this — its Vue
	// click interceptor handles the click). Empty-string return =
	// fall back to the original formidable:// URL.
	FormidableLinkURL func(templateFilename, datafile string) string
}

// Result is the dual-stage output of RenderForm.
type Result struct {
	Markdown string `json:"markdown"`
	HTML     string `json:"html"`
}
