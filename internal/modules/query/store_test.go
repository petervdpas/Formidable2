package query

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// storeMemFS is an in-memory storeFS. Paths are kept verbatim, so the tests
// also assert the on-disk layout the store chooses.
type storeMemFS struct {
	mu     sync.Mutex
	files  map[string]string
	failOn string
}

func newStoreMemFS() *storeMemFS { return &storeMemFS{files: map[string]string{}} }

func (m *storeMemFS) LoadFile(path string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.files[path]
	if !ok {
		return "", os.ErrNotExist
	}
	return v, nil
}

func (m *storeMemFS) SaveFile(path, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failOn != "" && strings.Contains(path, m.failOn) {
		return errors.New("disk full")
	}
	m.files[path] = content
	return nil
}

func (m *storeMemFS) DeleteFile(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, path)
	return nil
}

func (m *storeMemFS) ListDir(path string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failOn == "list" {
		return nil, errors.New("io error")
	}
	seen := map[string]bool{}
	out := []string{}
	for p := range m.files {
		dir, base := filepath.Split(p)
		if filepath.Clean(dir) != filepath.Clean(path) || seen[base] {
			continue
		}
		seen[base] = true
		out = append(out, base)
	}
	return out, nil
}

// newTestStore builds a store over both roots with the location the profile
// setting would report; the returned pointer lets a test flip it mid-run.
func newTestStoreAt(t *testing.T, loc *string) (*Store, *storeMemFS) {
	t.Helper()
	fs := newStoreMemFS()
	s := NewStore(fs, "queries", "storage", func() string { return *loc }, nil)
	tick := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	s.now = func() time.Time {
		tick = tick.Add(time.Minute)
		return tick
	}
	return s, fs
}

func newTestStore(t *testing.T) (*Store, *storeMemFS) {
	t.Helper()
	loc := LocationLocal
	return newTestStoreAt(t, &loc)
}

func sampleSaved(name string) SavedQuery {
	return SavedQuery{
		Name:     name,
		Template: "audit.yaml",
		Spec: Spec{
			Columns: []Column{
				{Header: "NBA-ID", Source: Source{Kind: "field", Key: "nba_id"}},
				{Header: "Categorie", Source: Source{Kind: "field", Key: "categorie"}},
			},
			Filters: []Filter{{Source: Source{Kind: "facet", Key: "flag"}, Op: "eq", Value: "IN SCOPE"}},
			OrderBy: []Sort{{Column: 0}},
			Limit:   25,
		},
	}
}

func TestStoreSaveGetRoundTrip(t *testing.T) {
	s, _ := newTestStore(t)

	saved, err := s.Save(sampleSaved("In scope by category"))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.ID != "in-scope-by-category" {
		t.Fatalf("derived id = %q, want in-scope-by-category", saved.ID)
	}

	got, err := s.Get("audit.yaml", saved.ID, LocationLocal)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != saved.Name || got.Template != "audit.yaml" {
		t.Fatalf("identity lost: %+v", got)
	}
	if len(got.Spec.Columns) != 2 || got.Spec.Columns[1].Source.Key != "categorie" {
		t.Fatalf("columns lost: %+v", got.Spec.Columns)
	}
	if len(got.Spec.Filters) != 1 || got.Spec.Filters[0].Value != "IN SCOPE" {
		t.Fatalf("filters lost: %+v", got.Spec.Filters)
	}
	if got.Spec.Limit != 25 {
		t.Fatalf("limit = %d, want 25", got.Spec.Limit)
	}
	if got.Spec.Template != "audit.yaml" {
		t.Fatalf("spec template = %q, want audit.yaml", got.Spec.Template)
	}
}

