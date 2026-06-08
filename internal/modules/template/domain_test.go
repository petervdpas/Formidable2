package template

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/system"
)

func newTestManager(t *testing.T) (*Manager, *system.Manager, string) {
	t.Helper()
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	return NewManager(sys, "templates", nil), sys, root
}

// failingFS wraps a real fs but forces SaveFile, DeleteFile, or ListFiles
// to error, so the manager's write/delete/list error branches are reachable
// without a flaky real filesystem. A nil error field passes through to the
// wrapped fs.
type failingFS struct {
	inner   fs
	saveErr error
	delErr  error
	listErr error
}

func (f *failingFS) ResolvePath(segments ...string) string { return f.inner.ResolvePath(segments...) }
func (f *failingFS) JoinPath(segments ...string) string    { return f.inner.JoinPath(segments...) }
func (f *failingFS) EnsureDirectory(path string) error     { return f.inner.EnsureDirectory(path) }
func (f *failingFS) FileExists(path string) bool           { return f.inner.FileExists(path) }
func (f *failingFS) LoadFile(path string) (string, error)  { return f.inner.LoadFile(path) }

func (f *failingFS) SaveFile(path string, content string) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	return f.inner.SaveFile(path, content)
}

func (f *failingFS) DeleteFile(path string) error {
	if f.delErr != nil {
		return f.delErr
	}
	return f.inner.DeleteFile(path)
}

func (f *failingFS) ListFiles(dir string) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.inner.ListFiles(dir)
}

// newFailingManager builds a Manager over a failingFS wrapping a real
// temp-dir system manager, returning both so tests can flip the error flags.
func newFailingManager(t *testing.T) (*Manager, *failingFS) {
	t.Helper()
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	ff := &failingFS{inner: sys}
	return NewManager(ff, "templates", nil), ff
}

// writeRaw drops a raw YAML body into the manager's templates dir, bypassing
// SaveTemplate so malformed/invalid shapes reach LoadTemplate untouched.
func writeRaw(t *testing.T, m *Manager, name, body string) {
	t.Helper()
	if err := m.EnsureTemplateDirectory(); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}
	if err := m.fs.SaveFile(m.fs.JoinPath(m.templatesDir, name), body); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Validation edge cases
// ─────────────────────────────────────────────────────────────────────

func TestValidate_NilTemplateReportsInvalid(t *testing.T) {
	errs := Validate(nil)
	if len(errs) != 1 || errs[0].Type != "invalid-template" {
		t.Errorf("expected invalid-template, got %+v", errs)
	}
}

func TestValidate_FieldsNilIsInvalid(t *testing.T) {
	errs := Validate(&Template{Name: "X"})
	if len(errs) != 1 || errs[0].Type != "invalid-template" {
		t.Errorf("expected invalid-template for nil Fields, got %+v", errs)
	}
}

func TestValidate_PrimaryKeyMultipleFlagged(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "a", Type: "text", PrimaryKey: true},
			{Key: "b", Type: "text", PrimaryKey: true},
		},
	})
	found := false
	for _, e := range errs {
		if e.Type == "multiple-primary-keys" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected multiple-primary-keys; got %+v", errs)
	}
}

func TestValidate_ApiMapKeyRequired(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "ref", Type: "api", Collection: "x", Map: []APIMap{{Key: ""}}},
		},
	})
	found := false
	for _, e := range errs {
		if e.Type == "api-map-key-required" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected api-map-key-required; got %+v", errs)
	}
}

func TestValidate_ApiMapDuplicateKeysCaseInsensitive(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "ref", Type: "api", Collection: "x",
				Map: []APIMap{{Key: "Name"}, {Key: "name"}}},
		},
	})
	found := false
	for _, e := range errs {
		if e.Type == "api-map-duplicate-keys" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected api-map-duplicate-keys; got %+v", errs)
	}
}

func TestValidate_ApiFilterFieldRequired(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "ref", Type: "api", Collection: "x",
				Filter: &APIFilter{FieldKey: "", Op: "eq", Value: "a"}},
		},
	})
	found := false
	for _, e := range errs {
		if e.Type == "api-filter-field-required" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected api-filter-field-required; got %+v", errs)
	}
}

func TestValidate_ApiFilterOpInvalid(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "ref", Type: "api", Collection: "x",
				Filter: &APIFilter{FieldKey: "status", Op: "contains", Value: "a"}},
		},
	})
	found := false
	for _, e := range errs {
		if e.Type == "api-filter-op-invalid" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected api-filter-op-invalid; got %+v", errs)
	}
}

func TestValidate_ApiFilterValidPasses(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "ref", Type: "api", Collection: "x",
				Filter: &APIFilter{FieldKey: "amount", Op: "ge", Value: "100"}},
		},
	})
	for _, e := range errs {
		if e.Type == "api-filter-field-required" || e.Type == "api-filter-op-invalid" {
			t.Errorf("valid filter flagged: %+v", e)
		}
	}
}

func TestValidate_ApiFilterForbiddenOnNonApi(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "t", Type: "text", Filter: &APIFilter{FieldKey: "x", Op: "eq"}},
		},
	})
	found := false
	for _, e := range errs {
		if e.Type == "forbidden-attribute" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected forbidden-attribute for filter on non-api; got %+v", errs)
	}
}

