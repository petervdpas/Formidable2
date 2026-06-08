package config

import (
	"testing"
	"time"
)

func TestGetTemplateStorageInfo_EmptyFilenameNil(t *testing.T) {
	m, _, _ := newTestManagerRootContext(t)
	if info := m.GetTemplateStorageInfo(""); info != nil {
		t.Errorf("empty filename must return nil, got %+v", info)
	}
}

func TestGetTemplateStorageInfo_MissingTemplateNil(t *testing.T) {
	m, _, _ := newTestManagerRootContext(t)
	if info := m.GetTemplateStorageInfo("nope.yaml"); info != nil {
		t.Errorf("unknown template must return nil, got %+v", info)
	}
}

// TestGetTemplateStorageInfo_ExtensionlessMatchesByStem confirms the lookup
// strips templateExt before keying, so "basic" resolves the same record as
// "basic.yaml".
func TestGetTemplateStorageInfo_ExtensionlessMatchesByStem(t *testing.T) {
	m, sys, _ := newTestManagerRootContext(t)
	if err := sys.SaveFile("templates/basic.yaml", "name: Basic"); err != nil {
		t.Fatalf("seed template: %v", err)
	}
	m.DirtyVirtualStructure()
	if _, err := m.GetVirtualStructure(); err != nil {
		t.Fatalf("build vfs: %v", err)
	}

	withExt := m.GetTemplateStorageInfo("basic.yaml")
	noExt := m.GetTemplateStorageInfo("basic")
	if withExt == nil || noExt == nil {
		t.Fatalf("both lookups should resolve: withExt=%v noExt=%v", withExt, noExt)
	}
	if withExt.Name != "basic" || noExt.Name != "basic" {
		t.Errorf("names = %q / %q, want basic", withExt.Name, noExt.Name)
	}
	if withExt.Filename != "basic.yaml" || noExt.Filename != "basic.yaml" {
		t.Errorf("Filename = %q / %q, want basic.yaml", withExt.Filename, noExt.Filename)
	}
	// Stem lookup must resolve the SAME record as the full-filename lookup.
	if withExt.Path != noExt.Path {
		t.Errorf("paths diverge: withExt=%q noExt=%q", withExt.Path, noExt.Path)
	}
}

// TestGetVirtualStructure_IgnoresNonTemplateFiles seeds a stray .txt next to
// a real .yaml; only the .yaml becomes a storage folder.
func TestGetVirtualStructure_IgnoresNonTemplateFiles(t *testing.T) {
	m, sys, _ := newTestManagerRootContext(t)
	if err := sys.SaveFile("templates/real.yaml", "name: Real"); err != nil {
		t.Fatalf("seed yaml: %v", err)
	}
	if err := sys.SaveFile("templates/readme.txt", "ignore me"); err != nil {
		t.Fatalf("seed txt: %v", err)
	}
	m.DirtyVirtualStructure()
	vfs, err := m.GetVirtualStructure()
	if err != nil {
		t.Fatalf("GetVirtualStructure: %v", err)
	}
	if len(vfs.TemplateStorageFolders) != 1 {
		t.Fatalf("expected exactly 1 folder, got %d (%v)", len(vfs.TemplateStorageFolders), vfs.TemplateStorageFolders)
	}
	if _, ok := vfs.TemplateStorageFolders["real"]; !ok {
		t.Error("real.yaml must yield a storage folder")
	}
	if _, ok := vfs.TemplateStorageFolders["readme"]; ok {
		t.Error("readme.txt must not yield a storage folder")
	}
}

// TestGetTemplateStorageInfo_TraversalNameNil confirms an odd lookup key (path
// traversal, slashes, dot-prefix) resolves to nil rather than matching a real
// stem or escaping the keyed map. The VFS keys on bare stems only.
func TestGetTemplateStorageInfo_TraversalNameNil(t *testing.T) {
	m, sys, _ := newTestManagerRootContext(t)
	if err := sys.SaveFile("templates/basic.yaml", "name: Basic"); err != nil {
		t.Fatalf("seed template: %v", err)
	}
	m.DirtyVirtualStructure()
	if _, err := m.GetVirtualStructure(); err != nil {
		t.Fatalf("build vfs: %v", err)
	}
	for _, odd := range []string{"../basic", "../basic.yaml", "storage/basic", "./basic", ".basic"} {
		if info := m.GetTemplateStorageInfo(odd); info != nil {
			t.Errorf("odd key %q must not resolve, got %+v", odd, info)
		}
	}
	// The clean stem still resolves: proves the odd keys failed on the key,
	// not because the VFS was empty.
	if info := m.GetTemplateStorageInfo("basic"); info == nil {
		t.Error("clean stem basic must still resolve")
	}
}

// TestGetVirtualStructure_TTLCacheHitServesStaleView pins the clock so the
// second call lands inside the TTL window: a template added to disk after
// the first build must NOT appear, proving the cache short-circuits the
// rebuild rather than rescanning every call.
func TestGetVirtualStructure_TTLCacheHitServesStaleView(t *testing.T) {
	m, sys, _ := newTestManagerRootContext(t)
	frozen := time.Unix(1_700_000_000, 0)
	m.SetNowFn(func() time.Time { return frozen })
	m.SetTTL(2 * time.Second)

	if err := sys.SaveFile("templates/first.yaml", "name: First"); err != nil {
		t.Fatalf("seed first template: %v", err)
	}
	m.DirtyVirtualStructure()
	first, err := m.GetVirtualStructure()
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	if _, ok := first.TemplateStorageFolders["first"]; !ok {
		t.Fatal("first.yaml must be present in the freshly built view")
	}

	// Add a second template, then read again WITHIN the TTL window (clock
	// unchanged). The cached view must be returned verbatim, so the new
	// template is absent.
	if err := sys.SaveFile("templates/second.yaml", "name: Second"); err != nil {
		t.Fatalf("seed second template: %v", err)
	}
	cached, err := m.GetVirtualStructure()
	if err != nil {
		t.Fatalf("cached read: %v", err)
	}
	if _, ok := cached.TemplateStorageFolders["second"]; ok {
		t.Error("cache-hit must serve the stale view; second.yaml should not appear yet")
	}

	// Advance past the TTL: the rebuild now picks up second.yaml.
	m.SetNowFn(func() time.Time { return frozen.Add(3 * time.Second) })
	fresh, err := m.GetVirtualStructure()
	if err != nil {
		t.Fatalf("post-TTL rebuild: %v", err)
	}
	if _, ok := fresh.TemplateStorageFolders["second"]; !ok {
		t.Error("after the TTL expires the rebuild must include second.yaml")
	}
}

// TestGetContextPath_EmptyContextFallsBackToRoot covers the odd-config case:
// an empty context_folder must not error; it resolves to the workspace root.
func TestGetContextPath_EmptyContextFallsBackToRoot(t *testing.T) {
	m, _, root := newTestManagerRootContext(t)
	if _, err := m.UpdateUserConfig(map[string]any{"context_folder": ""}); err != nil {
		t.Fatalf("set empty context: %v", err)
	}
	got, err := m.GetContextPath()
	if err != nil {
		t.Fatalf("GetContextPath: %v", err)
	}
	if got != root {
		t.Errorf("GetContextPath = %q, want root %q", got, root)
	}
}
