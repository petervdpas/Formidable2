package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

// writePluginI18n drops a <pluginDir>/i18n/<locale>.json file with
// the given JSON body. Plugin folder is assumed to already exist
// (writePlugin made it).
func writePluginI18n(t *testing.T, pluginDir, locale, body string) {
	t.Helper()
	dir := filepath.Join(pluginDir, "i18n")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir i18n: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, locale+".json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s.json: %v", locale, err)
	}
}

func TestLoadPluginI18n_ReadsLocaleFile(t *testing.T) {
	root := t.TempDir()
	dir := writePlugin(t, root, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	writePluginI18n(t, dir, "en", `{
		"name": "Demo Plugin",
		"description": "A demo.",
		"commands.run.label": "Run it"
	}`)

	msgs, err := loadPluginI18n(dir, "en")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if msgs["name"] != "Demo Plugin" {
		t.Errorf("name = %q, want %q", msgs["name"], "Demo Plugin")
	}
	if msgs["commands.run.label"] != "Run it" {
		t.Errorf("commands.run.label = %q", msgs["commands.run.label"])
	}
}

func TestLoadPluginI18n_MissingFileIsEmptyNotError(t *testing.T) {
	root := t.TempDir()
	dir := writePlugin(t, root, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")

	msgs, err := loadPluginI18n(dir, "en")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected empty map, got %v", msgs)
	}
}

func TestLoadPluginI18n_MalformedJSONIsError(t *testing.T) {
	root := t.TempDir()
	dir := writePlugin(t, root, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	writePluginI18n(t, dir, "en", `{not json`)

	if _, err := loadPluginI18n(dir, "en"); err == nil {
		t.Fatal("expected error for malformed json")
	}
}

func TestManager_MessagesForLocale_PrefixesByPluginID(t *testing.T) {
	m, pluginsDir := newTestManager(t)
	dirA := writePlugin(t, pluginsDir, "alpha", `{
		"manifest_version": 1, "id": "alpha", "name": "Alpha",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	dirB := writePlugin(t, pluginsDir, "beta", `{
		"manifest_version": 1, "id": "beta", "name": "Beta",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	writePluginI18n(t, dirA, "en", `{
		"name": "Alpha Plugin",
		"commands.run.label": "Go"
	}`)
	writePluginI18n(t, dirB, "en", `{
		"name": "Beta Plugin"
	}`)
	if err := m.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	msgs := m.MessagesForLocale("en")
	if msgs["plugin.alpha.name"] != "Alpha Plugin" {
		t.Errorf("plugin.alpha.name = %q", msgs["plugin.alpha.name"])
	}
	if msgs["plugin.alpha.commands.run.label"] != "Go" {
		t.Errorf("plugin.alpha.commands.run.label = %q", msgs["plugin.alpha.commands.run.label"])
	}
	if msgs["plugin.beta.name"] != "Beta Plugin" {
		t.Errorf("plugin.beta.name = %q", msgs["plugin.beta.name"])
	}
	// No collision: alpha keys never leak into beta namespace.
	if _, exists := msgs["plugin.beta.commands.run.label"]; exists {
		t.Errorf("plugin.beta.commands.run.label leaked across plugins")
	}
}

func TestManager_MessagesForLocale_SkipsPluginsWithoutLocaleFile(t *testing.T) {
	m, pluginsDir := newTestManager(t)
	dirA := writePlugin(t, pluginsDir, "alpha", `{
		"manifest_version": 1, "id": "alpha", "name": "Alpha",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	_ = writePlugin(t, pluginsDir, "beta", `{
		"manifest_version": 1, "id": "beta", "name": "Beta",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	writePluginI18n(t, dirA, "en", `{"name": "Alpha Plugin"}`)
	// beta intentionally has no i18n/ folder
	if err := m.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	msgs := m.MessagesForLocale("en")
	if msgs["plugin.alpha.name"] != "Alpha Plugin" {
		t.Errorf("alpha missing: %v", msgs)
	}
	for k := range msgs {
		if len(k) >= 12 && k[:12] == "plugin.beta." {
			t.Errorf("beta should have no messages, got %q", k)
		}
	}
}

func TestManager_MessagesForLocale_BadJSONInOnePluginDoesntBreakOthers(t *testing.T) {
	m, pluginsDir := newTestManager(t)
	dirA := writePlugin(t, pluginsDir, "alpha", `{
		"manifest_version": 1, "id": "alpha", "name": "Alpha",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	dirB := writePlugin(t, pluginsDir, "beta", `{
		"manifest_version": 1, "id": "beta", "name": "Beta",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	writePluginI18n(t, dirA, "en", `{"name": "Alpha Plugin"}`)
	writePluginI18n(t, dirB, "en", `{not json`)
	if err := m.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	msgs := m.MessagesForLocale("en")
	if msgs["plugin.alpha.name"] != "Alpha Plugin" {
		t.Errorf("alpha should survive bad beta i18n, got %v", msgs)
	}
}

func TestManager_MessagesForLocale_EmptyWhenNoPlugins(t *testing.T) {
	m, _ := newTestManager(t)
	if err := m.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	msgs := m.MessagesForLocale("en")
	if len(msgs) != 0 {
		t.Errorf("expected empty, got %v", msgs)
	}
}

func TestManager_MessagesForLocale_LocaleNotFoundIsEmpty(t *testing.T) {
	m, pluginsDir := newTestManager(t)
	dir := writePlugin(t, pluginsDir, "alpha", `{
		"manifest_version": 1, "id": "alpha", "name": "Alpha",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	writePluginI18n(t, dir, "en", `{"name": "Alpha Plugin"}`)
	if err := m.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	msgs := m.MessagesForLocale("nl")
	if len(msgs) != 0 {
		t.Errorf("nl should be empty (no nl.json), got %v", msgs)
	}
}
