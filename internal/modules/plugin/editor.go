// Editor surface (Create/Save/Delete/GetSource). All writes go through editorFS so production gets atomic+fsync'd writes via *system.Manager.
package plugin

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// editorFS is the fs surface the editor methods need.
type editorFS interface {
	EnsureDirectory(path string) error
	FileExists(path string) bool
	IsDir(path string) bool
	LoadFile(path string) (string, error)
	SaveFile(path, content string) error
	DeleteFile(path string) error
	DeleteFolder(path string) error
	// ListDir lists entries at path; a missing directory must return (nil, nil) so the archive exporter skips empty plugins instead of erroring.
	ListDir(path string) ([]string, error)
}

// SerializeManifest is the canonical plugin.json writer: 2-space indent, trailing newline.
func SerializeManifest(m Manifest) ([]byte, error) {
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("plugin: marshal manifest: %w", err)
	}
	return append(raw, '\n'), nil
}

// DefaultManifest builds a minimal valid manifest; id must already be validID-clean.
func DefaultManifest(id string) Manifest {
	return Manifest{
		ManifestVersion: ManifestSchemaVersion,
		ID:              id,
		Name:            id,
		Version:         "0.1.0",
		Commands:        []Command{{ID: "run", Label: "Run"}},
	}
}

// defaultMainLua is the hello-world stub; its single "run" command matches DefaultManifest.
const defaultMainLua = `-- ` + "`run`" + ` is invoked when the user clicks Plugin → Run → Run.
-- Return any JSON-shaped value (number, string, table, nil).
function run(ctx)
  formidable.log.info("hello from plugin!")
  return { ok = true }
end
`

// defaultFormJSON is the empty form definition. Entries are heterogeneous (template Field or formwidget.Widget) in one ordered list, so widgets can sit anywhere among fields.
const defaultFormJSON = "[]\n"

// Create scaffolds a new plugin folder at <PluginsDir>/<id> and refreshes the registry.
// Errors: ErrManifestInvalid (id fails validID, path-traversal-safe), ErrPluginExists, or underlying I/O.
func (m *Manager) Create(id string) error {
	if !validID(id) {
		return fmt.Errorf("%w: bad id %q", ErrManifestInvalid, id)
	}
	if m.deps.Editor == nil {
		return fmt.Errorf("plugin: editor fs not configured")
	}
	dir := filepath.Join(m.deps.PluginsDir, id)
	if m.deps.Editor.FileExists(dir) {
		return fmt.Errorf("%w: %s", ErrPluginExists, id)
	}
	if err := m.deps.Editor.EnsureDirectory(dir); err != nil {
		return fmt.Errorf("plugin: ensure %s: %w", dir, err)
	}
	manifest := DefaultManifest(id)
	raw, err := SerializeManifest(manifest)
	if err != nil {
		return err
	}
	if err := m.deps.Editor.SaveFile(filepath.Join(dir, "plugin.json"), string(raw)); err != nil {
		return fmt.Errorf("plugin: write manifest: %w", err)
	}
	if err := m.deps.Editor.SaveFile(filepath.Join(dir, "main.lua"), defaultMainLua); err != nil {
		return fmt.Errorf("plugin: write main.lua: %w", err)
	}
	if err := m.deps.Editor.SaveFile(filepath.Join(dir, "form.json"), defaultFormJSON); err != nil {
		return fmt.Errorf("plugin: write form.json: %w", err)
	}
	return m.Refresh()
}