func TestValidate_MultipleGuidFieldsFlagged(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "id", Type: "guid"},
			{Key: "alt", Type: "guid"},
		},
	})
	var got *ValidationError
	for i := range errs {
		if errs[i].Type == "multiple-guid-fields" {
			got = &errs[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("expected multiple-guid-fields; got %+v", errs)
	}
	if len(got.Keys) != 2 || got.Keys[0] != "id" || got.Keys[1] != "alt" {
		t.Errorf("expected keys [id alt]; got %v", got.Keys)
	}
	if !strings.Contains(got.Message, "id") || !strings.Contains(got.Message, "alt") {
		t.Errorf("message should mention both keys; got %q", got.Message)
	}
}

func TestValidate_SingleGuidFieldIsFine(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "id", Type: "guid"},
			{Key: "title", Type: "text"},
		},
	})
	for _, e := range errs {
		if e.Type == "multiple-guid-fields" {
			t.Errorf("single guid should not flag; got %+v", errs)
		}
	}
}

func TestValidate_NoGuidFieldIsFine(t *testing.T) {
	// missing-guid-for-collection only fires when EnableCollection is true;
	// a plain template with zero guids must be silent.
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "title", Type: "text"},
		},
	})
	for _, e := range errs {
		if e.Type == "multiple-guid-fields" {
			t.Errorf("no guid should not flag multiple-guid-fields; got %+v", errs)
		}
	}
}

