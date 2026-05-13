package form

import (
	"errors"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// ─────────────────────────────────────────────────────────────────────
// Test doubles. The form Manager talks to three narrow interfaces
// (templateLoader, formStore, configReader) — easy to fake in-process
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

func (s *fakeStorage) SaveForm(t, df string, data map[string]any) storage.SaveResult {
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
			meta.Flagged = v
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

type fakeConfig struct {
	loopCollapsed bool
}

func (c *fakeConfig) FormDefaults() ConfigDefaults {
	return ConfigDefaults{
		LoopStateCollapsed: c.loopCollapsed,
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