func TestStoreLocalLayoutStaysOutOfTheStorageTree(t *testing.T) {
	s, fs := newTestStore(t)
	if _, err := s.Save(sampleSaved("By category")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	want := filepath.Join("queries", "audit", "by-category.json")
	if _, ok := fs.files[want]; !ok {
		t.Fatalf("stored at %v, want %s", keys(fs.files), want)
	}
	for p := range fs.files {
		if strings.HasPrefix(p, "storage") {
			t.Fatalf("a local save reached the synced tree: %s", p)
		}
	}
}

func TestStoreSharedLayoutSitsInTheTemplateStorageFolder(t *testing.T) {
	loc := LocationShared
	s, fs := newTestStoreAt(t, &loc)
	saved, err := s.Save(sampleSaved("By category"))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.Location != LocationShared {
		t.Fatalf("location = %q, want shared", saved.Location)
	}
	want := filepath.Join("storage", "audit", "queries", "by-category.json")
	if _, ok := fs.files[want]; !ok {
		t.Fatalf("stored at %v, want %s", keys(fs.files), want)
	}
}

// The setting decides where a NEW save lands. It must never hide what is
// already stored: a peer's shared query stays visible to someone saving
// locally, and vice versa.
func TestStoreListsBothLocationsWhateverTheSetting(t *testing.T) {
	loc := LocationLocal
	s, _ := newTestStoreAt(t, &loc)

	mine := sampleSaved("Mine")
	if _, err := s.Save(mine); err != nil {
		t.Fatalf("Save local: %v", err)
	}
	theirs := sampleSaved("Theirs")
	theirs.Location = LocationShared
	if _, err := s.Save(theirs); err != nil {
		t.Fatalf("Save shared: %v", err)
	}

	for _, setting := range []string{LocationLocal, LocationShared} {
		loc = setting
		list, err := s.List("audit.yaml")
		if err != nil {
			t.Fatalf("List with setting %s: %v", setting, err)
		}
		if len(list) != 2 {
			t.Fatalf("setting %s: got %d saved queries, want both", setting, len(list))
		}
		if list[0].Name != "Mine" || list[0].Location != LocationLocal {
			t.Fatalf("setting %s: first = %+v, want the local one", setting, list[0])
		}
		if list[1].Name != "Theirs" || list[1].Location != LocationShared {
			t.Fatalf("setting %s: second = %+v, want the shared one", setting, list[1])
		}
	}
}

// Same id in both roots is two records, not one shadowing the other.
func TestStoreSameIdInBothLocationsStaysDistinct(t *testing.T) {
	loc := LocationLocal
	s, _ := newTestStoreAt(t, &loc)

	local := sampleSaved("Overlap")
	local.Spec.Limit = 1
	if _, err := s.Save(local); err != nil {
		t.Fatalf("Save local: %v", err)
	}
	shared := sampleSaved("Overlap")
	shared.Location = LocationShared
	shared.Spec.Limit = 2
	if _, err := s.Save(shared); err != nil {
		t.Fatalf("Save shared: %v", err)
	}

	list, err := s.List("audit.yaml")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d, want one per location", len(list))
	}
	gotLocal, err := s.Get("audit.yaml", "overlap", LocationLocal)
	if err != nil {
		t.Fatalf("Get local: %v", err)
	}
	gotShared, err := s.Get("audit.yaml", "overlap", LocationShared)
	if err != nil {
		t.Fatalf("Get shared: %v", err)
	}
	if gotLocal.Spec.Limit != 1 || gotShared.Spec.Limit != 2 {
		t.Fatalf("records crossed: local=%d shared=%d", gotLocal.Spec.Limit, gotShared.Spec.Limit)
	}
}

// Deleting the local copy must leave the shared one alone.
func TestStoreDeleteIsPerLocation(t *testing.T) {
	loc := LocationLocal
	s, _ := newTestStoreAt(t, &loc)
	if _, err := s.Save(sampleSaved("Overlap")); err != nil {
		t.Fatalf("Save local: %v", err)
	}
	shared := sampleSaved("Overlap")
	shared.Location = LocationShared
	if _, err := s.Save(shared); err != nil {
		t.Fatalf("Save shared: %v", err)
	}
	if err := s.Delete("audit.yaml", "overlap", LocationLocal); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	list, err := s.List("audit.yaml")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Location != LocationShared {
		t.Fatalf("got %+v, want only the shared one", list)
	}
}

// Re-saving a record keeps it in its own root: the setting is for new saves,
// so an opened shared query does not migrate into the local root behind the
// user's back (nor a local one into the synced tree).
func TestStoreResaveKeepsItsLocation(t *testing.T) {
	loc := LocationLocal
	s, fs := newTestStoreAt(t, &loc)
	shared := sampleSaved("Team query")
	shared.Location = LocationShared
	first, err := s.Save(shared)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	again := sampleSaved("Team query")
	again.ID = first.ID
	again.Location = first.Location
	again.Spec.Limit = 7
	stored, err := s.Save(again)
	if err != nil {
		t.Fatalf("re-Save: %v", err)
	}
	if stored.Location != LocationShared {
		t.Fatalf("location = %q, want shared", stored.Location)
	}
	if _, ok := fs.files[filepath.Join("queries", "audit", "team-query.json")]; ok {
		t.Fatal("re-save leaked a copy into the local root")
	}
	if stored.Created != first.Created {
		t.Fatalf("created drifted: %q -> %q", first.Created, stored.Created)
	}
}

