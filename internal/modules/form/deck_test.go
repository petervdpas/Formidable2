package form

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// deckManager builds a multi-deck presentation template: guid + sequence +
// slide + a slideset field declaring two decks (intro, deep).
func deckManager(t *testing.T) (*Manager, *fakeStorage) {
	t.Helper()
	m, tpls, store, _ := newTestManager()
	tpls.byName["deck.yaml"] = &template.Template{
		Filename:         "deck.yaml",
		EnableCollection: true,
		Fields: []template.Field{
			{Key: "id", Type: "guid"},
			{Key: "pos", Type: "sequence"},
			{Key: "slide", Type: "slide"},
			{Key: "slideset", Type: "slideset", Options: []any{
				map[string]any{"value": "intro", "label": "Intro"},
				map[string]any{"value": "deep", "label": "Deep dive"},
			}},
		},
	}
	return m, store
}

func seedDeckRecord(store *fakeStorage, df string, pos int, deck string) {
	if store.forms["deck.yaml"] == nil {
		store.forms["deck.yaml"] = map[string]*storage.Form{}
	}
	store.forms["deck.yaml"][df] = &storage.Form{
		Meta: storage.FormMeta{Template: "deck.yaml", ID: df},
		Data: map[string]any{"id": df, "pos": float64(pos), "slideset": deck},
	}
	store.listed["deck.yaml"] = append(store.listed["deck.yaml"], df)
}

func TestDecks_FromSlidesetOptions(t *testing.T) {
	m, _ := deckManager(t)
	decks, err := m.Decks("deck.yaml")
	if err != nil {
		t.Fatalf("Decks: %v", err)
	}
	if len(decks) != 2 ||
		decks[0] != (DeckOption{Value: "intro", Label: "Intro"}) ||
		decks[1] != (DeckOption{Value: "deep", Label: "Deep dive"}) {
		t.Errorf("decks = %+v, want intro/deep", decks)
	}
}

func TestPlayableDecks_OnlyDecksWithSlides(t *testing.T) {
	m, store := deckManager(t)
	// intro has two slides; deep has none.
	seedDeckRecord(store, "s1.meta.json", 10, "intro")
	seedDeckRecord(store, "s2.meta.json", 20, "intro")

	decks, err := m.PlayableDecks("deck.yaml")
	if err != nil {
		t.Fatalf("PlayableDecks: %v", err)
	}
	if len(decks) != 1 || decks[0].Value != "intro" {
		t.Errorf("playable decks = %+v, want only intro", decks)
	}
}

func TestDecks_EmptyWhenNoSlidesetField(t *testing.T) {
	m, tpls, _, _ := newTestManager()
	tpls.byName["plain.yaml"] = &template.Template{
		Filename: "plain.yaml", EnableCollection: true,
		Fields: []template.Field{{Key: "id", Type: "guid"}, {Key: "pos", Type: "sequence"}},
	}
	decks, err := m.Decks("plain.yaml")
	if err != nil {
		t.Fatalf("Decks: %v", err)
	}
	if len(decks) != 0 {
		t.Errorf("decks = %+v, want empty", decks)
	}
}

// DeckOrder returns only one deck's records, in that deck's sequence order,
// even when the other deck uses colliding sequence values. Unassigned records
// belong to no deck.
func TestDeckOrder_FiltersAndOrders(t *testing.T) {
	m, store := deckManager(t)
	seedDeckRecord(store, "a.meta.json", 20, "intro")
	seedDeckRecord(store, "b.meta.json", 10, "intro")
	seedDeckRecord(store, "c.meta.json", 10, "deep") // collides with b's value
	seedDeckRecord(store, "d.meta.json", 20, "deep")
	seedDeckRecord(store, "z.meta.json", 5, "") // unassigned

	intro, err := m.DeckOrder("deck.yaml", "intro")
	if err != nil {
		t.Fatalf("DeckOrder intro: %v", err)
	}
	if len(intro) != 2 || intro[0] != "b.meta.json" || intro[1] != "a.meta.json" {
		t.Errorf("intro order = %v, want [b, a]", intro)
	}
	deep, err := m.DeckOrder("deck.yaml", "deep")
	if err != nil {
		t.Fatalf("DeckOrder deep: %v", err)
	}
	if len(deep) != 2 || deep[0] != "c.meta.json" || deep[1] != "d.meta.json" {
		t.Errorf("deep order = %v, want [c, d]", deep)
	}
}

// NormalizeDeck renumbers one deck without touching the other.
func TestNormalizeDeck_IndependentPerDeck(t *testing.T) {
	m, store := deckManager(t)
	seedDeckRecord(store, "a.meta.json", 15, "intro")
	seedDeckRecord(store, "b.meta.json", 5, "intro")
	seedDeckRecord(store, "c.meta.json", 7, "deep")

	res, err := m.NormalizeDeck("deck.yaml", "intro")
	if err != nil {
		t.Fatalf("NormalizeDeck: %v", err)
	}
	if !res.Normalized {
		t.Errorf("normalize should report Normalized")
	}
	// intro order by value b(5), a(15) -> 10, 20.
	if got := posOf(t, store, "b.meta.json"); got != 10 {
		t.Errorf("b pos = %d, want 10", got)
	}
	if got := posOf(t, store, "a.meta.json"); got != 20 {
		t.Errorf("a pos = %d, want 20", got)
	}
	// deep record untouched.
	if got := posOf(t, store, "c.meta.json"); got != 7 {
		t.Errorf("c pos = %d, want 7 (other deck untouched)", got)
	}
}

// Reordering within a deck reuses ReorderSequence with the deck's file subset,
// so it only rewrites the moved record and never touches the other deck.
func TestReorderSequence_ScopedToDeckSubset(t *testing.T) {
	m, store := deckManager(t)
	seedDeckRecord(store, "a.meta.json", 10, "intro")
	seedDeckRecord(store, "b.meta.json", 30, "intro")
	seedDeckRecord(store, "c.meta.json", 10, "deep") // same value, other deck

	// Move b before a within the intro deck (subset order: b, a).
	res, err := m.ReorderSequence("deck.yaml", "b.meta.json",
		[]string{"b.meta.json", "a.meta.json"})
	if err != nil {
		t.Fatalf("ReorderSequence: %v", err)
	}
	if res.Normalized || len(res.Written) != 1 || res.Written[0] != "b.meta.json" {
		t.Errorf("expected only b rewritten, got %+v", res)
	}
	if got := posOf(t, store, "b.meta.json"); got != 0 {
		t.Errorf("b pos = %d, want 0 (before a at 10)", got)
	}
	if got := posOf(t, store, "c.meta.json"); got != 10 {
		t.Errorf("c pos = %d, want 10 (other deck untouched)", got)
	}
}
