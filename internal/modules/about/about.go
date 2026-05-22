// Package about exposes the application's identity (name, version,
// tagline, author) as a small Wails-bindable service. The splash
// window and any future "About" UI read from the same source.
package about

// App identity. Version is a var (not const) so the release workflow
// can inject the tag value at link time via
//   -ldflags "-X github.com/petervdpas/formidable2/internal/modules/about.Version=1.2.3"
// Local dev builds keep the "0.1.0" default.
const (
	Name    = "Formidable"
	Tagline = "A System for Templates and Markdown Forms"
	Author  = "Peter van de Pas"
)

var Version = "0.1.0"

// Info is the wire shape returned to the frontend.
type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Tagline string `json:"tagline"`
	Author  string `json:"author"`
}

// Library is one entry in the About panel's "Special thanks to" list:
// a canonical project ID and its display name. Descriptions are
// per-locale and live in i18n (workspace.information.about.thanks.lib.{id}.desc)
// so we don't translate product names.
type Library struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Libraries is the curated list of load-bearing direct dependencies
// surfaced in the About panel. Order is meaningful - items render
// top-to-bottom as listed. Curation rule: direct deps from go.mod and
// frontend/package.json that the app actively uses; transitive deps
// and build-only tools (vite, tsc, prettier) are excluded.
//
// Adding or removing an entry here MUST be paired with the matching
// i18n description keys: each Library.ID requires a
// `workspace.information.about.thanks.lib.<id>.desc` string in every
// locale. Tests catch a missing/extra ID across locales.
var Libraries = []Library{
	{ID: "wails", Name: "Wails 3"},
	{ID: "vue", Name: "Vue 3"},
	{ID: "codemirror", Name: "CodeMirror"},
	{ID: "easymde", Name: "EasyMDE"},
	{ID: "raymond", Name: "aymerick/raymond"},
	{ID: "goldmark", Name: "Goldmark"},
	{ID: "chroma", Name: "Chroma"},
	{ID: "picoloom", Name: "picoloom"},
	{ID: "pdfcpu", Name: "pdfcpu"},
	{ID: "gogit", Name: "go-git"},
	{ID: "expr", Name: "expr-lang/expr"},
	{ID: "gopherlua", Name: "gopher-lua"},
	{ID: "vuedraggable", Name: "vuedraggable"},
	{ID: "vuei18n", Name: "vue-i18n"},
	{ID: "datepicker", Name: "@vuepic/vue-datepicker"},
	{ID: "fontawesome", Name: "Font Awesome"},
	{ID: "keyring", Name: "zalando/go-keyring"},
	{ID: "godog", Name: "cucumber/godog"},
	{ID: "uuid", Name: "google/uuid"},
}
