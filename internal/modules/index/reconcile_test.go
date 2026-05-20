package index

import (
	"database/sql"
	"path/filepath"
	"sort"
	"testing"
)

// helpers for these tests — kept local so they don't pollute the
// scan/schema test files.

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := openIndexDB(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func readRev(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var raw string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key='rev'`).Scan(&raw); err != nil {
		// rev not stamped yet — counts as 0
		return 0
	}
	var v int64
	if _, err := scanInt64(raw, &v); err != nil {
		t.Fatalf("rev parse %q: %v", raw, err)
	}
	return v
}

// scanInt64 is a tiny helper so tests don't pull in strconv noise.
func scanInt64(raw string, dst *int64) (int64, error) {
	var v int64
	for _, c := range raw {
		if c < '0' || c > '9' {
			break
		}
		v = v*10 + int64(c-'0')
	}
	*dst = v
	return v, nil
}

func tplRow(name string, mtime int64) TemplateRow {
	return TemplateRow{
		Filename:            name + ".yaml",
		Name:                name,
		HasMarkdownTemplate: true,
		Mtime:               mtime,
	}
}

func formRow(tpl, file, id, title string, tags []string, mtime int64) FormRow {
	return FormRow{
		Template: tpl,
		Filename: file,
		ID:       id,
		Title:    title,
		Tags:     tags,
		Mtime:    mtime,
	}
}

func TestReconcile_AddsTemplatesAndBumpsRev(t *testing.T) {
	db := openTestDB(t)
	startRev := readRev(t, db)

	batch := ReconcileBatch{
		UpsertTemplates: []TemplateRow{tplRow("basic", 100), tplRow("looper", 200)},
	}
	if err := Reconcile(db, batch); err != nil {
		t.Fatal(err)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM templates`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("templates count = %d, want 2", n)
	}

	if got := readRev(t, db); got != startRev+1 {
		t.Errorf("rev did not bump exactly once: %d → %d", startRev, got)
	}
}

func TestReconcile_NoOpDoesNotBumpRev(t *testing.T) {
	// An empty batch shouldn't dirty the index. Avoids cache-buster
	// churn on the wiki HTTP server when nothing changed.
	db := openTestDB(t)
	startRev := readRev(t, db)

	if err := Reconcile(db, ReconcileBatch{}); err != nil {
		t.Fatal(err)
	}
	if got := readRev(t, db); got != startRev {
		t.Errorf("rev bumped on empty batch: %d → %d", startRev, got)
	}
}

func TestReconcile_UpsertTemplate_OverwritesExisting(t *testing.T) {
	db := openTestDB(t)

	if err := Reconcile(db, ReconcileBatch{
		UpsertTemplates: []TemplateRow{tplRow("basic", 100)},
	}); err != nil {
		t.Fatal(err)
	}
	updated := tplRow("basic", 200)
	updated.Name = "Renamed"
	if err := Reconcile(db, ReconcileBatch{
		UpsertTemplates: []TemplateRow{updated},
	}); err != nil {
		t.Fatal(err)
	}

	var name string
	var mtime int64
	if err := db.QueryRow(
		`SELECT name, mtime FROM templates WHERE filename='basic.yaml'`,
	).Scan(&name, &mtime); err != nil {
		t.Fatal(err)
	}
	if name != "Renamed" || mtime != 200 {
		t.Errorf("got name=%q mtime=%d, want Renamed/200", name, mtime)
	}
}

func TestReconcile_FormUpsertSyncsTags(t *testing.T) {
	db := openTestDB(t)
	if err := Reconcile(db, ReconcileBatch{
		UpsertTemplates: []TemplateRow{tplRow("basic", 100)},
	}); err != nil {
		t.Fatal(err)
	}

	// Insert form with tags [a, b].
	if err := Reconcile(db, ReconcileBatch{
		UpsertForms: []FormRow{
			formRow("basic.yaml", "one.meta.json", "id1", "One", []string{"a", "b"}, 100),
		},
	}); err != nil {
		t.Fatal(err)
	}
	if got := tagsForForm(t, db, "basic.yaml", "one.meta.json"); !equalStrings(got, []string{"a", "b"}) {
		t.Errorf("first round tags = %v, want [a b]", got)
	}

	// Upsert again with [b, c]. Old tag "a" must be gone, new tag "c" present.
	if err := Reconcile(db, ReconcileBatch{
		UpsertForms: []FormRow{
			formRow("basic.yaml", "one.meta.json", "id1", "One", []string{"b", "c"}, 200),
		},
	}); err != nil {
		t.Fatal(err)
	}
	if got := tagsForForm(t, db, "basic.yaml", "one.meta.json"); !equalStrings(got, []string{"b", "c"}) {
		t.Errorf("second round tags = %v, want [b c]", got)
	}
}