// The location is positional: the folder says it, the file must not, or a
// synced copy could contradict where it actually landed.
func TestStoreLocationIsNotWrittenToTheFile(t *testing.T) {
	s, fs := newTestStore(t)
	if _, err := s.Save(sampleSaved("Positional only")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw := fs.files[filepath.Join("queries", "audit", "positional-only.json")]
	if strings.Contains(raw, `"location"`) {
		t.Fatalf("file carries a location key:\n%s", raw)
	}
}

func TestStoreDefaultLocationFollowsTheSetting(t *testing.T) {
	loc := LocationShared
	s, _ := newTestStoreAt(t, &loc)
	if got := s.DefaultLocation(); got != LocationShared {
		t.Fatalf("DefaultLocation = %q, want shared", got)
	}
	loc = "nonsense"
	if got := s.DefaultLocation(); got != LocationLocal {
		t.Fatalf("DefaultLocation = %q, want the local fallback", got)
	}
	saved, err := s.Save(sampleSaved("Fallback"))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.Location != LocationLocal {
		t.Fatalf("an unreadable setting published to %q", saved.Location)
	}
}

func TestNormalizeLocation(t *testing.T) {
	cases := map[string]string{
		"shared":    LocationShared,
		" shared ":  LocationShared,
		"local":     LocationLocal,
		"":          LocationLocal,
		"Shared":    LocationLocal,
		"synced":    LocationLocal,
		"../shared": LocationLocal,
	}
	for in, want := range cases {
		if got := NormalizeLocation(in); got != want {
			t.Errorf("NormalizeLocation(%q) = %q, want %q", in, got, want)
		}
	}
	if len(StoreLocations) != 2 || StoreLocations[0] != LocationLocal || StoreLocations[1] != LocationShared {
		t.Fatalf("StoreLocations = %v, want [local shared]", StoreLocations)
	}
}

func TestStoreSaveForcesSpecTemplate(t *testing.T) {
	s, _ := newTestStore(t)
	q := sampleSaved("Mismatch")
	q.Spec.Template = "someone-else.yaml"
	saved, err := s.Save(q)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.Spec.Template != "audit.yaml" {
		t.Fatalf("spec template = %q, want audit.yaml", saved.Spec.Template)
	}
}

func TestStoreSaveKeepsCreatedBumpsUpdated(t *testing.T) {
	s, _ := newTestStore(t)
	first, err := s.Save(sampleSaved("Keep me"))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	second := sampleSaved("Keep me")
	second.ID = first.ID
	second.Spec.Limit = 99
	again, err := s.Save(second)
	if err != nil {
		t.Fatalf("re-Save: %v", err)
	}
	if again.Created != first.Created {
		t.Fatalf("created drifted: %q -> %q", first.Created, again.Created)
	}
	if again.Updated == first.Updated {
		t.Fatalf("updated not bumped: %q", again.Updated)
	}
	got, err := s.Get("audit.yaml", first.ID, LocationLocal)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Spec.Limit != 99 {
		t.Fatalf("overwrite lost the new spec: limit = %d", got.Spec.Limit)
	}
}

func TestStoreListSortsByNameAndScopesToTemplate(t *testing.T) {
	s, _ := newTestStore(t)
	for _, n := range []string{"zebra", "Alpha", "middle"} {
		if _, err := s.Save(sampleSaved(n)); err != nil {
			t.Fatalf("Save %s: %v", n, err)
		}
	}
	other := sampleSaved("other template query")
	other.Template = "other.yaml"
	if _, err := s.Save(other); err != nil {
		t.Fatalf("Save other: %v", err)
	}

	list, err := s.List("audit.yaml")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("got %d saved queries, want 3", len(list))
	}
	want := []string{"Alpha", "middle", "zebra"}
	for i, w := range want {
		if list[i].Name != w {
			t.Fatalf("order[%d] = %q, want %q", i, list[i].Name, w)
		}
	}
}

func TestStoreListEmptyWhenNothingSaved(t *testing.T) {
	s, _ := newTestStore(t)
	list, err := s.List("audit.yaml")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("got %d, want 0", len(list))
	}
}

