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

// A plugin with a sub-folder (i18n/<locale>.json is the driving case)
// must round-trip its nested entries through the archive - the
// pre-change exporter died on the directory because it treated every
// listing entry as a file.
func TestExport_BundlesSubdirectories(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	seedTestPlugin(t, plugins, "demo", nil)
	i18nDir := filepath.Join(plugins, "demo", "i18n")
	if err := os.MkdirAll(i18nDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(i18nDir, "en.json"), `{"name":"Demo"}`+"\n")
	mustWrite(t, filepath.Join(i18nDir, "nl.json"), `{"name":"Demo"}`+"\n")

	zipPath := filepath.Join(root, "demo.zip")
	res, err := exportPluginArchive(kvTestFS{}, plugins, "demo", zipPath)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	raw, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	entries := readZipEntries(t, string(raw))
	for _, want := range []string{"plugin.json", "main.lua", "form.json", "i18n/en.json", "i18n/nl.json"} {
		if _, ok := entries[want]; !ok {
			t.Errorf("zip missing %q (entries=%v)", want, res.Files)
		}
	}
	// No bare "i18n" entry - directory itself doesn't get serialised.
	if _, hit := entries["i18n"]; hit {
		t.Errorf("zip contains bare directory entry %q", "i18n")
	}
	if _, hit := entries["i18n/"]; hit {
		t.Errorf("zip contains directory entry %q", "i18n/")
	}
}

// A hidden subdirectory (.git, .vscode, …) gets skipped wholesale -
// matching the same rule applied to hidden top-level files.
func TestExport_SkipsHiddenSubdirectories(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	seedTestPlugin(t, plugins, "demo", nil)
	hidden := filepath.Join(plugins, "demo", ".git")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(hidden, "config"), "[core]\n")

	zipPath := filepath.Join(root, "demo.zip")
	if _, err := exportPluginArchive(kvTestFS{}, plugins, "demo", zipPath); err != nil {
		t.Fatalf("export: %v", err)
	}
	raw, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	for name := range readZipEntries(t, string(raw)) {
		if strings.HasPrefix(name, ".git") {
			t.Errorf("hidden subdir leaked: %q", name)
		}
	}
}

