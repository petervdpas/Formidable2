package form

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// ─────────────────────────────────────────────────────────────────────
// Test doubles. The form Manager talks to three narrow interfaces
// (templateLoader, formStore, configReader) - easy to fake in-process
// without touching the filesystem.
// ─────────────────────────────────────────────────────────────────────

type fakeTemplates struct {
	byName map[string]*template.Template
}

func (f *fakeTemplates) LoadTemplate(name string) (*template.Template, error) {
	t, ok := f.byName[name]
	if !ok {
		return nil, errors.New("not found: " + name)
	}
	return t, nil
}

type fakeStorage struct {
	forms     map[string]map[string]*storage.Form // template → datafile → form
	listed    map[string][]string                 // template → files
	summaries map[string][]storage.FormSummary
	deleted   []string
	saves     []saveCall
}

type saveCall struct {
	Template string
	Datafile string
	Data     map[string]any
}

func newFakeStorage() *fakeStorage {
	return &fakeStorage{
		forms:     map[string]map[string]*storage.Form{},
		listed:    map[string][]string{},
		summaries: map[string][]storage.FormSummary{},
	}
}

func (s *fakeStorage) EnsureFormDir(_ string) error { return nil }

func (s *fakeStorage) ListForms(t string) ([]string, error) { return s.listed[t], nil }

func (s *fakeStorage) ExtendedListForms(t string) ([]storage.FormSummary, error) {
	return s.summaries[t], nil
}

func (s *fakeStorage) LoadForm(t, df string) *storage.Form {
	tf := s.forms[t]
	if tf == nil {
		return nil
	}
	return tf[df]
}

func (s *fakeStorage) SaveForm(_ context.Context, t, df string, data map[string]any) storage.SaveResult {
	s.saves = append(s.saves, saveCall{Template: t, Datafile: df, Data: data})
	if s.forms[t] == nil {
		s.forms[t] = map[string]*storage.Form{}
	}
	// Mimic storage.Sanitize's relevant outputs: stash the data verbatim
	// under a Form so subsequent LoadForm returns it.
	meta := storage.FormMeta{
		Template: "tpl",
		Created:  storage.AuditEntry{At: "2026-05-05T00:00:00Z"},
		Updated:  storage.AuditEntry{At: "2026-05-05T00:00:00Z"},
	}
	if injected, ok := data["_meta"].(map[string]any); ok {
		if id, ok := injected["id"].(string); ok {
			meta.ID = id
		}
		if v, ok := injected["flagged"].(bool); ok {
			if meta.Facets == nil {
				meta.Facets = map[string]storage.FacetState{}
			}
			meta.Facets["flag"] = storage.FacetState{Set: v}
		}
	}
	cleaned := map[string]any{}
	for k, v := range data {
		if k == "_meta" {
			continue
		}
		cleaned[k] = v
	}
	s.forms[t][df] = &storage.Form{Meta: meta, Data: cleaned}
	return storage.SaveResult{Success: true, Path: df}
}

func (s *fakeStorage) DeleteForm(t, df string) error {
	s.deleted = append(s.deleted, t+"/"+df)
	delete(s.forms[t], df)
	return nil
}

func (s *fakeStorage) SortFieldValue(t, df, fieldKey, _, _ string) (any, error) {
	if f := s.LoadForm(t, df); f != nil {
		return f.Data[fieldKey], nil
	}
	return nil, nil
}

func (s *fakeStorage) DedupFieldValue(t, df, fieldKey, _ string) (any, error) {
	if f := s.LoadForm(t, df); f != nil {
		return f.Data[fieldKey], nil
	}
	return nil, nil
}

type fakeConfig struct {
	loopCollapsed bool
	relationSync  bool
}

func (c *fakeConfig) FormDefaults() ConfigDefaults {
	return ConfigDefaults{
		LoopStateCollapsed:  c.loopCollapsed,
		RelationSyncEnabled: c.relationSync,
	}
}

func newTestManager() (*Manager, *fakeTemplates, *fakeStorage, *fakeConfig) {
	tpls := &fakeTemplates{byName: map[string]*template.Template{}}
	store := newFakeStorage()
	cfg := &fakeConfig{}
	m := NewManager(tpls, store, cfg, nil)
	return m, tpls, store, cfg
}

// ─────────────────────────────────────────────────────────────────────
// BuildView
// ─────────────────────────────────────────────────────────────────────

func TestBuildView_TemplateNotFoundIsError(t *testing.T) {
	m, _, _, _ := newTestManager()
	if _, err := m.BuildView("ghost.yaml", ""); err == nil {
		t.Errorf("expected error for missing template")
	}
}

