package system

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newTestManager(t *testing.T) (*Manager, string) {
	t.Helper()
	root := t.TempDir()
	return NewManager(root, nil), root
}

func TestResolvePath_RelativeBecomesAbsolute(t *testing.T) {
	m, root := newTestManager(t)
	got := m.ResolvePath("foo", "bar")
	want := filepath.Join(root, "foo", "bar")
	if got != want {
		t.Fatalf("ResolvePath = %q, want %q", got, want)
	}
}

func TestResolvePath_AbsolutePathPreserved(t *testing.T) {
	m, _ := newTestManager(t)
	abs := filepath.Join(t.TempDir(), "elsewhere")
	got := m.ResolvePath(abs)
	if got != abs {
		t.Fatalf("ResolvePath = %q, want %q", got, abs)
	}
}

func TestMakeAppRootRelative_UnderRootBecomesDotSlash(t *testing.T) {
	// Picker output is always absolute; if it lands under AppRoot we
	// store the human-friendly "./<rel>" form so the same profile
	// stays portable across machines that share the AppRoot
	// convention.
	m, root := newTestManager(t)
	in := filepath.Join(root, "Examples")
	got := m.MakeAppRootRelative(in)
	want := "./Examples"
	if got != want {
		t.Fatalf("MakeAppRootRelative(%q) = %q, want %q", in, got, want)
	}
}

func TestMakeAppRootRelative_NestedPath(t *testing.T) {
	m, root := newTestManager(t)
	in := filepath.Join(root, "data", "templates", "x")
	got := m.MakeAppRootRelative(in)
	want := "./" + filepath.Join("data", "templates", "x")
	if got != want {
		t.Fatalf("MakeAppRootRelative(%q) = %q, want %q", in, got, want)
	}
}

func TestMakeAppRootRelative_RootItselfBecomesDot(t *testing.T) {
	m, root := newTestManager(t)
	got := m.MakeAppRootRelative(root)
	if got != "." {
		t.Fatalf("MakeAppRootRelative(root) = %q, want %q", got, ".")
	}
}

func TestMakeAppRootRelative_OutsideRootStaysAbsolute(t *testing.T) {
	// A folder that doesn't sit under AppRoot must round-trip
	// unchanged - collapsing to "../../foo" would be brittle and
	// breaks the "absolute path = picker output" guarantee.
	m, _ := newTestManager(t)
	outside := filepath.Join(t.TempDir(), "elsewhere")
	got := m.MakeAppRootRelative(outside)
	if got != outside {
		t.Fatalf("MakeAppRootRelative(%q) = %q, want unchanged", outside, got)
	}
}

func TestMakeAppRootRelative_EmptyStaysEmpty(t *testing.T) {
	m, _ := newTestManager(t)
	if got := m.MakeAppRootRelative(""); got != "" {
		t.Fatalf("MakeAppRootRelative(\"\") = %q, want empty", got)
	}
}

func TestMakeAppRootRelative_AlreadyRelativeReturnsAsIs(t *testing.T) {
	// Defensive: if a relative path leaks in (UI bug, future
	// migration), return it untouched rather than misinterpret it
	// as an absolute path outside AppRoot.
	m, _ := newTestManager(t)
	if got := m.MakeAppRootRelative("./Examples"); got != "./Examples" {
		t.Fatalf("got %q, want unchanged", got)
	}
}

