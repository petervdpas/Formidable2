package integrity

import (
	"context"
	"maps"
	"sort"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// sanitizingStore reproduces production's hazard: LoadForm sanitizes the raw
// data against the template (dropping any top-level key it no longer declares),
// while LoadFormRaw returns the on-disk data verbatim. The migration must read
// raw or a top-level renamed key is invisible.
type sanitizingStore struct {
	fields []template.Field
	raw    map[string]map[string]any
	saved  map[string]*storage.Form
}

func (s *sanitizingStore) ListForms(string) ([]string, error) {
	out := []string{}
	for fn := range s.raw {
		out = append(out, fn)
	}
	sort.Strings(out)
	return out, nil
}

func (s *sanitizingStore) LoadForm(_, fn string) *storage.Form {
	d, ok := s.raw[fn]
	if !ok {
		return nil
	}
	out := storage.Sanitize(cloneRawMap(d), s.fields, storage.SanitizeOptions{})
	return &out
}

func (s *sanitizingStore) LoadFormRaw(_, fn string) *storage.Form {
	d, ok := s.raw[fn]
	if !ok {
		return nil
	}
	return &storage.Form{Data: cloneRawMap(d)}
}

func (s *sanitizingStore) SaveForm(_ context.Context, _, fn string, f *storage.Form) error {
	if s.saved == nil {
		s.saved = map[string]*storage.Form{}
	}
	s.saved[fn] = f
	return nil
}

func cloneRawMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	maps.Copy(out, m)
	return out
}

func newSanitizingManager(t *testing.T, fields []template.Field, raw map[string]map[string]any) (*Manager, *sanitizingStore) {
	t.Helper()
	tpl := &template.Template{Name: "a", Filename: "a.yaml", Fields: fields}
	st := &stubTemplates{ts: map[string]*template.Template{"a.yaml": tpl}}
	store := &sanitizingStore{fields: fields, raw: raw, saved: map[string]*storage.Form{}}
	m := NewManager(st, store)
	m.SetWriter(store)
	return m, store
}

// The bug behind "0 moved": a sanitized load drops a top-level renamed key, so
// the move must read raw. This locks that fix.
func TestMigrate_TopLevelRename_SurvivesSanitizeViaRawRead(t *testing.T) {
	fields := []template.Field{
		{Key: "audit-control-identifier", Type: "text"},
		{Key: "naam", Type: "text"},
	}
	m, store := newSanitizingManager(t, fields, map[string]map[string]any{
		"r1.meta.json": {"audit-control-id": "CH.02", "naam": "X"},
	})

	// Guard: a plain sanitized load really does drop the orphan, proving the hazard.
	if _, present := store.LoadForm("a.yaml", "r1.meta.json").Data["audit-control-id"]; present {
		t.Fatal("expected Sanitize to drop the orphaned top-level key")
	}

	res, err := m.MigrateFieldKey("a.yaml", "audit-control-id", "audit-control-identifier")
	if err != nil {
		t.Fatal(err)
	}
	if res.KeysMoved != 1 || res.FormsSaved != 1 {
		t.Fatalf("KeysMoved=%d FormsSaved=%d; want 1/1: %+v", res.KeysMoved, res.FormsSaved, res)
	}
	saved := store.saved["r1.meta.json"]
	if saved == nil || saved.Data["audit-control-identifier"] != "CH.02" {
		t.Fatalf("value not moved onto new key: %+v", saved)
	}
	if _, present := saved.Data["audit-control-id"]; present {
		t.Errorf("old key should be gone from saved form")
	}
}

func TestRenameCandidates_FindsTopLevelOrphanAndFieldKeys(t *testing.T) {
	fields := []template.Field{
		{Key: "audit-control-identifier", Type: "text"},
		{Key: "naam", Type: "text"},
	}
	m, _ := newSanitizingManager(t, fields, map[string]map[string]any{
		"r1.meta.json": {"audit-control-id": "CH.02", "naam": "X"},
	})

	cand, err := m.RenameCandidates("a.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(cand.OrphanKeys) != 1 || cand.OrphanKeys[0] != "audit-control-id" {
		t.Fatalf("orphan keys=%v; want [audit-control-id]", cand.OrphanKeys)
	}
	if got := cand.FieldKeys; len(got) != 2 || got[0] != "audit-control-identifier" || got[1] != "naam" {
		t.Fatalf("field keys=%v; want [audit-control-identifier naam]", got)
	}
}