func TestBuildView_UnsavedFormGivesEmptyValuesAndDefaults(t *testing.T) {
	m, tpls, _, _ := newTestManager()
	tpls.byName["t.yaml"] = &template.Template{
		Filename: "t.yaml",
		Fields: []template.Field{
			{Key: "title", Type: "text", Default: "Hello"},
			{Key: "active", Type: "boolean"},
		},
	}
	view, err := m.BuildView("t.yaml", "")
	if err != nil {
		t.Fatalf("BuildView: %v", err)
	}
	if view.Saved {
		t.Errorf("unsaved form should have Saved=false")
	}
	if view.Datafile != "" {
		t.Errorf("unsaved form Datafile: want %q, got %q", "", view.Datafile)
	}
	if got := view.Values["title"]; got != "Hello" {
		t.Errorf("default not injected: %v", got)
	}
	if got := view.Values["active"]; got != false {
		t.Errorf("boolean default: want false, got %v", got)
	}
}

func TestBuildView_LoopGroupsComputed(t *testing.T) {
	m, tpls, _, _ := newTestManager()
	tpls.byName["t.yaml"] = &template.Template{
		Fields: []template.Field{
			{Key: "items", Type: "loopstart", SummaryField: "name"},
			{Key: "name", Type: "text"},
			{Key: "items", Type: "loopstop"},
		},
	}
	view, err := m.BuildView("t.yaml", "")
	if err != nil {
		t.Fatalf("BuildView: %v", err)
	}
	if len(view.LoopGroups) != 1 {
		t.Fatalf("want 1 loop group, got %d", len(view.LoopGroups))
	}
	if view.LoopGroups[0].SummaryFieldKey != "name" {
		t.Errorf("summary not propagated: %+v", view.LoopGroups[0])
	}
}

func TestBuildView_LoopCollapsedFromConfig(t *testing.T) {
	m, tpls, _, cfg := newTestManager()
	cfg.loopCollapsed = true
	tpls.byName["t.yaml"] = &template.Template{
		Fields: []template.Field{
			{Key: "k", Type: "loopstart"},
			{Key: "k", Type: "loopstop"},
		},
	}
	view, _ := m.BuildView("t.yaml", "")
	if !view.LoopGroups[0].DefaultCollapsed {
		t.Errorf("config-collapsed should reach the view")
	}
}

func TestBuildView_LoadsExistingForm(t *testing.T) {
	m, tpls, store, _ := newTestManager()
	tpls.byName["t.yaml"] = &template.Template{
		Fields: []template.Field{{Key: "title", Type: "text"}},
	}
	store.forms["t.yaml"] = map[string]*storage.Form{
		"f.meta.json": {
			Meta: storage.FormMeta{ID: "abc-123", Updated: storage.AuditEntry{At: "now"}},
			Data: map[string]any{"title": "Saved"},
		},
	}
	view, err := m.BuildView("t.yaml", "f.meta.json")
	if err != nil {
		t.Fatalf("BuildView: %v", err)
	}
	if !view.Saved {
		t.Errorf("loaded form should be Saved=true")
	}
	if view.Values["title"] != "Saved" {
		t.Errorf("title from disk lost: %v", view.Values["title"])
	}
	if view.Meta.ID != "abc-123" {
		t.Errorf("meta lost: %+v", view.Meta)
	}
}

func TestBuildView_LoadFormMissingFallsBackToUnsaved(t *testing.T) {
	// LoadForm returns nil for a missing file (mirrors storage's
	// "treat missing as unsaved"). BuildView should then synthesize
	// an empty view rather than erroring.
	m, tpls, _, _ := newTestManager()
	tpls.byName["t.yaml"] = &template.Template{
		Fields: []template.Field{{Key: "title", Type: "text", Default: "D"}},
	}
	view, err := m.BuildView("t.yaml", "doesnotexist.meta.json")
	if err != nil {
		t.Fatalf("BuildView should not error on missing datafile: %v", err)
	}
	if view.Saved {
		t.Errorf("missing file should give Saved=false")
	}
	if view.Values["title"] != "D" {
		t.Errorf("defaults should still inject: %v", view.Values["title"])
	}
}

// ─────────────────────────────────────────────────────────────────────
// SaveValues
// ─────────────────────────────────────────────────────────────────────

