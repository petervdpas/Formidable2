package connection

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
)

// memFS is an in-memory storeFS that also counts reads, so the catalog cache
// is observable without reaching into the Manager's internals.
type memFS struct {
	mu     sync.Mutex
	files  map[string]string
	reads  map[string]int
	failOn string
}

func newMemFS() *memFS {
	return &memFS{files: map[string]string{}, reads: map[string]int{}}
}

func (m *memFS) FileExists(path string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.files[path]
	return ok
}

func (m *memFS) LoadFile(path string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reads[path]++
	v, ok := m.files[path]
	if !ok {
		return "", os.ErrNotExist
	}
	return v, nil
}

func (m *memFS) SaveFile(path, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failOn != "" && strings.Contains(path, m.failOn) {
		return errors.New("disk full")
	}
	m.files[path] = content
	return nil
}

func (m *memFS) DeleteFile(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, path)
	return nil
}

func (m *memFS) ListDir(path string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	prefix := strings.TrimSuffix(path, "/") + "/"
	var out []string
	for p := range m.files {
		rest, ok := strings.CutPrefix(p, prefix)
		if !ok || strings.Contains(rest, "/") {
			continue
		}
		out = append(out, rest)
	}
	return out, nil
}

func (m *memFS) readCount(path string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reads[path]
}

