package index

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// rescanFixture sets up the on-disk tree, the matching fake loaders,
// and a Manager + EventHandler with `root` wired in. Each test seeds
// its own tree and then calls h.RescanAll.
type rescanFixture struct {
	root  string
	mgr   *Manager
	hand  *EventHandler
	tpls  map[string]*TemplateRecord
	forms map[string]*FormRecord
}

func newRescanFixture(t *testing.T) *rescanFixture {
	t.Helper()
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "i.db")
	m, err := NewManager(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })

	tpls := map[string]*TemplateRecord{}
	forms := map[string]*FormRecord{}
	h := NewEventHandler(m,
		&fakeTemplateLoader{tpls: tpls},
		&fakeFormStore{forms: forms},
	)
	h.SetRoot(root)
	return &rescanFixture{root: root, mgr: m, hand: h, tpls: tpls, forms: forms}
}

func (f *rescanFixture) addTemplateOnDisk(t *testing.T, stem string, content string, mtime int64) {
	t.Helper()
	path := filepath.Join(f.root, "templates", stem+".yaml")
	makeFile(t, path, content, mtime)
}

func (f *rescanFixture) addFormOnDisk(t *testing.T, stem, datafile, content string, mtime int64) {
	t.Helper()
	path := filepath.Join(f.root, "storage", stem, datafile)
	makeFile(t, path, content, mtime)
}

func (f *rescanFixture) registerTemplate(stem string, fields []template.Field, mtime int64) {
	rec := tplRecord(stem, fields, mtime)
	rec.Template.Filename = stem + ".yaml" // align loader with disk
	f.tpls[stem+".yaml"] = rec
}

func (f *rescanFixture) registerForm(stem, datafile string, data map[string]any, mtime int64) {
	f.forms[stem+".yaml/"+datafile] = formRecord(storage.FormMeta{}, data, mtime)
}

func sortedTemplateFilenames(rows []TemplateRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Filename
	}
	sort.Strings(out)
	return out
}

func sortedFormFilenames(rows []FormRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Filename
	}
	sort.Strings(out)
	return out
}

// TestRescanAll_EmptyIndex_PopulatesFromDisk is the first-run path:
// the SQLite file is brand-new (just schema, no rows), the disk has a
// few templates and forms, and a single RescanAll call should bring
// the index fully up to date.
func TestRescanAll_EmptyIndex_PopulatesFromDisk(t *testing.T) {
	f := newRescanFixture(t)

	// On-disk
	f.addTemplateOnDisk(t, "basic", "name: Basic\n", 1_700_000_001)
	f.addTemplateOnDisk(t, "looper", "name: Looper\n", 1_700_000_002)
	f.addFormOnDisk(t, "basic", "one.meta.json", `{"meta":{}}`, 1_700_000_010)
	f.addFormOnDisk(t, "basic", "two.meta.json", `{"meta":{}}`, 1_700_000_011)

	// Loaders mirror disk. RescanAll should see disk, ask loaders for
	// content, and reconcile.
	f.registerTemplate("basic", []template.Field{
		{Key: "id", Type: "guid"},
		{Key: "labels", Type: "tags"},
	}, 1_700_000_001)
	f.registerTemplate("looper", nil, 1_700_000_002)
	f.registerForm("basic", "one.meta.json",
		map[string]any{"id": "g1", "labels": []any{"alpha"}}, 1_700_000_010)
	f.registerForm("basic", "two.meta.json",
		map[string]any{"id": "g2", "labels": []any{"beta"}}, 1_700_000_011)

	if err := f.hand.RescanAll(context.Background()); err != nil {
		t.Fatal(err)
	}

	tplsRows, _ := f.mgr.ListTemplates()
	if got := sortedTemplateFilenames(tplsRows); !equalStrings(got, []string{"basic.yaml", "looper.yaml"}) {
		t.Errorf("templates = %v", got)
	}

	formRows, _ := f.mgr.ListForms("basic.yaml", QueryOpts{})
	if got := sortedFormFilenames(formRows); !equalStrings(got, []string{"one.meta.json", "two.meta.json"}) {
		t.Errorf("forms = %v", got)
	}

	if rev, _ := f.mgr.Rev(); rev != 1 {
		t.Errorf("rev after first rescan = %d, want 1", rev)
	}
}