func TestSaveValues_PersistsAndReturnsRoundTrippedView(t *testing.T) {
	m, tpls, store, _ := newTestManager()
	tpls.byName["t.yaml"] = &template.Template{
		Filename: "t.yaml",
		Fields:   []template.Field{{Key: "title", Type: "text"}},
	}
	view, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "row.meta.json",
		Values:   map[string]any{"title": "Hello"},
		Meta:     storage.FormMeta{ID: "id-1"},
	})
	if err != nil {
		t.Fatalf("SaveValues: %v", err)
	}
	if !view.Saved || view.Datafile != "row.meta.json" {
		t.Errorf("save view: %+v", view)
	}
	if got := view.Values["title"]; got != "Hello" {
		t.Errorf("round-trip lost data: %v", got)
	}
	if len(store.saves) != 1 {
		t.Errorf("expected 1 save call, got %d", len(store.saves))
	}
}

type fakeEdgeSyncer struct {
	calls    []edgeSyncCall
	addCalls []edgeSyncCall
	err      error
}

type edgeSyncCall struct {
	host string
	guid string
	data map[string]any
}

func (f *fakeEdgeSyncer) SyncReferenceEdges(host, guid string, _ []template.Field, data map[string]any) error {
	f.calls = append(f.calls, edgeSyncCall{host: host, guid: guid, data: data})
	return f.err
}

func (f *fakeEdgeSyncer) AddReferenceEdges(host, guid string, _ []template.Field, data map[string]any) error {
	f.addCalls = append(f.addCalls, edgeSyncCall{host: host, guid: guid, data: data})
	return f.err
}

func TestSaveValues_InvokesReferenceEdgeSyncerWithPersistedGuid(t *testing.T) {
	m, tpls, _, _ := newTestManager()
	tpls.byName["t.yaml"] = &template.Template{
		Filename: "t.yaml",
		Fields:   []template.Field{{Key: "ref", Type: "api", Collection: "people.yaml"}},
	}
	sync := &fakeEdgeSyncer{}
	m.SetReferenceEdgeSyncer(sync)

	if _, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "row.meta.json",
		Values:   map[string]any{"ref": "p-1"},
		Meta:     storage.FormMeta{ID: "id-1"},
	}); err != nil {
		t.Fatalf("SaveValues: %v", err)
	}
	if len(sync.calls) != 1 {
		t.Fatalf("expected 1 sync call, got %d", len(sync.calls))
	}
	c := sync.calls[0]
	if c.host != "t.yaml" || c.guid != "id-1" || c.data["ref"] != "p-1" {
		t.Errorf("sync call = %+v, want host=t.yaml guid=id-1 ref=p-1", c)
	}
}

func TestBuildView_HealsEdgesAddOnly(t *testing.T) {
	m, tpls, store, _ := newTestManager()
	tpls.byName["t.yaml"] = &template.Template{
		Filename: "t.yaml",
		Fields:   []template.Field{{Key: "ref", Type: "api", Collection: "people.yaml"}},
	}
	store.forms["t.yaml"] = map[string]*storage.Form{
		"row.meta.json": {
			Meta: storage.FormMeta{ID: "host-1", Template: "t.yaml"},
			Data: map[string]any{"ref": "p-1"},
		},
	}
	sync := &fakeEdgeSyncer{}
	m.SetReferenceEdgeSyncer(sync)

	if _, err := m.BuildView("t.yaml", "row.meta.json"); err != nil {
		t.Fatalf("BuildView: %v", err)
	}
	if len(sync.addCalls) != 1 {
		t.Fatalf("expected 1 add-only heal call, got %d", len(sync.addCalls))
	}
	c := sync.addCalls[0]
	if c.host != "t.yaml" || c.guid != "host-1" || c.data["ref"] != "p-1" {
		t.Errorf("add call = %+v, want host=t.yaml guid=host-1 ref=p-1", c)
	}
	// Load must not run the draining save reconcile.
	if len(sync.calls) != 0 {
		t.Errorf("BuildView ran the draining SyncReferenceEdges %d time(s); load must be add-only", len(sync.calls))
	}
}

func TestBuildView_UnsavedViewSkipsHeal(t *testing.T) {
	m, tpls, _, _ := newTestManager()
	tpls.byName["t.yaml"] = &template.Template{Filename: "t.yaml"}
	sync := &fakeEdgeSyncer{}
	m.SetReferenceEdgeSyncer(sync)

	if _, err := m.BuildView("t.yaml", ""); err != nil {
		t.Fatalf("BuildView: %v", err)
	}
	if len(sync.addCalls) != 0 || len(sync.calls) != 0 {
		t.Errorf("unsaved view must not touch edges: add=%d sync=%d", len(sync.addCalls), len(sync.calls))
	}
}

