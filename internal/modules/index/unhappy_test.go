package index

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// newEmptyManager returns a Manager with only the schema migrated, no rows.
func newEmptyManager(t *testing.T) *Manager {
	t.Helper()
	m, err := NewManager(filepath.Join(t.TempDir(), "empty.db"))
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(func() { m.Close() })
	return m
}

// TestEmptyIndex_ListTemplatesEmpty: a fresh index has zero template rows.
func TestEmptyIndex_ListTemplatesEmpty(t *testing.T) {
	m := newEmptyManager(t)
	rows, err := m.ListTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("ListTemplates on empty index = %d rows, want 0", len(rows))
	}
}

// TestEmptyIndex_ListFormsEmpty: ListForms over an empty index returns no rows.
func TestEmptyIndex_ListFormsEmpty(t *testing.T) {
	m := newEmptyManager(t)
	rows, err := m.ListForms("basic.yaml", QueryOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("ListForms on empty index = %d rows, want 0", len(rows))
	}
}

// TestEmptyIndex_ListByTagsEmpty: tag query over an empty index returns no rows.
func TestEmptyIndex_ListByTagsEmpty(t *testing.T) {
	m := newEmptyManager(t)
	rows, err := m.ListByTags([]string{"anything"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("ListByTags on empty index = %d rows, want 0", len(rows))
	}
}

// TestEmptyIndex_ListByTagsNilNoQuery: nil tag list short-circuits to nil, nil.
func TestEmptyIndex_ListByTagsNilNoQuery(t *testing.T) {
	m := newEmptyManager(t)
	rows, err := m.ListByTags(nil)
	if err != nil {
		t.Fatal(err)
	}
	if rows != nil {
		t.Errorf("ListByTags(nil) = %v, want nil", rows)
	}
}

// TestEmptyIndex_GetFormNotFound: GetForm on an empty index reports absent,
// not error.
func TestEmptyIndex_GetFormNotFound(t *testing.T) {
	m := newEmptyManager(t)
	row, ok, err := m.GetForm("basic.yaml", "x.meta.json")
	if err != nil {
		t.Fatal(err)
	}
	if ok || row != nil {
		t.Errorf("GetForm on empty index = (%v, %v), want (nil, false)", row, ok)
	}
}

// TestEmptyIndex_FormsWithValueEmpty: scalar-value lookup over an empty index
// returns no filenames.
func TestEmptyIndex_FormsWithValueEmpty(t *testing.T) {
	m := newEmptyManager(t)
	got, err := m.FormsWithValue("basic.yaml", "status", "open")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("FormsWithValue on empty index = %v, want empty", got)
	}
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

// TestEmptyIndex_RevZero: fresh index revision is 0.
func TestEmptyIndex_RevZero(t *testing.T) {
	m := newEmptyManager(t)
	rev, err := m.Rev()
	if err != nil {
		t.Fatal(err)
	}
	if rev != 0 {
		t.Errorf("fresh Rev = %d, want 0", rev)
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

// TestRescanTemplate_UnknownTemplate: force-reindexing a template that was
// never on disk and never indexed collapses to OnTemplateDeleted, which is a
// no-op delete: no error, no template row, no rev bump.
func TestRescanTemplate_UnknownTemplate(t *testing.T) {
	f := newRescanFixture(t)
	// Seed one real template so the index is non-empty and we can prove the
	// unknown reindex leaves it untouched.
	f.addTemplateOnDisk(t, "real", "name: Real\n", 1)
	f.addFormOnDisk(t, "real", "r.meta.json", "x", 10)
	f.registerTemplate("real", nil, 1)
	f.registerForm("real", "r.meta.json", map[string]any{}, 10)
	must(t, f.hand.RescanAll(context.Background()))
	revBefore, _ := f.mgr.Rev()

	if err := f.hand.RescanTemplate(context.Background(), "ghost.yaml"); err != nil {
		t.Fatalf("RescanTemplate on unknown template errored: %v", err)
	}

	// ACTUAL behavior: the delete-of-nonexistent path still bumps rev because
	// isEmpty checks slice lengths, not rows affected. See suspectedBugs.
	revAfter, _ := f.mgr.Rev()
	if revAfter != revBefore+1 {
		t.Errorf("unknown reindex rev = %d, want %d (current no-effect-delete behavior)", revAfter, revBefore+1)
	}
	tpls, _ := f.mgr.ListTemplates()
	if got := sortedTemplateFilenames(tpls); !equalStrings(got, []string{"real.yaml"}) {
		t.Errorf("templates after unknown reindex = %v, want [real.yaml]", got)
	}
	if rows, _ := f.mgr.ListForms("ghost.yaml", QueryOpts{}); len(rows) != 0 {
		t.Errorf("ghost template gained %d form rows", len(rows))
	}
}

// TestRescanTemplate_MissingTemplateRemovesIndexedRows: a template that WAS
// indexed but whose YAML is now gone on disk reindexes to a delete that
// cascades its forms away.
func TestRescanTemplate_MissingTemplateRemovesIndexedRows(t *testing.T) {
	f := newRescanFixture(t)
	f.addTemplateOnDisk(t, "doc", "name: Doc\n", 1)
	f.addFormOnDisk(t, "doc", "a.meta.json", "x", 10)
	f.registerTemplate("doc", []template.Field{{Key: "body", Type: "text"}}, 1)
	f.registerForm("doc", "a.meta.json", map[string]any{"body": "alpha"}, 10)
	must(t, f.hand.RescanAll(context.Background()))

	// Prove the row was indexed before we delete the YAML.
	if _, ok, _ := f.mgr.GetForm("doc.yaml", "a.meta.json"); !ok {
		t.Fatal("precondition: form should be indexed before YAML removal")
	}

	// YAML vanishes, but the storage subdir and indexed rows remain.
	if err := os.Remove(filepath.Join(f.root, "templates", "doc.yaml")); err != nil {
		t.Fatal(err)
	}
	must(t, f.hand.RescanTemplate(context.Background(), "doc.yaml"))

	if _, ok, _ := f.mgr.GetForm("doc.yaml", "a.meta.json"); ok {
		t.Error("form row survived a missing-template reindex (no cascade)")
	}
	if rows, _ := f.mgr.ListForms("doc.yaml", QueryOpts{}); len(rows) != 0 {
		t.Errorf("forms after missing-template reindex = %d, want 0", len(rows))
	}
	tpls, _ := f.mgr.ListTemplates()
	if got := sortedTemplateFilenames(tpls); len(got) != 0 {
		t.Errorf("templates after missing-template reindex = %v, want []", got)
	}
}

// TestConcurrentReads_DuringRescan exercises the read API from several
// goroutines while a force-reindex runs, under -race. SQLite serializes the
// access; the assertion is that every read returns a coherent, error-free
// result and that the final state is exactly the reindexed collection.
func TestConcurrentReads_DuringRescan(t *testing.T) {
	f := newRescanFixture(t)
	f.addTemplateOnDisk(t, "doc", "name: Doc\n", 1)
	f.registerTemplate("doc", []template.Field{{Key: "body", Type: "text"}}, 1)
	for _, name := range []string{"a", "b", "c", "d"} {
		f.addFormOnDisk(t, "doc", name+".meta.json", "x", 10)
		f.registerForm("doc", name+".meta.json", map[string]any{"body": name}, 10)
	}
	must(t, f.hand.RescanAll(context.Background()))

	var wg sync.WaitGroup
	errc := make(chan error, 64)

	// Writer: force-reindex repeatedly.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			if err := f.hand.RescanTemplate(context.Background(), "doc.yaml"); err != nil {
				errc <- err
				return
			}
		}
	}()

	// Readers: hammer the read paths.
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				if _, err := f.mgr.ListForms("doc.yaml", QueryOpts{}); err != nil {
					errc <- err
					return
				}
				if _, err := f.mgr.ListTemplates(); err != nil {
					errc <- err
					return
				}
				if _, err := f.mgr.SearchForms("doc.yaml", "a", QueryOpts{}); err != nil {
					errc <- err
					return
				}
				if _, _, err := f.mgr.GetForm("doc.yaml", "a.meta.json"); err != nil {
					errc <- err
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errc)
	for err := range errc {
		t.Fatalf("concurrent read/reindex errored: %v", err)
	}

	// Final state: exactly the four reindexed forms remain.
	rows, _ := f.mgr.ListForms("doc.yaml", QueryOpts{})
	if got := sortedFormFilenames(rows); !equalStrings(got, []string{"a.meta.json", "b.meta.json", "c.meta.json", "d.meta.json"}) {
		t.Errorf("final forms = %v, want the four seeded items", got)
	}
}

