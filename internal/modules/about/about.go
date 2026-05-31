// Package about exposes the application's identity (name, version, tagline,
// author) as a Wails-bindable service.
package about

const (
	Name    = "Formidable"
	Tagline = "A System for Templates and Markdown Forms"
	Author  = "Peter van de Pas"
	Website = "https://formidable.tools"
)

// Version is a var (not const) so the release workflow can inject the tag at
// link time via -ldflags "-X .../about.Version=1.2.3"; dev builds keep the
// default.
var Version = "0.1.0"

// Info is the wire shape returned to the frontend.
type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Tagline string `json:"tagline"`
	Author  string `json:"author"`
	Website string `json:"website"`
}

// Library is one entry in the About panel's "Special thanks to" list: a
// canonical project ID and its display name. Descriptions live in i18n under
// workspace.information.about.thanks.lib.{id}.desc, so product names aren't
// translated.
type Library struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Libraries is the curated list of direct dependencies surfaced in the About
// panel, rendered top-to-bottom in listed order. Curation rule: direct deps
// from go.mod and frontend/package.json the app actively uses; transitive deps
// and build-only tools are excluded. Adding or removing an entry MUST be paired
// with a `workspace.information.about.thanks.lib.<id>.desc` string in every
// locale (tests catch a missing/extra ID).
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
