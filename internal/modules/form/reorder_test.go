package form

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

func presentationManager(t *testing.T) (*Manager, *fakeStorage) {
	t.Helper()
	m, tpls, store, _ := newTestManager()
	tpls.byName["deck.yaml"] = &template.Template{
		Filename:         "deck.yaml",
		EnableCollection: true,
		Fields: []template.Field{
			{Key: "id", Type: "guid"},
			{Key: "pos", Type: "sequence"},
		},
	}
	return m, store
}

func seedRecord(store *fakeStorage, df string, pos int) {
	if store.forms["deck.yaml"] == nil {
		store.forms["deck.yaml"] = map[string]*storage.Form{}
	}
	store.forms["deck.yaml"][df] = &storage.Form{
		Meta: storage.FormMeta{Template: "deck.yaml", ID: df},
		Data: map[string]any{"id": df, "pos": float64(pos)},
	}
	store.listed["deck.yaml"] = append(store.listed["deck.yaml"], df)
}

func posOf(t *testing.T, store *fakeStorage, df string) int {
	t.Helper()
	f := store.forms["deck.yaml"][df]
	if f == nil {
		t.Fatalf("record %q missing", df)
	}
	v, ok := toInt(f.Data["pos"])
	if !ok {
		t.Fatalf("record %q has no numeric pos: %v", df, f.Data["pos"])
	}
	return v
}

func TestSequenceMidpoint(t *testing.T) {
	cases := []struct {
		name                      string
		hasPrev, hasNext          bool
		prev, next, step, wantVal int
		wantOK                    bool
	}{
		{"gap between neighbours", true, true, 10, 20, 10, 15, true},
		{"adjacent neighbours need renumber", true, true, 10, 11, 10, 0, false},
		{"moved to end", true, false, 30, 0, 10, 40, true},
		{"moved to front", false, true, 0, 10, 10, 0, true},
		{"single item", false, false, 0, 0, 10, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := sequenceMidpoint(c.hasPrev, c.prev, c.hasNext, c.next, c.step)
			if ok != c.wantOK || (ok && got != c.wantVal) {
				t.Errorf("got (%d,%v), want (%d,%v)", got, ok, c.wantVal, c.wantOK)
			}
		})
	}
}

func TestSequenceOrder_SortsByValue(t *testing.T) {
	m, store := presentationManager(t)
	seedRecord(store, "a.meta.json", 30)
	seedRecord(store, "b.meta.json", 10)
	seedRecord(store, "c.meta.json", 20)

	order, err := m.SequenceOrder("deck.yaml")
	if err != nil {
		t.Fatalf("SequenceOrder: %v", err)
	}
	want := []string{"b.meta.json", "c.meta.json", "a.meta.json"}
	if len(order) != 3 || order[0] != want[0] || order[1] != want[1] || order[2] != want[2] {
		t.Errorf("order = %v, want %v", order, want)
	}
}

func TestReorderSequence_MinimalWrite(t *testing.T) {
	m, store := presentationManager(t)
	seedRecord(store, "a.meta.json", 10)
	seedRecord(store, "b.meta.json", 20)
	seedRecord(store, "c.meta.json", 30)

	// Drag c between a and b: new order a, c, b.
	res, err := m.ReorderSequence("deck.yaml", "c.meta.json",
		[]string{"a.meta.json", "c.meta.json", "b.meta.json"})
	if err != nil {
		t.Fatalf("ReorderSequence: %v", err)
	}
	if res.Normalized {
		t.Errorf("expected a minimal write, not a renumber")
	}
	if len(res.Written) != 1 || res.Written[0] != "c.meta.json" {
		t.Errorf("written = %v, want only [c.meta.json]", res.Written)
	}
	if got := posOf(t, store, "c.meta.json"); got != 15 {
		t.Errorf("c pos = %d, want 15 (midpoint of 10 and 20)", got)
	}
	// Untouched records keep their value (and were never re-saved).
	if got := posOf(t, store, "a.meta.json"); got != 10 {
		t.Errorf("a pos = %d, want 10 (untouched)", got)
	}
	if len(store.saves) != 1 {
		t.Errorf("expected exactly one record saved, got %d", len(store.saves))
	}
}

func TestReorderSequence_RenumbersWhenNoGap(t *testing.T) {
	m, store := presentationManager(t)
	seedRecord(store, "a.meta.json", 10)
	seedRecord(store, "b.meta.json", 11)
	seedRecord(store, "c.meta.json", 12)

	// Drag c between a(10) and b(11): no integer slot -> renumber to 10/20/30.
	res, err := m.ReorderSequence("deck.yaml", "c.meta.json",
		[]string{"a.meta.json", "c.meta.json", "b.meta.json"})
	if err != nil {
		t.Fatalf("ReorderSequence: %v", err)
	}
	if !res.Normalized {
		t.Errorf("expected a renumber when neighbours are adjacent")
	}
	if got := posOf(t, store, "a.meta.json"); got != 10 {
		t.Errorf("a pos = %d, want 10", got)
	}
	if got := posOf(t, store, "c.meta.json"); got != 20 {
		t.Errorf("c pos = %d, want 20", got)
	}
	if got := posOf(t, store, "b.meta.json"); got != 30 {
		t.Errorf("b pos = %d, want 30", got)
	}
	// a was already at 10, so only c and b are rewritten (no collateral write).
	if len(res.Written) != 2 {
		t.Errorf("written = %v, want 2 (a already at target)", res.Written)
	}
}

func TestNormalizeSequence_RespreadsInOrder(t *testing.T) {
	m, store := presentationManager(t)
	seedRecord(store, "a.meta.json", 15)
	seedRecord(store, "b.meta.json", 5)
	seedRecord(store, "c.meta.json", 25)

	res, err := m.NormalizeSequence("deck.yaml")
	if err != nil {
		t.Fatalf("NormalizeSequence: %v", err)
	}
	if !res.Normalized {
		t.Errorf("normalize should report Normalized")
	}
	// Current order by value is b(5), a(15), c(25) -> 10, 20, 30.
	if got := posOf(t, store, "b.meta.json"); got != 10 {
		t.Errorf("b pos = %d, want 10", got)
	}
	if got := posOf(t, store, "a.meta.json"); got != 20 {
		t.Errorf("a pos = %d, want 20", got)
	}
	if got := posOf(t, store, "c.meta.json"); got != 30 {
		t.Errorf("c pos = %d, want 30", got)
	}
}

func TestReorderSequence_RejectsNonSequenceTemplate(t *testing.T) {
	m, _, _, _ := newTestManager()
	// Collection template, but no sequence field.
	mTpls := m.templates.(*fakeTemplates)
	mTpls.byName["plain.yaml"] = &template.Template{
		Filename:         "plain.yaml",
		EnableCollection: true,
		Fields:           []template.Field{{Key: "id", Type: "guid"}},
	}
	if _, err := m.ReorderSequence("plain.yaml", "a.meta.json", []string{"a.meta.json"}); err == nil {
		t.Errorf("expected an error reordering a template with no sequence field")
	}
}
