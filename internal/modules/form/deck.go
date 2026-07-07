package form

import (
	"errors"
	"fmt"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// DeckOption is one authored deck: the slideset field's option value (stored on
// each record that belongs to the deck) plus its display label.
type DeckOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Decks returns the decks a presentation template declares, read from its
// slideset field's options (the backend owns the deck list). Empty when the
// template has no slideset field (a single-deck presentation).
func (m *Manager) Decks(templateName string) ([]DeckOption, error) {
	tpl, err := m.templates.LoadTemplate(templateName)
	if err != nil {
		return nil, fmt.Errorf("form: load template %q: %w", templateName, err)
	}
	field := slidesetField(tpl)
	if field == nil {
		return []DeckOption{}, nil
	}
	out := []DeckOption{}
	for _, opt := range field.Options {
		mo, ok := opt.(map[string]any)
		if !ok {
			continue
		}
		val := toStr(mo["value"])
		if val == "" {
			continue
		}
		label := toStr(mo["label"])
		if label == "" {
			label = val
		}
		out = append(out, DeckOption{Value: val, Label: label})
	}
	return out, nil
}

// PlayableDecks returns the presentation's decks that actually contain slides
// (their DeckOrder is non-empty), for pickers that must not offer empty decks.
// Empty for a single-deck presentation (no slideset field): the caller treats
// the whole sequence as one deck.
func (m *Manager) PlayableDecks(templateName string) ([]DeckOption, error) {
	decks, err := m.Decks(templateName)
	if err != nil {
		return nil, err
	}
	out := make([]DeckOption, 0, len(decks))
	for _, d := range decks {
		order, err := m.DeckOrder(templateName, d.Value)
		if err != nil {
			return nil, err
		}
		if len(order) > 0 {
			out = append(out, d)
		}
	}
	return out, nil
}

// DeckOrder returns one deck's datafiles in sequence order: SequenceOrder
// filtered to records whose slideset value equals deck. Sequence numbering is
// per-deck, so two decks may share values; this scopes the order to one deck.
func (m *Manager) DeckOrder(templateName, deck string) ([]string, error) {
	tpl, _, err := m.sequenceContext(templateName)
	if err != nil {
		return nil, err
	}
	field := slidesetField(tpl)
	if field == nil {
		return nil, errors.New("form: template has no slideset field")
	}
	ordered, err := m.SequenceOrder(templateName)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ordered))
	for _, f := range ordered {
		if d, ok := m.slidesetOf(templateName, f, field.Key); ok && d == deck {
			out = append(out, f)
		}
	}
	return out, nil
}

// NormalizeDeck re-spreads one deck's records to clean step spacing (10, 20,
// 30…) independently of the other decks (which may share the same values).
func (m *Manager) NormalizeDeck(templateName, deck string) (ReorderResult, error) {
	ordered, err := m.DeckOrder(templateName, deck)
	if err != nil {
		return ReorderResult{}, err
	}
	_, seq, err := m.sequenceContext(templateName)
	if err != nil {
		return ReorderResult{}, err
	}
	return m.normalizeSequence(templateName, ordered, seq.Key, template.SequenceStep(*seq))
}

// slidesetField returns the template's sole slideset field, or nil.
func slidesetField(tpl *template.Template) *template.Field {
	for i := range tpl.Fields {
		if tpl.Fields[i].Type == "slideset" {
			return &tpl.Fields[i]
		}
	}
	return nil
}

// slidesetOf reads a record's deck membership (its slideset value), mirroring
// sequenceOf. Returns ("", false) when the record is missing or unassigned.
func (m *Manager) slidesetOf(templateName, datafile, deckKey string) (string, bool) {
	loaded := m.storage.LoadForm(templateName, datafile)
	if loaded == nil {
		return "", false
	}
	v, ok := loaded.Data[deckKey]
	if !ok {
		return "", false
	}
	return toStr(v), true
}

func toStr(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", x)
	}
}