// TestRescanAll_NoOp_WhenNothingChanged: a second rescan against an
// already-up-to-date index must NOT bump rev - that would churn the
// HTTP layer's ETags for no reason.
func TestRescanAll_NoOp_WhenNothingChanged(t *testing.T) {
	f := newRescanFixture(t)

	f.addTemplateOnDisk(t, "basic", "x", 1)
	f.addFormOnDisk(t, "basic", "x.meta.json", "y", 10)
	f.registerTemplate("basic", nil, 1)
	f.registerForm("basic", "x.meta.json", map[string]any{}, 10)

	must(t, f.hand.RescanAll(context.Background()))
	revBefore, _ := f.mgr.Rev()

	must(t, f.hand.RescanAll(context.Background()))
	revAfter, _ := f.mgr.Rev()

	if revAfter != revBefore {
		t.Errorf("idempotent rescan bumped rev: %d → %d", revBefore, revAfter)
	}
}

// TestRescanAll_DetectsAdded: file added on disk after the last scan
// (sync pulled new content) → next rescan picks it up.
func TestRescanAll_DetectsAdded(t *testing.T) {
	f := newRescanFixture(t)

	f.addTemplateOnDisk(t, "basic", "x", 1)
	f.registerTemplate("basic", nil, 1)
	must(t, f.hand.RescanAll(context.Background()))

	// New form lands on disk + in the loader (simulating a sync).
	f.addFormOnDisk(t, "basic", "new.meta.json", "y", 50)
	f.registerForm("basic", "new.meta.json", map[string]any{}, 50)

	must(t, f.hand.RescanAll(context.Background()))

	rows, _ := f.mgr.ListForms("basic.yaml", QueryOpts{})
	if len(rows) != 1 || rows[0].Filename != "new.meta.json" {
		t.Errorf("got %+v, want one row [new.meta.json]", rows)
	}
}

// TestRescanAll_DetectsChanged: form rewritten on disk with a new
// mtime → next rescan re-loads its content from the loader.
func TestRescanAll_DetectsChanged(t *testing.T) {
	f := newRescanFixture(t)

	f.addTemplateOnDisk(t, "basic", "x", 1)
	f.addFormOnDisk(t, "basic", "x.meta.json", "before", 10)
	f.registerTemplate("basic", []template.Field{{Key: "labels", Type: "tags"}}, 1)
	f.registerForm("basic", "x.meta.json",
		map[string]any{"labels": []any{"old"}}, 10)
	must(t, f.hand.RescanAll(context.Background()))

	// Simulate sync: disk content changed, mtime bumped, loader returns
	// the new shape.
	f.addFormOnDisk(t, "basic", "x.meta.json", "after", 20)
	f.registerForm("basic", "x.meta.json",
		map[string]any{"labels": []any{"fresh"}}, 20)

	must(t, f.hand.RescanAll(context.Background()))

	row, _, _ := f.mgr.GetForm("basic.yaml", "x.meta.json")
	if got := sortedCopy(row.Tags); !equalStrings(got, []string{"fresh"}) {
		t.Errorf("tags = %v, want [fresh] (rescan didn't pick up new content)", got)
	}
	// Disk mtime is stored in nanoseconds (matches scanDisk's
	// info.ModTime().UnixNano()); 20 seconds → 20e9 nanos.
	if row.Mtime != 20*1_000_000_000 {
		t.Errorf("mtime = %d, want %d", row.Mtime, int64(20*1_000_000_000))
	}
}

