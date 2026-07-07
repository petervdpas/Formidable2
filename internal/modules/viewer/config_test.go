package viewer

import (
	"os"
	"path/filepath"
	"testing"
)

// fileSaver is a plain atomic-ish saver for tests (temp + rename), standing in
// for the injected system.Manager.SaveBytes at runtime.
func fileSaver(t *testing.T) SaverFunc {
	t.Helper()
	return func(path string, data []byte) error {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, data, 0o644); err != nil {
			return err
		}
		return os.Rename(tmp, path)
	}
}

func newStore(t *testing.T) *ConfigStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "formidable-viewer", "config.json")
	return NewConfigStore(path, fileSaver(t))
}

func TestLoadMissingReturnsDefaults(t *testing.T) {
	s := newStore(t)
	got := s.Load()
	def := DefaultConfig()
	if got.Theme != def.Theme || got.DefaultZoom != def.DefaultZoom || got.RememberSize != def.RememberSize {
		t.Fatalf("Load on missing = %+v, want defaults %+v", got, def)
	}
	if got.RecentBundles == nil {
		t.Fatal("RecentBundles should be non-nil empty slice, got nil")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	s := newStore(t)
	in := Config{Theme: "dark", DefaultZoom: 1.25, RememberSize: false, WindowWidth: 900, WindowHeight: 700, RecentBundles: []string{"/a.zip"}}
	if err := s.Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := s.Load()
	if got.Theme != "dark" || got.DefaultZoom != 1.25 || got.RememberSize != false ||
		got.WindowWidth != 900 || got.WindowHeight != 700 || len(got.RecentBundles) != 1 || got.RecentBundles[0] != "/a.zip" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestNormalizeClampsBadValues(t *testing.T) {
	s := newStore(t)
	// Write a deliberately broken config straight to disk.
	if err := fileSaver(t)(s.path, []byte(`{"theme":"neon","default_zoom":99,"recent_bundles":null,"http_port":70000}`)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got := s.Load()
	if got.Theme != "system" {
		t.Errorf("bad theme not reset: %q", got.Theme)
	}
	if got.DefaultZoom != maxZoom {
		t.Errorf("zoom not clamped: %v", got.DefaultZoom)
	}
	if got.RecentBundles == nil {
		t.Error("nil recents not repaired")
	}
	if got.HTTPPort != defaultHTTPPort {
		t.Errorf("out-of-range port not reset: %d", got.HTTPPort)
	}
}

func TestLoadBrokenJSONReturnsDefaults(t *testing.T) {
	s := newStore(t)
	if err := fileSaver(t)(s.path, []byte(`{not json`)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if got := s.Load(); got.Theme != "system" {
		t.Fatalf("broken json did not fall back to defaults: %+v", got)
	}
}

func TestAddRecentDedupeOrderCap(t *testing.T) {
	s := newStore(t)
	for i := range maxRecentBundles + 5 {
		if err := s.AddRecent(filepathJoin(i)); err != nil {
			t.Fatalf("AddRecent: %v", err)
		}
	}
	got := s.Load()
	if len(got.RecentBundles) != maxRecentBundles {
		t.Fatalf("recents not capped: %d", len(got.RecentBundles))
	}
	// Most recent first.
	if got.RecentBundles[0] != filepathJoin(maxRecentBundles+4) {
		t.Fatalf("most-recent not at front: %v", got.RecentBundles[0])
	}

	// Re-adding an existing path moves it to front without growing the list.
	before := len(got.RecentBundles)
	if err := s.AddRecent(got.RecentBundles[3]); err != nil {
		t.Fatalf("AddRecent existing: %v", err)
	}
	reloaded := s.Load()
	if len(reloaded.RecentBundles) != before {
		t.Fatalf("dedupe grew the list: %d -> %d", before, len(reloaded.RecentBundles))
	}
	if reloaded.RecentBundles[0] != got.RecentBundles[3] {
		t.Fatalf("re-added path not moved to front")
	}
}

func filepathJoin(i int) string {
	return filepath.Join("/bundles", string(rune('a'+i))+".zip")
}