func TestValidate_MultipleGuidWithEmptyKeyUsesPlaceholder(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "", Type: "guid"},
			{Key: "id", Type: "guid"},
		},
	})
	var got *ValidationError
	for i := range errs {
		if errs[i].Type == "multiple-guid-fields" {
			got = &errs[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("expected multiple-guid-fields; got %+v", errs)
	}
	if len(got.Keys) != 2 || got.Keys[0] != "(no key)" {
		t.Errorf("empty key should render as placeholder; got %v", got.Keys)
	}
}

func TestValidate_NestedLoopsAtMaxDepthAreFine(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "outer", Type: "loopstart"},
			{Key: "inner", Type: "loopstart"},
			{Key: "x", Type: "text"},
			{Key: "inner", Type: "loopstop"},
			{Key: "outer", Type: "loopstop"},
		},
	})
	for _, e := range errs {
		if e.Type == "excessive-loop-nesting" {
			t.Errorf("depth-2 should not error; got %+v", errs)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────
// CRUD edge cases
// ─────────────────────────────────────────────────────────────────────

func TestLoadTemplate_RejectsEmptyName(t *testing.T) {
	m, _, _ := newTestManager(t)
	if _, err := m.LoadTemplate(""); err == nil {
		t.Fatal("expected error for empty name")
	}
}

// LoadTemplate caches the parsed result so a 50-row sidebar mount
// doesn't trigger 50 disk reads + 50 yaml.Unmarshal calls. We prove the
// cache hit by bypassing SaveTemplate (which invalidates) and rewriting
// the file directly through the system manager - a real cache must
// ignore the change until invalidated.
func TestLoadTemplate_HitsCacheUntilInvalidated(t *testing.T) {
	m, sys, _ := newTestManager(t)
	if err := m.SaveTemplate("x.yaml", &Template{
		Name:   "First",
		Fields: []Field{{Key: "a", Type: "text"}},
	}); err != nil {
		t.Fatal(err)
	}
	first, err := m.LoadTemplate("x.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if first.Name != "First" {
		t.Fatalf("first load name = %q, want First", first.Name)
	}

	if err := sys.SaveFile("templates/x.yaml", "name: Second\nfields: []\n"); err != nil {
		t.Fatal(err)
	}
	cached, err := m.LoadTemplate("x.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cached.Name != "First" {
		t.Errorf("after external rewrite, expected cached %q; got %q", "First", cached.Name)
	}
	if cached != first {
		t.Errorf("expected identical pointer from cache; got fresh allocation")
	}
}

func TestSaveTemplate_InvalidatesCache(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.SaveTemplate("x.yaml", &Template{
		Name:   "Original",
		Fields: []Field{{Key: "a", Type: "text"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.LoadTemplate("x.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := m.SaveTemplate("x.yaml", &Template{
		Name:   "Updated",
		Fields: []Field{{Key: "a", Type: "text"}},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := m.LoadTemplate("x.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Updated" {
		t.Errorf("after SaveTemplate, name = %q, want Updated", got.Name)
	}
}

func TestDeleteTemplate_InvalidatesCache(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.SaveTemplate("x.yaml", &Template{
		Name:   "Original",
		Fields: []Field{{Key: "a", Type: "text"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.LoadTemplate("x.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteTemplate("x.yaml"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.LoadTemplate("x.yaml"); err == nil {
		t.Error("expected error after delete; cache still returned a Template")
	}
}

// ----- CreationObserver -----------------------------------------------

func TestCreationObserver_FiresOnFirstSave(t *testing.T) {
	m, _, _ := newTestManager(t)
	var created []string
	m.AddCreationObserver(CreationObserverFunc(func(n string) error {
		created = append(created, n)
		return nil
	}))
	if err := m.SaveTemplate("brand-new.yaml", &Template{
		Name:   "Brand New",
		Fields: []Field{{Key: "k", Type: "text"}},
	}); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	if len(created) != 1 || created[0] != "brand-new.yaml" {
		t.Errorf("expected one OnTemplateCreated for brand-new.yaml, got %v", created)
	}
}

func TestCreationObserver_DoesNotFireOnUpdate(t *testing.T) {
	m, _, _ := newTestManager(t)
	// Save once without the observer wired - file exists.
	if err := m.SaveTemplate("kept.yaml", &Template{
		Name:   "Kept",
		Fields: []Field{{Key: "k", Type: "text"}},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var created []string
	m.AddCreationObserver(CreationObserverFunc(func(n string) error {
		created = append(created, n)
		return nil
	}))
	// Re-save - should be treated as an update, no creation event.
	if err := m.SaveTemplate("kept.yaml", &Template{
		Name:   "Kept (renamed)",
		Fields: []Field{{Key: "k", Type: "text"}},
	}); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	if len(created) != 0 {
		t.Errorf("update must not fire OnTemplateCreated, got %v", created)
	}
}

func TestCreationObserver_MultipleFireInOrder(t *testing.T) {
	m, _, _ := newTestManager(t)
	var order []string
	a := CreationObserverFunc(func(n string) error { order = append(order, "a:"+n); return nil })
	b := CreationObserverFunc(func(n string) error { order = append(order, "b:"+n); return nil })
	m.AddCreationObserver(a)
	m.AddCreationObserver(b)
	if err := m.SaveTemplate("x.yaml", &Template{
		Name:   "X",
		Fields: []Field{{Key: "k", Type: "text"}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	want := []string{"a:x.yaml", "b:x.yaml"}
	if len(order) != 2 || order[0] != want[0] || order[1] != want[1] {
		t.Errorf("order = %v, want %v", order, want)
	}
}

func TestCreationObserver_ErrorIsSwallowed(t *testing.T) {
	m, _, _ := newTestManager(t)
	m.AddCreationObserver(CreationObserverFunc(func(_ string) error {
		return errors.New("intentional")
	}))
	if err := m.SaveTemplate("z.yaml", &Template{
		Name:   "Z",
		Fields: []Field{{Key: "k", Type: "text"}},
	}); err != nil {
		t.Errorf("creation observer error must not propagate, got %v", err)
	}
}

func TestCreationObserver_NilIgnored(t *testing.T) {
	m, _, _ := newTestManager(t)
	m.AddCreationObserver(nil)
	if err := m.SaveTemplate("ok.yaml", &Template{
		Name:   "OK",
		Fields: []Field{{Key: "k", Type: "text"}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

// CreationObserver must NOT fire when the validation guard rejects the
// save - the file is never written, so semantically nothing was created.
func TestCreationObserver_NotFiredOnValidationFailure(t *testing.T) {
	m, _, _ := newTestManager(t)
	var fired bool
	m.AddCreationObserver(CreationObserverFunc(func(_ string) error {
		fired = true
		return nil
	}))
	// Duplicate field keys → Validate rejects → save returns an error
	// before the file is written.
	err := m.SaveTemplate("bad.yaml", &Template{
		Name: "Bad",
		Fields: []Field{
			{Key: "dup", Type: "text"},
			{Key: "dup", Type: "text"},
		},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if fired {
		t.Error("creation observer must not fire when save is rejected")
	}
}

// ----- Observer (deletion listener) -----------------------------------

// stubObserver records every OnTemplateDeleted call for assertion.
type stubObserver struct {
	calls []string
	err   error
}

func (s *stubObserver) OnTemplateDeleted(name string) error {
	s.calls = append(s.calls, name)
	return s.err
}

func TestAddObserver_DeleteFiresObserver(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.SaveTemplate("gone.yaml", &Template{
		Name:   "Going",
		Fields: []Field{{Key: "k", Type: "text"}},
	}); err != nil {
		t.Fatal(err)
	}
	obs := &stubObserver{}
	m.AddObserver(obs)

	if err := m.DeleteTemplate("gone.yaml"); err != nil {
		t.Fatalf("DeleteTemplate: %v", err)
	}
	if len(obs.calls) != 1 || obs.calls[0] != "gone.yaml" {
		t.Errorf("observer calls = %v, want [gone.yaml]", obs.calls)
	}
}

func TestAddObserver_MultipleAllFireInOrder(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.SaveTemplate("x.yaml", &Template{
		Name:   "X",
		Fields: []Field{{Key: "a", Type: "text"}},
	}); err != nil {
		t.Fatal(err)
	}
	var order []string
	a := ObserverFunc(func(n string) error { order = append(order, "a:"+n); return nil })
	b := ObserverFunc(func(n string) error { order = append(order, "b:"+n); return nil })
	m.AddObserver(a)
	m.AddObserver(b)

	if err := m.DeleteTemplate("x.yaml"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	want := []string{"a:x.yaml", "b:x.yaml"}
	if len(order) != 2 || order[0] != want[0] || order[1] != want[1] {
		t.Errorf("call order = %v, want %v", order, want)
	}
}

// Observer errors must be swallowed (logged), never propagated - the
// observer is best-effort, just like the Indexer.
func TestAddObserver_ErrorIsSwallowedNotPropagated(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.SaveTemplate("y.yaml", &Template{
		Name:   "Y",
		Fields: []Field{{Key: "a", Type: "text"}},
	}); err != nil {
		t.Fatal(err)
	}
	obs := &stubObserver{err: errors.New("intentional")}
	m.AddObserver(obs)
	if err := m.DeleteTemplate("y.yaml"); err != nil {
		t.Errorf("observer error must not propagate, got %v", err)
	}
}

func TestAddObserver_NilIsIgnored(t *testing.T) {
	m, _, _ := newTestManager(t)
	// Should not panic; should not register anything.
	m.AddObserver(nil)
	if err := m.SaveTemplate("z.yaml", &Template{
		Name:   "Z",
		Fields: []Field{{Key: "a", Type: "text"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteTemplate("z.yaml"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

// Observer must NOT fire for missing-file deletes either - the underlying
// fs.DeleteFile is a no-op, but the observer still fires because the
// caller asked us to delete "X" and we honored the request. This makes
// the prune downstream behave consistently regardless of "did the file
// actually exist on disk", which matches the broader "self-heal" intent.
func TestAddObserver_FiresEvenWhenFileMissing(t *testing.T) {
	m, _, _ := newTestManager(t)
	obs := &stubObserver{}
	m.AddObserver(obs)
	if err := m.DeleteTemplate("never-there.yaml"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(obs.calls) != 1 {
		t.Errorf("observer must fire for missing-file delete (downstream still needs to reconcile), got %v", obs.calls)
	}
}

// Many goroutines hammer LoadTemplate for the same filename at once.
// Without per-name serialization + cache, all N callers spin up
// concurrent yaml.Unmarshal goroutines - the exact mount-storm pattern
// that GC-trashed the dev binary. With the cache, N callers must agree
// on one *Template, and `go test -race` must not report a data race.
func TestLoadTemplate_ConcurrentSameNameNoRace(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.SaveTemplate("x.yaml", &Template{
		Name:   "Concurrent",
		Fields: []Field{{Key: "a", Type: "text"}},
	}); err != nil {
		t.Fatal(err)
	}
	const N = 64
	results := make([]*Template, N)
	errs := make([]error, N)
	var wg sync.WaitGroup
	wg.Add(N)
	for i := range N {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = m.LoadTemplate("x.yaml")
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}
	first := results[0]
	if first == nil {
		t.Fatal("first result is nil")
	}
	for i, r := range results {
		if r != first {
			t.Errorf("goroutine %d returned a different pointer than goroutine 0", i)
		}
	}
}

func TestLoadMany_HappyPath(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.SaveTemplate("a.yaml", &Template{Name: "A", Fields: []Field{{Key: "x", Type: "text"}}}); err != nil {
		t.Fatal(err)
	}
	if err := m.SaveTemplate("b.yaml", &Template{Name: "B", Fields: []Field{{Key: "y", Type: "text"}}}); err != nil {
		t.Fatal(err)
	}
	got := m.LoadMany([]string{"b.yaml", "a.yaml"})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Filename != "b.yaml" || got[0].Template == nil || got[0].Template.Name != "B" {
		t.Errorf("got[0] = %+v, want b.yaml/B", got[0])
	}
	if got[1].Filename != "a.yaml" || got[1].Template == nil || got[1].Template.Name != "A" {
		t.Errorf("got[1] = %+v, want a.yaml/A", got[1])
	}
}

func TestLoadMany_MissingFileEmitsErrorSlot(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.SaveTemplate("a.yaml", &Template{Name: "A", Fields: []Field{{Key: "x", Type: "text"}}}); err != nil {
		t.Fatal(err)
	}
	got := m.LoadMany([]string{"a.yaml", "nope.yaml"})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Template == nil || got[0].Error != "" {
		t.Errorf("got[0] should be a.yaml with Template, got %+v", got[0])
	}
	if got[1].Template != nil {
		t.Errorf("got[1].Template should be nil for missing file, got %+v", got[1].Template)
	}
	if got[1].Error == "" {
		t.Errorf("got[1].Error should carry the missing-file reason")
	}
	if got[1].Filename != "nope.yaml" {
		t.Errorf("got[1].Filename = %q, want nope.yaml", got[1].Filename)
	}
}

func TestLoadMany_EmptyInputReturnsEmpty(t *testing.T) {
	m, _, _ := newTestManager(t)
	if got := m.LoadMany(nil); len(got) != 0 {
		t.Errorf("nil names should produce empty slice, got %+v", got)
	}
}

func TestLoadMany_PointerEqualityWithLoadTemplateCache(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.SaveTemplate("a.yaml", &Template{Name: "A", Fields: []Field{{Key: "x", Type: "text"}}}); err != nil {
		t.Fatal(err)
	}
	single, err := m.LoadTemplate("a.yaml")
	if err != nil {
		t.Fatal(err)
	}
	batch := m.LoadMany([]string{"a.yaml"})
	if batch[0].Template != single {
		t.Errorf("LoadMany should hit the same cache slot as LoadTemplate; got different pointer")
	}
}

func TestSaveTemplate_RejectsNil(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.SaveTemplate("x.yaml", nil); err == nil {
		t.Fatal("expected error for nil template")
	}
}

func TestSaveTemplate_BackfillsFilename(t *testing.T) {
	m, sys, _ := newTestManager(t)
	tmpl := &Template{Name: "X", Fields: []Field{{Key: "a", Type: "text"}}}
	if err := m.SaveTemplate("x.yaml", tmpl); err != nil {
		t.Fatal(err)
	}
	body, _ := sys.LoadFile("templates/x.yaml")
	if !strings.Contains(body, "filename: x.yaml") {
		t.Errorf("filename not persisted: %q", body)
	}
}

// SaveTemplate stamps the active profile's identity onto AuthorName
// and AuthorEmail when the caller leaves them empty. PullWithStash's
// override path reads these to surface "who last touched this template"
// in the same way it reads meta.author_name from records.
func TestSaveTemplate_FillsAuthorFromConfigWhenMissing(t *testing.T) {
	m, sys, _ := newTestManager(t)
	m.SetAuthorReader(AuthorFunc(func() (string, string) {
		return "Alice", "alice@example.com"
	}))

	tmpl := &Template{Name: "X", Filename: "x.yaml", Fields: []Field{{Key: "a", Type: "text"}}}
	if err := m.SaveTemplate("x.yaml", tmpl); err != nil {
		t.Fatal(err)
	}
	body, _ := sys.LoadFile("templates/x.yaml")
	if !strings.Contains(body, "author_name: Alice") {
		t.Errorf("author_name not persisted: %q", body)
	}
	if !strings.Contains(body, "author_email: alice@example.com") {
		t.Errorf("author_email not persisted: %q", body)
	}
	// Round-trip: reload and compare.
	loaded, err := m.LoadTemplate("x.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AuthorName != "Alice" || loaded.AuthorEmail != "alice@example.com" {
		t.Errorf("loaded author = %q/%q, want Alice/alice@example.com",
			loaded.AuthorName, loaded.AuthorEmail)
	}
}

// Explicitly-set author values pass through unchanged - important for
// sync round-trips where a template authored by Alice should keep
// Alice's identity even when Bob saves it via a backend that bypassed
// the editor (HTTP write, indexer reconcile).
func TestSaveTemplate_PreservesExplicitAuthor(t *testing.T) {
	m, sys, _ := newTestManager(t)
	m.SetAuthorReader(AuthorFunc(func() (string, string) {
		return "Bob", "bob@example.com"
	}))

	tmpl := &Template{
		Name:        "X",
		Filename:    "x.yaml",
		AuthorName:  "Alice",
		AuthorEmail: "alice@example.com",
		Fields:      []Field{{Key: "a", Type: "text"}},
	}
	if err := m.SaveTemplate("x.yaml", tmpl); err != nil {
		t.Fatal(err)
	}
	body, _ := sys.LoadFile("templates/x.yaml")
	if !strings.Contains(body, "author_name: Alice") {
		t.Errorf("explicit author_name not preserved: %q", body)
	}
	if strings.Contains(body, "Bob") || strings.Contains(body, "bob@example.com") {
		t.Errorf("config-author leaked into explicit-author template: %q", body)
	}
}

// Without a wired AuthorReader, save still succeeds - fields stay
// empty, omitempty drops them from the YAML.
func TestSaveTemplate_NilAuthorReader_LeavesFieldsEmpty(t *testing.T) {
	m, sys, _ := newTestManager(t)
	// No SetAuthorReader call.
	tmpl := &Template{Name: "X", Filename: "x.yaml", Fields: []Field{{Key: "a", Type: "text"}}}
	if err := m.SaveTemplate("x.yaml", tmpl); err != nil {
		t.Fatal(err)
	}
	body, _ := sys.LoadFile("templates/x.yaml")
	if strings.Contains(body, "author_name") || strings.Contains(body, "author_email") {
		t.Errorf("author fields leaked despite nil reader: %q", body)
	}
}

// GetDescriptor backfills missing author fields the first time a
// template is opened - first opener gets the credit. Subsequent loads
// see the stamped values on disk and don't overwrite them.
func TestGetDescriptor_BackfillsAuthorOnFirstOpen(t *testing.T) {
	m, sys, _ := newTestManager(t)
	// Seed an unstamped template directly to disk so it's truly
	// "imported / pre-existing" as far as the manager is concerned.
	if err := sys.SaveFile("templates/x.yaml",
		"name: X\nfilename: x.yaml\nfields:\n  - key: a\n    type: text\n"); err != nil {
		t.Fatal(err)
	}

	m.SetAuthorReader(AuthorFunc(func() (string, string) {
		return "Alice", "alice@example.com"
	}))

	desc, err := m.GetDescriptor("x.yaml", "/tmp/x")
	if err != nil {
		t.Fatal(err)
	}
	if desc.YAML.AuthorName != "Alice" || desc.YAML.AuthorEmail != "alice@example.com" {
		t.Errorf("descriptor not stamped, got %q/%q",
			desc.YAML.AuthorName, desc.YAML.AuthorEmail)
	}
	// On-disk file should now carry the stamp too.
	body, _ := sys.LoadFile("templates/x.yaml")
	if !strings.Contains(body, "author_name: Alice") {
		t.Errorf("author_name not persisted on first open: %q", body)
	}
	if !strings.Contains(body, "author_email: alice@example.com") {
		t.Errorf("author_email not persisted on first open: %q", body)
	}
}

// Second opener (with a different identity) does NOT overwrite the
// existing stamp. Author identity is sticky - first one wins.
func TestGetDescriptor_DoesNotOverwriteExistingAuthor(t *testing.T) {
	m, sys, _ := newTestManager(t)
	if err := sys.SaveFile("templates/x.yaml",
		"name: X\nfilename: x.yaml\nauthor_name: Alice\nauthor_email: alice@example.com\nfields:\n  - key: a\n    type: text\n"); err != nil {
		t.Fatal(err)
	}

	m.SetAuthorReader(AuthorFunc(func() (string, string) {
		return "Bob", "bob@example.com"
	}))

	desc, err := m.GetDescriptor("x.yaml", "")
	if err != nil {
		t.Fatal(err)
	}
	if desc.YAML.AuthorName != "Alice" {
		t.Errorf("author_name overwritten: got %q, want Alice", desc.YAML.AuthorName)
	}
	body, _ := sys.LoadFile("templates/x.yaml")
	if strings.Contains(body, "Bob") || strings.Contains(body, "bob@example.com") {
		t.Errorf("Bob's identity leaked into Alice's template: %q", body)
	}
}

// GetDescriptor backfills even when the template has other validation
// errors - opening shouldn't be punitive. SaveTemplate would refuse
// such a template, but the YAML write here bypasses Validate.
func TestGetDescriptor_BackfillsEvenWhenTemplateInvalid(t *testing.T) {
	m, sys, _ := newTestManager(t)
	// Two guid fields = "multiple-guid-fields" validation error.
	if err := sys.SaveFile("templates/broken.yaml",
		"name: B\nfilename: broken.yaml\nfields:\n  - key: id1\n    type: guid\n  - key: id2\n    type: guid\n"); err != nil {
		t.Fatal(err)
	}

	m.SetAuthorReader(AuthorFunc(func() (string, string) {
		return "Alice", "alice@example.com"
	}))

	desc, err := m.GetDescriptor("broken.yaml", "")
	if err != nil {
		t.Fatalf("GetDescriptor must succeed even on broken template: %v", err)
	}
	if desc.YAML.AuthorName != "Alice" {
		t.Errorf("descriptor not stamped on broken template, got %q",
			desc.YAML.AuthorName)
	}
	body, _ := sys.LoadFile("templates/broken.yaml")
	if !strings.Contains(body, "author_name: Alice") {
		t.Errorf("backfill skipped on validation-failing template: %q", body)
	}
}

func TestSaveTemplate_RejectsValidationFailure(t *testing.T) {
	m, sys, _ := newTestManager(t)
	tmpl := &Template{
		Name:     "Bad",
		Filename: "bad.yaml",
		Fields: []Field{
			{Key: "id", Type: "guid"},
			{Key: "alt", Type: "guid"},
		},
	}
	err := m.SaveTemplate("bad.yaml", tmpl)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	var verr *ValidationFailedError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ValidationFailedError, got %T: %v", err, err)
	}
	found := false
	for _, ve := range verr.Errors {
		if ve.Type == "multiple-guid-fields" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected multiple-guid-fields in errors; got %+v", verr.Errors)
	}
	if sys.FileExists("templates/bad.yaml") {
		t.Error("validation failure must not write the file to disk")
	}
}

func TestSaveTemplate_AcceptsEmptyFieldsTemplate(t *testing.T) {
	// Empty templates are valid drafts - the editor creates them when
	// the user hits "New Template" before adding any fields.
	m, sys, _ := newTestManager(t)
	tmpl := &Template{Name: "Draft", Filename: "draft.yaml"}
	if err := m.SaveTemplate("draft.yaml", tmpl); err != nil {
		t.Fatalf("empty template should save: %v", err)
	}
	if !sys.FileExists("templates/draft.yaml") {
		t.Error("file not written")
	}
}

func TestSaveTemplate_DoesNotFireIndexerOnValidationFailure(t *testing.T) {
	m, _, _ := newTestManager(t)
	idx := &recordingIndexer{}
	m.SetIndexer(idx)
	tmpl := &Template{
		Name:     "Bad",
		Filename: "bad.yaml",
		Fields: []Field{
			{Key: "t1", Type: "tags"},
			{Key: "t2", Type: "tags"},
		},
	}
	if err := m.SaveTemplate("bad.yaml", tmpl); err == nil {
		t.Fatal("expected validation error")
	}
	if len(idx.changed) != 0 {
		t.Errorf("indexer fired on validation failure: %v", idx.changed)
	}
}

func TestDeleteTemplate_MissingIsNoOp(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.DeleteTemplate("ghost.yaml"); err != nil {
		t.Errorf("delete missing should be no-op: %v", err)
	}
}

func TestListTemplates_FiltersNonYAML(t *testing.T) {
	m, sys, _ := newTestManager(t)
	_ = sys.SaveFile("templates/a.yaml", "name: A\nfields: []\n")
	_ = sys.SaveFile("templates/notes.txt", "x")
	files, err := m.ListTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != "a.yaml" {
		t.Errorf("filter failed: %v", files)
	}
}

func TestHasTemplates_FalseOnEmptyAndMissingDir(t *testing.T) {
	m, _, _ := newTestManager(t)
	// Directory not yet created.
	if m.HasTemplates() {
		t.Error("expected false for missing templates dir")
	}
}

func TestHasTemplates_TrueAfterYAMLAdded(t *testing.T) {
	m, sys, _ := newTestManager(t)
	_ = sys.SaveFile("templates/a.yaml", "name: A\nfields: []\n")
	if !m.HasTemplates() {
		t.Error("expected true after adding a.yaml")
	}
}

func TestHasTemplates_IgnoresNonYAML(t *testing.T) {
	m, sys, _ := newTestManager(t)
	_ = sys.SaveFile("templates/notes.txt", "x")
	if m.HasTemplates() {
		t.Error("expected false when only non-YAML files exist")
	}
}

func TestSeedBasicIfEmpty_PreservesExisting(t *testing.T) {
	m, sys, _ := newTestManager(t)
	_ = sys.SaveFile("templates/other.yaml", "name: Other\nfields: []\n")
	if err := m.SeedBasicIfEmpty(); err != nil {
		t.Fatal(err)
	}
	if sys.FileExists(sys.JoinPath("templates", "basic.yaml")) {
		t.Error("basic.yaml should NOT have been created")
	}
}

func TestTopLevelTextFields_IgnoresNonTextAndLooped(t *testing.T) {
	fields := []Field{
		{Key: "title", Type: "text"},
		{Key: "n", Type: "number"},
		{Key: "items", Type: "loopstart"},
		{Key: "inner", Type: "text"},
		{Key: "items", Type: "loopstop"},
		{Key: "tail", Type: "text", Label: "Tail"},
	}
	got := TopLevelTextFields(fields)
	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %v", got)
	}
	if got[0].Key != "title" || got[1].Key != "tail" {
		t.Errorf("keys wrong: %v", got)
	}
	if got[1].Label != "Tail" {
		t.Errorf("label not preserved: %v", got[1])
	}
}

func TestYAMLRoundTrip_PreservesCustomFields(t *testing.T) {
	m, sys, _ := newTestManager(t)
	src := `name: Custom
filename: custom.yaml
fields:
  - key: x
    type: text
    custom_prop: hello
`
	_ = sys.SaveFile("templates/custom.yaml", src)
	loaded, err := m.LoadTemplate("custom.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := loaded.Fields[0].Extra["custom_prop"]; !ok || got != "hello" {
		t.Errorf("custom_prop not preserved: %+v", loaded.Fields[0].Extra)
	}
}

// LoadTemplate must surface a parse error wrapped with the "template: parse"
// prefix for syntactically broken YAML, never a partial Template.
func TestLoadTemplate_MalformedYAMLReturnsParseError(t *testing.T) {
	m, _, _ := newTestManager(t)
	writeRaw(t, m, "bad.yaml", "name: X\nfields: [ {key: a, type: text }\n")
	got, err := m.LoadTemplate("bad.yaml")
	if err == nil {
		t.Fatal("expected parse error for malformed YAML, got nil")
	}
	if got != nil {
		t.Errorf("expected nil Template on parse error, got %+v", got)
	}
	if !strings.HasPrefix(err.Error(), `template: parse "bad.yaml":`) {
		t.Errorf("error = %q, want template: parse prefix", err.Error())
	}
}

// Duplicate top-level YAML mapping keys are a yaml.v3 unmarshal error, so they
// reach LoadTemplate as the wrapped parse error, not silent last-wins.
func TestLoadTemplate_DuplicateMappingKeyIsParseError(t *testing.T) {
	m, _, _ := newTestManager(t)
	writeRaw(t, m, "dupmap.yaml", "name: X\nname: Y\nfields: []\n")
	got, err := m.LoadTemplate("dupmap.yaml")
	if err == nil {
		t.Fatal("expected parse error for duplicate mapping key, got nil")
	}
	if got != nil {
		t.Errorf("expected nil Template, got %+v", got)
	}
	if !strings.Contains(err.Error(), `mapping key "name" already defined`) {
		t.Errorf("error = %q, want duplicate mapping key reason", err.Error())
	}
}

// Boundary: a wide template of many distinct text fields loads every field and
// validates clean.
func TestLoadTemplate_ManyDistinctFieldsValidates(t *testing.T) {
	m, _, _ := newTestManager(t)
	var b strings.Builder
	b.WriteString("name: Wide\nfields:\n")
	const n = 64
	for i := 0; i < n; i++ {
		b.WriteString("  - key: f")
		b.WriteString(itoa(i))
		b.WriteString("\n    type: text\n")
	}
	writeRaw(t, m, "wide.yaml", b.String())
	tmpl, err := m.LoadTemplate("wide.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(tmpl.Fields) != n {
		t.Fatalf("fields = %d, want %d", len(tmpl.Fields), n)
	}
	if errs := Validate(tmpl); len(errs) != 0 {
		t.Errorf("%d distinct text fields should validate clean, got %+v", n, errs)
	}
}

// (itoa lives in scope.go and is reused by the wide-template fixture above.)

// LoadMany over a mix of valid, malformed, and missing files: valid slots carry
// a Template with no Error, the malformed slot carries the parse error, the
// missing slot carries its own error. The batch never aborts.
func TestLoadMany_MixedValidAndInvalidSurfacesNonFatally(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.SaveTemplate("ok.yaml", &Template{
		Name:   "OK",
		Fields: []Field{{Key: "a", Type: "text"}},
	}); err != nil {
		t.Fatal(err)
	}
	writeRaw(t, m, "broken.yaml", "name: X\nfields: [ {key: a\n")

	got := m.LoadMany([]string{"ok.yaml", "broken.yaml", "gone.yaml"})
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}

	if got[0].Filename != "ok.yaml" || got[0].Template == nil || got[0].Error != "" {
		t.Fatalf("got[0] should be loaded ok.yaml, got %+v", got[0])
	}
	if got[0].Template.Name != "OK" {
		t.Errorf("got[0] name = %q, want OK", got[0].Template.Name)
	}
	if len(got[0].Template.Fields) != 1 || got[0].Template.Fields[0].Key != "a" {
		t.Errorf("got[0] fields = %+v, want one field keyed a", got[0].Template.Fields)
	}

	if got[1].Filename != "broken.yaml" || got[1].Template != nil {
		t.Errorf("got[1] should be broken.yaml with nil Template, got %+v", got[1])
	}
	if !strings.HasPrefix(got[1].Error, `template: parse "broken.yaml":`) {
		t.Errorf("got[1].Error = %q, want template: parse prefix", got[1].Error)
	}

	if got[2].Filename != "gone.yaml" || got[2].Template != nil {
		t.Errorf("got[2] should be gone.yaml with nil Template, got %+v", got[2])
	}
	if !strings.Contains(got[2].Error, "file not found") {
		t.Errorf("got[2].Error = %q, want file-not-found reason", got[2].Error)
	}
}

// SaveTemplate must surface a filesystem write failure to the caller (the
// error returns as-is, before the cache is cleared or the indexer fires).
func TestSaveTemplate_SaveFileFailurePropagates(t *testing.T) {
	m, ff := newFailingManager(t)
	ff.saveErr = errors.New("disk full")
	idx := &recordingIndexer{}
	m.SetIndexer(idx)
	var created []string
	m.AddCreationObserver(CreationObserverFunc(func(n string) error {
		created = append(created, n)
		return nil
	}))

	err := m.SaveTemplate("x.yaml", &Template{
		Name:   "X",
		Fields: []Field{{Key: "a", Type: "text"}},
	})
	if err == nil {
		t.Fatal("expected write error, got nil")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("error = %q, want disk-full reason", err.Error())
	}
	// A failed write is not a create: no indexer hook, no creation observer.
	if len(idx.changed) != 0 {
		t.Errorf("indexer fired on write failure: %v", idx.changed)
	}
	if len(created) != 0 {
		t.Errorf("creation observer fired on write failure: %v", created)
	}
}

// DeleteTemplate must surface a filesystem delete failure before the cache is
// cleared or any observer fires.
func TestDeleteTemplate_DeleteFileFailurePropagates(t *testing.T) {
	m, ff := newFailingManager(t)
	if err := m.SaveTemplate("gone.yaml", &Template{
		Name:   "Gone",
		Fields: []Field{{Key: "a", Type: "text"}},
	}); err != nil {
		t.Fatal(err)
	}
	obs := &stubObserver{}
	m.AddObserver(obs)
	ff.delErr = errors.New("permission denied")

	err := m.DeleteTemplate("gone.yaml")
	if err == nil {
		t.Fatal("expected delete error, got nil")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error = %q, want permission-denied reason", err.Error())
	}
	if len(obs.calls) != 0 {
		t.Errorf("observer fired on delete failure: %v", obs.calls)
	}
}

// ListTemplates wraps an underlying ListFiles error with the "template: list"
// prefix rather than returning a partial slice.
func TestListTemplates_ListFilesErrorWrapped(t *testing.T) {
	m, ff := newFailingManager(t)
	// The dir must exist so ListTemplates reaches ListFiles (the missing-dir
	// short-circuit returns an empty slice with no error).
	if err := m.EnsureTemplateDirectory(); err != nil {
		t.Fatal(err)
	}
	ff.listErr = errors.New("io error")
	files, err := m.ListTemplates()
	if err == nil {
		t.Fatal("expected list error, got nil")
	}
	if files != nil {
		t.Errorf("expected nil slice on error, got %v", files)
	}
	if !strings.HasPrefix(err.Error(), "template: list:") {
		t.Errorf("error = %q, want template: list: prefix", err.Error())
	}
}

// HasTemplates collapses any ListTemplates error to false rather than panicking
// or propagating.
func TestHasTemplates_ListErrorCollapsesToFalse(t *testing.T) {
	m, ff := newFailingManager(t)
	if err := m.EnsureTemplateDirectory(); err != nil {
		t.Fatal(err)
	}
	ff.listErr = errors.New("io error")
	if m.HasTemplates() {
		t.Error("expected false when ListTemplates errors")
	}
}

// GetDescriptor still returns the stamped descriptor when the best-effort
// author backfill write fails: the backfill is logged, never fatal.
func TestGetDescriptor_BackfillWriteFailureIsNonFatal(t *testing.T) {
	m, ff := newFailingManager(t)
	if err := ff.inner.SaveFile(ff.inner.JoinPath("templates", "x.yaml"),
		"name: X\nfilename: x.yaml\nfields:\n  - key: a\n    type: text\n"); err != nil {
		t.Fatal(err)
	}
	m.SetAuthorReader(AuthorFunc(func() (string, string) {
		return "Alice", "alice@example.com"
	}))
	ff.saveErr = errors.New("disk full")

	desc, err := m.GetDescriptor("x.yaml", "")
	if err != nil {
		t.Fatalf("GetDescriptor must succeed despite backfill write failure: %v", err)
	}
	// In-memory descriptor is still stamped even though the disk write failed.
	if desc.YAML.AuthorName != "Alice" {
		t.Errorf("descriptor not stamped in memory, got %q", desc.YAML.AuthorName)
	}
}

// SeedBasicIfEmpty surfaces a write failure when it tries to create basic.yaml
// into an empty templates dir.
func TestSeedBasicIfEmpty_WriteFailurePropagates(t *testing.T) {
	m, ff := newFailingManager(t)
	ff.saveErr = errors.New("disk full")
	err := m.SeedBasicIfEmpty()
	if err == nil {
		t.Fatal("expected write error, got nil")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("error = %q, want disk-full reason", err.Error())
	}
}

// LoadTemplate is cached and serialized per-name; concurrent loads of one file
// must all return the same pointer with the same field count and no race.
func TestLoadTemplate_ConcurrentLoadsShareCachedPointer(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.SaveTemplate("conc.yaml", &Template{
		Name:   "Conc",
		Fields: []Field{{Key: "a", Type: "text"}, {Key: "b", Type: "text"}},
	}); err != nil {
		t.Fatal(err)
	}

	const goroutines = 16
	ptrs := make([]*Template, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			got, err := m.LoadTemplate("conc.yaml")
			if err != nil {
				t.Errorf("goroutine %d load: %v", idx, err)
				return
			}
			ptrs[idx] = got
		}(i)
	}
	wg.Wait()

	first := ptrs[0]
	if first == nil {
		t.Fatal("first load returned nil")
	}
	if len(first.Fields) != 2 {
		t.Fatalf("fields = %d, want 2", len(first.Fields))
	}
	for i := 1; i < goroutines; i++ {
		if ptrs[i] != first {
			t.Errorf("goroutine %d pointer = %p, want shared %p", i, ptrs[i], first)
		}
	}
}