// TestRescanAll_DetectsRemoved: file deleted on disk → next rescan
// removes the row + cascades dependent rows.
func TestRescanAll_DetectsRemoved(t *testing.T) {
	f := newRescanFixture(t)

	f.addTemplateOnDisk(t, "basic", "x", 1)
	f.addFormOnDisk(t, "basic", "doomed.meta.json", "y", 10)
	f.registerTemplate("basic", []template.Field{{Key: "labels", Type: "tags"}}, 1)
	f.registerForm("basic", "doomed.meta.json",
		map[string]any{"labels": []any{"a"}}, 10)
	must(t, f.hand.RescanAll(context.Background()))

	// Sync deletes the form on disk.
	must(t, os.Remove(filepath.Join(f.root, "storage", "basic", "doomed.meta.json")))

	must(t, f.hand.RescanAll(context.Background()))

	rows, _ := f.mgr.ListForms("basic.yaml", QueryOpts{})
	if len(rows) != 0 {
		t.Errorf("form not removed: %+v", rows)
	}
}

// TestRescanAll_SkipsUnloadableForm: one form on disk is malformed
// (the loader returns an error for it). RescanAll must NOT abort the
// whole batch - it must index every other form (across this template
// AND other templates) and return the per-file error so the caller
// can log it.
func TestRescanAll_SkipsUnloadableForm(t *testing.T) {
	f := newRescanFixture(t)

	f.addTemplateOnDisk(t, "basic", "x", 1)
	f.addTemplateOnDisk(t, "looper", "y", 2)
	f.addFormOnDisk(t, "basic", "good.meta.json", `{}`, 10)
	f.addFormOnDisk(t, "basic", "BAD.meta.json", `not json`, 11)
	f.addFormOnDisk(t, "looper", "also-good.meta.json", `{}`, 12)

	f.registerTemplate("basic", nil, 1)
	f.registerTemplate("looper", nil, 2)
	f.registerForm("basic", "good.meta.json", map[string]any{}, 10)
	// "BAD.meta.json" is on disk but NOT registered with the loader -
	// the fake form store returns "not found" → adapter returns error
	// → RescanAll must skip this row and keep going.
	f.registerForm("looper", "also-good.meta.json", map[string]any{}, 12)

	err := f.hand.RescanAll(context.Background())
	if err == nil {
		t.Fatal("expected error surfaced from bad form, got nil")
	}
	if !strings.Contains(err.Error(), "BAD.meta.json") {
		t.Errorf("error %q should mention the bad form", err)
	}

	basicRows, _ := f.mgr.ListForms("basic.yaml", QueryOpts{})
	if got := sortedFormFilenames(basicRows); !equalStrings(got, []string{"good.meta.json"}) {
		t.Errorf("basic forms = %v, want [good.meta.json] (bad row skipped, good row kept)", got)
	}

	looperRows, _ := f.mgr.ListForms("looper.yaml", QueryOpts{})
	if got := sortedFormFilenames(looperRows); !equalStrings(got, []string{"also-good.meta.json"}) {
		t.Errorf("looper forms = %v - sibling templates must NOT be collateral damage from one bad form", got)
	}
}

// TestRescanAll_SkipsUnloadableTemplate: a template .yaml on disk can't
// be parsed (loader errors). The template row is dropped from the
// batch, but its storage subdir is also skipped (no FK target) and
// sibling templates still index fully.
func TestRescanAll_SkipsUnloadableTemplate(t *testing.T) {
	f := newRescanFixture(t)

	f.addTemplateOnDisk(t, "broken", "garbage", 1)
	f.addTemplateOnDisk(t, "ok", "y", 2)
	f.addFormOnDisk(t, "ok", "one.meta.json", `{}`, 10)

	// Only "ok" is registered with the loader. "broken" → loader returns
	// "not found" → RescanAll must skip its row and continue.
	f.registerTemplate("ok", nil, 2)
	f.registerForm("ok", "one.meta.json", map[string]any{}, 10)

	err := f.hand.RescanAll(context.Background())
	if err == nil {
		t.Fatal("expected error surfaced from bad template, got nil")
	}
	if !strings.Contains(err.Error(), "broken.yaml") {
		t.Errorf("error %q should mention the bad template", err)
	}

	tplsRows, _ := f.mgr.ListTemplates()
	if got := sortedTemplateFilenames(tplsRows); !equalStrings(got, []string{"ok.yaml"}) {
		t.Errorf("templates = %v, want [ok.yaml] only (broken skipped, sibling kept)", got)
	}

	okRows, _ := f.mgr.ListForms("ok.yaml", QueryOpts{})
	if got := sortedFormFilenames(okRows); !equalStrings(got, []string{"one.meta.json"}) {
		t.Errorf("ok forms = %v - sibling forms must NOT be collateral damage from a broken template", got)
	}
}