func TestSaveValues_SyncerErrorDoesNotFailSave(t *testing.T) {
	m, tpls, _, _ := newTestManager()
	tpls.byName["t.yaml"] = &template.Template{
		Filename: "t.yaml",
		Fields:   []template.Field{{Key: "ref", Type: "api", Collection: "people.yaml"}},
	}
	m.SetReferenceEdgeSyncer(&fakeEdgeSyncer{err: errors.New("boom")})

	view, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "row.meta.json",
		Values:   map[string]any{"ref": "p-1"},
		Meta:     storage.FormMeta{ID: "id-1"},
	})
	if err != nil {
		t.Fatalf("syncer error should not fail save: %v", err)
	}
	if !view.Saved {
		t.Error("view should report saved despite syncer error")
	}
}

func TestSaveValues_EmptyDatafileIsError(t *testing.T) {
	// Per the spec we picked: caller (UI) gathers a name from the
	// user. SaveValues rejects empty rather than auto-generating.
	m, tpls, _, _ := newTestManager()
	tpls.byName["t.yaml"] = &template.Template{
		Fields: []template.Field{{Key: "k", Type: "text"}},
	}
	_, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "",
		Values:   map[string]any{"k": "v"},
	})
	if err == nil {
		t.Errorf("empty datafile should error")
	}
}

func TestSaveValues_TemplateNotFoundIsError(t *testing.T) {
	m, _, _, _ := newTestManager()
	if _, err := m.SaveValues("ghost.yaml", SavePayload{Datafile: "x.meta.json"}); err == nil {
		t.Errorf("expected error for missing template")
	}
}

// ─────────────────────────────────────────────────────────────────────
// DeleteForm + ListForms (passthroughs)
// ─────────────────────────────────────────────────────────────────────

func TestDeleteForm_PassesThrough(t *testing.T) {
	m, _, store, _ := newTestManager()
	store.forms["t.yaml"] = map[string]*storage.Form{
		"row.meta.json": {Data: map[string]any{}},
	}
	if err := m.DeleteForm("t.yaml", "row.meta.json"); err != nil {
		t.Fatalf("DeleteForm: %v", err)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "t.yaml/row.meta.json" {
		t.Errorf("delete not propagated: %+v", store.deleted)
	}
}

func TestListForms_PassesThrough(t *testing.T) {
	m, _, store, _ := newTestManager()
	store.summaries["t.yaml"] = []storage.FormSummary{
		{Filename: "a.meta.json"},
		{Filename: "b.meta.json"},
	}
	got, err := m.ListForms("t.yaml")
	if err != nil {
		t.Fatalf("ListForms: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 summaries, got %d", len(got))
	}
}

// realStack wires system -> sfr -> storage -> form on a tempdir and also
// hands back the storage manager + tempdir so tests can inspect on-disk
// files. Distinct from round_trip_test.go's newRealStack, which returns
// only (form, template). Do not collapse the two: this one exposes paths.
func realStack(t *testing.T) (*Manager, *template.Manager, *storage.Manager, string) {
	t.Helper()
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	tplM := template.NewManager(sys, "templates", nil)
	sfrM := sfr.NewManager(sys, nil)
	stoM := storage.NewManager(sys, sfrM, tplM, "storage", nil)
	formM := NewManager(tplM, stoM, nil, nil)
	return formM, tplM, stoM, root
}

// formPath returns the absolute on-disk path of a saved form file.
func formPath(root, templateFilename, datafile string) string {
	stem := templateFilename[:len(templateFilename)-len(filepath.Ext(templateFilename))]
	return filepath.Join(root, "storage", stem, datafile)
}

// ─────────────────────────────────────────────────────────────────────
// Malformed form JSON on read: LoadForm returns nil, BuildView falls
// back to a defaults-filled unsaved view rather than erroring.
// ─────────────────────────────────────────────────────────────────────

func TestBuildView_MalformedJSONReadsAsUnsaved(t *testing.T) {
	m, tplM, _, root := realStack(t)
	if err := tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text", Default: "D"}},
	}); err != nil {
		t.Fatalf("save template: %v", err)
	}
	// Write garbage into the form's slot so the JSON decode fails.
	path := formPath(root, "t.yaml", "broken.meta.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not: valid json,,,"), 0o644); err != nil {
		t.Fatalf("write garbage: %v", err)
	}

	view, err := m.BuildView("t.yaml", "broken.meta.json")
	if err != nil {
		t.Fatalf("BuildView on malformed file should not error: %v", err)
	}
	if view.Saved {
		t.Errorf("malformed file should yield Saved=false, got true")
	}
	if got := view.Values["title"]; got != "D" {
		t.Errorf("default not injected on malformed read: want %q, got %v", "D", got)
	}
	if view.Datafile != "broken.meta.json" {
		t.Errorf("datafile lost: want %q, got %q", "broken.meta.json", view.Datafile)
	}
}

