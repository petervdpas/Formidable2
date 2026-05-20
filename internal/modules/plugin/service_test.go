package plugin

import (
	"path/filepath"
	"testing"
)

func TestService_GetI18nMessages_ReturnsMergedMessages(t *testing.T) {
	s, pluginsDir := newTestServiceWithPlugin(t, `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	writePluginI18n(t, filepath.Join(pluginsDir, "demo"), "en", `{
		"name": "Demo Plugin",
		"commands.run.label": "Run it"
	}`)
	got := s.GetI18nMessages("en")
	if got["plugin.demo.name"] != "Demo Plugin" {
		t.Errorf("plugin.demo.name = %q", got["plugin.demo.name"])
	}
	if got["plugin.demo.commands.run.label"] != "Run it" {
		t.Errorf("plugin.demo.commands.run.label = %q", got["plugin.demo.commands.run.label"])
	}
}

func TestService_SaveAndGetPluginI18n_RoundTrips(t *testing.T) {
	s, _ := newTestServiceWithPlugin(t, `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	want := map[string]string{
		"name":               "Demo Plugin",
		"commands.run.label": "Run it",
	}
	if err := s.SavePluginI18n("demo", "en", want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := s.GetPluginI18n("demo", "en")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got["name"] != want["name"] || got["commands.run.label"] != want["commands.run.label"] {
		t.Errorf("round-trip got %v", got)
	}
	// And the namespaced merge picks it up too.
	merged := s.GetI18nMessages("en")
	if merged["plugin.demo.name"] != want["name"] {
		t.Errorf("expected plugin.demo.name in merged map, got %v", merged)
	}
}

func TestService_ListPluginLocales_AfterSave(t *testing.T) {
	s, _ := newTestServiceWithPlugin(t, `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	if err := s.SavePluginI18n("demo", "en", map[string]string{"name": "X"}); err != nil {
		t.Fatalf("save en: %v", err)
	}
	if err := s.SavePluginI18n("demo", "nl", map[string]string{"name": "Y"}); err != nil {
		t.Fatalf("save nl: %v", err)
	}
	got, err := s.ListPluginLocales("demo")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 || got[0] != "en" || got[1] != "nl" {
		t.Errorf("got %v, want [en nl]", got)
	}
}

func TestService_DeletePluginI18n_DropsMergedKeys(t *testing.T) {
	s, _ := newTestServiceWithPlugin(t, `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	if err := s.SavePluginI18n("demo", "en", map[string]string{"name": "Demo Plugin"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := s.DeletePluginI18n("demo", "en"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	merged := s.GetI18nMessages("en")
	if _, exists := merged["plugin.demo.name"]; exists {
		t.Errorf("plugin.demo.name should be gone after delete, got %v", merged)
	}
}

func TestService_GetI18nMessages_UnknownLocaleIsEmpty(t *testing.T) {
	s, _ := newTestServiceWithPlugin(t, `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	got := s.GetI18nMessages("nl")
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func newTestServiceWithPlugin(t *testing.T, manifest, main string) (*Service, string) {
	t.Helper()
	root := t.TempDir()
	pluginsDir := filepath.Join(root, "plugins")
	m := NewManager(ManagerDeps{
		PluginsDir: pluginsDir,
		KV:         NewKV(kvTestFS{}, filepath.Join(pluginsDir, ".kv")),
		Editor:     kvTestFS{},
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

func TestService_Run_PassesThroughToasts(t *testing.T) {
	s, _ := newTestServiceWithPlugin(t, `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, `function run()
		formidable.toast.success("yay")
		formidable.toast.error("nope")
	end`)
	dto := s.Run("demo", "run", nil)
	if dto.Kind != "ok" {
		t.Fatalf("got %+v", dto)
	}
	if len(dto.Toasts) != 2 {
		t.Fatalf("toasts: %+v", dto.Toasts)
	}
	if dto.Toasts[0].Level != "success" || dto.Toasts[1].Level != "error" {
		t.Fatalf("levels: %+v", dto.Toasts)
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

func TestService_ListWorkspaces_ReturnsClosedEnum(t *testing.T) {
	// Frontend manifest editor reads this to populate the workspace
	// dropdown — the contract is the backend owns the list.
	s, _ := newTestServiceWithPlugin(t, `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	got := s.ListWorkspaces()
	if len(got) != len(ValidWorkspaces()) {
		t.Fatalf("got %d, want %d", len(got), len(ValidWorkspaces()))
	}
	if got[0] != WorkspaceStorage {
		t.Fatalf("got[0] = %q, want %q", got[0], WorkspaceStorage)
	}
}

func TestService_ListForWorkspace_FiltersByManifest(t *testing.T) {
	// Round-trip the manifest with an attached workspace through the
	// service so Vue's ListResult shape stays in sync with manager.
	root := t.TempDir()
	pluginsDir := filepath.Join(root, "plugins")
	m := NewManager(ManagerDeps{
		PluginsDir: pluginsDir,
		KV:         NewKV(kvTestFS{}, filepath.Join(pluginsDir, ".kv")),
	})
	writePlugin(t, pluginsDir, "a", `{
		"manifest_version": 1, "id": "a", "name": "A",
		"version": "0.1.0",
		"workspaces": ["storage"],
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	writePlugin(t, pluginsDir, "b", `{
		"manifest_version": 1, "id": "b", "name": "B",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	if err := m.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	s := NewService(m)
	got := s.ListForWorkspace(WorkspaceStorage)
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("storage: %+v", got)
	}
	if g := s.ListForWorkspace("bogus"); len(g) != 0 {
		t.Fatalf("unknown ws: %+v", g)
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