// TestRescanAll_DeletesOrphanTemplates: the user removed a template
// .yaml; the storage dir may or may not still be there. Either way
// the templates row goes (and FK cascades take its forms with it).
func TestRescanAll_DeletesOrphanTemplates(t *testing.T) {
	f := newRescanFixture(t)

	f.addTemplateOnDisk(t, "basic", "x", 1)
	f.addFormOnDisk(t, "basic", "one.meta.json", "y", 10)
	f.registerTemplate("basic", nil, 1)
	f.registerForm("basic", "one.meta.json", map[string]any{}, 10)
	must(t, f.hand.RescanAll(context.Background()))

	// Sync deletes the .yaml.
	must(t, os.Remove(filepath.Join(f.root, "templates", "basic.yaml")))

	must(t, f.hand.RescanAll(context.Background()))

	tplsRows, _ := f.mgr.ListTemplates()
	if len(tplsRows) != 0 {
		t.Errorf("template not removed: %+v", tplsRows)
	}
	// FK cascade: forms keyed off basic.yaml are gone too.
	formRows, _ := f.mgr.ListForms("basic.yaml", QueryOpts{})
	if len(formRows) != 0 {
		t.Errorf("forms not cascaded: %+v", formRows)
	}
}

// TestRescanTemplate_ForceReindexesItems is the core reason the function
// exists: RescanAll is mtime-diff based, so a storage item whose bytes
// didn't change is skipped even when its derived search body should be
// rebuilt. RescanTemplate force re-reads every item regardless of mtime.
func TestRescanTemplate_ForceReindexesItems(t *testing.T) {
	f := newRescanFixture(t)
	f.addTemplateOnDisk(t, "doc", "name: Doc\n", 1)
	f.addFormOnDisk(t, "doc", "a.meta.json", "x", 10)
	f.registerTemplate("doc", []template.Field{{Key: "body", Type: "text"}}, 1)
	f.registerForm("doc", "a.meta.json", map[string]any{"body": "alpha"}, 10)
	must(t, f.hand.RescanAll(context.Background()))

	if rows, _ := f.mgr.SearchForms("doc.yaml", "alpha", QueryOpts{}); len(rows) != 1 {
		t.Fatalf("initial body not indexed: %d hits", len(rows))
	}

	// Item content changes but the on-disk mtime/size do not (e.g. the
	// projection logic changed, not the file). RescanAll must NOT pick it
	// up - that's exactly the gap RescanTemplate fills.
	f.registerForm("doc", "a.meta.json", map[string]any{"body": "omega"}, 10)
	must(t, f.hand.RescanAll(context.Background()))
	if rows, _ := f.mgr.SearchForms("doc.yaml", "omega", QueryOpts{}); len(rows) != 0 {
		t.Fatalf("RescanAll re-read an unchanged file (test premise broken): %d hits", len(rows))
	}

	must(t, f.hand.RescanTemplate(context.Background(), "doc.yaml"))
	if rows, _ := f.mgr.SearchForms("doc.yaml", "omega", QueryOpts{}); len(rows) != 1 {
		t.Errorf("force reindex missed the new body: %d hits", len(rows))
	}
	if rows, _ := f.mgr.SearchForms("doc.yaml", "alpha", QueryOpts{}); len(rows) != 0 {
		t.Errorf("stale body still searchable after reindex: %d hits", len(rows))
	}
}

