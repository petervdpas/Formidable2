package system

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// ── Happy paths ──────────────────────────────────────────────────────

func TestAtomicWrite_RoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "x.txt")

	if err := atomicWriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatalf("atomicWriteFile: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("content = %q, want %q", got, "hello")
	}
}

func TestAtomicWrite_OverwriteExisting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := atomicWriteFile(target, []byte("new"), 0o644); err != nil {
		t.Fatalf("atomicWriteFile: %v", err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "new" {
		t.Fatalf("content = %q, want %q", got, "new")
	}
}

func TestAtomicWrite_NoTempResidueOnSuccess(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "x.txt")
	if err := atomicWriteFile(target, []byte("ok"), 0o644); err != nil {
		t.Fatalf("atomicWriteFile: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("dir entries after save = %d, want 1; got %v", len(entries), entries)
	}
	if entries[0].Name() != "x.txt" {
		t.Fatalf("unexpected file in dir: %q", entries[0].Name())
	}
}

func TestAtomicWrite_LargeContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "big.bin")
	// 5 MiB of pseudo-content. Big enough to exercise multi-page write +
	// fsync, small enough to stay fast under -race.
	content := strings.Repeat("abcdefgh", 5*1024*1024/8)
	if err := atomicWriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatalf("atomicWriteFile: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != len(content) || string(got) != content {
		t.Fatalf("content mismatch: got %d bytes, want %d", len(got), len(content))
	}
}

// ── Concurrent writers (the headline atomic guarantee) ───────────────

// TestSaveFile_ConcurrentWritersAllValid fires N goroutines, each
// saving distinct content to the SAME path. After all complete, the
// final file content must equal exactly one of the inputs - never a
// partial mix or an empty/truncated file.
func TestSaveFile_ConcurrentWritersAllValid(t *testing.T) {
	t.Parallel()
	m, _ := newTestManager(t)

	const N = 30
	inputs := make([]string, N)
	for i := range N {
		// Each input has a unique payload + a checksum-friendly tail
		// so we can verify "is one of the inputs" cheaply.
		inputs[i] = strings.Repeat(string(rune('a'+i%26)), 1024) + "\n#" + strings.Repeat("z", i)
	}
	want := make(map[string]bool, N)
	for _, s := range inputs {
		want[s] = true
	}

	var wg sync.WaitGroup
	for i := range N {
		wg.Go(func() {
			if err := m.SaveFile("shared.txt", inputs[i]); err != nil {
				t.Errorf("SaveFile #%d: %v", i, err)
			}
		})
	}
	wg.Wait()

	final, err := m.LoadFile("shared.txt")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if !want[final] {
		t.Fatalf("final content is not one of the inputs (length %d)", len(final))
	}
}

// TestSaveFile_ConcurrentReadersNeverSeePartial fires writers + readers
// in parallel; readers must always observe a complete, valid input -
// never an empty or partially-written file. Stress-y but bounded.
func TestSaveFile_ConcurrentReadersNeverSeePartial(t *testing.T) {
	t.Parallel()
	m, _ := newTestManager(t)

	// Seed once so the very first read still sees a valid value.
	const seed = "SEED"
	if err := m.SaveFile("share.txt", seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	inputs := []string{seed, "AAAA", "BBBB", "CCCC", "DDDD"}
	valid := make(map[string]bool, len(inputs))
	for _, s := range inputs {
		valid[s] = true
	}

	var stop atomic.Bool
	var wg sync.WaitGroup

	// Writers: one per non-seed input.
	for i := range len(inputs) - 1 {
		content := inputs[1+i]
		wg.Go(func() {
			for range 50 {
				if stop.Load() {
					return
				}
				_ = m.SaveFile("share.txt", content)
			}
		})
	}

	// Readers
	for range 5 {
		wg.Go(func() {
			for range 100 {
				if stop.Load() {
					return
				}
				got, err := m.LoadFile("share.txt")
				if err != nil {
					t.Errorf("LoadFile: %v", err)
					stop.Store(true)
					return
				}
				if !valid[got] {
					t.Errorf("partial read observed: %q", got)
					stop.Store(true)
					return
				}
			}
		})
	}

	wg.Wait()
}

// ── Unhappy paths ────────────────────────────────────────────────────

// TestAtomicWrite_FailedStreamLeavesTargetIntact: pre-write target
// content, then attempt an atomicWriteStream whose writer fails. The
// original target content must remain unchanged and no temp residue
// must remain.
func TestAtomicWrite_FailedStreamLeavesTargetIntact(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "x.txt")
	original := "ORIGINAL"
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	wantErr := errors.New("simulated mid-stream failure")
	got := atomicWriteStream(target, 0o644, func(w io.Writer) error {
		_, _ = w.Write([]byte("partial..."))
		return wantErr
	})
	if !errors.Is(got, wantErr) {
		t.Fatalf("err = %v, want %v", got, wantErr)
	}

	final, _ := os.ReadFile(target)
	if string(final) != original {
		t.Fatalf("target content changed after failed write: got %q, want %q", final, original)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temp residue left: %v", entries)
	}
}

