package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// newEditorTestManager mirrors newTestManager but also wires the
// Editor surface (kvTestFS satisfies it) so CRUD tests can hit
// every code path. Returned dir is the plugins root.
func newEditorTestManager(t *testing.T) (*Manager, string) {
	t.Helper()
	root := t.TempDir()
	pluginsDir := filepath.Join(root, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	m := NewManager(ManagerDeps{
		PluginsDir: pluginsDir,
		KV:         NewKV(kvTestFS{}, filepath.Join(pluginsDir, ".kv")),
		Editor:     kvTestFS{},
	})
	return m, pluginsDir
}

// ── Create ────────────────────────────────────────────────────────────

func TestManager_Create_GeneratesValidPlugin(t *testing.T) {
	m, dir := newEditorTestManager(t)
	if err := m.Create("hello"); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Manifest exists and is a valid plugin (LoadManifest agrees).
	got, err := LoadManifest(filepath.Join(dir, "hello"))
	if err != nil {
		t.Fatalf("scaffolded manifest invalid: %v", err)
	}
	if got.ID != "hello" || got.Name == "" || got.Version == "" {
		t.Fatalf("unexpected default manifest: %+v", got)
	}
	if len(got.Commands) == 0 {
		t.Fatal("default manifest must have at least one command")
	}
	// main.lua exists and is non-empty.
	src, err := os.ReadFile(filepath.Join(dir, "hello", "main.lua"))
	if err != nil {
		t.Fatalf("read main.lua: %v", err)
	}
	if len(src) == 0 {
		t.Fatal("scaffolded main.lua should not be empty")
	}
	// And the registry knows about it without an explicit Refresh.
	if _, ok := m.Get("hello"); !ok {
		t.Fatal("registry didn't pick up new plugin after Create")
	}
}

func TestManager_Create_ScaffoldsEmptyFormJSON(t *testing.T) {
	// Plugins get a form.json next to plugin.json + main.lua so the
	// future visual builder always has something to load. Default
	// content is an empty JSON array — "no fields yet."
	m, dir := newEditorTestManager(t)
	if err := m.Create("hello"); err != nil {
		t.Fatalf("create: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "hello", "form.json"))
	if err != nil {
		t.Fatalf("form.json missing: %v", err)
	}
	var fields []any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("form.json must parse as JSON array: %v (got %q)", err, raw)
	}
	if len(fields) != 0 {
		t.Fatalf("default form.json should be empty, got %d fields", len(fields))
	}
}

func TestManager_Create_RejectsInvalidID(t *testing.T) {
	m, _ := newEditorTestManager(t)
	cases := []string{"", "..", "a/b", "Demo", "has space"}
	for _, id := range cases {
		t.Run(id, func(t *testing.T) {
			if err := m.Create(id); !errors.Is(err, ErrManifestInvalid) {
				t.Fatalf("Create(%q): want ErrManifestInvalid, got %v", id, err)
			}
		})
	}
}

func TestManager_Create_RejectsExisting(t *testing.T) {
	m, _ := newEditorTestManager(t)
	if err := m.Create("dup"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	err := m.Create("dup")
	if !errors.Is(err, ErrPluginExists) {
		t.Fatalf("second create: want ErrPluginExists, got %v", err)
	}
}

func TestManager_Create_RejectsTraversalUp(t *testing.T) {
	// Belt-and-braces: "../escape" must never reach the filesystem
	// layer. validID rejects "/" but extra-paranoid here.
	m, dir := newEditorTestManager(t)
	if err := m.Create("../escape"); !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("traversal accepted: %v", err)
	}
	// Sibling of plugins dir must not exist.
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape")); err == nil {
		t.Fatal("traversal write created a sibling folder")
	}
}

// ── Save ──────────────────────────────────────────────────────────────

func TestManager_Save_RoundtripsManifestAndSource(t *testing.T) {
	m, _ := newEditorTestManager(t)
	if err := m.Create("demo"); err != nil {
		t.Fatalf("create: %v", err)
	}
	updated := Manifest{
		ManifestVersion: 1,
		ID:              "demo",
		Name:            "Demo Edited",
		Version:         "0.2.0",
		Description:     "edited",
		Author:          "peter",
		Commands: []Command{
			{ID: "main", Label: "Main"},
			{ID: "extra", Label: "Extra", Fn: "do_extra"},
		},
	}
	src := "function main() return 'hi' end\nfunction do_extra() return 1 end\n"
	if err := m.Save("demo", updated, src); err != nil {
		t.Fatalf("save: %v", err)
	}
	// Read it back via the Manager (registry stays consistent).
	p, ok := m.Get("demo")
	if !ok {
		t.Fatal("plugin gone after save")
	}
	if p.Manifest.Name != "Demo Edited" || p.Manifest.Version != "0.2.0" {
		t.Fatalf("got %+v", p.Manifest)
	}
	if len(p.Manifest.Commands) != 2 || p.Manifest.Commands[1].Fn != "do_extra" {
		t.Fatalf("commands: %+v", p.Manifest.Commands)
	}
	gotSrc, err := m.GetSource("demo")
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if gotSrc != src {
		t.Fatalf("source roundtrip: got %q", gotSrc)
	}
}

func TestManager_Save_RejectsIDMismatch(t *testing.T) {
	m, _ := newEditorTestManager(t)
	if err := m.Create("demo"); err != nil {
		t.Fatalf("create: %v", err)
	}
	bogus := Manifest{
		ManifestVersion: 1,
		ID:              "different", // mismatch with the path id
		Name:            "X",
		Version:         "0.1.0",
		Commands:        []Command{{ID: "run", Label: "Run"}},
	}
	err := m.Save("demo", bogus, "function run() end")
	if !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("want ErrManifestInvalid for id mismatch, got %v", err)
	}
}

