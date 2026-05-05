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
// final file content must equal exactly one of the inputs — never a
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
// in parallel; readers must always observe a complete, valid input —
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
