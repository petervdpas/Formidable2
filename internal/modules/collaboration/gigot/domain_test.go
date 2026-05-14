package gigot

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── fakeFS — minimal Filesystem stub satisfying the SaveFile surface.

type fakeFS struct {
	writes map[string]string
	fail   error
}

func newFakeFS() *fakeFS {
	return &fakeFS{writes: map[string]string{}}
}

func (f *fakeFS) SaveFile(path, content string) error {
	if f.fail != nil {
		return f.fail
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}
	f.writes[path] = content
	return nil
}

// ── GitBlobSha ──────────────────────────────────────────────────────

func TestGitBlobSha_EmptyMatchesGitsKnownHash(t *testing.T) {
	// git's empty blob hash is fixed: SHA1("blob 0\0").
	got := GitBlobSha(nil)
	want := "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391"
	if got != want {
		t.Fatalf("GitBlobSha(nil) = %q, want %q", got, want)
	}
}

func TestGitBlobSha_KnownNonEmpty(t *testing.T) {
	// "hello\n" → 1 hash git uses for the same content.
	got := GitBlobSha([]byte("hello\n"))
	want := "ce013625030ba8dba906f756967f9e9ca394464a"
	if got != want {
		t.Fatalf("GitBlobSha(hello) = %q, want %q", got, want)
	}
}

func TestGitBlobSha_DifferentBytesDifferentHashes(t *testing.T) {
	a := GitBlobSha([]byte("alpha"))
	b := GitBlobSha([]byte("beta"))
	if a == b {
		t.Fatal("hash collision on distinct inputs")
	}
}

// ── IsFormidablePath ────────────────────────────────────────────────

func TestIsFormidablePath_TemplateYaml(t *testing.T) {
	if !IsFormidablePath("templates/basic.yaml") {
		t.Fatal("templates/basic.yaml should be Formidable-managed")
	}
}

func TestIsFormidablePath_StorageNested(t *testing.T) {
	if !IsFormidablePath("storage/addresses/oak.meta.json") {
		t.Fatal("storage/<tpl>/<file> should be managed")
	}
	if !IsFormidablePath("storage/addresses/images/photo.jpg") {
		t.Fatal("storage subtree (incl. images) should be managed")
	}
}

func TestIsFormidablePath_RootAllowlist(t *testing.T) {
	for _, p := range []string{"README.md", ".gitignore"} {
		if !IsFormidablePath(p) {
			t.Fatalf("%q should be managed (root allowlist)", p)
		}
	}
}

func TestIsFormidablePath_RejectsOther(t *testing.T) {
	for _, p := range []string{
		"",
		".",
		"random.txt",
		"docs/intro.md",
		".formidable/sync.json",
		".formidable/context.json",
	} {
		if IsFormidablePath(p) {
			t.Errorf("%q should NOT be managed", p)
		}
	}
}

func TestIsFormidablePath_RejectsTraversal(t *testing.T) {
	for _, p := range []string{
		"..",
		"../etc/passwd",
		"templates/../../oops",
	} {
		if IsFormidablePath(p) {
			t.Errorf("%q traversal must be rejected", p)
		}
	}
}

// ── TrackRecord I/O ─────────────────────────────────────────────────

func TestReadTrackRecord_EmptyWhenContextBlank(t *testing.T) {
	got := ReadTrackRecord("")
	if got.Version != "" || got.LastSync != "" || len(got.Files) != 0 {
		t.Fatalf("blank context should yield empty record, got %+v", got)
	}
	if got.Files == nil {
		t.Fatal("Files must be non-nil so writers can index without nil-check")
	}
}

func TestReadTrackRecord_EmptyWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	got := ReadTrackRecord(dir)
	if got.Version != "" || len(got.Files) != 0 {
		t.Fatalf("missing file should yield empty record, got %+v", got)
	}
}

func TestReadTrackRecord_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	fs := newFakeFS()
	m := NewManager(fs)

	rec := TrackRecord{
		Version:  "abc123",
		LastSync: "2026-05-14T12:34:56Z",
		Files: map[string]string{
			"templates/basic.yaml":      "deadbeef",
			"storage/x/one.meta.json":   "cafef00d",
		},
	}
	if err := m.WriteTrackRecord(dir, rec); err != nil {
		t.Fatalf("WriteTrackRecord: %v", err)
	}

	got := ReadTrackRecord(dir)
	if got.Version != rec.Version {
		t.Errorf("version round-trip lost: got %q, want %q", got.Version, rec.Version)
	}
	if got.LastSync != rec.LastSync {
		t.Errorf("lastSync round-trip lost: got %q, want %q", got.LastSync, rec.LastSync)
	}
	if len(got.Files) != len(rec.Files) {
		t.Fatalf("files len differs: got %d, want %d", len(got.Files), len(rec.Files))
	}
	for k, v := range rec.Files {
		if got.Files[k] != v {
			t.Errorf("files[%q] = %q, want %q", k, got.Files[k], v)
		}
	}
}

