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
