package plugin

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedTestPlugin writes a minimal valid plugin tree under
// <pluginsDir>/<id>/ so the export tests have something to bundle.
func seedTestPlugin(t *testing.T, pluginsDir, id string, sideFiles map[string]string) {
	t.Helper()
	dir := filepath.Join(pluginsDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "manifest_version": 1,
  "id": "` + id + `",
  "name": "Seed",
  "version": "0.1.0",
  "commands": [{"id": "run", "label": "Run"}]
}
`
	mustWrite(t, filepath.Join(dir, "plugin.json"), manifest)
	mustWrite(t, filepath.Join(dir, "main.lua"), "function run(ctx) return { ok = true } end\n")
	mustWrite(t, filepath.Join(dir, "form.json"), "[]\n")
	for name, body := range sideFiles {
		mustWrite(t, filepath.Join(dir, name), body)
	}
}

func mustWrite(t *testing.T, p, body string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readZipEntries(t *testing.T, raw string) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader([]byte(raw)), int64(len(raw)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = string(b)
	}
	return out
}

// --- Export ---

func TestExport_HappyPath(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	seedTestPlugin(t, plugins, "demo", nil)

	zipPath := filepath.Join(root, "demo.zip")
	res, err := exportPluginArchive(kvTestFS{}, plugins, "demo", zipPath)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if res.ID != "demo" {
		t.Errorf("ID = %q, want %q", res.ID, "demo")
	}
	if len(res.Files) != 3 {
		t.Errorf("Files = %v, want 3 entries", res.Files)
	}
	raw, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	entries := readZipEntries(t, string(raw))
	for _, name := range []string{"plugin.json", "main.lua", "form.json"} {
		if _, ok := entries[name]; !ok {
			t.Errorf("zip missing %q", name)
		}
	}
}

func TestExport_IncludesSideFiles(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	seedTestPlugin(t, plugins, "demo", map[string]string{
		"README.md":  "# demo\n",
		"helper.lua": "return {}\n",
	})

	zipPath := filepath.Join(root, "demo.zip")
	res, err := exportPluginArchive(kvTestFS{}, plugins, "demo", zipPath)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(res.Files) != 5 {
		t.Errorf("Files = %v, want 5 entries (3 core + 2 side)", res.Files)
	}
}

func TestExport_SkipsHiddenFiles(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	seedTestPlugin(t, plugins, "demo", map[string]string{
		".scratch":     "ignore me",
		".DS_Store":    "macos cruft",
		"keepme.lua":   "return {}\n",
	})

	zipPath := filepath.Join(root, "demo.zip")
	res, err := exportPluginArchive(kvTestFS{}, plugins, "demo", zipPath)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	for _, name := range res.Files {
		if strings.HasPrefix(name, ".") {
			t.Errorf("hidden file %q leaked into archive", name)
		}
	}
	hasKeep := false
	for _, name := range res.Files {
		if name == "keepme.lua" {
			hasKeep = true
		}
	}
	if !hasKeep {
		t.Errorf("non-hidden side file keepme.lua missing from archive")
	}
}

func TestExport_RejectsBadID(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "demo.zip")

	cases := []string{"", "../escape", "with spaces", "dot.in.id"}
	for _, id := range cases {
		_, err := exportPluginArchive(kvTestFS{}, root, id, zipPath)
		if !errors.Is(err, ErrManifestInvalid) {
			t.Errorf("id %q: err = %v, want ErrManifestInvalid", id, err)
		}
	}
}

func TestExport_PluginNotFound(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	if err := os.MkdirAll(plugins, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := exportPluginArchive(kvTestFS{}, plugins, "does-not-exist", filepath.Join(root, "x.zip"))
	if !errors.Is(err, ErrPluginArchiveNotFound) {
		t.Errorf("err = %v, want ErrPluginArchiveNotFound", err)
	}
}

func TestExport_NilFSRejected(t *testing.T) {
	_, err := exportPluginArchive(nil, "/", "demo", "/tmp/x.zip")
	if !errors.Is(err, ErrPluginArchiveInvalid) {
		t.Errorf("err = %v, want ErrPluginArchiveInvalid", err)
	}
}

// --- Import ---

// buildArchive creates an in-memory zip with the given entries and
// writes it to zipPath via the supplied fs so import has something
// to read.
func buildArchive(t *testing.T, fs editorFS, zipPath string, entries map[string]string) {
	t.Helper()
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fs.SaveFile(zipPath, buf.String()); err != nil {
		t.Fatal(err)
	}
}

func validManifest(id string) string {
	return `{
  "manifest_version": 1,
  "id": "` + id + `",
  "name": "Imported",
  "version": "0.1.0",
  "commands": [{"id": "run", "label": "Run"}]
}
`
}

func TestImport_HappyPath(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	if err := os.MkdirAll(plugins, 0o755); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(root, "incoming.zip")
	buildArchive(t, kvTestFS{}, zipPath, map[string]string{
		"plugin.json": validManifest("imported"),
		"main.lua":    "function run(ctx) end\n",
		"form.json":   "[]\n",
	})

	res, err := importPluginArchive(kvTestFS{}, plugins, zipPath, false)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.ID != "imported" {
		t.Errorf("ID = %q, want %q", res.ID, "imported")
	}
	if res.Overwritten {
		t.Errorf("Overwritten = true, want false (fresh import)")
	}
	for _, name := range []string{"plugin.json", "main.lua", "form.json"} {
		if _, err := os.Stat(filepath.Join(plugins, "imported", name)); err != nil {
			t.Errorf("missing on disk: %s (%v)", name, err)
		}
	}
}

func TestImport_RefusesExistingWithoutOverwrite(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	seedTestPlugin(t, plugins, "duplicated", nil)

	zipPath := filepath.Join(root, "incoming.zip")
	buildArchive(t, kvTestFS{}, zipPath, map[string]string{
		"plugin.json": validManifest("duplicated"),
		"main.lua":    "function run(ctx) end\n",
	})

	_, err := importPluginArchive(kvTestFS{}, plugins, zipPath, false)
	if !errors.Is(err, ErrPluginArchiveExists) {
		t.Errorf("err = %v, want ErrPluginArchiveExists", err)
	}
}

func TestImport_OverwriteReplacesExisting(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	seedTestPlugin(t, plugins, "duplicated", map[string]string{
		"stale.txt": "old side file",
	})

	zipPath := filepath.Join(root, "incoming.zip")
	buildArchive(t, kvTestFS{}, zipPath, map[string]string{
		"plugin.json": validManifest("duplicated"),
		"main.lua":    "function run(ctx) end\n",
		"form.json":   "[]\n",
	})

	res, err := importPluginArchive(kvTestFS{}, plugins, zipPath, true)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if !res.Overwritten {
		t.Errorf("Overwritten = false, want true")
	}
	// Old side file should be gone — overwrite is a full replace.
	if _, err := os.Stat(filepath.Join(plugins, "duplicated", "stale.txt")); !os.IsNotExist(err) {
		t.Errorf("stale.txt should have been removed during overwrite; err = %v", err)
	}
}

func TestImport_MissingManifest(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	zipPath := filepath.Join(root, "incoming.zip")
	buildArchive(t, kvTestFS{}, zipPath, map[string]string{
		"main.lua": "function run(ctx) end\n",
	})

	_, err := importPluginArchive(kvTestFS{}, plugins, zipPath, false)
	if !errors.Is(err, ErrPluginArchiveInvalid) {
		t.Errorf("err = %v, want ErrPluginArchiveInvalid", err)
	}
}

func TestImport_MissingMainLua(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	zipPath := filepath.Join(root, "incoming.zip")
	buildArchive(t, kvTestFS{}, zipPath, map[string]string{
		"plugin.json": validManifest("noscript"),
	})

	_, err := importPluginArchive(kvTestFS{}, plugins, zipPath, false)
	if !errors.Is(err, ErrPluginArchiveInvalid) {
		t.Errorf("err = %v, want ErrPluginArchiveInvalid", err)
	}
}

func TestImport_RejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	zipPath := filepath.Join(root, "incoming.zip")
	buildArchive(t, kvTestFS{}, zipPath, map[string]string{
		"plugin.json":            validManifest("evil"),
		"main.lua":               "function run(ctx) end\n",
		"../../etc/passwd":       "rooted",
	})

	_, err := importPluginArchive(kvTestFS{}, plugins, zipPath, false)
	if !errors.Is(err, ErrPluginArchiveTraversal) {
		t.Errorf("err = %v, want ErrPluginArchiveTraversal", err)
	}
}

func TestImport_RejectsNestedEntries(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	zipPath := filepath.Join(root, "incoming.zip")
	buildArchive(t, kvTestFS{}, zipPath, map[string]string{
		"plugin.json":      validManifest("nested"),
		"main.lua":         "function run(ctx) end\n",
		"sub/extra.lua":    "return {}\n",
	})

	_, err := importPluginArchive(kvTestFS{}, plugins, zipPath, false)
	if !errors.Is(err, ErrPluginArchiveInvalid) {
		t.Errorf("err = %v, want ErrPluginArchiveInvalid", err)
	}
}

func TestImport_RejectsBadManifest(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	zipPath := filepath.Join(root, "incoming.zip")
	buildArchive(t, kvTestFS{}, zipPath, map[string]string{
		"plugin.json": "{not valid json",
		"main.lua":    "function run(ctx) end\n",
	})

	_, err := importPluginArchive(kvTestFS{}, plugins, zipPath, false)
	if !errors.Is(err, ErrPluginArchiveInvalid) {
		t.Errorf("err = %v, want ErrPluginArchiveInvalid", err)
	}
}

func TestImport_RejectsMismatchedManifestVersion(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	zipPath := filepath.Join(root, "incoming.zip")
	buildArchive(t, kvTestFS{}, zipPath, map[string]string{
		"plugin.json": `{"manifest_version":999,"id":"futuristic","name":"X","version":"0.1.0","commands":[{"id":"run","label":"Run"}]}`,
		"main.lua":    "function run(ctx) end\n",
	})

	_, err := importPluginArchive(kvTestFS{}, plugins, zipPath, false)
	if !errors.Is(err, ErrManifestVersion) {
		t.Errorf("err = %v, want ErrManifestVersion", err)
	}
}

func TestImport_ZipNotFound(t *testing.T) {
	root := t.TempDir()
	_, err := importPluginArchive(kvTestFS{}, filepath.Join(root, "plugins"), filepath.Join(root, "missing.zip"), false)
	if !errors.Is(err, ErrPluginArchiveNotFound) {
		t.Errorf("err = %v, want ErrPluginArchiveNotFound", err)
	}
}

// --- Round-trip ---

func TestRoundTrip_ExportThenImport(t *testing.T) {
	srcRoot := t.TempDir()
	srcPlugins := filepath.Join(srcRoot, "plugins")
	seedTestPlugin(t, srcPlugins, "trip", map[string]string{
		"side.txt": "side body\n",
	})

	zipPath := filepath.Join(srcRoot, "trip.zip")
	if _, err := exportPluginArchive(kvTestFS{}, srcPlugins, "trip", zipPath); err != nil {
		t.Fatalf("export: %v", err)
	}

	// Fresh destination — simulates importing on another machine.
	dstRoot := t.TempDir()
	dstPlugins := filepath.Join(dstRoot, "plugins")
	dstZip := filepath.Join(dstRoot, "trip.zip")
	raw, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dstRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dstZip, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := importPluginArchive(kvTestFS{}, dstPlugins, dstZip, false)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.ID != "trip" {
		t.Errorf("ID = %q, want %q", res.ID, "trip")
	}
	// Side file made it through the trip.
	body, err := os.ReadFile(filepath.Join(dstPlugins, "trip", "side.txt"))
	if err != nil {
		t.Fatalf("side.txt missing on destination: %v", err)
	}
	if string(body) != "side body\n" {
		t.Errorf("side.txt body = %q, want %q", string(body), "side body\n")
	}
}
