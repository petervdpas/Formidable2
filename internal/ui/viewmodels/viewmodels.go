// Package viewmodels holds the typed data shapes passed to HTML
// templates. Every page that the ui handlers render fills one of these
// structs, never a raw map[string]any — the type safety is the whole
// point of going server-rendered.
package viewmodels

// Layout is the shared shell every page embeds. Filled by the handler
// before rendering and exposed via Go's `html/template` as `.` in the
// layout template.
type Layout struct {
	Title       string // <title>
	Theme       string // "light" | "dark" | "purplish"
	Active      string // current nav id ("templates", "forms", ...)
	BaseURL     string // typically empty (Wails uses absolute paths)
	AppName     string // "Formidable2"
	AppVersion  string // optional
	ContentTmpl string // name of the body template the layout invokes
}

// Index is the studio home page. Lists every template currently in the
// active context's templates folder.
type Index struct {
	Layout
	Templates []TemplateRow
}

// TemplateRow is one entry in the home-page list. FormCount is best-
// effort and falls back to 0 if the storage scan fails.
type TemplateRow struct {
	Filename  string // "basic.yaml"
	Name      string // "Basic Form" — display name from the YAML
	FormCount int
}