// TestEmptyBatchReconcile_NoRevBump pins the isEmpty short-circuit: a reconcile
// with no upserts and no deletes must not begin a transaction or bump rev. This
// is the contract the ghost-delete path (see suspectedBugs) fails to honor,
// because a delete of a nonexistent template is non-empty by slice length even
// though it changes zero rows.
func TestEmptyBatchReconcile_NoRevBump(t *testing.T) {
	m := newEmptyManager(t)
	if rev, _ := m.Rev(); rev != 0 {
		t.Fatalf("precondition rev = %d, want 0", rev)
	}
	must(t, Reconcile(m.DB(), ReconcileBatch{}))
	if rev, _ := m.Rev(); rev != 0 {
		t.Errorf("empty-batch reconcile bumped rev to %d, want 0", rev)
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

// TestEmptyIndex_ListFormsPagingAndOrder: limit/offset/order shaping over a
// zero-row index returns no rows for every variant, including an unknown
// OrderBy that must fall back rather than fail.
func TestEmptyIndex_ListFormsPagingAndOrder(t *testing.T) {
	m := newEmptyManager(t)
	for _, opts := range []QueryOpts{
		{Limit: 10},
		{Offset: 5},
		{Limit: 3, Offset: 2},
		{OrderBy: "title_asc"},
		{OrderBy: "bogus_order_value"},
		{Tags: []string{"a", "b"}},
	} {
		rows, err := m.ListForms("basic.yaml", opts)
		if err != nil {
			t.Fatalf("ListForms(%+v) errored: %v", opts, err)
		}
		if len(rows) != 0 {
			t.Errorf("ListForms(%+v) = %d rows, want 0", opts, len(rows))
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
