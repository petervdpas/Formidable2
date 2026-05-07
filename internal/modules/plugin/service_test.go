package plugin

import (
	"path/filepath"
	"testing"
)

func newTestServiceWithPlugin(t *testing.T, manifest, main string) (*Service, string) {
	t.Helper()
	root := t.TempDir()
	pluginsDir := filepath.Join(root, "plugins")
	m := NewManager(ManagerDeps{
		PluginsDir: pluginsDir,
		KV:         NewKV(kvTestFS{}, filepath.Join(pluginsDir, ".kv")),
	})
	writePlugin(t, pluginsDir, "demo", manifest, main)
	if err := m.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	return NewService(m), pluginsDir
}

func TestService_List_ReturnsManifestSummary(t *testing.T) {
	s, _ := newTestServiceWithPlugin(t, `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() return 1 end")
	got := s.List()
	if len(got) != 1 {
		t.Fatalf("got %d", len(got))
	}
	if got[0].ID != "demo" || got[0].Manifest.Name != "Demo" {
		t.Fatalf("got %+v", got[0])
	}
}

func TestService_Run_Ok(t *testing.T) {
	s, _ := newTestServiceWithPlugin(t, `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run(ctx) return 'ok' end")
	dto := s.Run("demo", "run", nil)
	if dto.Kind != "ok" || dto.Value != "ok" {
		t.Fatalf("got %+v", dto)
	}
}

func TestService_Run_PluginNotFound(t *testing.T) {
	s, _ := newTestServiceWithPlugin(t, `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	dto := s.Run("ghost", "run", nil)
	if dto.Kind != "plugin_not_found" {
		t.Fatalf("got %+v", dto)
	}
}

func TestService_Run_CommandNotFound(t *testing.T) {
	s, _ := newTestServiceWithPlugin(t, `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	dto := s.Run("demo", "ghost", nil)
	if dto.Kind != "command_not_found" {
		t.Fatalf("got %+v", dto)
	}
}

func TestService_Run_RuntimeError(t *testing.T) {
	s, _ := newTestServiceWithPlugin(t, `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, `function run() error("boom") end`)
	dto := s.Run("demo", "run", nil)
	if dto.Kind != "runtime_error" {
		t.Fatalf("got %+v", dto)
	}
	if dto.Message == "" {
		t.Fatalf("message empty")
	}
}

func TestService_Refresh_PicksUpNewPlugin(t *testing.T) {
	root := t.TempDir()
	pluginsDir := filepath.Join(root, "plugins")
	m := NewManager(ManagerDeps{
		PluginsDir: pluginsDir,
		KV:         NewKV(kvTestFS{}, filepath.Join(pluginsDir, ".kv")),
	})
	s := NewService(m)
	if got, _ := s.Refresh(); len(got) != 0 {
		t.Fatalf("expected empty initial, got %d", len(got))
	}
	writePlugin(t, pluginsDir, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() return 1 end")
	got, err := s.Refresh()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("after refresh: %d", len(got))
	}
}
