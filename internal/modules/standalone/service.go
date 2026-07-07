// Package standalone exports one template as a single, self-contained HTML page
// that opens offline with no server: a reveal.js deck for a presentation, a
// prose document otherwise. It layers on the render module's public surface
// (RenderForm / BuildDeck + the embedded prose/reveal/deck/katex assets),
// inlining every stylesheet, script, image, and font into one file.
package standalone

import (
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/render"
)

// Renderer is the render surface a standalone export needs. *render.Manager
// satisfies it; the export uses a Manager wired with data-URL images so the
// result is server-less.
type Renderer interface {
	RenderForm(templateName, datafile string) (*render.Result, error)
	BuildDeck(templateName string, datafiles []string) (render.RevealDeck, error)
}

// Plan is what to export for one template: the display title, whether it is a
// presentation, and the ordered datafiles (a deck's slides, or a document's
// records). The Source resolves it; the Service turns it into HTML.
type Plan struct {
	Title        string
	Presentation bool
	Datafiles    []string
}

// Source resolves the export plan for a template. deck selects one slideset of
// a multi-deck presentation ("" = first/whole); it is ignored for documents.
// The app satisfies it over template + form + storage (the same Decks /
// DeckOrder / SequenceOrder seam the wiki uses).
type Source interface {
	Plan(templateName, deck string) (Plan, error)
}

// Service is the Wails-bound facade: one call renders a template to a
// self-contained HTML string the frontend saves to disk.
type Service struct {
	ren Renderer
	src Source
}

// NewService wraps the render surface and plan source, panicking on nil so a
// composition-root bug surfaces at boot.
func NewService(ren Renderer, src Source) *Service {
	if ren == nil || src == nil {
		panic("standalone: NewService called with nil renderer or source")
	}
	return &Service{ren: ren, src: src}
}

// Export returns a self-contained HTML document for the template. deck selects
// the slideset for a multi-deck presentation ("" = first/whole); it is ignored
// for document templates.
func (s *Service) Export(templateName, deck string) (string, error) {
	plan, err := s.src.Plan(templateName, deck)
	if err != nil {
		return "", err
	}
	title := resolveTitle(plan.Title, templateName)
	if plan.Presentation {
		built, err := s.ren.BuildDeck(templateName, plan.Datafiles)
		if err != nil {
			return "", err
		}
		return composeDeck(title, built), nil
	}

	var body strings.Builder
	for _, df := range plan.Datafiles {
		res, err := s.ren.RenderForm(templateName, df)
		if err != nil {
			return "", err
		}
		body.WriteString(`<article class="formidable-doc-record">` + "\n")
		body.WriteString(res.HTML)
		body.WriteString("\n</article>\n")
	}
	return composeDoc(title, body.String()), nil
}

// resolveTitle prefers the plan title, falling back to the template filename
// stem, then "Untitled".
func resolveTitle(title, templateName string) string {
	if t := strings.TrimSpace(title); t != "" {
		return t
	}
	if s := stemOf(templateName); s != "" {
		return s
	}
	return "Untitled"
}

// stemOf strips a trailing ".yaml" (or any extension) from a template filename.
func stemOf(name string) string {
	if i := strings.LastIndex(name, "."); i > 0 {
		return name[:i]
	}
	return name
}