func TestReconcile_DeleteTemplateCascades(t *testing.T) {
	// Foreign-key cascades: removing a template must take its forms,
	// form_tags, and images with it. This is the "user deleted a
	// template" path — the reconciler shouldn't have to manually
	// delete from each table.
	db := openTestDB(t)
	if err := Reconcile(db, ReconcileBatch{
		UpsertTemplates: []TemplateRow{tplRow("basic", 100)},
		UpsertForms: []FormRow{
			formRow("basic.yaml", "one.meta.json", "id1", "One", []string{"x"}, 100),
		},
		UpsertImages: []ImageRow{
			{Template: "basic.yaml", Filename: "logo.png", Mtime: 100},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if err := Reconcile(db, ReconcileBatch{
		DeleteTemplates: []string{"basic.yaml"},
	}); err != nil {
		t.Fatal(err)
	}

	// Everything keyed off basic.yaml should be gone.
	for _, q := range []string{
		`SELECT COUNT(*) FROM templates WHERE filename='basic.yaml'`,
		`SELECT COUNT(*) FROM forms     WHERE template='basic.yaml'`,
		`SELECT COUNT(*) FROM form_tags WHERE template='basic.yaml'`,
		`SELECT COUNT(*) FROM images    WHERE template='basic.yaml'`,
	} {
		var n int
		if err := db.QueryRow(q).Scan(&n); err != nil {
			t.Fatalf("count %q: %v", q, err)
		}
		if n != 0 {
			t.Errorf("after cascade, %q got %d, want 0", q, n)
		}
	}
}

func TestReconcile_FormUpsertSyncsFacets(t *testing.T) {
	db := openTestDB(t)
	must(t, Reconcile(db, ReconcileBatch{
		UpsertTemplates: []TemplateRow{tplRow("basic", 100)},
	}))

	// Initial upsert with two facets.
	first := formRow("basic.yaml", "one.meta.json", "id1", "One", nil, 100)
	first.Facets = []FormFacet{
		{Key: "stage", Set: true, Selected: "draft"},
		{Key: "flag", Set: true, Selected: ""},
	}
	must(t, Reconcile(db, ReconcileBatch{UpsertForms: []FormRow{first}}))

	if got := facetsForForm(t, db, "basic.yaml", "one.meta.json"); len(got) != 2 {
		t.Errorf("first round facets = %v, want 2 entries", got)
	}

	// Upsert again with a single facet (the others must disappear; the
	// remaining one must reflect the new state).
	second := formRow("basic.yaml", "one.meta.json", "id1", "One", nil, 200)
	second.Facets = []FormFacet{
		{Key: "stage", Set: true, Selected: "published"},
	}
	must(t, Reconcile(db, ReconcileBatch{UpsertForms: []FormRow{second}}))

	got := facetsForForm(t, db, "basic.yaml", "one.meta.json")
	if len(got) != 1 || got[0].Key != "stage" || got[0].Selected != "published" {
		t.Errorf("second round facets = %+v, want one stage=published row", got)
	}
}

func TestReconcile_FormFacetsRoundTripThroughQueryForms(t *testing.T) {
	db := openTestDB(t)
	m := managerFromDB(t, db)

	must(t, Reconcile(db, ReconcileBatch{
		UpsertTemplates: []TemplateRow{tplRow("basic", 100)},
	}))

	row := formRow("basic.yaml", "one.meta.json", "id1", "One", []string{"x"}, 100)
	row.CreatedName = "Peter"
	row.CreatedEmail = "peter@example.com"
	row.UpdatedName = "Peter"
	row.UpdatedEmail = "peter@example.com"
	row.Facets = []FormFacet{
		{Key: "stage", Set: true, Selected: "draft"},
		{Key: "priority", Set: false, Selected: ""},
	}
	must(t, Reconcile(db, ReconcileBatch{UpsertForms: []FormRow{row}}))

	rows, err := m.ListForms("basic.yaml", QueryOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListForms returned %d rows, want 1", len(rows))
	}
	out := rows[0]
	if out.CreatedName != "Peter" || out.UpdatedEmail != "peter@example.com" {
		t.Errorf("audit identity lost: %+v", out)
	}
	if len(out.Facets) != 2 {
		t.Fatalf("facets returned %d, want 2: %+v", len(out.Facets), out.Facets)
	}
	byKey := map[string]FormFacet{}
	for _, f := range out.Facets {
		byKey[f.Key] = f
	}
	if s, ok := byKey["stage"]; !ok || !s.Set || s.Selected != "draft" {
		t.Errorf("stage facet wrong: %+v", s)
	}
	if p, ok := byKey["priority"]; !ok || p.Set || p.Selected != "" {
		t.Errorf("priority facet wrong: %+v", p)
	}
}

func TestReconcile_DeleteFormCascadesTags(t *testing.T) {
	db := openTestDB(t)
	must(t, Reconcile(db, ReconcileBatch{
		UpsertTemplates: []TemplateRow{tplRow("basic", 100)},
		UpsertForms: []FormRow{
			formRow("basic.yaml", "one.meta.json", "id1", "One", []string{"a", "b"}, 100),
		},
	}))

	must(t, Reconcile(db, ReconcileBatch{
		DeleteForms: []FormRef{{Template: "basic.yaml", Filename: "one.meta.json"}},
	}))

	for _, q := range []string{
		`SELECT COUNT(*) FROM forms WHERE template='basic.yaml' AND filename='one.meta.json'`,
		`SELECT COUNT(*) FROM form_tags WHERE template='basic.yaml' AND filename='one.meta.json'`,
	} {
		var n int
		if err := db.QueryRow(q).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%q expected 0 after delete, got %d", q, n)
		}
	}
}

func TestReconcile_RollbackOnError(t *testing.T) {
	// A bad batch (form references a non-existent template) must roll
	// back ALL changes including any earlier upserts in the same call.
	db := openTestDB(t)

	startRev := readRev(t, db)

	bad := ReconcileBatch{
		UpsertTemplates: []TemplateRow{tplRow("basic", 100)},
		// Form references "ghost.yaml" which is NOT in this batch and
		// not previously inserted — FK violation.
		UpsertForms: []FormRow{
			formRow("ghost.yaml", "one.meta.json", "id", "t", nil, 100),
		},
	}
	if err := Reconcile(db, bad); err == nil {
		t.Fatal("expected error from FK violation")
	}

	// Templates table should be empty (rolled back).
	var n int
	must(t, db.QueryRow(`SELECT COUNT(*) FROM templates`).Scan(&n))
	if n != 0 {
		t.Errorf("after rollback, templates = %d, want 0", n)
	}
	if got := readRev(t, db); got != startRev {
		t.Errorf("rev bumped despite rollback: %d → %d", startRev, got)
	}
}

// managerFromDB returns a *Manager wrapping the supplied DB. The
// production NewManager owns its handle (Close releases the file); for
// tests that already opened the DB via openTestDB we want a manager
// that shares the handle without taking ownership of close-time.
func managerFromDB(t *testing.T, db *sql.DB) *Manager {
	t.Helper()
	return &Manager{db: db}
}

// facetsForForm returns the facet rows on disk for one form, sorted by
// facet_key so tests can compare deterministically.
func facetsForForm(t *testing.T, db *sql.DB, tpl, file string) []FormFacet {
	t.Helper()
	rows, err := db.Query(
		`SELECT facet_key, set_flag, selected FROM form_facets
		   WHERE template = ? AND filename = ?
		   ORDER BY facet_key`,
		tpl, file,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []FormFacet
	for rows.Next() {
		var key string
		var setFlag int
		var sel sql.NullString
		if err := rows.Scan(&key, &setFlag, &sel); err != nil {
			t.Fatal(err)
		}
		out = append(out, FormFacet{Key: key, Set: setFlag != 0, Selected: sel.String})
	}
	return out
}

func tagsForForm(t *testing.T, db *sql.DB, tpl, file string) []string {
	t.Helper()
	rows, err := db.Query(
		`SELECT tag FROM form_tags WHERE template=? AND filename=? ORDER BY tag`,
		tpl, file,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			t.Fatal(err)
		}
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