// seeded returns a Manager holding one importable spec plus the baseline
// connection already saved.
func seeded(t *testing.T) (*Manager, *memFS) {
	t.Helper()
	fs := newMemFS()
	m := NewManager(fs, nil)
	if _, err := m.ImportSpec("crm-prod", []byte(specV3JSON)); err != nil {
		t.Fatalf("ImportSpec: %v", err)
	}
	c := validConn()
	if err := m.Save(&c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return m, fs
}

func TestImportSpec_WritesJSONSpecAndReturnsCatalog(t *testing.T) {
	fs := newMemFS()
	m := NewManager(fs, nil)
	info, err := m.ImportSpec("crm-prod", []byte(specV3JSON))
	if err != nil {
		t.Fatal(err)
	}
	if info.File != "crm-prod.json" {
		t.Errorf("spec file = %q, want crm-prod.json", info.File)
	}
	if info.Catalog.Title != "CRM" {
		t.Errorf("catalog title = %q", info.Catalog.Title)
	}
	if !fs.FileExists("api-clients/specs/crm-prod.json") {
		t.Fatalf("spec not written; files = %v", fs.files)
	}
}

func TestImportSpec_KeepsYAMLExtensionForYAMLInput(t *testing.T) {
	fs := newMemFS()
	m := NewManager(fs, nil)
	if _, err := m.ImportSpec("tiny", []byte(specV3YAML)); err != nil {
		t.Fatal(err)
	}
	if !fs.FileExists("api-clients/specs/tiny.yaml") {
		t.Fatalf("yaml spec not written; files = %v", fs.files)
	}
}

func TestImportSpec_StoresTheUploadVerbatim(t *testing.T) {
	fs := newMemFS()
	m := NewManager(fs, nil)
	if _, err := m.ImportSpec("crm-prod", []byte(specV3JSON)); err != nil {
		t.Fatal(err)
	}
	got, _ := fs.LoadFile("api-clients/specs/crm-prod.json")
	if got != specV3JSON {
		t.Error("spec was rewritten; the stored copy must be byte-identical to the upload")
	}
}

func TestImportSpec_RejectsUnparseableSpec(t *testing.T) {
	m := NewManager(newMemFS(), nil)
	if _, err := m.ImportSpec("broken", []byte("not a spec at all")); err == nil {
		t.Fatal("want an error")
	}
}

func TestImportSpec_RejectsBadID(t *testing.T) {
	m := NewManager(newMemFS(), nil)
	for _, id := range []string{"", "../escape", "Has Spaces", "a/b"} {
		if _, err := m.ImportSpec(id, []byte(specV3JSON)); err == nil {
			t.Errorf("id %q must be rejected", id)
		}
	}
}

func TestSaveAndGet_RoundTrip(t *testing.T) {
	m, _ := seeded(t)
	got, cat, err := m.Get("crm-prod")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "CRM Production" || got.BaseURL != "https://crm.example.com/v2" {
		t.Errorf("connection = %+v", got)
	}
	if len(got.Resources) != 1 || got.Resources[0].SearchParam != "q" {
		t.Errorf("resources = %+v", got.Resources)
	}
	if got.Resources[0].List.Params["tenant"] != "acme" {
		t.Errorf("fixed params lost: %+v", got.Resources[0].List.Params)
	}
	if _, ok := cat.Op("listCustomers"); !ok {
		t.Error("catalog not returned alongside the connection")
	}
}

func TestSave_RejectsInvalidBinding(t *testing.T) {
	m, _ := seeded(t)
	c := validConn()
	c.Resources[0].List.Operation = "noSuchOperation"
	err := m.Save(&c)
	if err == nil {
		t.Fatal("want a validation error")
	}
	var vfe *ValidationFailedError
	if !errors.As(err, &vfe) {
		t.Fatalf("want a *ValidationFailedError, got %T", err)
	}
	if !hasType(vfe.Errors, "unknown-operation") {
		t.Errorf("errors = %s", types(vfe.Errors))
	}
}

func TestSave_RequiresAnImportedSpec(t *testing.T) {
	m := NewManager(newMemFS(), nil)
	c := validConn()
	if err := m.Save(&c); err == nil {
		t.Fatal("saving against a connection with no spec on disk must fail")
	}
}

func TestSave_DoesNotWriteWhenValidationFails(t *testing.T) {
	fs := newMemFS()
	m := NewManager(fs, nil)
	if _, err := m.ImportSpec("crm-prod", []byte(specV3JSON)); err != nil {
		t.Fatal(err)
	}
	c := validConn()
	c.Resources[0].List.Operation = "noSuchOperation"
	_ = m.Save(&c)
	if fs.FileExists("api-clients/crm-prod.yaml") {
		t.Fatal("an invalid connection must leave no file behind")
	}
}

func TestGet_MissingIsNotFound(t *testing.T) {
	m, _ := seeded(t)
	if _, _, err := m.Get("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGet_RejectsTraversalID(t *testing.T) {
	m, _ := seeded(t)
	for _, id := range []string{"../../etc/passwd", "a/b", ""} {
		if _, _, err := m.Get(id); err == nil {
			t.Errorf("id %q must be rejected", id)
		}
	}
}

func TestList_SortedAndSkipsNonDefinitions(t *testing.T) {
	m, fs := seeded(t)
	if _, err := m.ImportSpec("alpha", []byte(specV3YAML)); err != nil {
		t.Fatal(err)
	}
	alpha := Connection{
		ID: "alpha", Name: "Alpha", SpecFile: "alpha.yaml",
		BaseURL:   "https://alpha.example.com",
		Resources: []Resource{{Key: "things", List: OpRef{Operation: "listThings"}}},
	}
	if err := m.Save(&alpha); err != nil {
		t.Fatal(err)
	}
	_ = fs.SaveFile("api-clients/readme.txt", "ignore me")
	_ = fs.SaveFile("api-clients/.hidden.yaml", "ignore me too")

	list, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 summaries, got %d: %+v", len(list), list)
	}
	if list[0].ID != "alpha" || list[1].ID != "crm-prod" {
		t.Errorf("not sorted by id: %+v", list)
	}
	if list[1].Title != "CRM" || list[1].Resources != 1 {
		t.Errorf("summary = %+v", list[1])
	}
	if !list[0].OK || !list[1].OK {
		t.Errorf("both connections should report OK: %+v", list)
	}
}

func TestList_EmptyDirectory(t *testing.T) {
	m := NewManager(newMemFS(), nil)
	list, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("want no summaries, got %+v", list)
	}
}

func TestList_SurvivesAMalformedDefinition(t *testing.T) {
	m, fs := seeded(t)
	_ = fs.SaveFile("api-clients/broken.yaml", "id: [this is not a connection")

	list, err := m.List()
	if err != nil {
		t.Fatalf("one bad file must not fail the whole list: %v", err)
	}
	var broken *Summary
	for i := range list {
		if list[i].ID == "broken" {
			broken = &list[i]
		}
	}
	if broken == nil {
		t.Fatalf("the broken definition must still be listed: %+v", list)
	}
	if broken.OK || broken.Error == "" {
		t.Errorf("broken summary must carry a reason: %+v", broken)
	}
}

func TestList_ReportsAConnectionWhoseSpecWentMissing(t *testing.T) {
	m, fs := seeded(t)
	_ = fs.DeleteFile("api-clients/specs/crm-prod.json")
	m.Reload()

	list, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].OK {
		t.Fatalf("a connection with no spec must not report OK: %+v", list)
	}
}

