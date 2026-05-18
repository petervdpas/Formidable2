package plugin

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestScaffoldPlugins_WritesSeedsToEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := ScaffoldPlugins(kvTestFS{}, dir, slog.Default()); err != nil {
		t.Fatalf("ScaffoldPlugins: %v", err)
	}
	for _, name := range []string{"plugin.json", "main.lua", "form.json"} {
		p := filepath.Join(dir, "test-plugin", name)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("seed not scaffolded at %q: %v", p, err)
		}
	}
}

func TestScaffoldPlugins_LeavesExistingFilesAlone(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "test-plugin", "plugin.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	userBody := `{"manifest_version":1,"id":"test-plugin","name":"User Override"}`
	if err := os.WriteFile(manifestPath, []byte(userBody), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ScaffoldPlugins(kvTestFS{}, dir, slog.Default()); err != nil {
		t.Fatalf("ScaffoldPlugins: %v", err)
	}
	got, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != userBody {
		t.Errorf("user-edited plugin.json was clobbered; got %q", string(got))
	}
	// Other seeds still get written.
	if _, err := os.Stat(filepath.Join(dir, "test-plugin", "main.lua")); err != nil {
		t.Errorf("main.lua should have been scaffolded alongside user-edited manifest: %v", err)
	}
}

func TestScaffoldPlugins_IdempotentOnRepeatedRuns(t *testing.T) {
	dir := t.TempDir()
	if err := ScaffoldPlugins(kvTestFS{}, dir, slog.Default()); err != nil {
		t.Fatalf("first scaffold: %v", err)
	}
	manifestPath := filepath.Join(dir, "test-plugin", "plugin.json")
	first, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	statBefore, _ := os.Stat(manifestPath)

	if err := ScaffoldPlugins(kvTestFS{}, dir, slog.Default()); err != nil {
		t.Fatalf("second scaffold: %v", err)
	}
	second, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	statAfter, _ := os.Stat(manifestPath)

	if string(first) != string(second) {
		t.Errorf("second scaffold modified plugin.json")
	}
	if !statBefore.ModTime().Equal(statAfter.ModTime()) {
		t.Errorf("second scaffold rewrote untouched file (mtime changed)")
	}
}

func TestScaffoldPlugins_RewritesDeletedFiles(t *testing.T) {
	dir := t.TempDir()
	if err := ScaffoldPlugins(kvTestFS{}, dir, slog.Default()); err != nil {
		t.Fatalf("first scaffold: %v", err)
	}
	manifestPath := filepath.Join(dir, "test-plugin", "plugin.json")
	if err := os.Remove(manifestPath); err != nil {
		t.Fatal(err)
	}
	if err := ScaffoldPlugins(kvTestFS{}, dir, slog.Default()); err != nil {
		t.Fatalf("second scaffold: %v", err)
	}
	if _, err := os.Stat(manifestPath); err != nil {
		t.Errorf("plugin.json should have been rescaffolded after deletion: %v", err)
	}
}

func TestScaffoldPlugins_NilFSIsNoOp(t *testing.T) {
	if err := ScaffoldPlugins(nil, t.TempDir(), slog.Default()); err != nil {
		t.Errorf("nil fs: err = %v, want nil (no-op)", err)
	}
}

func TestScaffoldPlugins_NilLoggerUsesDefault(t *testing.T) {
	if err := ScaffoldPlugins(kvTestFS{}, t.TempDir(), nil); err != nil {
		t.Errorf("nil logger should fall back to slog.Default; err = %v", err)
	}
}

// failingFS satisfies editorFS but errors on every SaveFile. Confirms
// the scaffold treats per-file write errors as non-fatal — matches
// cover_scaffold's "logs and moves on" stance.
type failingFS struct {
	kvTestFS
	saveErr error
}

func (f failingFS) SaveFile(string, string) error { return f.saveErr }

func TestScaffoldPlugins_SaveErrorLoggedNotFatal(t *testing.T) {
	dir := t.TempDir()
	fs := failingFS{saveErr: errors.New("disk full")}
	if err := ScaffoldPlugins(fs, dir, slog.Default()); err != nil {
		t.Errorf("scaffold returned err = %v, want nil (per-file failures non-fatal)", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "test-plugin", "plugin.json")); err == nil {
		t.Errorf("plugin.json should not exist after all-failing scaffold")
	}
}

func TestScaffoldPlugins_SeededFilesAreValid(t *testing.T) {
	dir := t.TempDir()
	if err := ScaffoldPlugins(kvTestFS{}, dir, slog.Default()); err != nil {
		t.Fatalf("scaffold: %v", err)
	}

	for _, id := range []string{"test-plugin", "wikiwonder"} {
		m, err := LoadManifest(filepath.Join(dir, id))
		if err != nil {
			t.Errorf("scaffolded manifest fails LoadManifest for %q: %v", id, err)
			continue
		}
		if m.ID != id {
			t.Errorf("scaffolded manifest id = %q, want %q", m.ID, id)
		}

		src, err := os.ReadFile(filepath.Join(dir, id, "main.lua"))
		if err != nil {
			t.Errorf("read main.lua for %q: %v", id, err)
			continue
		}
		L := lua.NewState()
		if err := L.DoString(string(src)); err != nil {
			t.Errorf("scaffolded main.lua for %q does not parse: %v", id, err)
		}
		L.Close()
	}
}