func TestRoundTrip_WithI18nDir(t *testing.T) {
	srcRoot := t.TempDir()
	srcPlugins := filepath.Join(srcRoot, "plugins")
	seedTestPlugin(t, srcPlugins, "intl", nil)
	i18n := filepath.Join(srcPlugins, "intl", "i18n")
	if err := os.MkdirAll(i18n, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(i18n, "en.json"), `{"name":"Intl"}`+"\n")
	mustWrite(t, filepath.Join(i18n, "nl.json"), `{"name":"Intl-nl"}`+"\n")

	zipPath := filepath.Join(srcRoot, "intl.zip")
	if _, err := exportPluginArchive(kvTestFS{}, srcPlugins, "intl", zipPath); err != nil {
		t.Fatalf("export: %v", err)
	}

	dstRoot := t.TempDir()
	dstPlugins := filepath.Join(dstRoot, "plugins")
	dstZip := filepath.Join(dstRoot, "intl.zip")
	raw, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dstZip, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := importPluginArchive(kvTestFS{}, dstPlugins, dstZip, false); err != nil {
		t.Fatalf("import: %v", err)
	}

	for rel, want := range map[string]string{
		"i18n/en.json": `{"name":"Intl"}` + "\n",
		"i18n/nl.json": `{"name":"Intl-nl"}` + "\n",
	} {
		got, err := os.ReadFile(filepath.Join(dstPlugins, "intl", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("missing on destination: %s (%v)", rel, err)
		}
		if string(got) != want {
			t.Errorf("%s body = %q, want %q", rel, string(got), want)
		}
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
	return validManifestVersion(id, "0.1.0")
}

func validManifestVersion(id, version string) string {
	return `{
  "manifest_version": 1,
  "id": "` + id + `",
  "name": "Imported",
  "version": "` + version + `",
  "commands": [{"id": "run", "label": "Run"}]
}
`
}

// seedTestPluginVersion writes a plugin with an explicit version
// number so the import-version gate has something to compare against.
func seedTestPluginVersion(t *testing.T, pluginsDir, id, version string) {
	t.Helper()
	dir := filepath.Join(pluginsDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "plugin.json"), validManifestVersion(id, version))
	mustWrite(t, filepath.Join(dir, "main.lua"), "function run(ctx) return { ok = true } end\n")
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
	// Old side file should be gone - overwrite is a full replace.
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

// Nested entries are accepted (i18n/<locale>.json is the driving case)
// as long as no segment escapes the plugin folder.
func TestImport_AcceptsNestedEntries(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	zipPath := filepath.Join(root, "incoming.zip")
	buildArchive(t, kvTestFS{}, zipPath, map[string]string{
		"plugin.json":     validManifest("nested"),
		"main.lua":        "function run(ctx) end\n",
		"i18n/en.json":    `{"name":"Nested"}` + "\n",
		"i18n/nl.json":    `{"name":"Genest"}` + "\n",
	})

	res, err := importPluginArchive(kvTestFS{}, plugins, zipPath, false)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.ID != "nested" {
		t.Errorf("ID = %q, want %q", res.ID, "nested")
	}
	for _, rel := range []string{"plugin.json", "main.lua", "i18n/en.json", "i18n/nl.json"} {
		full := filepath.Join(plugins, "nested", filepath.FromSlash(rel))
		if _, err := os.Stat(full); err != nil {
			t.Errorf("missing on disk after import: %s (%v)", rel, err)
		}
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

// --- Import: version gate ---

// The version field's sole job is to block a downgrade: an incoming
// zip whose plugin.json declares a version lower than what is
// already on disk must be rejected even when the caller asked to
// overwrite. Same/higher version follows the existing overwrite
// gate.

func TestImport_RejectsLowerVersionEvenWithOverwrite(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	seedTestPluginVersion(t, plugins, "ver", "0.2.0")

	zipPath := filepath.Join(root, "incoming.zip")
	buildArchive(t, kvTestFS{}, zipPath, map[string]string{
		"plugin.json": validManifestVersion("ver", "0.1.0"),
		"main.lua":    "function run(ctx) end\n",
	})

	_, err := importPluginArchive(kvTestFS{}, plugins, zipPath, true)
	if !errors.Is(err, ErrPluginArchiveOlderVersion) {
		t.Errorf("err = %v, want ErrPluginArchiveOlderVersion", err)
	}

	// On-disk plugin.json must remain at the higher version.
	got, err := os.ReadFile(filepath.Join(plugins, "ver", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"version": "0.2.0"`) {
		t.Errorf("on-disk version was clobbered; got %q", string(got))
	}
}

func TestImport_RejectsLowerVersionWithoutOverwrite(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	seedTestPluginVersion(t, plugins, "ver", "0.2.0")

	zipPath := filepath.Join(root, "incoming.zip")
	buildArchive(t, kvTestFS{}, zipPath, map[string]string{
		"plugin.json": validManifestVersion("ver", "0.1.0"),
		"main.lua":    "function run(ctx) end\n",
	})

	_, err := importPluginArchive(kvTestFS{}, plugins, zipPath, false)
	if !errors.Is(err, ErrPluginArchiveOlderVersion) {
		t.Errorf("err = %v, want ErrPluginArchiveOlderVersion (version takes precedence over Exists)", err)
	}
}

func TestImport_AllowsHigherVersionWithOverwrite(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	seedTestPluginVersion(t, plugins, "ver", "0.1.0")

	zipPath := filepath.Join(root, "incoming.zip")
	buildArchive(t, kvTestFS{}, zipPath, map[string]string{
		"plugin.json": validManifestVersion("ver", "0.2.0"),
		"main.lua":    "function run(ctx) end\n",
	})

	res, err := importPluginArchive(kvTestFS{}, plugins, zipPath, true)
	if err != nil {
		t.Fatalf("import upgrade with overwrite: %v", err)
	}
	if !res.Overwritten {
		t.Errorf("Overwritten = false, want true")
	}
	got, _ := os.ReadFile(filepath.Join(plugins, "ver", "plugin.json"))
	if !strings.Contains(string(got), `"version": "0.2.0"`) {
		t.Errorf("on-disk version not updated to 0.2.0; got %q", string(got))
	}
}

func TestImport_AllowsSameVersionWithOverwrite(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	seedTestPluginVersion(t, plugins, "ver", "0.2.0")

	zipPath := filepath.Join(root, "incoming.zip")
	buildArchive(t, kvTestFS{}, zipPath, map[string]string{
		"plugin.json": validManifestVersion("ver", "0.2.0"),
		"main.lua":    "function run(ctx) end\n",
	})

	if _, err := importPluginArchive(kvTestFS{}, plugins, zipPath, true); err != nil {
		t.Fatalf("same-version reinstall should succeed with overwrite: %v", err)
	}
}

func TestImport_FreshInstallIgnoresVersion(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	if err := os.MkdirAll(plugins, 0o755); err != nil {
		t.Fatal(err)
	}

	zipPath := filepath.Join(root, "incoming.zip")
	buildArchive(t, kvTestFS{}, zipPath, map[string]string{
		"plugin.json": validManifestVersion("fresh", "0.0.1"),
		"main.lua":    "function run(ctx) end\n",
	})

	if _, err := importPluginArchive(kvTestFS{}, plugins, zipPath, false); err != nil {
		t.Fatalf("fresh install must not consult version: %v", err)
	}
}

// A pre-existing plugin folder whose plugin.json is unreadable can
// happen after a partial write or manual tampering. The version gate
// must not panic or block; it falls through to the existing Exists
// gate so the user sees the same "use overwrite" message as before.
func TestImport_UnreadableExistingManifestFallsThrough(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	dir := filepath.Join(plugins, "broken")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "plugin.json"), "{not valid json")
	mustWrite(t, filepath.Join(dir, "main.lua"), "")

	zipPath := filepath.Join(root, "incoming.zip")
	buildArchive(t, kvTestFS{}, zipPath, map[string]string{
		"plugin.json": validManifestVersion("broken", "0.1.0"),
		"main.lua":    "function run(ctx) end\n",
	})

	_, err := importPluginArchive(kvTestFS{}, plugins, zipPath, false)
	if !errors.Is(err, ErrPluginArchiveExists) {
		t.Errorf("err = %v, want ErrPluginArchiveExists (no version comparable, normal flow)", err)
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

	// Fresh destination - simulates importing on another machine.
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
