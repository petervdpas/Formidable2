package app

import (
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/form"
	"github.com/petervdpas/formidable2/internal/modules/standalone"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// standaloneSource satisfies standalone.Source: presentation detection + title
// from the template, deck ordering from the form manager (the same Decks /
// DeckOrder / SequenceOrder seam the wiki uses), and the plain record list from
// storage for a document without a sequence field.
type standaloneSource struct {
	tpl  *template.Manager
	form *form.Manager
	sto  *storage.Manager
}

// Plan resolves what to export for one template. deck selects a slideset of a
// multi-deck presentation; it is ignored for documents.
func (s standaloneSource) Plan(templateName, deck string) (standalone.Plan, error) {
	t, err := s.tpl.LoadTemplate(templateName)
	if err != nil {
		return standalone.Plan{}, err
	}
	plan := standalone.Plan{Title: strings.TrimSpace(t.Name), Presentation: t.Presentation}
	if t.Presentation {
		plan.Datafiles, err = s.deckRecords(templateName, deck)
	} else {
		plan.Datafiles, err = s.docRecords(templateName)
	}
	if err != nil {
		return standalone.Plan{}, err
	}
	return plan, nil
}

// docRecords returns records in sequence order when the template has a sequence
// field; otherwise the storage record list (SequenceOrder errors without one).
func (s standaloneSource) docRecords(templateName string) ([]string, error) {
	if order, err := s.form.SequenceOrder(templateName); err == nil {
		return order, nil
	}
	return s.sto.ListForms(templateName)
}

// deckRecords resolves one deck's datafiles: an empty selector plays the first
// slideset, or the whole sequence for a single-deck presentation.
func (s standaloneSource) deckRecords(templateName, deck string) ([]string, error) {
	decks, err := s.form.Decks(templateName)
	if err != nil {
		return nil, err
	}
	if len(decks) == 0 {
		return s.form.SequenceOrder(templateName)
	}
	sel := deck
	if sel == "" {
		sel = decks[0].Value
	}
	return s.form.DeckOrder(templateName, sel)
}
