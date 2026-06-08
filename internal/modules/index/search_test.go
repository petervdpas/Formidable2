package index

import (
	"path/filepath"
	"slices"
	"sort"
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

// mustSearch runs SearchForms and fails the test on error, returning the rows.
func mustSearch(t *testing.T, m *Manager, template, query string) []FormRow {
	t.Helper()
	rows, err := m.SearchForms(template, query, QueryOpts{})
	if err != nil {
		t.Fatalf("SearchForms(%q, %q): %v", template, query, err)
	}
	return rows
}

// sortStrings returns a sorted copy so set comparisons ignore result ordering.
func sortStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// TestEmptyIndex_SearchFormsEmpty: a non-empty query against an empty FTS index
// returns no rows and no error.
func TestEmptyIndex_SearchFormsEmpty(t *testing.T) {
	m := newEmptyManager(t)
	rows, err := m.SearchForms("basic.yaml", "cable", QueryOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("SearchForms on empty index = %d rows, want 0", len(rows))
	}
}

// TestBuildMatchQuery_EmptyAndWhitespace: empty and whitespace-only inputs
// yield the empty match string so SearchForms short-circuits to no rows.
func TestBuildMatchQuery_EmptyAndWhitespace(t *testing.T) {
	for _, in := range []string{"", "   ", "\t\n", "  \t  "} {
		if got := buildMatchQuery(in); got != "" {
			t.Errorf("buildMatchQuery(%q) = %q, want empty", in, got)
		}
	}
}

// TestBuildMatchQuery_SpecialCharsQuoted: FTS5 operator characters are stripped
// to token boundaries and every surviving token is quoted-prefix, so no raw
// operator can reach the engine. Exact output is pinned.
func TestBuildMatchQuery_SpecialCharsQuoted(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`"`, ""},
		{`()`, ""},
		{`* ^ -`, ""},
		{`AND OR NOT`, `"AND"* "OR"* "NOT"*`},
		{`"quoted phrase"`, `"quoted"* "phrase"*`},
		{`cable AND (fiber OR duct)`, `"cable"* "AND"* "fiber"* "OR"* "duct"*`},
		{`a:b`, `"a"* "b"*`},
		{`NEAR(x y)`, `"NEAR"* "x"* "y"*`},
		{`c++`, `"c"*`},
		{`foo^2`, `"foo"* "2"*`},
		{`-minus`, `"minus"*`},
	}
	for _, c := range cases {
		if got := buildMatchQuery(c.in); got != c.want {
			t.Errorf("buildMatchQuery(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestSearchForms_SpecialCharQueriesDoNotErrorOnSeed runs raw FTS5 syntax
// against a populated index and pins that sanitization keeps it from throwing
// while still matching on the surviving tokens.
func TestSearchForms_SpecialCharQueriesDoNotErrorOnSeed(t *testing.T) {
	m := newSearchManager(t)
	// "(cable)" sanitizes to the single token "cable"*: parens drop out, so the
	// hit set is identical to a bare "cable" query. In net.yaml only a's body
	// holds "cable" as a prefix; c holds "cabling" which "cable"* does not
	// match (e vs i at index 4), and b holds neither. Pin the exact set.
	paren := filenames(mustSearch(t, m, "net.yaml", `(cable)`))
	bare := filenames(mustSearch(t, m, "net.yaml", `cable`))
	if !equalStrings(sortStrings(paren), sortStrings(bare)) {
		t.Errorf("sanitized parens query %v differs from bare query %v", paren, bare)
	}
	if got := sortStrings(paren); !equalStrings(got, []string{"a.meta.json"}) {
		t.Errorf("sanitized 'cable' query = %v, want [a.meta.json]", got)
	}
	// FTS5 keyword AND survives quoting as a LITERAL term, so "cable AND" is an
	// implicit AND of the words "cable" and "and"; the body has no "and" token,
	// so this matches nothing. Pins that operators are neutered, not honored.
	andRows, err := m.SearchForms("net.yaml", `cable AND duct`, QueryOpts{})
	if err != nil {
		t.Fatalf("operator-keyword query errored: %v", err)
	}
	if len(andRows) != 0 {
		t.Errorf(`"cable AND duct" matched %d rows; AND must be a literal token, not an operator`, len(andRows))
	}
	// Pure-operator input has no surviving token, so it returns nothing.
	none, err := m.SearchForms("net.yaml", `*^()-`, QueryOpts{})
	if err != nil {
		t.Fatalf("pure-operator query errored: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("pure-operator query = %d rows, want 0", len(none))
	}
}

// TestEmptyIndex_SearchSpecialAndPaging: against a zero-row FTS index every
// search shape (pure-operator, sanitized token, limit, offset) returns no rows
// and never errors. Pins that an empty index is benign across the search API.
func TestEmptyIndex_SearchSpecialAndPaging(t *testing.T) {
	m := newEmptyManager(t)
	cases := []struct {
		query string
		opts  QueryOpts
	}{
		{`*^()-`, QueryOpts{}},
		{`cable AND (fiber OR duct)`, QueryOpts{}},
		{`cable`, QueryOpts{Limit: 5}},
		{`cable`, QueryOpts{Offset: 3}},
		{`cable`, QueryOpts{Limit: 2, Offset: 1}},
	}
	for _, c := range cases {
		rows, err := m.SearchForms("basic.yaml", c.query, c.opts)
		if err != nil {
			t.Fatalf("SearchForms(%q, %+v) errored: %v", c.query, c.opts, err)
		}
		if len(rows) != 0 {
			t.Errorf("SearchForms(%q, %+v) = %d rows, want 0", c.query, c.opts, len(rows))
		}
	}
}