func TestReadTrackRecord_CorruptYieldsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".formidable"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, ".formidable", "sync.json"),
		[]byte("not json{{{"), 0o644,
	); err != nil {
		t.Fatal(err)
	}
	got := ReadTrackRecord(dir)
	if got.Version != "" || len(got.Files) != 0 {
		t.Fatalf("corrupt file should self-heal to empty, got %+v", got)
	}
}

func TestWriteTrackRecord_RequiresFilesystem(t *testing.T) {
	m := &Manager{}
	err := m.WriteTrackRecord(t.TempDir(), EmptyTrackRecord())
	if err == nil {
		t.Fatal("write without filesystem should error")
	}
}

func TestWriteTrackRecord_RejectsBlankContext(t *testing.T) {
	m := NewManager(newFakeFS())
	if err := m.WriteTrackRecord("", EmptyTrackRecord()); !errors.Is(err, ErrMissingContext) {
		t.Fatalf("want ErrMissingContext, got %v", err)
	}
}

func TestWriteTrackRecord_NormalisesNilFilesMap(t *testing.T) {
	dir := t.TempDir()
	fs := newFakeFS()
	m := NewManager(fs)
	if err := m.WriteTrackRecord(dir, TrackRecord{Version: "v1"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(TrackRecordPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("written record not valid JSON: %v", err)
	}
	files, ok := back["files"]
	if !ok {
		t.Fatal("written record missing 'files' field")
	}
	if files == nil {
		t.Fatal("'files' should serialise as {} not null")
	}
}

// ── CollectFormidableFiles ──────────────────────────────────────────

func writeFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCollectFormidableFiles_BlankContextErrors(t *testing.T) {
	if _, err := CollectFormidableFiles(""); !errors.Is(err, ErrMissingContext) {
		t.Fatalf("want ErrMissingContext, got %v", err)
	}
}

func TestCollectFormidableFiles_PicksTemplatesStorageRootAllowlist(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "templates/basic.yaml", "name: basic")
	writeFile(t, dir, "templates/notes.yaml", "name: notes")
	writeFile(t, dir, "storage/addresses/oak.meta.json", `{"meta":{}}`)
	writeFile(t, dir, "storage/addresses/images/photo.jpg", "binary")
	writeFile(t, dir, "README.md", "# Hi")
	writeFile(t, dir, ".gitignore", ".formidable/sync.json")

	// Distractors that must not appear.
	writeFile(t, dir, "templates/README.md", "ignored — non-yaml")
	writeFile(t, dir, "templates/images/oops.png", "ignored — non-yaml")
	writeFile(t, dir, "notes.txt", "ignored — not allowlisted")
	writeFile(t, dir, ".formidable/sync.json", `{"version":""}`)
	writeFile(t, dir, ".formidable/context.json", `{"version":1}`)

	got, err := CollectFormidableFiles(dir)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]bool{
		"templates/basic.yaml":                  true,
		"templates/notes.yaml":                  true,
		"storage/addresses/oak.meta.json":       true,
		"storage/addresses/images/photo.jpg":    true,
		"README.md":                             true,
		".gitignore":                            true,
	}
	if len(got) != len(want) {
		t.Fatalf("walker returned %d files, want %d: %+v", len(got), len(want), pathsOf(got))
	}
	for _, f := range got {
		if !want[f.Path] {
			t.Errorf("unexpected path %q in walker output", f.Path)
		}
		if f.Sha == "" {
			t.Errorf("path %q has empty SHA", f.Path)
		}
		if len(f.Bytes) == 0 && f.Path != "templates/basic.yaml" {
			// basic.yaml is non-empty by construction; any zero-byte
			// match here means the walker mis-read the file.
			continue
		}
	}
}

func TestCollectFormidableFiles_SkipsDotFormidable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".formidable/sync.json", `{"version":"x"}`)
	writeFile(t, dir, ".formidable/context.json", `{"version":1}`)
	writeFile(t, dir, "templates/basic.yaml", "name: basic")

	got, err := CollectFormidableFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range got {
		if strings.HasPrefix(f.Path, ".formidable/") {
			t.Errorf(".formidable/ leaked into walker output: %q", f.Path)
		}
	}
}

