package index

import (
	"path/filepath"
	"sort"
	"testing"
)

// Build a fully-populated test index. Returns the Manager so each
// test can exercise read methods against a known seed.
func newSeededManager(t *testing.T) *Manager {
	t.Helper()
	m, err := NewManager(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(func() { m.Close() })

	must(t, Reconcile(m.DB(), ReconcileBatch{
		UpsertTemplates: []TemplateRow{
			{Filename: "basic.yaml", Name: "Basic", HasMarkdownTemplate: true, Mtime: 100},
			{Filename: "looper.yaml", Name: "Looper", Mtime: 200},
		},
		UpsertForms: []FormRow{
			{Template: "basic.yaml", Filename: "first.meta.json", ID: "g1",
				Title: "First", UpdatedName: "Alice", Updated: "2026-01-01T00:00:00Z",
				Tags: []string{"alpha", "common"}, Mtime: 1},
			{Template: "basic.yaml", Filename: "second.meta.json", ID: "g2",
				Title: "Second", UpdatedName: "Bob", Updated: "2026-03-01T00:00:00Z",
				Tags: []string{"beta", "common"}, Mtime: 2},
			{Template: "basic.yaml", Filename: "third.meta.json", ID: "g3",
				Title: "Third", UpdatedName: "Carol", Updated: "2026-02-01T00:00:00Z",
				Tags: []string{"common"}, Mtime: 3},
			{Template: "looper.yaml", Filename: "loop.meta.json", ID: "lg",
				Title: "LoopOne", UpdatedName: "Dave", Updated: "2026-04-01T00:00:00Z",
				Tags: []string{"alpha"}, Mtime: 4},
		},
		UpsertImages: []ImageRow{
			{Template: "basic.yaml", Filename: "logo.png", Mtime: 50},
		},
	}))
	return m
}

func TestManager_ListTemplates(t *testing.T) {
	m := newSeededManager(t)
	rows, err := m.ListTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	// Stable order: filename ascending.
	if rows[0].Filename != "basic.yaml" || rows[1].Filename != "looper.yaml" {
		t.Errorf("order = %q,%q", rows[0].Filename, rows[1].Filename)
	}
	if rows[0].Name != "Basic" || rows[0].HasMarkdownTemplate != true {
		t.Errorf("basic row payload wrong: %+v", rows[0])
	}
}

func TestManager_ListForms_Default(t *testing.T) {
	m := newSeededManager(t)
	rows, err := m.ListForms("basic.yaml", QueryOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d, want 3", len(rows))
	}
	// Default sort: updated DESC.
	wantOrder := []string{"second.meta.json", "third.meta.json", "first.meta.json"}
	for i, want := range wantOrder {
		if rows[i].Filename != want {
			t.Errorf("rows[%d] = %q, want %q", i, rows[i].Filename, want)
		}
	}
	// Tags should round-trip.
	if got := sortedCopy(rows[0].Tags); !equalStrings(got, []string{"beta", "common"}) {
		t.Errorf("second.meta.json tags = %v, want [beta common]", got)
	}
}

func TestManager_ListForms_LimitOffset(t *testing.T) {
	m := newSeededManager(t)
	rows, err := m.ListForms("basic.yaml", QueryOpts{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	// updated-DESC order, offset 1 → "third.meta.json".
	if rows[0].Filename != "third.meta.json" {
		t.Errorf("got %q, want third.meta.json", rows[0].Filename)
	}
}

func TestManager_ListForms_OrderByTitle(t *testing.T) {
	m := newSeededManager(t)
	rows, err := m.ListForms("basic.yaml", QueryOpts{OrderBy: "title_asc"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"First", "Second", "Third"}
	for i, w := range want {
		if rows[i].Title != w {
			t.Errorf("rows[%d].Title = %q, want %q", i, rows[i].Title, w)
		}
	}
}

func TestManager_ListForms_TagFilterAND(t *testing.T) {
	// "alpha + common" matches only first.meta.json (looper's loop.meta.json
	// has alpha but not common).
	m := newSeededManager(t)
	rows, err := m.ListForms("basic.yaml", QueryOpts{Tags: []string{"alpha", "common"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Filename != "first.meta.json" {
		t.Errorf("got %d rows, want first.meta.json only; got=%+v", len(rows), rows)
	}
}

func TestManager_ListForms_UnknownTemplate(t *testing.T) {
	m := newSeededManager(t)
	rows, err := m.ListForms("ghost.yaml", QueryOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("expected empty for unknown template, got %d", len(rows))
	}
}

func TestManager_GetForm_Found(t *testing.T) {
	m := newSeededManager(t)
	row, ok, err := m.GetForm("basic.yaml", "first.meta.json")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected found")
	}
	if row.ID != "g1" || row.Title != "First" || row.UpdatedName != "Alice" {
		t.Errorf("payload wrong: %+v", row)
	}
	if got := sortedCopy(row.Tags); !equalStrings(got, []string{"alpha", "common"}) {
		t.Errorf("tags = %v", got)
	}
}

func TestManager_GetForm_NotFound(t *testing.T) {
	m := newSeededManager(t)
	_, ok, err := m.GetForm("basic.yaml", "missing.meta.json")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Errorf("expected not-found, got ok=true")
	}
}

func TestManager_ListByTags_AcrossTemplates(t *testing.T) {
	// "alpha" exists in both basic.yaml (first.meta.json) and looper.yaml
	// (loop.meta.json). ListByTags must span templates.
	m := newSeededManager(t)
	rows, err := m.ListByTags([]string{"alpha"})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, r := range rows {
		got = append(got, r.Template+"/"+r.Filename)
	}
	sort.Strings(got)
	want := []string{"basic.yaml/first.meta.json", "looper.yaml/loop.meta.json"}
	if !equalStrings(got, want) {
		t.Errorf("ListByTags(alpha) = %v, want %v", got, want)
	}
}

func TestManager_ListByTags_AND(t *testing.T) {
	// "alpha + common" — same as the per-template AND test, but global.
	m := newSeededManager(t)
	rows, err := m.ListByTags([]string{"alpha", "common"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Filename != "first.meta.json" {
		t.Errorf("got %d rows, want first.meta.json; got=%+v", len(rows), rows)
	}
}

func TestManager_Rev(t *testing.T) {
	// Fresh DB → 0. Each Reconcile bumps once.
	m, err := NewManager(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	if r, err := m.Rev(); err != nil || r != 0 {
		t.Errorf("fresh rev = %d, err=%v; want 0,nil", r, err)
	}
	must(t, Reconcile(m.DB(), ReconcileBatch{
		UpsertTemplates: []TemplateRow{{Filename: "x.yaml", Mtime: 1}},
	}))
	if r, err := m.Rev(); err != nil || r != 1 {
		t.Errorf("after 1 batch rev = %d, err=%v; want 1,nil", r, err)
	}
	must(t, Reconcile(m.DB(), ReconcileBatch{
		UpsertTemplates: []TemplateRow{{Filename: "y.yaml", Mtime: 2}},
	}))
	if r, err := m.Rev(); err != nil || r != 2 {
		t.Errorf("after 2 batches rev = %d, want 2", r)
	}
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
