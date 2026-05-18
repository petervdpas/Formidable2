package plugin

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"
)

// Plugin-archive errors. Mirror the sentinel-error style used by the
// pdf cover archive so callers can branch with errors.Is. Surfaced
// through the Wails service so the frontend can render targeted
// recovery hints instead of generic "import failed" text.
var (
	// ErrPluginArchiveInvalid wraps shape problems with the archive:
	// missing plugin.json, malformed JSON, missing main.lua, or any
	// other "this zip isn't a plugin bundle" failure.
	ErrPluginArchiveInvalid = errors.New("plugin: archive invalid")

	// ErrPluginArchiveTraversal blocks entries whose Clean'd name
	// escapes the plugin folder (foo/../../bar, absolute paths, etc.).
	// Treated as a hard refusal — never resolved.
	ErrPluginArchiveTraversal = errors.New("plugin: archive path traversal blocked")

	// ErrPluginArchiveExists fires when the target plugin id already
	// has an on-disk folder and the caller didn't set overwrite=true.
	// Lets the frontend show a "replace? cancel?" prompt before
	// destroying user state.
	ErrPluginArchiveExists = errors.New("plugin: target already exists (set overwrite=true to replace)")

	// ErrPluginArchiveNotFound is returned when the named plugin isn't
	// present at <pluginsDir>/<id>. Distinct from ErrPluginNotFound so
	// the export caller can give a more specific error.
	ErrPluginArchiveNotFound = errors.New("plugin: archive source plugin not found")

	// ErrPluginArchiveOlderVersion fires when an import would overwrite
	// an installed plugin with a strictly lower version. This is the
	// only meaning of the `version` field: downgrade-protection on
	// import. The check is unconditional — overwrite=true cannot
	// bypass it; the user has to roll the incoming plugin's version
	// forward (or wipe the on-disk copy by hand) to install it.
	ErrPluginArchiveOlderVersion = errors.New("plugin: archive version is older than installed")
)

// ExportArchiveResult describes what the export bundled. Files is the
// list of zip-entry names (relative to the plugin folder root) so the
// UI can render a confirmation panel and the user can spot side files
// they didn't expect to ship.
type ExportArchiveResult struct {
	ID      string   `json:"id"`
	ZipPath string   `json:"zip_path"`
	Files   []string `json:"files"`
}

// ImportArchiveResult describes what was materialised on import.
// Overwritten flags whether an existing plugin's files were replaced.
type ImportArchiveResult struct {
	ID          string   `json:"id"`
	Overwritten bool     `json:"overwritten"`
	Files       []string `json:"files"`
}

// exportPluginArchive bundles <pluginsDir>/<id>/* into a zip at
// zipPath. The id is validated up front (validID — same rules as
// Create/Save/Delete) so we never zip from a path the rest of the
// module would refuse. Hidden files and subfolders that start with
// "." are skipped — keeps the bundle clean of editor scratch state
// and matches what Refresh ignores.
//
// The .kv state file at <pluginsDir>/.kv/<id>.json is intentionally
// NOT bundled: it's per-user runtime state, not part of the plugin's
// distributable shape. A re-import on another machine starts with a
// fresh KV — same semantic as installing a new plugin.
func exportPluginArchive(fs editorFS, pluginsDir, id, zipPath string) (ExportArchiveResult, error) {
	var zero ExportArchiveResult
	if fs == nil {
		return zero, fmt.Errorf("%w: editor fs not configured", ErrPluginArchiveInvalid)
	}
	if !validID(id) {
		return zero, fmt.Errorf("%w: bad id %q", ErrManifestInvalid, id)
	}
	if zipPath == "" {
		return zero, fmt.Errorf("%w: zip path required", ErrPluginArchiveInvalid)
	}

	dir := filepath.Join(pluginsDir, id)
	if !fs.FileExists(dir) {
		return zero, fmt.Errorf("%w: %s", ErrPluginArchiveNotFound, id)
	}

	entries, err := fs.ListDir(dir)
	if err != nil {
		return zero, fmt.Errorf("list %s: %w", dir, err)
	}

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	var files []string
	for _, name := range entries {
		// Skip hidden + dirs-ish names. We only walk the top of the
		// plugin folder for v1 — recursive subdirs land when a real
		// plugin needs them.
		if strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(dir, name)
		body, err := fs.LoadFile(full)
		if err != nil {
			return zero, fmt.Errorf("read %s: %w", full, err)
		}
		if err := writeArchiveEntry(zw, name, body); err != nil {
			return zero, err
		}
		files = append(files, name)
	}
	if err := zw.Close(); err != nil {
		return zero, fmt.Errorf("finalize zip: %w", err)
	}

	if err := fs.SaveFile(zipPath, buf.String()); err != nil {
		return zero, fmt.Errorf("write zip: %w", err)
	}

	return ExportArchiveResult{
		ID:      id,
		ZipPath: zipPath,
		Files:   files,
	}, nil
}

