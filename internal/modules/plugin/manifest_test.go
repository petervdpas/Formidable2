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
	// current build doesn't understand. We must not error on those -
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

func TestLoadManifest_RunModeRoundtrip(t *testing.T) {
	// Manifest.RunMode tells the runtime whether the plugin's form
	// is the entry point ("form") or whether commands are run from
	// the modal directly ("modal"). Empty/missing = "modal".
	root := t.TempDir()
	dir := writePlugin(t, root, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"run_mode": "form",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	got, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.RunMode != "form" {
		t.Fatalf("RunMode = %q, want %q", got.RunMode, "form")
	}
}

func TestLoadManifest_RunModeDefaultsEmpty(t *testing.T) {
	// A manifest without run_mode loads with RunMode == "" - the
	// frontend treats that as "modal" without writing a default
	// into older manifests.
	root := t.TempDir()
	dir := writePlugin(t, root, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	got, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.RunMode != "" {
		t.Fatalf("RunMode = %q, want empty default", got.RunMode)
	}
}

func TestLoadManifest_RunModeRejectsUnknown(t *testing.T) {
	// Reserved enum - keeps the contract tight. A typo like
	// "Form" or "Modal" should fail loading rather than silently
	// fall back to a default and surprise the author later.
	root := t.TempDir()
	dir := writePlugin(t, root, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"run_mode": "FORMULAIC",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	_, err := LoadManifest(dir)
	if !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("want ErrManifestInvalid, got %v", err)
	}
}

func TestLoadManifest_WorkspacesRoundtrip(t *testing.T) {
	// A plugin can declare which workspaces it contributes a topbar
	// entry to. Each entry must be a known workspace id; the closed
	// enum lives in types.go.
	root := t.TempDir()
	dir := writePlugin(t, root, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"workspaces": ["storage", "templates"],
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	got, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got.Workspaces) != 2 ||
		got.Workspaces[0] != WorkspaceStorage ||
		got.Workspaces[1] != WorkspaceTemplates {
		t.Fatalf("workspaces: %+v", got.Workspaces)
	}
}

func TestLoadManifest_WorkspacesOmittedDefaultsEmpty(t *testing.T) {
	// Omitted `workspaces` is the common case - older manifests
	// must keep loading. Surfaces as a nil/empty slice; downstream
	// callers treat that as "no topbar attachment".
	root := t.TempDir()
	dir := writePlugin(t, root, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	got, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got.Workspaces) != 0 {
		t.Fatalf("workspaces should default empty: %+v", got.Workspaces)
	}
}

func TestLoadManifest_WorkspacesRejectsUnknown(t *testing.T) {
	// "settings" and "plugins" are intentionally not in the enum -
	// the management workspace already lists every plugin, and
	// Settings has no domain object to act on. Unknown ids surface
	// as ErrManifestInvalid so a typo fails at load, not at click.
	cases := map[string]string{
		"settings is not allowed": `{
			"manifest_version": 1, "id": "demo", "name": "X", "version": "0.1.0",
			"workspaces": ["settings"],
			"commands": [{"id": "run", "label": "Run"}]}`,
		"plugins is not allowed": `{
			"manifest_version": 1, "id": "demo", "name": "X", "version": "0.1.0",
			"workspaces": ["plugins"],
			"commands": [{"id": "run", "label": "Run"}]}`,
		"typo": `{
			"manifest_version": 1, "id": "demo", "name": "X", "version": "0.1.0",
			"workspaces": ["storag"],
			"commands": [{"id": "run", "label": "Run"}]}`,
		"empty string in list": `{
			"manifest_version": 1, "id": "demo", "name": "X", "version": "0.1.0",
			"workspaces": [""],
			"commands": [{"id": "run", "label": "Run"}]}`,
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

func TestLoadManifest_WorkspacesRejectsDuplicates(t *testing.T) {
	// A duplicate entry is almost certainly a paste error and would
	// double-emit the same menu item. Reject it at load.
	root := t.TempDir()
	dir := writePlugin(t, root, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "X", "version": "0.1.0",
		"workspaces": ["storage", "storage"],
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	_, err := LoadManifest(dir)
	if !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("want ErrManifestInvalid, got %v", err)
	}
}

func TestValidWorkspaces_StableContent(t *testing.T) {
	// The enum is the frontend's source of truth - pin the values so
	// a careless rename here is a test failure, not a silent UI break.
	got := ValidWorkspaces()
	want := []string{
		WorkspaceStorage, WorkspaceTemplates, WorkspaceProfiles,
		WorkspaceCollaboration, WorkspaceInformation,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d workspaces, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("workspaces[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLoadManifest_PreservesCommandHideFlags(t *testing.T) {
	// A command can opt out of showing its Result/Log panels in the
	// Run modal - useful for "fire and forget" actions where the
	// return value is irrelevant. Default (omitted) = both visible.
	root := t.TempDir()
	dir := writePlugin(t, root, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [
			{"id": "loud", "label": "Loud"},
			{"id": "silent", "label": "Silent", "hide_output": true, "hide_log": true}
		]
	}`, "function loud() end\nfunction silent() end")

	got, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.Commands[0].HideOutput || got.Commands[0].HideLog {
		t.Fatalf("loud command should default to visible: %+v", got.Commands[0])
	}
	if !got.Commands[1].HideOutput || !got.Commands[1].HideLog {
		t.Fatalf("silent command should preserve hide flags: %+v", got.Commands[1])
	}
}

