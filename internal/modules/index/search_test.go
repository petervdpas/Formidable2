package index

import (
	"path/filepath"
	"slices"
	"testing"
)

// newSearchManager builds an index seeded with bodies so the FTS path
// has prose to match. Two templates share a term ("cable") so the
// scope test can prove SearchForms never leaks across collections.
func newSearchManager(t *testing.T) *Manager {
	t.Helper()
	m, err := NewManager(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(func() { m.Close() })

	must(t, Reconcile(m.DB(), ReconcileBatch{
		UpsertTemplates: []TemplateRow{
			{Filename: "net.yaml", Name: "Network", Mtime: 1},
			{Filename: "doc.yaml", Name: "Docs", Mtime: 2},
		},
		UpsertForms: []FormRow{
			{Template: "net.yaml", Filename: "a.meta.json", Title: "Fiber cable run",
				SearchBody: "fiber optic cable pulled through the north duct", Tags: []string{"infra"}, Mtime: 1},
			{Template: "net.yaml", Filename: "b.meta.json", Title: "Switch config",
				SearchBody: "managed switch with VLAN trunking on port eight", Mtime: 2},
			{Template: "net.yaml", Filename: "c.meta.json", Title: "Patch panel",
				SearchBody: "copper cabling terminated at the patch panel", Mtime: 3},
			{Template: "doc.yaml", Filename: "d.meta.json", Title: "Cable policy",
				SearchBody: "company policy on cable management and labelling", Mtime: 4},
		},
	}))
	return m
}

func filenames(rows []FormRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Filename
	}
	return out
}

func contains(ss []string, want string) bool {
	return slices.Contains(ss, want)
}

func TestSchemaV4_FTSTableExists(t *testing.T) {
	m := newSearchManager(t)
	var name string
	err := m.DB().QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='form_fts'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("form_fts not created (FTS5 missing?): %v", err)
	}
}

func TestSearchForms_MatchesTitle(t *testing.T) {
	m := newSearchManager(t)
	// "cable" is in a's title and body, not b's anything.
	rows, err := m.SearchForms("net.yaml", "cable", QueryOpts{})
	if err != nil {
		t.Fatal(err)
	}
	got := filenames(rows)
	if !contains(got, "a.meta.json") {
		t.Errorf("title match failed: want a, got %v", got)
	}
	if contains(got, "b.meta.json") {
		t.Errorf("b has no cable, should not match: %v", got)
	}
}

func TestSearchForms_MatchesBodyOnly(t *testing.T) {
	m := newSearchManager(t)
	// "copper" lives only in c's body; its title is "Patch panel".
	rows, err := m.SearchForms("net.yaml", "copper", QueryOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if got := filenames(rows); len(got) != 1 || got[0] != "c.meta.json" {
		t.Fatalf("body match failed: want [c.meta.json], got %v", got)
	}
}

func TestSearchForms_ScopedToTemplate(t *testing.T) {
	m := newSearchManager(t)
	rows, err := m.SearchForms("doc.yaml", "cable", QueryOpts{})
	if err != nil {
		t.Fatal(err)
	}
	got := filenames(rows)
	if len(got) != 1 || got[0] != "d.meta.json" {
		t.Fatalf("scope leaked: want [d.meta.json], got %v", got)
	}
}

func TestSearchForms_MultiTermIsAnd(t *testing.T) {
	m := newSearchManager(t)
	// "fiber" + "duct" both only in a.
	rows, err := m.SearchForms("net.yaml", "fiber duct", QueryOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if got := filenames(rows); len(got) != 1 || got[0] != "a.meta.json" {
		t.Fatalf("AND failed: want [a.meta.json], got %v", got)
	}
}

func TestSearchForms_PrefixMatch(t *testing.T) {
	m := newSearchManager(t)
	// "cabl" prefix should match "cable" / "cabling".
	rows, err := m.SearchForms("net.yaml", "cabl", QueryOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if got := filenames(rows); !contains(got, "a.meta.json") || !contains(got, "c.meta.json") {
		t.Errorf("prefix match failed: %v", got)
	}
}

func TestSearchForms_StitchesTagsAndFacets(t *testing.T) {
	m := newSearchManager(t)
	rows, err := m.SearchForms("net.yaml", "fiber", QueryOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if len(rows[0].Tags) != 1 || rows[0].Tags[0] != "infra" {
		t.Errorf("tags not stitched: %v", rows[0].Tags)
	}
}

func TestSearchForms_EmptyQueryReturnsNothing(t *testing.T) {
	m := newSearchManager(t)
	for _, q := range []string{"", "   ", "\t"} {
		rows, err := m.SearchForms("net.yaml", q, QueryOpts{})
		if err != nil {
			t.Fatalf("empty query %q errored: %v", q, err)
		}
		if len(rows) != 0 {
			t.Errorf("empty query %q returned %d rows", q, len(rows))
		}
	}
}

func TestSearchForms_SpecialCharsDoNotError(t *testing.T) {
	m := newSearchManager(t)
	// Raw FTS5 syntax in user input must not throw - it's sanitized.
	for _, q := range []string{`"`, `cable AND (`, `*`, `c++ NEAR`, `^foo`} {
		if _, err := m.SearchForms("net.yaml", q, QueryOpts{}); err != nil {
			t.Errorf("query %q errored: %v", q, err)
		}
	}
}

func TestSearchForms_LimitApplies(t *testing.T) {
	m := newSearchManager(t)
	rows, err := m.SearchForms("net.yaml", "cabl", QueryOpts{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("limit not applied: got %d", len(rows))
	}
}

func TestSearchForms_DeleteRemovesFromIndex(t *testing.T) {
	m := newSearchManager(t)
	must(t, Reconcile(m.DB(), ReconcileBatch{
		DeleteForms: []FormRef{{Template: "net.yaml", Filename: "a.meta.json"}},
	}))
	rows, err := m.SearchForms("net.yaml", "fiber", QueryOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("deleted form still searchable: %v", filenames(rows))
	}
}

func TestSearchForms_ReindexUpdatesBody(t *testing.T) {
	m := newSearchManager(t)
	// Re-upsert b with a new body; the old terms must stop matching and
	// the new one must start.
	must(t, Reconcile(m.DB(), ReconcileBatch{
		UpsertForms: []FormRow{
			{Template: "net.yaml", Filename: "b.meta.json", Title: "Switch config",
				SearchBody: "replaced with router firmware notes", Mtime: 9},
		},
	}))
	if rows, _ := m.SearchForms("net.yaml", "trunking", QueryOpts{}); len(rows) != 0 {
		t.Errorf("stale term still matches: %v", filenames(rows))
	}
	if rows, _ := m.SearchForms("net.yaml", "router", QueryOpts{}); len(rows) != 1 {
		t.Errorf("new term not indexed: %v", filenames(rows))
	}
}

func TestBuildMatchQuery(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"   ", ""},
		{"cable", `"cable"*`},
		{"fiber duct", `"fiber"* "duct"*`},
		{`say "hi"`, `"say"* "hi"*`},
		{`a*b`, `"a"* "b"*`},
		{`(foo)`, `"foo"*`},
	}
	for _, c := range cases {
		if got := buildMatchQuery(c.in); got != c.want {
			t.Errorf("buildMatchQuery(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
