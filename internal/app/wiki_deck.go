package app

import (
	"github.com/petervdpas/formidable2/internal/modules/form"
	"github.com/petervdpas/formidable2/internal/modules/render"
	"github.com/petervdpas/formidable2/internal/modules/wiki"
)

// wikiDeckAdapter satisfies wiki.DeckProvider: deck listing/ordering come from
// the form manager (form.Decks/DeckOrder/SequenceOrder), the reveal build from
// the wiki render manager (render.BuildDeck, the same seam the in-app previewer
// uses). This keeps the wiki decoupled from form/render concrete types.
type wikiDeckAdapter struct {
	form *form.Manager
	ren  *render.Manager
}

func (a wikiDeckAdapter) Decks(templateName string) ([]wiki.DeckList, error) {
	opts, err := a.form.Decks(templateName)
	if err != nil {
		return nil, err
	}
	out := make([]wiki.DeckList, len(opts))
	for i, o := range opts {
		out[i] = wiki.DeckList{Value: o.Value, Label: o.Label}
	}
	return out, nil
}

func (a wikiDeckAdapter) DeckOrder(templateName, deck string) ([]string, error) {
	return a.form.DeckOrder(templateName, deck)
}

func (a wikiDeckAdapter) SequenceOrder(templateName string) ([]string, error) {
	return a.form.SequenceOrder(templateName)
}

func (a wikiDeckAdapter) BuildDeck(templateName string, datafiles []string) (render.RevealDeck, error) {
	return a.ren.BuildDeck(templateName, datafiles)
}
