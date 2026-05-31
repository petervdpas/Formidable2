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

var (
	// ErrPluginArchiveInvalid wraps any "this zip isn't a plugin bundle" failure.
	ErrPluginArchiveInvalid = errors.New("plugin: archive invalid")

	// ErrPluginArchiveTraversal is a hard refusal of entries whose Clean'd name escapes the plugin folder.
	ErrPluginArchiveTraversal = errors.New("plugin: archive path traversal blocked")

	// ErrPluginArchiveExists fires when the target id exists and overwrite wasn't set.
	ErrPluginArchiveExists = errors.New("plugin: target already exists (set overwrite=true to replace)")

	// ErrPluginArchiveNotFound fires when the source plugin isn't at <pluginsDir>/<id>.
	ErrPluginArchiveNotFound = errors.New("plugin: archive source plugin not found")

	// ErrPluginArchiveOlderVersion is the only meaning of the version field: downgrade protection.
	// The check is unconditional; overwrite=true cannot bypass it.
	ErrPluginArchiveOlderVersion = errors.New("plugin: archive version is older than installed")
)

// ExportArchiveResult describes the bundle; Files lists zip-entry names relative to the plugin root.
type ExportArchiveResult struct {
	ID      string   `json:"id"`
	ZipPath string   `json:"zip_path"`
	Files   []string `json:"files"`
}

// ImportArchiveResult describes the import; Overwritten flags whether an existing plugin was replaced.
type ImportArchiveResult struct {
	ID          string   `json:"id"`
	Overwritten bool     `json:"overwritten"`
	Files       []string `json:"files"`
}

// exportPluginArchive bundles <pluginsDir>/<id>/** into a zip at zipPath. id is validated up front; hidden entries
// are skipped recursively; zip entry names use forward slashes for cross-OS portability.
// The .kv state file is intentionally NOT bundled: it's per-user runtime state, not part of the distributable shape.
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

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	var files []string
	if err := walkArchiveEntries(fs, dir, "", zw, &files); err != nil {
		return zero, err
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

// walkArchiveEntries depth-first walks dir, adding every non-hidden regular file to zw; rel is the forward-slash zip-entry prefix.
func walkArchiveEntries(fs editorFS, dir, rel string, zw *zip.Writer, files *[]string) error {
	entries, err := fs.ListDir(dir)
	if err != nil {
		return fmt.Errorf("list %s: %w", dir, err)
	}
	for _, name := range entries {
		if strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(dir, name)
		entryName := name
		if rel != "" {
			entryName = path.Join(rel, name)
		}
		if fs.IsDir(full) {
			if err := walkArchiveEntries(fs, full, entryName, zw, files); err != nil {
				return err
			}
			continue
		}
		body, err := fs.LoadFile(full)
		if err != nil {
			return fmt.Errorf("read %s: %w", full, err)
		}
		if err := writeArchiveEntry(zw, entryName, body); err != nil {
			return err
		}
		*files = append(*files, entryName)
	}
	return nil
}

// importPluginArchive writes a zip's contents to <pluginsDir>/<id>/, with id inferred from the embedded plugin.json (NOT the zip filename).
// The zip must contain a valid plugin.json + main.lua at the root and no post-Clean traversal segments.
// An existing id refuses unless overwrite=true, which first removes the whole folder so stale side files don't survive.
// The plugin's .kv state is left alone: runtime state outlives the manifest, as a Save would.
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
		full := filepath.Join(target, filepath.FromSlash(name))
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

// readExistingVersion extracts version from the on-disk plugin.json, returning ("", false) when missing/unparseable
// so the caller falls back to the Exists gate and a tampered install can still be replaced via overwrite=true.
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

// ExportArchive bundles <PluginsDir>/<id>/* into a zip at zipPath; see exportPluginArchive for the file-shape contract.
func (m *Manager) ExportArchive(id, zipPath string) (ExportArchiveResult, error) {
	if m.deps.Editor == nil {
		return ExportArchiveResult{}, fmt.Errorf("plugin: editor fs not configured")
	}
	return exportPluginArchive(m.deps.Editor, m.deps.PluginsDir, id, zipPath)
}

// ImportArchive materialises a plugin-archive zip under <PluginsDir>/, refusing to overwrite unless overwrite=true, then Refreshes the registry.
func (m *Manager) ImportArchive(zipPath string, overwrite bool) (ImportArchiveResult, error) {
	if m.deps.Editor == nil {
		return ImportArchiveResult{}, fmt.Errorf("plugin: editor fs not configured")
	}
	res, err := importPluginArchive(m.deps.Editor, m.deps.PluginsDir, zipPath, overwrite)
	if err != nil {
		return res, err
	}
	if refreshErr := m.Refresh(); refreshErr != nil {
		// Files already landed; return success since a refresh hiccup is recoverable via the workspace Refresh button.
		m.log.Warn("plugin: refresh after import failed", "id", res.ID, "err", refreshErr)
	}
	return res, nil
}