func migrateForm(data map[string]any) *storage.Form {
	return &storage.Form{
		Meta: storage.FormMeta{
			Created: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
			Updated: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
		},
		Data: data,
	}
}

func TestMigrate_TopLevelRename_MovesValue(t *testing.T) {
	// Template renamed "title" -> "heading"; the form data still carries "title".
	tpl := &template.Template{
		Name: "Basic", Filename: "basic.yaml",
		Fields: []template.Field{
			{Key: "heading", Type: "text"},
			{Key: "count", Type: "number"},
		},
	}
	f := migrateForm(map[string]any{"title": "Hello", "count": float64(3)})
	h := newFixHarness(t, tpl, map[string]*storage.Form{"a.meta.json": f})

	res, err := h.m.MigrateFieldKey("basic.yaml", "title", "heading")
	if err != nil {
		t.Fatal(err)
	}
	if res.KeysMoved != 1 || res.FormsSaved != 1 {
		t.Fatalf("KeysMoved=%d FormsSaved=%d; want 1/1: %+v", res.KeysMoved, res.FormsSaved, res)
	}
	saved := h.loadSaved("a.meta.json")
	if saved.Data["heading"] != "Hello" {
		t.Errorf("heading=%v; want Hello", saved.Data["heading"])
	}
	if _, present := saved.Data["title"]; present {
		t.Errorf("old key title should be gone after migrate")
	}
}

func TestMigrate_LoopNestedRename_MovesEachOccurrence(t *testing.T) {
	// Inner loop field renamed: data items still carry "name", template now wants "label".
	tpl := &template.Template{
		Name: "Looped", Filename: "loop.yaml",
		Fields: []template.Field{
			{Key: "title", Type: "text"},
			{Key: "items", Type: "loopstart"},
			{Key: "label", Type: "text"},
			{Key: "qty", Type: "number"},
			{Key: "items", Type: "loopstop"},
		},
	}
	f := migrateForm(map[string]any{
		"title": "hi",
		"items": []any{
			map[string]any{"name": "a", "qty": float64(1)},
			map[string]any{"name": "b", "qty": float64(2)},
		},
	})
	h := newFixHarness(t, tpl, map[string]*storage.Form{"a.meta.json": f})

	res, err := h.m.MigrateFieldKey("loop.yaml", "name", "label")
	if err != nil {
		t.Fatal(err)
	}
	if res.KeysMoved != 2 || res.FormsSaved != 1 {
		t.Fatalf("KeysMoved=%d FormsSaved=%d; want 2/1: %+v", res.KeysMoved, res.FormsSaved, res)
	}
	items := h.loadSaved("a.meta.json").Data["items"].([]any)
	want := []string{"a", "b"}
	for i, raw := range items {
		m := raw.(map[string]any)
		if _, present := m["name"]; present {
			t.Errorf("item %d still has old key name: %v", i, m)
		}
		if m["label"] != want[i] {
			t.Errorf("item %d label=%v; want %q", i, m["label"], want[i])
		}
	}
}

func TestMigrate_LoopKeyRename_MovesWholeArray(t *testing.T) {
	// The loop key itself is renamed: data["items"] should move to data["rows"].
	tpl := &template.Template{
		Name: "Looped", Filename: "loop.yaml",
		Fields: []template.Field{
			{Key: "title", Type: "text"},
			{Key: "rows", Type: "loopstart"},
			{Key: "name", Type: "text"},
			{Key: "rows", Type: "loopstop"},
		},
	}
	f := migrateForm(map[string]any{
		"title": "hi",
		"items": []any{map[string]any{"name": "a"}},
	})
	h := newFixHarness(t, tpl, map[string]*storage.Form{"a.meta.json": f})

	res, err := h.m.MigrateFieldKey("loop.yaml", "items", "rows")
	if err != nil {
		t.Fatal(err)
	}
	if res.KeysMoved != 1 || res.FormsSaved != 1 {
		t.Fatalf("KeysMoved=%d FormsSaved=%d; want 1/1: %+v", res.KeysMoved, res.FormsSaved, res)
	}
	saved := h.loadSaved("a.meta.json")
	if _, present := saved.Data["items"]; present {
		t.Errorf("old loop key items should be gone")
	}
	rows, ok := saved.Data["rows"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("rows=%v; want a 1-item array", saved.Data["rows"])
	}
}