func TestManager_Save_RejectsInvalidManifest(t *testing.T) {
	m, _ := newEditorTestManager(t)
	if err := m.Create("demo"); err != nil {
		t.Fatalf("create: %v", err)
	}
	cases := map[string]Manifest{
		"empty name": {
			ManifestVersion: 1, ID: "demo", Name: "",
			Version: "0.1.0", Commands: []Command{{ID: "run", Label: "Run"}},
		},
		"empty version": {
			ManifestVersion: 1, ID: "demo", Name: "X",
			Version: "", Commands: []Command{{ID: "run", Label: "Run"}},
		},
		"no commands": {
			ManifestVersion: 1, ID: "demo", Name: "X",
			Version: "0.1.0", Commands: nil,
		},
		"command empty id": {
			ManifestVersion: 1, ID: "demo", Name: "X",
			Version: "0.1.0", Commands: []Command{{ID: "", Label: "Run"}},
		},
		"wrong schema version": {
			ManifestVersion: 99, ID: "demo", Name: "X",
			Version: "0.1.0", Commands: []Command{{ID: "run", Label: "Run"}},
		},
	}
	for name, mf := range cases {
		t.Run(name, func(t *testing.T) {
			err := m.Save("demo", mf, "function run() end")
			if err == nil {
				t.Fatalf("expected error for %s", name)
			}
		})
	}
}

func TestManager_Save_NotFound(t *testing.T) {
	m, _ := newEditorTestManager(t)
	mf := Manifest{
		ManifestVersion: 1, ID: "ghost", Name: "X",
		Version: "0.1.0", Commands: []Command{{ID: "run", Label: "Run"}},
	}
	err := m.Save("ghost", mf, "function run() end")
	if !errors.Is(err, ErrPluginNotFound) {
		t.Fatalf("want ErrPluginNotFound, got %v", err)
	}
}

func TestManager_Save_RejectsBadID(t *testing.T) {
	m, _ := newEditorTestManager(t)
	mf := Manifest{
		ManifestVersion: 1, ID: "..", Name: "X",
		Version: "0.1.0", Commands: []Command{{ID: "run", Label: "Run"}},
	}
	err := m.Save("..", mf, "function run() end")
	if !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("want ErrManifestInvalid, got %v", err)
	}
}

// ── Delete ────────────────────────────────────────────────────────────

func TestManager_Delete_RemovesFolderAndKV(t *testing.T) {
	m, dir := newEditorTestManager(t)
	if err := m.Create("temp"); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Drop a KV entry — its file should also be removed.
	if err := m.deps.KV.Set("temp", "k", "v"); err != nil {
		t.Fatalf("kv: %v", err)
	}
	kvPath := filepath.Join(dir, ".kv", "temp.json")
	if _, err := os.Stat(kvPath); err != nil {
		t.Fatalf("kv file should exist: %v", err)
	}
	if err := m.Delete("temp"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "temp")); err == nil {
		t.Fatal("plugin folder still exists after delete")
	}
	if _, err := os.Stat(kvPath); err == nil {
		t.Fatal("kv file still exists after delete")
	}
	if _, ok := m.Get("temp"); ok {
		t.Fatal("registry still knows about deleted plugin")
	}
}

func TestManager_Delete_RejectsBadID(t *testing.T) {
	m, _ := newEditorTestManager(t)
	for _, id := range []string{"", "..", "a/b", "DEMO"} {
		t.Run(id, func(t *testing.T) {
			if err := m.Delete(id); !errors.Is(err, ErrManifestInvalid) {
				t.Fatalf("Delete(%q): want ErrManifestInvalid, got %v", id, err)
			}
		})
	}
}

func TestManager_Delete_NotFound(t *testing.T) {
	m, _ := newEditorTestManager(t)
	if err := m.Delete("ghost"); !errors.Is(err, ErrPluginNotFound) {
		t.Fatalf("want ErrPluginNotFound, got %v", err)
	}
}

