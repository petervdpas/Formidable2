package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// kvTestFS is the bare fs surface KV needs. Real wiring uses
// *system.Manager (see app.go). Tests use a tiny implementation
// that talks to t.TempDir() directly so we don't pull in the
// full system module.
type kvTestFS struct{}

func (kvTestFS) EnsureDirectory(p string) error { return os.MkdirAll(p, 0o755) }
func (kvTestFS) FileExists(p string) bool       { _, err := os.Stat(p); return err == nil }
func (kvTestFS) IsDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
func (kvTestFS) LoadFile(p string) (string, error) {
	b, err := os.ReadFile(p)
	return string(b), err
}
func (kvTestFS) SaveFile(p, content string) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(content), 0o644)
}
func (kvTestFS) DeleteFile(p string) error { return os.Remove(p) }
func (kvTestFS) DeleteFolder(p string) error {
	// RemoveAll is silent on missing - same shape as system.Manager.DeleteFolder.
	return os.RemoveAll(p)
}
func (kvTestFS) ListDir(p string) ([]string, error) {
	entries, err := os.ReadDir(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out, nil
}

func newTestKV(t *testing.T) *KV {
	t.Helper()
	return NewKV(kvTestFS{}, filepath.Join(t.TempDir(), "kv"))
}

func TestKV_GetMissingKey(t *testing.T) {
	kv := newTestKV(t)
	got, ok, err := kv.Get("p", "missing")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok || got != nil {
		t.Fatalf("expected miss, got (%v, %v)", got, ok)
	}
}

func TestKV_SetGetRoundtrip(t *testing.T) {
	kv := newTestKV(t)
	if err := kv.Set("p", "k", "v"); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, ok, err := kv.Get("p", "k")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if got != "v" {
		t.Fatalf("got %v", got)
	}
}

func TestKV_PersistsAcrossInstances(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "kv")
	a := NewKV(kvTestFS{}, dir)
	if err := a.Set("p", "name", "Alice"); err != nil {
		t.Fatalf("set: %v", err)
	}
	// Fresh KV - must load from disk.
	b := NewKV(kvTestFS{}, dir)
	got, ok, _ := b.Get("p", "name")
	if !ok || got != "Alice" {
		t.Fatalf("round-trip failed: got %v ok=%v", got, ok)
	}
}

func TestKV_Delete(t *testing.T) {
	kv := newTestKV(t)
	_ = kv.Set("p", "k", 1)
	if err := kv.Delete("p", "k"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, ok, _ := kv.Get("p", "k")
	if ok {
		t.Fatal("expected key to be gone")
	}
}

func TestKV_Keys_SortedAndScopedToPlugin(t *testing.T) {
	kv := newTestKV(t)
	_ = kv.Set("a", "z", 1)
	_ = kv.Set("a", "m", 1)
	_ = kv.Set("a", "y", 1)
	_ = kv.Set("b", "x", 1)
	keys, err := kv.Keys("a")
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	if len(keys) != 3 || keys[0] != "m" || keys[1] != "y" || keys[2] != "z" {
		t.Fatalf("expected [m y z], got %v", keys)
	}
	keysB, _ := kv.Keys("b")
	if len(keysB) != 1 || keysB[0] != "x" {
		t.Fatalf("plugin b leaked from a: %v", keysB)
	}
}

func TestKV_CrossPluginIsolation(t *testing.T) {
	kv := newTestKV(t)
	_ = kv.Set("a", "shared", "from-a")
	_ = kv.Set("b", "shared", "from-b")
	gotA, _, _ := kv.Get("a", "shared")
	gotB, _, _ := kv.Get("b", "shared")
	if gotA != "from-a" || gotB != "from-b" {
		t.Fatalf("isolation broken: a=%v b=%v", gotA, gotB)
	}
}

func TestKV_CorruptJSONReturnsError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "kv")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "p.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	kv := NewKV(kvTestFS{}, dir)
	_, _, err := kv.Get("p", "anything")
	if err == nil {
		t.Fatal("expected error on corrupt JSON")
	}
}

// failingKVFS wraps kvTestFS but can fail individual operations so the
// KV I/O error branches (ensure-dir, load, save) get exercised.
type failingKVFS struct {
	kvTestFS
	ensureErr error
	loadErr   error
	saveErr   error
	exists    bool
}

func (f failingKVFS) EnsureDirectory(p string) error {
	if f.ensureErr != nil {
		return f.ensureErr
	}
	return f.kvTestFS.EnsureDirectory(p)
}
func (f failingKVFS) FileExists(string) bool { return f.exists }
func (f failingKVFS) LoadFile(p string) (string, error) {
	if f.loadErr != nil {
		return "", f.loadErr
	}
	return f.kvTestFS.LoadFile(p)
}
func (f failingKVFS) SaveFile(p, content string) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	return f.kvTestFS.SaveFile(p, content)
}

func TestKV_Set_EnsureDirectoryError(t *testing.T) {
	kv := NewKV(failingKVFS{ensureErr: errors.New("mkdir denied")}, "/kv")
	if err := kv.Set("p", "k", "v"); err == nil {
		t.Error("Set should fail when EnsureDirectory errors")
	}
}

func TestKV_Set_SaveFileError(t *testing.T) {
	kv := NewKV(failingKVFS{saveErr: errors.New("disk full")}, "/kv")
	if err := kv.Set("p", "k", "v"); err == nil {
		t.Error("Set should fail when SaveFile errors")
	}
}

func TestKV_Get_LoadFileError(t *testing.T) {
	// File reports as present but the read fails: the LoadFile error
	// branch distinct from the corrupt-JSON (parse) branch.
	kv := NewKV(failingKVFS{exists: true, loadErr: errors.New("read denied")}, "/kv")
	if _, _, err := kv.Get("p", "k"); err == nil {
		t.Error("Get should propagate a LoadFile error")
	}
}

func TestKV_ConcurrentSetSafe(t *testing.T) {
	// Race-detector check - mutex must serialize per-plugin writes.
	kv := newTestKV(t)
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = kv.Set("p", "counter", i)
		}(i)
	}
	wg.Wait()
	// We don't care about which write wins; we care the cache and disk
	// agree and the JSON didn't end up half-written. Round-trip via a
	// fresh KV must succeed.
	fresh := NewKV(kvTestFS{}, kv.root)
	if _, _, err := fresh.Get("p", "counter"); err != nil {
		t.Fatalf("post-concurrent corrupt: %v", err)
	}
}

func TestKV_DeleteOnMissingKeyIsNoop(t *testing.T) {
	kv := newTestKV(t)
	if err := kv.Delete("p", "nope"); err != nil {
		t.Fatalf("delete missing should be silent: %v", err)
	}
}

func TestKV_RejectsBadPluginID(t *testing.T) {
	kv := newTestKV(t)
	cases := []string{"", "..", "a/b", "Demo"}
	for _, id := range cases {
		t.Run(id, func(t *testing.T) {
			if err := kv.Set(id, "k", "v"); !errors.Is(err, ErrManifestInvalid) {
				t.Fatalf("Set(%q): want ErrManifestInvalid, got %v", id, err)
			}
		})
	}
}
