package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writePlugin lays out a plugin folder for the test. manifest is
// written as plugin.json verbatim; main is the contents of
// main.lua (skipped when empty).
func writePlugin(t *testing.T, root, id, manifest, main string) string {
	t.Helper()
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if manifest != "" {
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
	}
	if main != "" {
		if err := os.WriteFile(filepath.Join(dir, "main.lua"), []byte(main), 0o644); err != nil {
			t.Fatalf("write main: %v", err)
		}
	}
	return dir
}

func TestLoadManifest_Happy(t *testing.T) {
	root := t.TempDir()
	dir := writePlugin(t, root, "demo", `{
		"manifest_version": 1,
		"id": "demo",
		"name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() return 1 end")

	got, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.ID != "demo" || got.Name != "Demo" || got.Version != "0.1.0" {
		t.Fatalf("got %+v", got)
	}
	if len(got.Commands) != 1 || got.Commands[0].ID != "run" {
		t.Fatalf("commands: %+v", got.Commands)
	}
}

func TestLoadManifest_MissingFile(t *testing.T) {
	root := t.TempDir()
	dir := writePlugin(t, root, "demo", "", "function run() end")
	_, err := LoadManifest(dir)
	if !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("want ErrManifestInvalid, got %v", err)
	}
}

func TestLoadManifest_MalformedJSON(t *testing.T) {
	root := t.TempDir()
	dir := writePlugin(t, root, "demo", `{not json`, "function run() end")
	_, err := LoadManifest(dir)
	if !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("want ErrManifestInvalid, got %v", err)
	}
}

func TestLoadManifest_WrongSchemaVersion(t *testing.T) {
	root := t.TempDir()
	dir := writePlugin(t, root, "demo", `{
		"manifest_version": 99,
		"id": "demo", "name": "Demo", "version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	_, err := LoadManifest(dir)
	if !errors.Is(err, ErrManifestVersion) {
		t.Fatalf("want ErrManifestVersion, got %v", err)
	}
}

func TestLoadManifest_MissingMainLua(t *testing.T) {
	root := t.TempDir()
	dir := writePlugin(t, root, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "")
	_, err := LoadManifest(dir)
	if !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("want ErrManifestInvalid, got %v", err)
	}
}

func TestLoadManifest_RejectsBadFields(t *testing.T) {
	cases := map[string]string{
		"empty id": `{
			"manifest_version": 1, "id": "", "name": "X", "version": "0.1.0",
			"commands": [{"id": "run", "label": "Run"}]}`,
		"id with slash": `{
			"manifest_version": 1, "id": "a/b", "name": "X", "version": "0.1.0",
			"commands": [{"id": "run", "label": "Run"}]}`,
		"id traversal": `{
			"manifest_version": 1, "id": "..", "name": "X", "version": "0.1.0",
			"commands": [{"id": "run", "label": "Run"}]}`,
		"id uppercase": `{
			"manifest_version": 1, "id": "DEMO", "name": "X", "version": "0.1.0",
			"commands": [{"id": "run", "label": "Run"}]}`,
		"empty name": `{
			"manifest_version": 1, "id": "demo", "name": "", "version": "0.1.0",
			"commands": [{"id": "run", "label": "Run"}]}`,
		"empty version": `{
			"manifest_version": 1, "id": "demo", "name": "X", "version": "",
			"commands": [{"id": "run", "label": "Run"}]}`,
		"no commands": `{
			"manifest_version": 1, "id": "demo", "name": "X", "version": "0.1.0",
			"commands": []}`,
		"command empty id": `{
			"manifest_version": 1, "id": "demo", "name": "X", "version": "0.1.0",
			"commands": [{"id": "", "label": "Run"}]}`,
	}
	for name, manifest := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			dir := writePlugin(t, root, "demo", manifest, "function run() end")
			_, err := LoadManifest(dir)
			if !errors.Is(err, ErrManifestInvalid) {
				t.Fatalf("want ErrManifestInvalid, got %v", err)
			}
		})
	}
}

func TestLoadManifest_TolerantToUnknownFields(t *testing.T) {
	// Forwards-compat: a future Formidable might add fields the
	// current build doesn't understand. We must not error on those —
	// just ignore them. (Bumping ManifestSchemaVersion is the
	// signal that something changed in a *required* way.)
	root := t.TempDir()
	dir := writePlugin(t, root, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "X",
		"version": "0.1.0",
		"future_field": {"nested": [1,2,3]},
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	if _, err := LoadManifest(dir); err != nil {
		t.Fatalf("want tolerance, got: %v", err)
	}
}

func TestLoadManifest_CommandFnDefaultsToID(t *testing.T) {
	// `Fn` is optional; when omitted the command id is also the
	// Lua function name. Documented in types.go.
	root := t.TempDir()
	dir := writePlugin(t, root, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "X",
		"version": "0.1.0",
		"commands": [{"id": "do_thing", "label": "Do thing"}]
	}`, "function do_thing() end")
	got, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.Commands[0].Fn != "" {
		t.Fatalf("Fn should remain empty when not set; got %q", got.Commands[0].Fn)
	}
	// FnNameFor resolves the default.
	if name := FnNameFor(got.Commands[0]); name != "do_thing" {
		t.Fatalf("FnNameFor: got %q", name)
	}
}

func TestFnNameFor_ExplicitOverridesID(t *testing.T) {
	if FnNameFor(Command{ID: "do_thing", Fn: "actually_this"}) != "actually_this" {
		t.Fatal("explicit Fn should win")
	}
}
