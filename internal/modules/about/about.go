// Package about exposes the application's identity (name, version,
// tagline, author) as a small Wails-bindable service. The splash
// window and any future "About" UI read from the same source.
package about

// App identity. Bump Version on release.
const (
	Name    = "Formidable"
	Version = "0.1.0"
	Tagline = "A System for Templates and Markdown Forms"
	Author  = "Peter van de Pas"
)

// Info is the wire shape returned to the frontend.
type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Tagline string `json:"tagline"`
	Author  string `json:"author"`
}