func TestMigrate_NoOccurrences_NoWrite(t *testing.T) {
	// Neither key appears in the data, so nothing is moved and nothing is saved.
	h := newFixHarness(t, tplBasic(), map[string]*storage.Form{"a.meta.json": cleanForm()})

	res, err := h.m.MigrateFieldKey("basic.yaml", "ghost", "phantom")
	if err != nil {
		t.Fatal(err)
	}
	if res.KeysMoved != 0 || res.FormsSaved != 0 || res.FormsTouched != 0 {
		t.Fatalf("want a no-op; got %+v", res)
	}
	if h.wr.calls != 0 {
		t.Errorf("writer called %d times; want 0 on a no-op migrate", h.wr.calls)
	}
}

func TestMigrate_Idempotent_SecondRunIsNoOp(t *testing.T) {
	tpl := &template.Template{
		Name: "Basic", Filename: "basic.yaml",
		Fields: []template.Field{{Key: "heading", Type: "text"}},
	}
	f := migrateForm(map[string]any{"title": "Hello"})
	h := newFixHarness(t, tpl, map[string]*storage.Form{"a.meta.json": f})

	if _, err := h.m.MigrateFieldKey("basic.yaml", "title", "heading"); err != nil {
		t.Fatal(err)
	}
	res, err := h.m.MigrateFieldKey("basic.yaml", "title", "heading")
	if err != nil {
		t.Fatal(err)
	}
	if res.KeysMoved != 0 || res.FormsSaved != 0 {
		t.Fatalf("second run not idempotent: %+v", res)
	}
}

func TestMigrate_IdenticalKeys_Errors(t *testing.T) {
	h := newFixHarness(t, tplBasic(), map[string]*storage.Form{"a.meta.json": cleanForm()})
	if _, err := h.m.MigrateFieldKey("basic.yaml", "title", "title"); err == nil {
		t.Fatal("expected error when old and new key are identical")
	}
}

func TestMigrate_EmptyKey_Errors(t *testing.T) {
	h := newFixHarness(t, tplBasic(), map[string]*storage.Form{"a.meta.json": cleanForm()})
	if _, err := h.m.MigrateFieldKey("basic.yaml", "", "heading"); err == nil {
		t.Fatal("expected error on empty old key")
	}
	if _, err := h.m.MigrateFieldKey("basic.yaml", "title", "  "); err == nil {
		t.Fatal("expected error on blank new key")
	}
}

func TestMigrate_NoWriterConfigured_Errors(t *testing.T) {
	tpl := tplBasic()
	st := &stubTemplates{ts: map[string]*template.Template{tpl.Filename: tpl}}
	so := &stubStorage{forms: map[string]map[string]*storage.Form{
		tpl.Filename: {"a.meta.json": cleanForm()},
	}}
	m := NewManager(st, so) // no SetWriter

	if _, err := m.MigrateFieldKey(tpl.Filename, "title", "heading"); err == nil {
		t.Fatal("expected error when writer is unconfigured")
	}
}

func TestService_MigrateFieldKey_EmitsStorageChangedOnWrite(t *testing.T) {
	tpl := &template.Template{
		Name: "Basic", Filename: "basic.yaml",
		Fields: []template.Field{{Key: "heading", Type: "text"}},
	}
	f := migrateForm(map[string]any{"title": "Hello"})
	h := newFixHarness(t, tpl, map[string]*storage.Form{"a.meta.json": f})

	fe := &recordingEmitter{}
	res, err := NewService(h.m, fe).MigrateFieldKey(tpl.Filename, "title", "heading")
	if err != nil {
		t.Fatal(err)
	}
	if res.FormsSaved != 1 {
		t.Fatalf("FormsSaved=%d; want 1", res.FormsSaved)
	}
	if len(fe.names) != 1 || fe.names[0] != "storage:changed" || fe.data[0] != tpl.Filename {
		t.Errorf("emitted %v / %v; want one storage:changed for %s", fe.names, fe.data, tpl.Filename)
	}
}

func TestService_MigrateFieldKey_NoEmitWhenNothingSaved(t *testing.T) {
	h := newFixHarness(t, tplBasic(), map[string]*storage.Form{"a.meta.json": cleanForm()})

	fe := &recordingEmitter{}
	if _, err := NewService(h.m, fe).MigrateFieldKey(h.tpl.Filename, "ghost", "phantom"); err != nil {
		t.Fatal(err)
	}
	if len(fe.names) != 0 {
		t.Errorf("emitted %v; want nothing on a no-op migrate", fe.names)
	}
}