// A non-array JSON top level (valid JSON, wrong shape) is also tolerated:
// LoadForm requires a map, anything else reads as nil -> unsaved.
func TestBuildView_NonObjectJSONReadsAsUnsaved(t *testing.T) {
	m, tplM, _, root := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "n", Type: "number", Default: 7}},
	})
	path := formPath(root, "t.yaml", "arr.meta.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("[1,2,3]"), 0o644); err != nil {
		t.Fatalf("write array: %v", err)
	}
	view, err := m.BuildView("t.yaml", "arr.meta.json")
	if err != nil {
		t.Fatalf("BuildView: %v", err)
	}
	if view.Saved {
		t.Errorf("array-shaped JSON should be Saved=false")
	}
	if got := view.Values["n"]; got != 7 {
		t.Errorf("number default not injected: want 7, got %v", got)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Missing file: never written, BuildView synthesizes an unsaved view.
// ─────────────────────────────────────────────────────────────────────

func TestBuildView_MissingFileIsUnsavedWithDefaults(t *testing.T) {
	m, tplM, _, root := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text", Default: "Z"}},
	})
	// Confirm the file genuinely does not exist before the call.
	path := formPath(root, "t.yaml", "ghost.meta.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("precondition: file should not exist, stat err=%v", err)
	}
	view, err := m.BuildView("t.yaml", "ghost.meta.json")
	if err != nil {
		t.Fatalf("BuildView missing file should not error: %v", err)
	}
	if view.Saved {
		t.Errorf("missing file should be Saved=false")
	}
	if got := view.Values["title"]; got != "Z" {
		t.Errorf("default not injected: want %q, got %v", "Z", got)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Save-no-collateral-damage: saving one form must not touch a sibling
// form's file (mtime + bytes unchanged).
// ─────────────────────────────────────────────────────────────────────

func TestSaveValues_DoesNotTouchSiblingForm(t *testing.T) {
	m, tplM, _, root := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})

	// Save form B first, then snapshot its file.
	if _, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "b.meta.json",
		Values:   map[string]any{"title": "Bee"},
	}); err != nil {
		t.Fatalf("save B: %v", err)
	}
	bPath := formPath(root, "t.yaml", "b.meta.json")
	bInfo, err := os.Stat(bPath)
	if err != nil {
		t.Fatalf("stat B: %v", err)
	}
	bBytes, err := os.ReadFile(bPath)
	if err != nil {
		t.Fatalf("read B: %v", err)
	}
	bMtime := bInfo.ModTime()

	// Ensure the clock advances so an accidental rewrite would change mtime.
	time.Sleep(20 * time.Millisecond)

	// Now save form A twice (create then update).
	if _, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "a.meta.json",
		Values:   map[string]any{"title": "Aye"},
	}); err != nil {
		t.Fatalf("save A create: %v", err)
	}
	if _, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "a.meta.json",
		Values:   map[string]any{"title": "Aye-2"},
	}); err != nil {
		t.Fatalf("save A update: %v", err)
	}

	// B must be byte-for-byte and mtime identical: untouched.
	afterInfo, err := os.Stat(bPath)
	if err != nil {
		t.Fatalf("re-stat B: %v", err)
	}
	if !afterInfo.ModTime().Equal(bMtime) {
		t.Errorf("sibling B mtime changed: before=%v after=%v", bMtime, afterInfo.ModTime())
	}
	afterBytes, err := os.ReadFile(bPath)
	if err != nil {
		t.Fatalf("re-read B: %v", err)
	}
	if string(afterBytes) != string(bBytes) {
		t.Errorf("sibling B content changed.\nbefore:\n%s\nafter:\n%s", bBytes, afterBytes)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Concurrent saves under -race: distinct datafiles in parallel all land,
// each readable with its own value, no data races.
// ─────────────────────────────────────────────────────────────────────

func TestSaveValues_ConcurrentDistinctFiles(t *testing.T) {
	m, tplM, _, _ := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})

	const n = 12
	want := make(map[string]string, n)
	var wg sync.WaitGroup
	for i := range n {
		df := "row" + string(rune('a'+i)) + ".meta.json"
		val := "v" + string(rune('a'+i))
		want[df] = val
		wg.Add(1)
		go func(df, val string) {
			defer wg.Done()
			if _, err := m.SaveValues("t.yaml", SavePayload{
				Datafile: df,
				Values:   map[string]any{"title": val},
			}); err != nil {
				t.Errorf("concurrent save %s: %v", df, err)
			}
		}(df, val)
	}
	wg.Wait()

	// Every file must be present with its own value, no cross-contamination.
	for df, val := range want {
		view, err := m.BuildView("t.yaml", df)
		if err != nil {
			t.Fatalf("build %s: %v", df, err)
		}
		if !view.Saved {
			t.Errorf("%s not saved", df)
		}
		if got := view.Values["title"]; got != val {
			t.Errorf("%s title: want %q, got %v", df, val, got)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────
// Bad / out-of-range facet selections.
//
// The form + storage layers do NOT validate a facet selection against
// the template's declared option set, nor that the facet key exists.
// These tests pin the ACTUAL current behavior: the selection round-trips
// verbatim. See suspectedBugs in the report.
// ─────────────────────────────────────────────────────────────────────

// A selection that is no longer a declared option (and the facet has no field
// default) is cleared to empty on save, while the facet stays Set.
func TestSaveValues_OutOfRangeFacetSelectionClearedToEmpty(t *testing.T) {
	m, tplM, _, _ := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Facets: []template.Facet{{
			Key:  "status",
			Icon: "fa-flag",
			Options: []template.FacetOption{
				{Label: "OPEN", Color: "green"},
				{Label: "CLOSED", Color: "red"},
			},
		}},
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})

	res, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "f.meta.json",
		Values:   map[string]any{"title": "x"},
		Meta: storage.FormMeta{
			Facets: map[string]storage.FacetState{
				"status": {Set: true, Selected: "bogus"},
			},
		},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok := res.Meta.Facets["status"]
	if !ok {
		t.Fatalf("declared facet should remain; facets=%+v", res.Meta.Facets)
	}
	if !got.Set || got.Selected != "" {
		t.Errorf("out-of-range selection should clear to empty (Set stays true), got %+v", got)
	}
	view, err := m.BuildView("t.yaml", "f.meta.json")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if s := view.Meta.Facets["status"]; s.Selected != "" {
		t.Errorf("disk kept invalid selection: %+v", s)
	}
}