// ── GetSource ─────────────────────────────────────────────────────────

func TestManager_GetSource_Returns(t *testing.T) {
	m, _ := newEditorTestManager(t)
	if err := m.Create("demo"); err != nil {
		t.Fatalf("create: %v", err)
	}
	src, err := m.GetSource("demo")
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if !strings.Contains(src, "function") {
		t.Fatalf("default main.lua should contain a function definition: %q", src)
	}
}

func TestManager_GetSource_NotFound(t *testing.T) {
	m, _ := newEditorTestManager(t)
	_, err := m.GetSource("ghost")
	if !errors.Is(err, ErrPluginNotFound) {
		t.Fatalf("want ErrPluginNotFound, got %v", err)
	}
}

func TestManager_GetSource_RejectsBadID(t *testing.T) {
	m, _ := newEditorTestManager(t)
	for _, id := range []string{"", "..", "a/b"} {
		t.Run(id, func(t *testing.T) {
			if _, err := m.GetSource(id); !errors.Is(err, ErrManifestInvalid) {
				t.Fatalf("GetSource(%q): want ErrManifestInvalid, got %v", id, err)
			}
		})
	}
}

// ── Concurrent CRUD safe under -race ─────────────────────────────────

func TestManager_ConcurrentCRUD_Safe(t *testing.T) {
	m, _ := newEditorTestManager(t)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("p%d", i)
			_ = m.Create(id)
			_, _ = m.GetSource(id)
			_ = m.Delete(id)
		}(i)
	}
	wg.Wait()
	// No assertion on count — racing with itself; the test exists
	// to force -race to flag any unsafe map/file access.
}

// ── Service surface ──────────────────────────────────────────────────

func TestService_Create_AppearsInList(t *testing.T) {
	m, _ := newEditorTestManager(t)
	s := NewService(m)
	got, err := s.Create("hello")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(got) != 1 || got[0].ID != "hello" {
		t.Fatalf("after create: %+v", got)
	}
}

func TestService_Create_PluginExistsErr(t *testing.T) {
	m, _ := newEditorTestManager(t)
	s := NewService(m)
	if _, err := s.Create("dup"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := s.Create("dup")
	if !errors.Is(err, ErrPluginExists) {
		t.Fatalf("want ErrPluginExists, got %v", err)
	}
}

func TestService_Save_Roundtrip(t *testing.T) {
	m, _ := newEditorTestManager(t)
	s := NewService(m)
	if _, err := s.Create("demo"); err != nil {
		t.Fatalf("create: %v", err)
	}
	mf := Manifest{
		ManifestVersion: 1, ID: "demo", Name: "Edited",
		Version: "0.3.0", Commands: []Command{{ID: "run", Label: "Go"}},
	}
	got, err := s.Save("demo", mf, "function run() return 1 end")
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if len(got) != 1 || got[0].Manifest.Version != "0.3.0" {
		t.Fatalf("after save: %+v", got)
	}
}

func TestService_Delete_RemovesFromList(t *testing.T) {
	m, _ := newEditorTestManager(t)
	s := NewService(m)
	if _, err := s.Create("demo"); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := s.Delete("demo")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("after delete: %+v", got)
	}
}

func TestService_GetSource(t *testing.T) {
	m, _ := newEditorTestManager(t)
	s := NewService(m)
	if _, err := s.Create("demo"); err != nil {
		t.Fatalf("create: %v", err)
	}
	src, err := s.GetSource("demo")
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if !strings.Contains(src, "function") {
		t.Fatalf("expected a function in default source: %q", src)
	}
}

// ── DefaultManifest serialization round-trip ─────────────────────────

func TestSerializeManifest_AlwaysWritesBoolFlags(t *testing.T) {
	// All four command boolean flags are written explicitly (no
	// omitempty) so hand-editors see every option at a glance and
	// diffs read "true → false" rather than "field appeared".
	in := Manifest{
		ManifestVersion: 1, ID: "x", Name: "X", Version: "0.1.0",
		Commands: []Command{{ID: "run", Label: "Run"}},
	}
	raw, err := SerializeManifest(in)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	for _, f := range []string{"hide_output", "hide_log", "log_as_toast", "form_button"} {
		if !strings.Contains(string(raw), f) {
			t.Fatalf("expected %q in manifest, got: %s", f, raw)
		}
	}
}

func TestSerializeManifest_RoundTrip(t *testing.T) {
	in := Manifest{
		ManifestVersion: 1, ID: "x", Name: "X", Version: "0.1.0",
		Description: "with \"quotes\" and unicode ✓",
		Commands:    []Command{{ID: "run", Label: "Run"}},
	}
	raw, err := SerializeManifest(in)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	var got Manifest
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Description != in.Description {
		t.Fatalf("description lost: %q", got.Description)
	}
	if !strings.HasSuffix(string(raw), "\n") {
		t.Fatal("serialized manifest must end with newline")
	}
}