func TestDelete_RemovesDefinitionAndSpec(t *testing.T) {
	m, fs := seeded(t)
	if err := m.Delete("crm-prod"); err != nil {
		t.Fatal(err)
	}
	if fs.FileExists("api-clients/crm-prod.yaml") || fs.FileExists("api-clients/specs/crm-prod.json") {
		t.Fatalf("leftovers: %v", fs.files)
	}
}

func TestDelete_IsIdempotent(t *testing.T) {
	m, _ := seeded(t)
	if err := m.Delete("never-existed"); err != nil {
		t.Fatalf("deleting a missing connection must not error: %v", err)
	}
}

func TestDelete_RejectsTraversalID(t *testing.T) {
	m, _ := seeded(t)
	if err := m.Delete("../crm-prod"); err == nil {
		t.Fatal("want an error")
	}
}

func TestCatalog_IsCachedUntilReload(t *testing.T) {
	m, fs := seeded(t)
	const specPath = "api-clients/specs/crm-prod.json"
	before := fs.readCount(specPath)

	for range 5 {
		if _, err := m.Catalog("crm-prod"); err != nil {
			t.Fatal(err)
		}
	}
	if got := fs.readCount(specPath) - before; got != 0 {
		t.Errorf("cached catalog still read the spec %d times", got)
	}

	m.Reload()
	if _, err := m.Catalog("crm-prod"); err != nil {
		t.Fatal(err)
	}
	if got := fs.readCount(specPath) - before; got != 1 {
		t.Errorf("after Reload want exactly 1 re-read, got %d", got)
	}
}

// specTinyJSON re-imports over crm-prod.json: JSON in, so it lands on the same
// filename the baseline connection already points at.
const specTinyJSON = `{"openapi":"3.0.0","info":{"title":"Tiny","version":"1.0"},` +
	`"servers":[{"url":"https://tiny.example.com"}],` +
	`"paths":{"/things":{"get":{"operationId":"listThings"}}}}`

func TestImportSpec_InvalidatesTheCachedCatalog(t *testing.T) {
	m, _ := seeded(t)
	if _, err := m.ImportSpec("crm-prod", []byte(specTinyJSON)); err != nil {
		t.Fatal(err)
	}
	cat, err := m.Catalog("crm-prod")
	if err != nil {
		t.Fatal(err)
	}
	if cat.Title != "Tiny" {
		t.Fatalf("stale catalog after re-import: %q", cat.Title)
	}
}

func TestDelete_KeepsASpecAnotherConnectionUses(t *testing.T) {
	m, fs := seeded(t)
	twin := validConn()
	twin.ID = "crm-acc"
	twin.Name = "CRM Acceptance"
	twin.BaseURL = "https://acc.crm.example.com/v2"
	if err := m.Save(&twin); err != nil {
		t.Fatal(err)
	}
	if err := m.Delete("crm-prod"); err != nil {
		t.Fatal(err)
	}
	if !fs.FileExists("api-clients/specs/crm-prod.json") {
		t.Fatal("a spec still referenced by another connection must survive")
	}
	if _, _, err := m.Get("crm-acc"); err != nil {
		t.Fatalf("the surviving connection broke: %v", err)
	}
}

func TestSaveFailureSurfaces(t *testing.T) {
	fs := newMemFS()
	m := NewManager(fs, nil)
	if _, err := m.ImportSpec("crm-prod", []byte(specV3JSON)); err != nil {
		t.Fatal(err)
	}
	fs.failOn = "api-clients/crm-prod.yaml"
	c := validConn()
	if err := m.Save(&c); err == nil {
		t.Fatal("a write failure must not be swallowed")
	}
}

func TestManager_ConcurrentAccess(t *testing.T) {
	m, _ := seeded(t)
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			switch i % 4 {
			case 0:
				_, _, _ = m.Get("crm-prod")
			case 1:
				_, _ = m.List()
			case 2:
				_, _ = m.Catalog("crm-prod")
			case 3:
				c := validConn()
				_ = m.Save(&c)
			}
		}(i)
	}
	wg.Wait()
}

func TestNewManager_NilFSDegradesGracefully(t *testing.T) {
	m := NewManager(nil, nil)
	if _, err := m.List(); err == nil {
		t.Error("List without a filesystem must error rather than panic")
	}
	if _, _, err := m.Get("x"); err == nil {
		t.Error("Get without a filesystem must error rather than panic")
	}
}
