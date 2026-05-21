// Package plugin's editor surface — Create / Save / Delete / GetSource.
// These are the CRUD primitives the Plugins workspace calls when the
// user scaffolds a new plugin, edits its manifest + main.lua, or
// removes it. All writes go through editorFS so production gets
// atomic+fsync'd writes (via *system.Manager) while tests can swap in
// a tiny in-memory shim.
package plugin

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// editorFS is the fs surface the editor methods need. *system.Manager
// satisfies it (its absolute-path branch keeps things working when the
// plugins folder lives outside AppRoot). Mirrors kvFS but adds
// DeleteFolder for tearing the whole plugin tree down.
type editorFS interface {
	EnsureDirectory(path string) error
	FileExists(path string) bool
	IsDir(path string) bool
	LoadFile(path string) (string, error)
	SaveFile(path, content string) error
	DeleteFile(path string) error
	DeleteFolder(path string) error
	// ListDir returns the names of the entries (files + subfolders)
	// at path. A missing directory must return (nil, nil) — the
	// archive exporter relies on that to skip plugins with no files
	// rather than erroring at the boundary.
	ListDir(path string) ([]string, error)
}

// SerializeManifest is the canonical writer for plugin.json: 2-space
// indent, trailing newline, JSON-escaped. Used by Create + Save so
// the on-disk shape is consistent regardless of write origin.
func SerializeManifest(m Manifest) ([]byte, error) {
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("plugin: marshal manifest: %w", err)
	}
	return append(raw, '\n'), nil
}

// DefaultManifest builds a minimal valid manifest for a fresh plugin
// scaffold. id must already be validID-clean.
func DefaultManifest(id string) Manifest {
	return Manifest{
		ManifestVersion: ManifestSchemaVersion,
		ID:              id,
		Name:            id,
		Version:         "0.1.0",
		Commands:        []Command{{ID: "run", Label: "Run"}},
	}
}

// defaultMainLua is the hello-world stub written next to a fresh
// manifest. Single command "run" matches DefaultManifest.
const defaultMainLua = `-- ` + "`run`" + ` is invoked when the user clicks Plugin → Run → Run.
-- Return any JSON-shaped value (number, string, table, nil).
function run(ctx)
  formidable.log.info("hello from plugin!")
  return { ok = true }
end
`

// defaultFormJSON is the empty form definition written alongside a
// fresh plugin. The visual form builder reads/writes this file; an
// empty JSON array means "no entries yet". Entries are heterogeneous
// — each is either a template Field (input) or a formwidget.Widget
// (live display slot); they share the same ordered list so the
// author can place widgets anywhere relative to fields without a
// second file.
const defaultFormJSON = "[]\n"

// Create scaffolds a new plugin folder at <PluginsDir>/<id> with a
// default manifest and a hello-world main.lua. Refreshes the
// registry on success so the Plugins workspace sees the new entry
// immediately.
//
// Errors:
//   - ErrManifestInvalid when id fails validID (path-traversal-safe).
//   - ErrPluginExists when the target folder already exists.
//   - underlying I/O errors otherwise.
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

// Save writes plugin.json + main.lua + form.json for an existing
// plugin. The path-side id and manifest.ID must agree — renames go
// through Delete + Create, not Save. formJSON is the raw text of
// the field schema; the backend treats it as opaque (the schema
// lives on the frontend, shared with template fields), and an
// empty string is interpreted as "leave form.json untouched."
//
// Errors:
//   - ErrManifestInvalid when id is malformed, ids disagree, or the
//     manifest itself fails validation.
//   - ErrManifestVersion when manifest.ManifestVersion is unsupported.
//   - ErrPluginNotFound when no folder exists at <PluginsDir>/<id>.
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
	// Empty formJSON = "no change to form.json" so callers that don't
	// touch the form schema can skip a redundant write.
	if formJSON != "" {
		if err := m.deps.Editor.SaveFile(filepath.Join(dir, "form.json"), formJSON); err != nil {
			return fmt.Errorf("plugin: write form.json: %w", err)
		}
	}
	return m.Refresh()
}

// GetForm reads <PluginsDir>/<id>/form.json. Returns the default
// empty-array placeholder when the file is missing — keeps callers
// (the visual builder) free of "is the file here yet?" branches.
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

// Delete removes <PluginsDir>/<id> and the plugin's KV file.
// Refreshes the registry. Idempotent on partial state — if the
// KV file is missing, the folder removal still proceeds.
//
// Errors:
//   - ErrManifestInvalid for bad ids.
//   - ErrPluginNotFound when nothing exists at the target path.
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
	// KV file lives at <plugins>/.kv/<id>.json. DeleteFile is
	// silent on missing in production (system.Manager) and via os.Remove
	// returns ENOENT which we ignore — kvTestFS uses os.Remove directly.
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