// TestSaveFile_TargetIsDirectoryFails: atomic write should not silently
// turn a directory into a file. Useful sanity check for misconfigured
// paths.
func TestSaveFile_TargetIsDirectoryFails(t *testing.T) {
	t.Parallel()
	m, root := newTestManager(t)
	if err := os.MkdirAll(filepath.Join(root, "blocked"), 0o755); err != nil {
		t.Fatalf("seed dir: %v", err)
	}
	if err := m.SaveFile("blocked", "oops"); err == nil {
		t.Fatalf("expected error saving over a directory; got nil")
	}
}

// TestAtomicWrite_PermissionApplied: explicit perm honored on rename.
func TestAtomicWrite_PermissionApplied(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "x.txt")
	if err := atomicWriteFile(target, []byte("p"), 0o600); err != nil {
		t.Fatalf("atomicWriteFile: %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %v, want 0600", info.Mode().Perm())
	}
}

// noTempResidue asserts that no .tmp- hidden temp file from atomicWriteStream
// remains in dir after a write settles.
func noTempResidue(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("stray temp file left behind: %q", e.Name())
		}
	}
}

// TestSaveFile_CreatesMissingDirectory: SaveFile must MkdirAll the parent.
// Asserts the file lands with exact content and no temp residue.
func TestSaveFile_CreatesMissingDirectory(t *testing.T) {
	t.Parallel()
	m, root := newTestManager(t)

	if err := m.SaveFile(filepath.Join("a", "b", "c", "deep.txt"), "payload"); err != nil {
		t.Fatalf("SaveFile into missing dir: %v", err)
	}
	full := filepath.Join(root, "a", "b", "c", "deep.txt")
	got, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "payload" {
		t.Fatalf("content = %q, want %q", got, "payload")
	}
	noTempResidue(t, filepath.Dir(full))
}

// TestSaveFile_EmptyContent: zero-length content is a valid whole state.
// Asserts the file exists with exactly 0 bytes, not a missing file.
func TestSaveFile_EmptyContent(t *testing.T) {
	t.Parallel()
	m, root := newTestManager(t)

	if err := m.SaveFile("empty.txt", ""); err != nil {
		t.Fatalf("SaveFile empty: %v", err)
	}
	full := filepath.Join(root, "empty.txt")
	info, err := os.Stat(full)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("size = %d, want 0", info.Size())
	}
	noTempResidue(t, root)
}