// Save writes plugin.json + main.lua + form.json. The path id and manifest.ID must agree (renames go through Delete + Create).
// Backend treats formJSON as opaque; empty means "leave form.json untouched".
// Errors: ErrManifestInvalid, ErrManifestVersion, ErrPluginNotFound.
func (m *Manager) Save(id string, manifest Manifest, luaSource, formJSON string) error {
	if !validID(id) {
		return fmt.Errorf("%w: bad id %q", ErrManifestInvalid, id)
	}
	if id != manifest.ID {
		return fmt.Errorf("%w: id %q != manifest.id %q", ErrManifestInvalid, id, manifest.ID)
	}
	if manifest.ManifestVersion != ManifestSchemaVersion {
		return fmt.Errorf("%w: got %d, want %d",
			ErrManifestVersion, manifest.ManifestVersion, ManifestSchemaVersion)
	}
	if err := validateManifest(&manifest); err != nil {
		return err
	}
	if m.deps.Editor == nil {
		return fmt.Errorf("plugin: editor fs not configured")
	}
	dir := filepath.Join(m.deps.PluginsDir, id)
	if !m.deps.Editor.FileExists(dir) {
		return fmt.Errorf("%w: %s", ErrPluginNotFound, id)
	}
	raw, err := SerializeManifest(manifest)
	if err != nil {
		return err
	}
	if err := m.deps.Editor.SaveFile(filepath.Join(dir, "plugin.json"), string(raw)); err != nil {
		return fmt.Errorf("plugin: write manifest: %w", err)
	}
	if err := m.deps.Editor.SaveFile(filepath.Join(dir, "main.lua"), luaSource); err != nil {
		return fmt.Errorf("plugin: write main.lua: %w", err)
	}
	if formJSON != "" {
		if err := m.deps.Editor.SaveFile(filepath.Join(dir, "form.json"), formJSON); err != nil {
			return fmt.Errorf("plugin: write form.json: %w", err)
		}
	}
	return m.Refresh()
}

// GetForm reads <PluginsDir>/<id>/form.json, returning the empty-array placeholder when the file is missing.
func (m *Manager) GetForm(id string) (string, error) {
	if !validID(id) {
		return "", fmt.Errorf("%w: bad id %q", ErrManifestInvalid, id)
	}
	if m.deps.Editor == nil {
		return "", fmt.Errorf("plugin: editor fs not configured")
	}
	dir := filepath.Join(m.deps.PluginsDir, id)
	if !m.deps.Editor.FileExists(dir) {
		return "", fmt.Errorf("%w: %s", ErrPluginNotFound, id)
	}
	formPath := filepath.Join(dir, "form.json")
	if !m.deps.Editor.FileExists(formPath) {
		return defaultFormJSON, nil
	}
	src, err := m.deps.Editor.LoadFile(formPath)
	if err != nil {
		return "", fmt.Errorf("plugin: read form.json: %w", err)
	}
	return src, nil
}

// Delete removes <PluginsDir>/<id> and the plugin's KV file, then refreshes the registry.
// Errors: ErrManifestInvalid, ErrPluginNotFound.
func (m *Manager) Delete(id string) error {
	if !validID(id) {
		return fmt.Errorf("%w: bad id %q", ErrManifestInvalid, id)
	}
	if m.deps.Editor == nil {
		return fmt.Errorf("plugin: editor fs not configured")
	}
	dir := filepath.Join(m.deps.PluginsDir, id)
	if !m.deps.Editor.FileExists(dir) {
		return fmt.Errorf("%w: %s", ErrPluginNotFound, id)
	}
	if err := m.deps.Editor.DeleteFolder(dir); err != nil {
		return fmt.Errorf("plugin: delete %s: %w", dir, err)
	}
	kvPath := filepath.Join(m.deps.PluginsDir, ".kv", id+".json")
	if m.deps.Editor.FileExists(kvPath) {
		_ = m.deps.Editor.DeleteFile(kvPath)
	}
	return m.Refresh()
}

// GetSource reads <PluginsDir>/<id>/main.lua. Returns
// ErrPluginNotFound when the plugin folder is missing.
func (m *Manager) GetSource(id string) (string, error) {
	if !validID(id) {
		return "", fmt.Errorf("%w: bad id %q", ErrManifestInvalid, id)
	}
	if m.deps.Editor == nil {
		return "", fmt.Errorf("plugin: editor fs not configured")
	}
	dir := filepath.Join(m.deps.PluginsDir, id)
	if !m.deps.Editor.FileExists(dir) {
		return "", fmt.Errorf("%w: %s", ErrPluginNotFound, id)
	}
	src, err := m.deps.Editor.LoadFile(filepath.Join(dir, "main.lua"))
	if err != nil {
		return "", fmt.Errorf("plugin: read main.lua: %w", err)
	}
	return src, nil
}