// When the facet field declares a default, an out-of-range selection falls back
// to that default rather than emptying.
func TestSaveValues_OutOfRangeFacetSelectionUsesDefault(t *testing.T) {
	m, tplM, _, _ := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Facets: []template.Facet{{
			Key:  "status",
			Icon: "fa-flag",
			Options: []template.FacetOption{
				{Label: "OPEN", Color: "green"},
				{Label: "CLOSED", Color: "red"},
			},
		}},
		Fields: []template.Field{
			{Key: "title", Type: "text"},
			{Key: "status-field", Type: "facet", FacetKey: "status", Format: "radio", Default: "OPEN"},
		},
	})

	res, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "f.meta.json",
		Values:   map[string]any{"title": "x"},
		Meta: storage.FormMeta{
			Facets: map[string]storage.FacetState{
				"status": {Set: true, Selected: "bogus"},
			},
		},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := res.Meta.Facets["status"]; !got.Set || got.Selected != "OPEN" {
		t.Errorf("out-of-range should fall back to default OPEN, got %+v", got)
	}
}

// A facet key the template does not declare is dropped on save (meta synced to
// the template's declared keys).
func TestSaveValues_UndeclaredFacetKeyDropped(t *testing.T) {
	m, tplM, _, _ := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})

	res, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "f.meta.json",
		Values:   map[string]any{"title": "x"},
		Meta: storage.FormMeta{
			Facets: map[string]storage.FacetState{
				"ghostkey": {Set: true, Selected: "whatever"},
			},
		},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, ok := res.Meta.Facets["ghostkey"]; ok {
		t.Errorf("undeclared facet key should be dropped, got %+v", res.Meta.Facets)
	}
}