// importPluginArchive reads a zip at zipPath and writes its contents
// to <pluginsDir>/<id>/. id is inferred from the embedded plugin.json
// (NOT from the zip filename), matching the cover-archive contract
// where the .html stem is authoritative. The zip must:
//
//   - Contain plugin.json at the root with a valid id + manifest_version.
//   - Contain main.lua at the root (mandatory per the manifest contract).
//   - Place every entry at the top level (no nested folders for v1).
//   - Contain no path-traversal segments after Clean.
//
// When a plugin with the same id is already present, refuses unless
// overwrite=true. Overwrite first removes the entire existing folder
// (so stale side files don't survive) before writing the new tree.
// Note: the plugin's .kv state at <pluginsDir>/.kv/<id>.json is left
// alone — the user's runtime state outlives the manifest, matching
// what a Save would do.
func importPluginArchive(fs editorFS, pluginsDir, zipPath string, overwrite bool) (ImportArchiveResult, error) {
	var zero ImportArchiveResult
	if fs == nil {
		return zero, fmt.Errorf("%w: editor fs not configured", ErrPluginArchiveInvalid)
	}
	if zipPath == "" {
		return zero, fmt.Errorf("%w: zip path required", ErrPluginArchiveInvalid)
	}
	if !fs.FileExists(zipPath) {
		return zero, fmt.Errorf("%w: %s", ErrPluginArchiveNotFound, zipPath)
	}

	raw, err := fs.LoadFile(zipPath)
	if err != nil {
		return zero, fmt.Errorf("read zip: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader([]byte(raw)), int64(len(raw)))
	if err != nil {
		return zero, fmt.Errorf("%w: %v", ErrPluginArchiveInvalid, err)
	}

	manifestBody := ""
	mainLuaPresent := false
	entries := map[string]string{}
	for _, f := range zr.File {
		clean := path.Clean(f.Name)
		if clean == "." || clean == "" || strings.HasPrefix(clean, "/") || strings.HasPrefix(clean, "..") || strings.Contains(clean, "/../") {
			return zero, fmt.Errorf("%w: %q", ErrPluginArchiveTraversal, f.Name)
		}
		if strings.ContainsRune(clean, '/') {
			return zero, fmt.Errorf("%w: nested entry %q (v1 plugins are single-level)", ErrPluginArchiveInvalid, f.Name)
		}
		if f.FileInfo().IsDir() {
			continue
		}
		body, err := readArchiveEntry(f)
		if err != nil {
			return zero, fmt.Errorf("%w: read %s: %v", ErrPluginArchiveInvalid, clean, err)
		}
		entries[clean] = body
		if clean == "plugin.json" {
			manifestBody = body
		}
		if clean == "main.lua" {
			mainLuaPresent = true
		}
	}
	if manifestBody == "" {
		return zero, fmt.Errorf("%w: missing plugin.json", ErrPluginArchiveInvalid)
	}
	if !mainLuaPresent {
		return zero, fmt.Errorf("%w: missing main.lua", ErrPluginArchiveInvalid)
	}

	var m Manifest
	if err := json.Unmarshal([]byte(manifestBody), &m); err != nil {
		return zero, fmt.Errorf("%w: parse plugin.json: %v", ErrPluginArchiveInvalid, err)
	}
	if m.ManifestVersion != ManifestSchemaVersion {
		return zero, fmt.Errorf("%w: manifest_version %d, want %d", ErrManifestVersion, m.ManifestVersion, ManifestSchemaVersion)
	}
	if err := validateManifest(&m); err != nil {
		return zero, err
	}

	id := m.ID
	target := filepath.Join(pluginsDir, id)
	overwritten := false
	if fs.FileExists(target) {
		if existing, ok := readExistingVersion(fs, target); ok {
			if compareVersions(m.Version, existing) < 0 {
				return zero, fmt.Errorf("%w: incoming %s < installed %s (id=%s)",
					ErrPluginArchiveOlderVersion, m.Version, existing, id)
			}
		}
		if !overwrite {
			return zero, fmt.Errorf("%w: %s", ErrPluginArchiveExists, id)
		}
		if err := fs.DeleteFolder(target); err != nil {
			return zero, fmt.Errorf("clear existing %s: %w", target, err)
		}
		overwritten = true
	}
	if err := fs.EnsureDirectory(target); err != nil {
		return zero, fmt.Errorf("ensure %s: %w", target, err)
	}

	var written []string
	for name, body := range entries {
		full := filepath.Join(target, name)
		if err := fs.SaveFile(full, body); err != nil {
			return zero, fmt.Errorf("write %s: %w", full, err)
		}
		written = append(written, name)
	}

	return ImportArchiveResult{
		ID:          id,
		Overwritten: overwritten,
		Files:       written,
	}, nil
}

func writeArchiveEntry(zw *zip.Writer, name, body string) error {
	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("zip create %s: %w", name, err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		return fmt.Errorf("zip write %s: %w", name, err)
	}
	return nil
}

func readArchiveEntry(f *zip.File) (string, error) {
	r, err := f.Open()
	if err != nil {
		return "", err
	}
	defer r.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// readExistingVersion attempts to extract the version from the
// plugin.json already on disk at <target>/plugin.json. Returns
// (version, true) on success and ("", false) when the manifest is
// missing, unreadable, or unparseable — the caller falls back to
// the normal Exists gate in that case so a tampered install can
// still be replaced via overwrite=true.
func readExistingVersion(fs editorFS, target string) (string, bool) {
	manifestPath := filepath.Join(target, "plugin.json")
	if !fs.FileExists(manifestPath) {
		return "", false
	}
	body, err := fs.LoadFile(manifestPath)
	if err != nil {
		return "", false
	}
	var m struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return "", false
	}
	if strings.TrimSpace(m.Version) == "" {
		return "", false
	}
	return m.Version, true
}

// ExportArchive bundles <PluginsDir>/<id>/* into a zip at zipPath.
// The id is validated against validID; the editor fs handles the
// I/O. Refreshes are not necessary — exporting doesn't mutate the
// registry. See exportPluginArchive for the file-shape contract.
func (m *Manager) ExportArchive(id, zipPath string) (ExportArchiveResult, error) {
	if m.deps.Editor == nil {
		return ExportArchiveResult{}, fmt.Errorf("plugin: editor fs not configured")
	}
	return exportPluginArchive(m.deps.Editor, m.deps.PluginsDir, id, zipPath)
}

// ImportArchive materialises a plugin-archive zip at zipPath under
// <PluginsDir>/. Refuses to overwrite an existing plugin unless
// overwrite=true. On success the registry is Refresh'd so the
// Plugins workspace sees the new/replaced entry immediately.
func (m *Manager) ImportArchive(zipPath string, overwrite bool) (ImportArchiveResult, error) {
	if m.deps.Editor == nil {
		return ImportArchiveResult{}, fmt.Errorf("plugin: editor fs not configured")
	}
	res, err := importPluginArchive(m.deps.Editor, m.deps.PluginsDir, zipPath, overwrite)
	if err != nil {
		return res, err
	}
	if refreshErr := m.Refresh(); refreshErr != nil {
		// Log via the manager's logger but return the import result —
		// the files landed on disk; a refresh hiccup is recoverable
		// via the workspace's Refresh button.
		m.log.Warn("plugin: refresh after import failed", "id", res.ID, "err", refreshErr)
	}
	return res, nil
}