// TestSaveFile_OverwriteEmptyTruncates: overwriting non-empty content with
// empty must leave exactly 0 bytes, never the old tail.
func TestSaveFile_OverwriteEmptyTruncates(t *testing.T) {
	t.Parallel()
	m, root := newTestManager(t)

	if err := m.SaveFile("f.txt", "0123456789"); err != nil {
		t.Fatalf("seed save: %v", err)
	}
	if err := m.SaveFile("f.txt", ""); err != nil {
		t.Fatalf("overwrite empty: %v", err)
	}
	info, err := os.Stat(filepath.Join(root, "f.txt"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("size after empty overwrite = %d, want 0", info.Size())
	}
}

// TestSaveFile_LargeContent: ~8 MiB through SaveFile (MkdirAll + atomic +
// emit path), asserts exact byte length and prefix/suffix integrity.
func TestSaveFile_LargeContent(t *testing.T) {
	t.Parallel()
	m, root := newTestManager(t)

	const size = 8 * 1024 * 1024
	content := strings.Repeat("0123456789abcdef", size/16)
	if err := m.SaveFile("big.bin", content); err != nil {
		t.Fatalf("SaveFile large: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "big.bin"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != len(content) {
		t.Fatalf("len = %d, want %d", len(got), len(content))
	}
	if string(got) != content {
		t.Fatalf("content mismatch at large size")
	}
	noTempResidue(t, root)
}

// TestSaveFile_TraversalEscapesRoot documents the ACTUAL current behavior:
// SaveFile resolves "../escape.txt" relative to AppRoot via filepath.Join,
// so the cleaned path lands OUTSIDE the root. SaveFile does not clamp to
// AppRoot. See suspectedBugs. We assert the escape so a future clamp fix
// flips this test deliberately.
func TestSaveFile_TraversalRejected(t *testing.T) {
	t.Parallel()
	m, root := newTestManager(t)

	// A relative key with .. that resolves outside the app root must be rejected.
	if err := m.SaveFile(filepath.Join("..", "escape.txt"), "out"); err == nil {
		t.Fatal("SaveFile traversal err = nil, want rejection")
	}
	// Nothing written outside root.
	outside := filepath.Join(filepath.Dir(root), "escape.txt")
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		_ = os.Remove(outside)
		t.Fatalf("file escaped to %q; stat err = %v", outside, err)
	}
	// Nor inside it.
	if _, err := os.Stat(filepath.Join(root, "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("unexpected file under root; stat err = %v", err)
	}
}

// TestSaveFile_ReadOnlyParentDirFails: when the target's parent directory is
// read-only, the atomic CreateTemp must fail and SaveFile must surface a
// permission error. Skipped when running as root (perm bits ignored).
func TestSaveFile_ReadOnlyParentDirFails(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("running as root: directory perm bits do not block writes")
	}
	m, root := newTestManager(t)

	roDir := filepath.Join(root, "ro")
	if err := os.Mkdir(roDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Pre-create the target so SaveFile's MkdirAll on the existing dir is a
	// no-op, then strip write perms so CreateTemp inside it fails.
	if err := os.WriteFile(filepath.Join(roDir, "t.txt"), []byte("seed"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := os.Chmod(roDir, 0o555); err != nil {
		t.Fatalf("chmod ro: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })

	err := m.SaveFile(filepath.Join("ro", "t.txt"), "new")
	if err == nil {
		t.Fatalf("expected permission error writing into read-only dir; got nil")
	}
	if !os.IsPermission(err) {
		t.Fatalf("err = %v, want a permission error", err)
	}
	// Original content must be untouched.
	got, _ := os.ReadFile(filepath.Join(roDir, "t.txt"))
	if string(got) != "seed" {
		t.Fatalf("original clobbered after failed write: %q", got)
	}
}

// TestSaveFile_ConcurrentSameKeyNoTorn drives many goroutines writing
// distinct fixed-length payloads to one key, then asserts the settled file
// is byte-for-byte one of the inputs and that no temp residue survived. The
// distinct lengths make a torn write (mixed prefix/suffix) detectable.
func TestSaveFile_ConcurrentSameKeyNoTorn(t *testing.T) {
	t.Parallel()
	m, root := newTestManager(t)

	const N = 40
	inputs := make([]string, N)
	valid := make(map[string]bool, N)
	for i := range N {
		// Length grows with i so any torn write yields an off-length or
		// out-of-set string.
		inputs[i] = strings.Repeat(string(rune('A'+i%26)), 64+i*7)
		valid[inputs[i]] = true
	}

	var wg sync.WaitGroup
	for i := range N {
		wg.Go(func() {
			if err := m.SaveFile("hot.txt", inputs[i]); err != nil {
				t.Errorf("SaveFile #%d: %v", i, err)
			}
		})
	}
	wg.Wait()

	final, err := os.ReadFile(filepath.Join(root, "hot.txt"))
	if err != nil {
		t.Fatalf("read final: %v", err)
	}
	if !valid[string(final)] {
		t.Fatalf("settled content is not one of the inputs (len %d)", len(final))
	}
	noTempResidue(t, root)
}

// TestSaveFile_ConcurrentDistinctKeysIsolated writes K keys concurrently;
// each settled file must hold exactly its own intended content. Verifies
// per-key isolation and that concurrent CreateTemp calls in the same
// directory do not collide or leave residue.
func TestSaveFile_ConcurrentDistinctKeysIsolated(t *testing.T) {
	t.Parallel()
	m, root := newTestManager(t)

	const K = 50
	want := make(map[string]string, K)
	for i := range K {
		name := "k" + strings.Repeat("x", i%5) + "_" + itoa(i) + ".txt"
		want[name] = "content-" + itoa(i) + "-" + strings.Repeat("Z", i)
	}

	var wg sync.WaitGroup
	for name, content := range want {
		n, c := name, content
		wg.Go(func() {
			if err := m.SaveFile(n, c); err != nil {
				t.Errorf("SaveFile %q: %v", n, err)
			}
		})
	}
	wg.Wait()

	for name, content := range want {
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %q: %v", name, err)
		}
		if string(got) != content {
			t.Fatalf("key %q content = %q, want %q", name, got, content)
		}
	}
	noTempResidue(t, root)
}

// metaJournal captures the meta map passed to RecordOp so a test can assert
// the exact byte count SaveFile emits. The package's stubJournal drops meta,
// so this is a distinct local type.
type metaJournal struct {
	mu   sync.Mutex
	ops  []string
	meta []map[string]any
}

func (j *metaJournal) RecordOp(op, _ string, meta map[string]any) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.ops = append(j.ops, op)
	j.meta = append(j.meta, meta)
}

func (j *metaJournal) snapshot() ([]string, []map[string]any) {
	j.mu.Lock()
	defer j.mu.Unlock()
	ops := make([]string, len(j.ops))
	copy(ops, j.ops)
	meta := make([]map[string]any, len(j.meta))
	copy(meta, j.meta)
	return ops, meta
}

// TestSaveFile_EmitsExactByteCount: SaveFile must journal the exact content
// length in meta["bytes"], create-then-update, so the journal reflects the
// real write size, not a stale or rounded value.
func TestSaveFile_EmitsExactByteCount(t *testing.T) {
	t.Parallel()
	m, _ := newTestManager(t)
	j := &metaJournal{}
	m.SetJournal(j)

	if err := m.SaveFile("note.txt", "hello"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := m.SaveFile("note.txt", "longer-content"); err != nil {
		t.Fatalf("update: %v", err)
	}

	ops, meta := j.snapshot()
	if len(ops) != 2 {
		t.Fatalf("ops = %d, want 2", len(ops))
	}
	if ops[0] != "create" || ops[1] != "update" {
		t.Fatalf("ops = %v, want [create update]", ops)
	}
	if got := meta[0]["bytes"]; got != len("hello") {
		t.Fatalf("create bytes = %v, want %d", got, len("hello"))
	}
	if got := meta[1]["bytes"]; got != len("longer-content") {
		t.Fatalf("update bytes = %v, want %d", got, len("longer-content"))
	}
}

// TestSaveFile_EmptyPathFailsAndPreservesRoot drives the boundary where the
// key resolves to AppRoot itself. The atomic rename targets the existing,
// non-empty root directory and must fail; AppRoot must survive as a directory.
func TestSaveFile_EmptyPathFailsAndPreservesRoot(t *testing.T) {
	t.Parallel()
	m, root := newTestManager(t)
	// Put one entry under root so it is a non-empty directory.
	if err := m.SaveFile("keep.txt", "k"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := m.SaveFile("", "should-not-write"); err == nil {
		t.Fatalf("expected error saving to empty key (resolves to AppRoot); got nil")
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatalf("stat root: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("AppRoot is no longer a directory after empty-key save")
	}
	// The seeded file must still be intact.
	got, err := os.ReadFile(filepath.Join(root, "keep.txt"))
	if err != nil {
		t.Fatalf("read seeded file: %v", err)
	}
	if string(got) != "k" {
		t.Fatalf("seeded content = %q, want %q", got, "k")
	}
	noTempResidue(t, filepath.Dir(root))
}

// itoa is a tiny base-10 formatter, avoiding an fmt import for filenames.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