func TestCollectFormidableFiles_EmptyContextReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	got, err := CollectFormidableFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("empty context should yield no files, got %+v", pathsOf(got))
	}
}

func TestCollectFormidableFiles_SortedByPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "templates/zeta.yaml", "z")
	writeFile(t, dir, "templates/alpha.yaml", "a")
	writeFile(t, dir, "storage/x/two.meta.json", "{}")
	writeFile(t, dir, "storage/x/one.meta.json", "{}")

	got, err := CollectFormidableFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Path > got[i].Path {
			t.Errorf("walker output not sorted: %q > %q", got[i-1].Path, got[i].Path)
		}
	}
}

// ── DiffAgainstRecord ───────────────────────────────────────────────

func TestDiff_NoChangesNoop(t *testing.T) {
	local := []LocalFile{
		{Path: "templates/basic.yaml", Sha: "aaa"},
		{Path: "storage/x/one.meta.json", Sha: "bbb"},
	}
	rec := TrackRecord{
		Version: "v1",
		Files: map[string]string{
			"templates/basic.yaml":    "aaa",
			"storage/x/one.meta.json": "bbb",
		},
	}
	d := DiffAgainstRecord(local, rec)
	if len(d.Changed) != 0 || len(d.Deleted) != 0 {
		t.Fatalf("identical state should be no-op, got %+v", d)
	}
}

func TestDiff_PutOnShaMismatch(t *testing.T) {
	local := []LocalFile{{Path: "templates/basic.yaml", Sha: "newhash"}}
	rec := TrackRecord{
		Version: "v1",
		Files:   map[string]string{"templates/basic.yaml": "oldhash"},
	}
	d := DiffAgainstRecord(local, rec)
	if len(d.Changed) != 1 || d.Changed[0].Path != "templates/basic.yaml" {
		t.Fatalf("expected single change, got %+v", d.Changed)
	}
}

func TestDiff_DeleteForVanishedManagedPath(t *testing.T) {
	local := []LocalFile{}
	rec := TrackRecord{
		Version: "v1",
		Files: map[string]string{
			"templates/basic.yaml": "old",
			"random.txt":           "old", // not managed → must not appear in deletes
		},
	}
	d := DiffAgainstRecord(local, rec)
	if len(d.Deleted) != 1 || d.Deleted[0] != "templates/basic.yaml" {
		t.Fatalf("expected delete for templates/basic.yaml only, got %+v", d.Deleted)
	}
}

func TestDiff_FirstSyncSuppressesDeletes(t *testing.T) {
	local := []LocalFile{}
	rec := TrackRecord{
		Version: "", // first sync — record was seeded but never committed
		Files:   map[string]string{"templates/basic.yaml": "old"},
	}
	d := DiffAgainstRecord(local, rec)
	if len(d.Deleted) != 0 {
		t.Fatalf("first sync must not produce deletes, got %+v", d.Deleted)
	}
}

func TestDiff_NewLocalFileNotInRecordIsChange(t *testing.T) {
	local := []LocalFile{{Path: "storage/new/fresh.meta.json", Sha: "zzz"}}
	rec := TrackRecord{Version: "v1", Files: map[string]string{}}
	d := DiffAgainstRecord(local, rec)
	if len(d.Changed) != 1 {
		t.Fatalf("new local file should be a change, got %+v", d.Changed)
	}
}

// ── helpers ─────────────────────────────────────────────────────────

func pathsOf(files []LocalFile) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Path
	}
	return out
}

// ── validateConn ────────────────────────────────────────────────────

func TestValidateConn_RequiresBaseURL(t *testing.T) {
	if err := validateConn(Connection{Token: "t"}, false); !errors.Is(err, ErrMissingBaseURL) {
		t.Fatalf("want ErrMissingBaseURL, got %v", err)
	}
}

func TestValidateConn_RequiresToken(t *testing.T) {
	if err := validateConn(Connection{BaseURL: "https://x"}, false); !errors.Is(err, ErrMissingToken) {
		t.Fatalf("want ErrMissingToken, got %v", err)
	}
}

func TestValidateConn_RepoRequiredForScopedOps(t *testing.T) {
	conn := Connection{BaseURL: "https://x", Token: "t"}
	if err := validateConn(conn, true); !errors.Is(err, ErrMissingRepo) {
		t.Fatalf("want ErrMissingRepo, got %v", err)
	}
}

func TestValidateConn_HappyPath(t *testing.T) {
	conn := Connection{BaseURL: "https://x", Token: "t", RepoName: "r"}
	if err := validateConn(conn, true); err != nil {
		t.Fatalf("happy path errored: %v", err)
	}
}

