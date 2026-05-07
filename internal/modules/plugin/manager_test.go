package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// newTestManager builds a Manager rooted at a fresh temp dir and
// pre-wired with a real *KV plus the in-test mocks. Each test
// adds plugin folders under deps.PluginsDir before calling Refresh.
func newTestManager(t *testing.T) (*Manager, string) {
	t.Helper()
	root := t.TempDir()
	pluginsDir := filepath.Join(root, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	m := NewManager(ManagerDeps{
		PluginsDir: pluginsDir,
		KV:         NewKV(kvTestFS{}, filepath.Join(pluginsDir, ".kv")),
	})
	return m, pluginsDir
}

func TestManager_Refresh_EmptyDir(t *testing.T) {
	m, _ := newTestManager(t)
	if err := m.Refresh(); err != nil {
		t.Fatalf("err: %v", err)
	}
	if got := m.List(); len(got) != 0 {
		t.Fatalf("got %d plugins, want 0", len(got))
	}
}

func TestManager_Refresh_DiscoversValidPlugin(t *testing.T) {
	m, pluginsDir := newTestManager(t)
	writePlugin(t, pluginsDir, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run(ctx) return 42 end")
	if err := m.Refresh(); err != nil {
		t.Fatalf("err: %v", err)
	}
	got := m.List()
	if len(got) != 1 || got[0].Manifest.ID != "demo" {
		t.Fatalf("got %+v", got)
	}
}

func TestManager_Refresh_SkipsLooseFiles(t *testing.T) {
	m, pluginsDir := newTestManager(t)
	// Stray file at plugins root — should be ignored.
	if err := os.WriteFile(filepath.Join(pluginsDir, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	writePlugin(t, pluginsDir, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() return 1 end")
	_ = m.Refresh()
	if len(m.List()) != 1 {
		t.Fatalf("got %d", len(m.List()))
	}
}

func TestManager_Refresh_SkipsFoldersWithoutManifest(t *testing.T) {
	m, pluginsDir := newTestManager(t)
	// Folder without plugin.json — silently skipped.
	if err := os.MkdirAll(filepath.Join(pluginsDir, "noplugin"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_ = m.Refresh()
	if len(m.List()) != 0 {
		t.Fatalf("got %d", len(m.List()))
	}
}

func TestManager_Refresh_SkipsHiddenAndKVDir(t *testing.T) {
	// `.kv` is the K/V root; anything starting with "." should be
	// skipped so plugin authors can store helper files alongside.
	m, pluginsDir := newTestManager(t)
	if err := os.MkdirAll(filepath.Join(pluginsDir, ".kv"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(pluginsDir, ".cache"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_ = m.Refresh()
	if len(m.List()) != 0 {
		t.Fatalf("got %d", len(m.List()))
	}
}

func TestManager_Refresh_SkipsBadManifestKeepsValid(t *testing.T) {
	// Corrupt manifest in one folder must not crash the scan or
	// poison the others. The good plugin still loads.
	m, pluginsDir := newTestManager(t)
	writePlugin(t, pluginsDir, "broken", `{not json`, "function run() end")
	writePlugin(t, pluginsDir, "good", `{
		"manifest_version": 1, "id": "good", "name": "Good",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() return 1 end")
	_ = m.Refresh()
	got := m.List()
	if len(got) != 1 || got[0].Manifest.ID != "good" {
		t.Fatalf("got %+v", got)
	}
}

func TestManager_Run_UnknownPlugin(t *testing.T) {
	m, _ := newTestManager(t)
	_, err := m.Run("ghost", "run", nil)
	if !errors.Is(err, ErrPluginNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestManager_Run_UnknownCommand(t *testing.T) {
	m, pluginsDir := newTestManager(t)
	writePlugin(t, pluginsDir, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() return 1 end")
	_ = m.Refresh()
	_, err := m.Run("demo", "ghost", nil)
	if !errors.Is(err, ErrCommandNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestManager_Run_HappyReturnsValue(t *testing.T) {
	m, pluginsDir := newTestManager(t)
	writePlugin(t, pluginsDir, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run(ctx) return { value = 42 } end")
	_ = m.Refresh()
	res, err := m.Run("demo", "run", nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	got := res.Value.(map[string]any)
	if got["value"] != float64(42) {
		t.Fatalf("got %v", got)
	}
}

func TestManager_Run_PassesCtxArgument(t *testing.T) {
	m, pluginsDir := newTestManager(t)
	writePlugin(t, pluginsDir, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "echo", "label": "Echo"}]
	}`, "function echo(ctx) return ctx.greeting end")
	_ = m.Refresh()
	res, _ := m.Run("demo", "echo", map[string]any{"greeting": "hi"})
	if res.Value != "hi" {
		t.Fatalf("got %v", res.Value)
	}
}

func TestManager_Run_ExplicitFnOverridesID(t *testing.T) {
	// Command with an explicit "fn" hits the named function instead
	// of one matching the command id.
	m, pluginsDir := newTestManager(t)
	writePlugin(t, pluginsDir, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "user_facing", "label": "Run", "fn": "actual_fn"}]
	}`, "function actual_fn() return 'right one' end")
	_ = m.Refresh()
	res, err := m.Run("demo", "user_facing", nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Value != "right one" {
		t.Fatalf("got %v", res.Value)
	}
}

func TestManager_Run_KVScopedToPluginID(t *testing.T) {
	// A plugin's KV is keyed by the plugin id; two plugins setting
	// the same key see independent values.
	m, pluginsDir := newTestManager(t)
	writePlugin(t, pluginsDir, "a", `{
		"manifest_version": 1, "id": "a", "name": "A",
		"version": "0.1.0",
		"commands": [{"id": "set", "label": "set"}, {"id": "get", "label": "get"}]
	}`, `
		function set(ctx) formidable.kv.set("k", "from-a") end
		function get(ctx) return formidable.kv.get("k") end`)
	writePlugin(t, pluginsDir, "b", `{
		"manifest_version": 1, "id": "b", "name": "B",
		"version": "0.1.0",
		"commands": [{"id": "set", "label": "set"}, {"id": "get", "label": "get"}]
	}`, `
		function set(ctx) formidable.kv.set("k", "from-b") end
		function get(ctx) return formidable.kv.get("k") end`)
	_ = m.Refresh()
	_, _ = m.Run("a", "set", nil)
	_, _ = m.Run("b", "set", nil)
	gotA, _ := m.Run("a", "get", nil)
	gotB, _ := m.Run("b", "get", nil)
	if gotA.Value != "from-a" || gotB.Value != "from-b" {
		t.Fatalf("isolation broken: a=%v b=%v", gotA.Value, gotB.Value)
	}
}