// An empty Selected with Set=true is stamped-but-unselected: the canonical
// boundary between "facet touched" and "facet has a value". It must round
// trip as {Set:true, Selected:""} rather than collapsing to absent.
func TestSaveValues_FacetSetButEmptySelectionRoundTrips(t *testing.T) {
	m, tplM, _, _ := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Facets: []template.Facet{{
			Key:     "status",
			Icon:    "fa-flag",
			Options: []template.FacetOption{{Label: "OPEN", Color: "green"}},
		}},
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})
	res, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "f.meta.json",
		Values:   map[string]any{"title": "x"},
		Meta: storage.FormMeta{
			Facets: map[string]storage.FacetState{
				"status": {Set: true, Selected: ""},
			},
		},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok := res.Meta.Facets["status"]
	if !ok {
		t.Fatalf("set-but-empty facet dropped; facets=%+v", res.Meta.Facets)
	}
	if !got.Set || got.Selected != "" {
		t.Errorf("set-but-empty facet changed: want {Set:true Selected:\"\"}, got %+v", got)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Path traversal: a datafile escaping the template dir must be rejected
// by the storage guard, surfaced as a SaveValues error, and no file may
// land outside the storage subtree.
// ─────────────────────────────────────────────────────────────────────

func TestSaveValues_TraversalDatafileIsRejected(t *testing.T) {
	m, tplM, _, root := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})

	_, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "../escape.meta.json",
		Values:   map[string]any{"title": "x"},
	})
	if err == nil {
		t.Fatalf("traversal datafile must error")
	}
	if !strings.Contains(err.Error(), "invalid datafile") {
		t.Errorf("want invalid-datafile error, got %q", err.Error())
	}
	// The escape target must not exist anywhere near the storage root.
	if _, statErr := os.Stat(root + "/escape.meta.json"); !os.IsNotExist(statErr) {
		t.Errorf("escape file should not exist, stat err=%v", statErr)
	}
}

func TestSaveValues_SlashDatafileIsRejected(t *testing.T) {
	m, tplM, _, _ := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})
	_, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "sub/dir.meta.json",
		Values:   map[string]any{"title": "x"},
	})
	if err == nil {
		t.Fatalf("datafile with slash must error")
	}
	if !strings.Contains(err.Error(), "invalid datafile") {
		t.Errorf("want invalid-datafile error, got %q", err.Error())
	}
}

// ─────────────────────────────────────────────────────────────────────
// Nil Values payload still persists: storage injects type-defaults so a
// declared text field reads back as "" and the form is Saved.
// ─────────────────────────────────────────────────────────────────────

func TestSaveValues_NilValuesPersistsWithFieldDefaults(t *testing.T) {
	m, tplM, _, _ := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})
	view, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "e.meta.json",
		Values:   nil,
	})
	if err != nil {
		t.Fatalf("save nil values: %v", err)
	}
	if !view.Saved {
		t.Errorf("nil-values save should be Saved=true")
	}
	if got := view.Values["title"]; got != "" {
		t.Errorf("declared text field should default to empty string, got %v (%T)", got, got)
	}
	// Re-read independently from disk.
	again, err := m.BuildView("t.yaml", "e.meta.json")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !again.Saved || again.Values["title"] != "" {
		t.Errorf("disk read mismatch: saved=%v title=%v", again.Saved, again.Values["title"])
	}
}

// ─────────────────────────────────────────────────────────────────────
// Audit stamping: with no AuthorProvider wired, every save stamps the
// fallback identity on Created + Updated and a non-empty timestamp.
// ─────────────────────────────────────────────────────────────────────

func TestSaveValues_StampsFallbackIdentity(t *testing.T) {
	m, tplM, _, _ := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})
	view, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "f.meta.json",
		Values:   map[string]any{"title": "x"},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if view.Meta.Created.Name != "Unknown" {
		t.Errorf("created name: want Unknown, got %q", view.Meta.Created.Name)
	}
	if view.Meta.Created.Email != "unknown@example.com" {
		t.Errorf("created email: want unknown@example.com, got %q", view.Meta.Created.Email)
	}
	if view.Meta.Updated.Name != "Unknown" {
		t.Errorf("updated name: want Unknown, got %q", view.Meta.Updated.Name)
	}
	if view.Meta.Created.At == "" {
		t.Errorf("created timestamp must be stamped")
	}
	if view.Meta.Updated.At == "" {
		t.Errorf("updated timestamp must be stamped")
	}
}

// ─────────────────────────────────────────────────────────────────────
// Valid declared facet selection round-trips with the exact label, both
// in the returned view and on a fresh disk read.
// ─────────────────────────────────────────────────────────────────────