// TestRescanTemplate_DropsOrphans: an item removed from disk must lose
// its index row when the collection is reindexed.
func TestRescanTemplate_DropsOrphans(t *testing.T) {
	f := newRescanFixture(t)
	f.addTemplateOnDisk(t, "doc", "name: Doc\n", 1)
	f.addFormOnDisk(t, "doc", "a.meta.json", "x", 10)
	f.addFormOnDisk(t, "doc", "b.meta.json", "x", 11)
	f.registerTemplate("doc", []template.Field{{Key: "body", Type: "text"}}, 1)
	f.registerForm("doc", "a.meta.json", map[string]any{"body": "alpha"}, 10)
	f.registerForm("doc", "b.meta.json", map[string]any{"body": "beta"}, 11)
	must(t, f.hand.RescanAll(context.Background()))

	if err := os.Remove(filepath.Join(f.root, "storage", "doc", "b.meta.json")); err != nil {
		t.Fatal(err)
	}
	must(t, f.hand.RescanTemplate(context.Background(), "doc.yaml"))

	if _, ok, _ := f.mgr.GetForm("doc.yaml", "b.meta.json"); ok {
		t.Error("orphan b.meta.json still indexed")
	}
	if _, ok, _ := f.mgr.GetForm("doc.yaml", "a.meta.json"); !ok {
		t.Error("a.meta.json should remain")
	}
}

// TestRescanTemplate_TemplateGoneDeletes: reindexing a template whose
// YAML no longer exists on disk collapses to a delete (cascade clears
// the collection) rather than erroring.
func TestRescanTemplate_TemplateGoneDeletes(t *testing.T) {
	f := newRescanFixture(t)
	f.addTemplateOnDisk(t, "doc", "name: Doc\n", 1)
	f.addFormOnDisk(t, "doc", "a.meta.json", "x", 10)
	f.registerTemplate("doc", []template.Field{{Key: "body", Type: "text"}}, 1)
	f.registerForm("doc", "a.meta.json", map[string]any{"body": "alpha"}, 10)
	must(t, f.hand.RescanAll(context.Background()))

	if err := os.Remove(filepath.Join(f.root, "templates", "doc.yaml")); err != nil {
		t.Fatal(err)
	}
	must(t, f.hand.RescanTemplate(context.Background(), "doc.yaml"))

	tpls, _ := f.mgr.ListTemplates()
	for _, tr := range tpls {
		if tr.Filename == "doc.yaml" {
			t.Error("template row not deleted")
		}
	}
	if rows, _ := f.mgr.ListForms("doc.yaml", QueryOpts{}); len(rows) != 0 {
		t.Errorf("collection not cascaded: %+v", rows)
	}
}

// TestRescanTemplate_RootUnset is the guard for a misconfigured handler:
// RescanTemplate needs the context root to scan disk, so it errors
// rather than silently no-opping when SetRoot was never called.
func TestRescanTemplate_RootUnset(t *testing.T) {
	m, err := NewManager(filepath.Join(t.TempDir(), "i.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	h := NewEventHandler(m,
		&fakeTemplateLoader{tpls: map[string]*TemplateRecord{}},
		&fakeFormStore{forms: map[string]*FormRecord{}},
	)
	// No SetRoot.
	if err := h.RescanTemplate(context.Background(), "doc.yaml"); err == nil {
		t.Fatal("expected error when root is unset")
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

	// Reindexing a template that was never indexed deletes zero rows, so the
	// ETag (rev) must not churn.
	revAfter, _ := f.mgr.Rev()
	if revAfter != revBefore {
		t.Errorf("unknown reindex rev = %d, want %d (no-op delete must not bump)", revAfter, revBefore)
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
