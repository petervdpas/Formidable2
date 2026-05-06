package index

import (
	"context"
	"os"
	"path/filepath"
	"sort"
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
// already-up-to-date index must NOT bump rev — that would churn the
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