func TestSaveValues_ValidFacetSelectionRoundTrips(t *testing.T) {
	m, tplM, _, _ := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Facets: []template.Facet{{
			Key:  "status",
			Icon: "fa-flag",
			Options: []template.FacetOption{
				{Label: "OPEN", Color: "green"},
				{Label: "CLOSED", Color: "red"},
			},
		}},
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})
	res, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "f.meta.json",
		Values:   map[string]any{"title": "x"},
		Meta: storage.FormMeta{
			Facets: map[string]storage.FacetState{
				"status": {Set: true, Selected: "CLOSED"},
			},
		},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	got := res.Meta.Facets["status"]
	if !got.Set || got.Selected != "CLOSED" {
		t.Errorf("valid facet not preserved: want {Set:true Selected:CLOSED}, got %+v", got)
	}
	view, err := m.BuildView("t.yaml", "f.meta.json")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if s := view.Meta.Facets["status"]; !s.Set || s.Selected != "CLOSED" {
		t.Errorf("disk read lost facet: %+v", s)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Concurrent saves to the SAME datafile under -race: atomic writes mean
// the last writer wins and the file is never half-written. After the
// storm, the file is valid JSON, readable, and holds one of the values.
// ─────────────────────────────────────────────────────────────────────

func TestSaveValues_ConcurrentSameFileNoCorruption(t *testing.T) {
	m, tplM, _, root := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})

	const n = 20
	wanted := make(map[string]bool, n)
	var wg sync.WaitGroup
	for i := range n {
		val := "v" + string(rune('a'+i))
		wanted[val] = true
		wg.Add(1)
		go func(val string) {
			defer wg.Done()
			if _, err := m.SaveValues("t.yaml", SavePayload{
				Datafile: "same.meta.json",
				Values:   map[string]any{"title": val},
			}); err != nil {
				t.Errorf("concurrent same-file save %s: %v", val, err)
			}
		}(val)
	}
	wg.Wait()

	// The on-disk bytes must parse as JSON (no torn write).
	path := formPath(root, "t.yaml", "same.meta.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read same: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("on-disk JSON is corrupt: %v\n%s", err, raw)
	}

	// The surviving value must be exactly one of the writers' values.
	view, err := m.BuildView("t.yaml", "same.meta.json")
	if err != nil {
		t.Fatalf("build same: %v", err)
	}
	if !view.Saved {
		t.Errorf("same-file form should be Saved=true")
	}
	got, _ := view.Values["title"].(string)
	if !wanted[got] {
		t.Errorf("surviving title %q is not one of the written values", got)
	}
}

// ─────────────────────────────────────────────────────────────────────
// DeleteForm: empty datafile is a hard error; deleting a never-written
// file is a tolerated no-op (mirrors "missing reads as unsaved").
// ─────────────────────────────────────────────────────────────────────

func TestDeleteForm_EmptyDatafileIsError(t *testing.T) {
	m, _, _, _ := realStack(t)
	err := m.DeleteForm("t.yaml", "")
	if err == nil {
		t.Fatalf("empty datafile must error")
	}
	if !strings.Contains(err.Error(), "empty datafile") {
		t.Errorf("want empty-datafile error, got %q", err.Error())
	}
}

func TestDeleteForm_MissingFileIsNoOp(t *testing.T) {
	m, tplM, _, _ := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})
	if err := m.DeleteForm("t.yaml", "ghost.meta.json"); err != nil {
		t.Errorf("delete of missing file should be a no-op, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Type-default injection for the array-shaped and range field types on
// an unsaved view, plus a per-field Default override winning over the
// type-default.
// ─────────────────────────────────────────────────────────────────────

func TestBuildView_TypeDefaultsForArrayAndRange(t *testing.T) {
	m, tplM, _, _ := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{
			{Key: "tags", Type: "multioption"},
			{Key: "level", Type: "range"},
			{Key: "picks", Type: "list"},
		},
	})
	view, err := m.BuildView("t.yaml", "")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if got, ok := view.Values["tags"].([]any); !ok || len(got) != 0 {
		t.Errorf("multioption default: want empty []any, got %#v", view.Values["tags"])
	}
	if got := view.Values["level"]; got != 50 {
		t.Errorf("range default: want 50, got %v (%T)", got, got)
	}
	if got, ok := view.Values["picks"].([]any); !ok || len(got) != 0 {
		t.Errorf("list default: want empty []any, got %#v", view.Values["picks"])
	}
}

// ─────────────────────────────────────────────────────────────────────
// Save then overwrite the same file: the second save updates the value
// and bumps Updated, but Created identity/time stays the lifetime value.
// ─────────────────────────────────────────────────────────────────────

func TestSaveValues_ResaveKeepsCreatedBumpsValue(t *testing.T) {
	m, tplM, _, _ := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})
	first, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "f.meta.json",
		Values:   map[string]any{"title": "one"},
	})
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	createdAt := first.Meta.Created.At
	id := first.Meta.ID

	second, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "f.meta.json",
		Values:   map[string]any{"title": "two"},
	})
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if got := second.Values["title"]; got != "two" {
		t.Errorf("resave value: want two, got %v", got)
	}
	if second.Meta.Created.At != createdAt {
		t.Errorf("created timestamp must not change on resave: %q -> %q", createdAt, second.Meta.Created.At)
	}
	if id != "" && second.Meta.ID != id {
		t.Errorf("id must be stable across resave: %q -> %q", id, second.Meta.ID)
	}
}