func TestStoreListSkipsUnreadableFile(t *testing.T) {
	s, fs := newTestStore(t)
	if _, err := s.Save(sampleSaved("Good one")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	fs.files[filepath.Join("queries", "audit", "broken.json")] = "{not json"
	fs.files[filepath.Join("queries", "audit", "notes.txt")] = "ignore me"

	list, err := s.List("audit.yaml")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Name != "Good one" {
		t.Fatalf("got %+v, want only the readable one", list)
	}
}

func TestStoreListPropagatesIOError(t *testing.T) {
	s, fs := newTestStore(t)
	fs.failOn = "list"
	if _, err := s.List("audit.yaml"); err == nil {
		t.Fatal("want error from a failing ListDir")
	}
}

func TestStoreDelete(t *testing.T) {
	s, fs := newTestStore(t)
	saved, err := s.Save(sampleSaved("Drop me"))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Delete("audit.yaml", saved.ID, LocationLocal); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(fs.files) != 0 {
		t.Fatalf("file survived: %v", keys(fs.files))
	}
	if err := s.Delete("audit.yaml", saved.ID, LocationLocal); err != nil {
		t.Fatalf("Delete twice: %v", err)
	}
}

func TestStoreGetMissing(t *testing.T) {
	s, _ := newTestStore(t)
	if _, err := s.Get("audit.yaml", "nope", LocationLocal); !errors.Is(err, ErrQueryNotFound) {
		t.Fatalf("err = %v, want ErrQueryNotFound", err)
	}
}

func TestStoreSaveRejectsBadInput(t *testing.T) {
	s, _ := newTestStore(t)
	cases := map[string]func(q *SavedQuery){
		"no name":         func(q *SavedQuery) { q.Name = "   " },
		"no template":     func(q *SavedQuery) { q.Template = "" },
		"no columns":      func(q *SavedQuery) { q.Spec.Columns = nil },
		"unslugged name":  func(q *SavedQuery) { q.Name = "***" },
		"id traversal":    func(q *SavedQuery) { q.ID = "../escape" },
		"id uppercase":    func(q *SavedQuery) { q.ID = "MyQuery" },
		"id leading dash": func(q *SavedQuery) { q.ID = "-lead" },
		"template path":   func(q *SavedQuery) { q.Template = "../../etc/passwd" },
		"template sep":    func(q *SavedQuery) { q.Template = "sub/audit.yaml" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			q := sampleSaved("Fine name")
			mutate(&q)
			if _, err := s.Save(q); err == nil {
				t.Fatalf("want error for %s", name)
			}
		})
	}
}

func TestStoreGetRejectsTraversal(t *testing.T) {
	s, _ := newTestStore(t)
	if _, err := s.Get("audit.yaml", "../../secret", LocationLocal); err == nil {
		t.Fatal("want error for a traversal id")
	}
	if err := s.Delete("audit.yaml", "../../secret", LocationLocal); err == nil {
		t.Fatal("want error for a traversal id")
	}
	if _, err := s.List("../../etc"); err == nil {
		t.Fatal("want error for a traversal template")
	}
}

func TestStoreSaveSurfacesWriteFailure(t *testing.T) {
	s, fs := newTestStore(t)
	fs.failOn = "audit"
	if _, err := s.Save(sampleSaved("Nope")); err == nil {
		t.Fatal("want error when the write fails")
	}
}

func TestStoreGroupedSpecRoundTrips(t *testing.T) {
	s, _ := newTestStore(t)
	q := sampleSaved("Counts per category")
	q.Spec.GroupBy = []int{1}
	q.Spec.Measures = []Measure{{Func: "count", Header: "Count"}, {Func: "sum", Source: Source{Kind: "field", Key: "cost"}, Header: "Cost"}}
	q.Spec.Distinct = true
	if _, err := s.Save(q); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Get("audit.yaml", "counts-per-category", LocationLocal)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Spec.GroupBy) != 1 || got.Spec.GroupBy[0] != 1 {
		t.Fatalf("groupBy lost: %+v", got.Spec.GroupBy)
	}
	if len(got.Spec.Measures) != 2 || got.Spec.Measures[1].Source.Key != "cost" {
		t.Fatalf("measures lost: %+v", got.Spec.Measures)
	}
	if !got.Spec.Distinct {
		t.Fatal("distinct lost")
	}
}

func TestStoreSlugify(t *testing.T) {
	cases := map[string]string{
		"In scope by category": "in-scope-by-category",
		"  Trim  me  ":         "trim-me",
		"A/B: 100% done!":      "a-b-100-done",
		"UPPER":                "upper",
		"---":                  "",
		"héllo wörld":          "h-llo-w-rld",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStoreConcurrentSaves(t *testing.T) {
	s, _ := newTestStore(t)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			q := sampleSaved("Shared name")
			q.Spec.Limit = n
			if _, err := s.Save(q); err != nil {
				t.Errorf("Save: %v", err)
			}
		}(i)
	}
	wg.Wait()
	list, err := s.List("audit.yaml")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("got %d saved queries, want 1", len(list))
	}
	if list[0].Created == "" || list[0].Updated == "" {
		t.Fatalf("timestamps missing: %+v", list[0])
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