func TestResolveAbsolutePath_Empty(t *testing.T) {
	// Empty in, empty out - never coerce nothing into a path. The
	// path-field components rely on this so a freshly created field
	// keeps an unset value rather than auto-populating to cwd.
	m, _ := newTestManager(t)
	got, err := m.ResolveAbsolutePath("")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestResolveAbsolutePath_AbsoluteIsCleaned(t *testing.T) {
	m, _ := newTestManager(t)
	got, err := m.ResolveAbsolutePath("/var/log/../tmp/x")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "/var/tmp/x" {
		t.Fatalf("got %q, want /var/tmp/x", got)
	}
}

func TestResolveAbsolutePath_TildeExpands(t *testing.T) {
	// `~` and `~/sub` expand to the user's home dir. Anything else
	// starting with `~` (e.g. `~someuser`) is left alone - that's
	// shell-only sugar we're not trying to reimplement.
	m, _ := newTestManager(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no user home in this env")
	}
	tilde, err := m.ResolveAbsolutePath("~")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if tilde != home {
		t.Fatalf("`~` = %q, want %q", tilde, home)
	}
	sub, err := m.ResolveAbsolutePath("~/Documents")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := filepath.Join(home, "Documents")
	if sub != want {
		t.Fatalf("`~/Documents` = %q, want %q", sub, want)
	}
}

func TestResolveAbsolutePath_RelativeUsesCwd(t *testing.T) {
	m, _ := newTestManager(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	got, err := m.ResolveAbsolutePath("foo/bar")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := filepath.Join(cwd, "foo/bar")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestEnsureDirectory_CreatesNested(t *testing.T) {
	m, root := newTestManager(t)
	if err := m.EnsureDirectory("a/b/c"); err != nil {
		t.Fatalf("EnsureDirectory: %v", err)
	}
	info, err := os.Stat(filepath.Join(root, "a/b/c"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected directory")
	}
}

func TestAppendFile_CreatesAndAppends(t *testing.T) {
	m, _ := newTestManager(t)
	if err := m.AppendFile("log/x.log", "line1\n"); err != nil {
		t.Fatalf("first AppendFile: %v", err)
	}
	if err := m.AppendFile("log/x.log", "line2\n"); err != nil {
		t.Fatalf("second AppendFile: %v", err)
	}
	got, err := m.LoadFile("log/x.log")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if got != "line1\nline2\n" {
		t.Fatalf("AppendFile content = %q, want %q", got, "line1\nline2\n")
	}
}

func TestSaveLoadFile_RoundTrip(t *testing.T) {
	m, _ := newTestManager(t)
	const content = "hello\nworld\n"
	if err := m.SaveFile("dir/file.txt", content); err != nil {
		t.Fatalf("SaveFile: %v", err)
	}
	got, err := m.LoadFile("dir/file.txt")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if got != content {
		t.Fatalf("LoadFile = %q, want %q", got, content)
	}
}

func TestLoadFile_Missing(t *testing.T) {
	m, _ := newTestManager(t)
	if _, err := m.LoadFile("nope.txt"); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestFileExists(t *testing.T) {
	m, _ := newTestManager(t)
	if m.FileExists("missing.txt") {
		t.Fatal("expected false for missing")
	}
	if err := m.SaveFile("present.txt", "x"); err != nil {
		t.Fatalf("save: %v", err)
	}
	if !m.FileExists("present.txt") {
		t.Fatal("expected true after save")
	}
}

func TestDeleteFile_NoOpOnMissing(t *testing.T) {
	m, _ := newTestManager(t)
	if err := m.DeleteFile("nope.txt"); err != nil {
		t.Fatalf("DeleteFile on missing: %v", err)
	}
}

func TestDeleteFile_Removes(t *testing.T) {
	m, _ := newTestManager(t)
	_ = m.SaveFile("x.txt", "x")
	if err := m.DeleteFile("x.txt"); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if m.FileExists("x.txt") {
		t.Fatal("file should be gone")
	}
}

func TestEmptyFolder_RemovesContents(t *testing.T) {
	m, root := newTestManager(t)
	_ = m.SaveFile("dir/a.txt", "a")
	_ = m.SaveFile("dir/sub/b.txt", "b")
	if err := m.EmptyFolder("dir"); err != nil {
		t.Fatalf("EmptyFolder: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "dir"))
	if len(entries) != 0 {
		t.Fatalf("expected empty, got %d entries", len(entries))
	}
}

func TestDeleteFolder_RemovesRecursively(t *testing.T) {
	m, root := newTestManager(t)
	_ = m.SaveFile("d/a.txt", "a")
	_ = m.SaveFile("d/sub/b.txt", "b")
	if err := m.DeleteFolder("d"); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "d")); !os.IsNotExist(err) {
		t.Fatal("folder should be gone")
	}
}

func TestCopyFile_OverwriteAndSkip(t *testing.T) {
	m, _ := newTestManager(t)
	_ = m.SaveFile("src.txt", "v1")
	_ = m.SaveFile("dst.txt", "existing")

	// overwrite=false should leave existing content
	if err := m.CopyFile("src.txt", "dst.txt", false); err != nil {
		t.Fatalf("CopyFile no-overwrite: %v", err)
	}
	got, _ := m.LoadFile("dst.txt")
	if got != "existing" {
		t.Fatalf("expected dst preserved, got %q", got)
	}

	// overwrite=true replaces
	if err := m.CopyFile("src.txt", "dst.txt", true); err != nil {
		t.Fatalf("CopyFile overwrite: %v", err)
	}
	got, _ = m.LoadFile("dst.txt")
	if got != "v1" {
		t.Fatalf("expected dst overwritten, got %q", got)
	}
}

func TestCopyFolder_PreservesTree(t *testing.T) {
	m, _ := newTestManager(t)
	_ = m.SaveFile("src/a.txt", "a")
	_ = m.SaveFile("src/sub/b.txt", "b")
	if err := m.CopyFolder("src", "dst", true); err != nil {
		t.Fatalf("CopyFolder: %v", err)
	}
	a, _ := m.LoadFile("dst/a.txt")
	b, _ := m.LoadFile("dst/sub/b.txt")
	if a != "a" || b != "b" {
		t.Fatalf("copied tree mismatch: a=%q b=%q", a, b)
	}
}

func TestListFiles_FoldersAndDirEntries(t *testing.T) {
	m, _ := newTestManager(t)
	_ = m.SaveFile("d/file1.txt", "x")
	_ = m.SaveFile("d/file2.txt", "y")
	_ = m.EnsureDirectory("d/sub")

	files, err := m.ListFiles("d")
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("ListFiles got %d, want 2", len(files))
	}

	folders, _ := m.ListFolders("d")
	if len(folders) != 1 || folders[0] != "sub" {
		t.Fatalf("ListFolders got %v, want [sub]", folders)
	}

	entries, _ := m.ListDirectoryEntries("d")
	if len(entries) != 3 {
		t.Fatalf("ListDirectoryEntries got %d, want 3", len(entries))
	}
}

func TestWalkFiles(t *testing.T) {
	m, root := newTestManager(t)
	_ = m.SaveFile("w/a.txt", "a")
	_ = m.SaveFile("w/sub/b.txt", "b")
	files, err := m.WalkFiles("w")
	if err != nil {
		t.Fatalf("WalkFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("WalkFiles got %d, want 2", len(files))
	}
	for _, f := range files {
		if !strings.HasPrefix(f, root) {
			t.Fatalf("expected absolute path under %s, got %s", root, f)
		}
	}
}

func TestExecuteCommand_BasicEcho(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping shell test in short mode")
	}
	m, _ := newTestManager(t)
	out, err := m.ExecuteCommand("echo hello")
	if err != nil {
		t.Fatalf("ExecuteCommand: %v", err)
	}
	if !strings.Contains(out, "hello") {
		t.Fatalf("expected output to contain 'hello', got %q", out)
	}
}

func TestExecuteCommand_RejectsEmpty(t *testing.T) {
	m, _ := newTestManager(t)
	if _, err := m.ExecuteCommand("   "); err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestProxyFetchRemote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test") != "yes" {
			t.Errorf("expected X-Test header, got %q", r.Header.Get("X-Test"))
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok body"))
	}))
	defer srv.Close()

	m, _ := newTestManager(t)
	res, err := m.ProxyFetchRemote(srv.URL, FetchOptions{
		Method:  "GET",
		Headers: map[string]string{"X-Test": "yes"},
	})
	if err != nil {
		t.Fatalf("ProxyFetchRemote: %v", err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if res.Body != "ok body" {
		t.Fatalf("body = %q", res.Body)
	}
	if !strings.Contains(res.Headers["Content-Type"], "text/plain") {
		t.Fatalf("missing Content-Type: %v", res.Headers)
	}
}

// stubJournal records emit calls so we can verify SaveFile/DeleteFile etc.
// fan out to the journal hook when one is wired.
type stubJournal struct {
	mu  sync.Mutex
	ops []journalCall
}

type journalCall struct {
	op   string
	path string
}

func (s *stubJournal) RecordOp(op, path string, _ map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ops = append(s.ops, journalCall{op, path})
}

func (s *stubJournal) calls() []journalCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]journalCall, len(s.ops))
	copy(out, s.ops)
	return out
}

func TestJournalHook_EmitsOnMutation(t *testing.T) {
	m, _ := newTestManager(t)
	j := &stubJournal{}
	m.SetJournal(j)

	if err := m.SaveFile("a.txt", "x"); err != nil {
		t.Fatal(err)
	}
	if err := m.SaveFile("a.txt", "y"); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteFile("a.txt"); err != nil {
		t.Fatal(err)
	}

	calls := j.calls()
	if len(calls) != 3 {
		t.Fatalf("expected 3 journal calls, got %d", len(calls))
	}
	wantOps := []string{"create", "update", "delete"}
	for i, c := range calls {
		if c.op != wantOps[i] {
			t.Errorf("call[%d].op = %q, want %q", i, c.op, wantOps[i])
		}
	}
}

func TestJournalHook_NilIsSafe(t *testing.T) {
	m, _ := newTestManager(t)
	// no journal wired - must not panic
	if err := m.SaveFile("x.txt", "y"); err != nil {
		t.Fatalf("SaveFile without journal: %v", err)
	}
}
