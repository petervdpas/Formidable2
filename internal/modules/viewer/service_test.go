package viewer

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/bundle"
)

func writeZipFile(t *testing.T, name string) string {
	t.Helper()
	zb := makeZip(t, map[string]string{"index.html": "<h1>HOME</h1>"})
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, zb, 0o644); err != nil {
		t.Fatalf("write zip: %v", err)
	}
	return path
}

// writeBundleFile packs a minimal wiki zip into a .bundle at a temp path,
// sealed with password (empty = unencrypted). Title is set so the manifest
// surfacing can be asserted.
func writeBundleFile(t *testing.T, name, password string) string {
	t.Helper()
	zb := makeZip(t, map[string]string{"index.html": "<h1>PACKED</h1>"})
	packed, err := bundle.Pack(bundle.Manifest{Title: "Packed Pack", Description: "a test pack"}, zb, password)
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, packed, 0o644); err != nil {
		t.Fatalf("write bundle: %v", err)
	}
	return path
}

func newService(t *testing.T) *Service {
	t.Helper()
	store := NewConfigStore(filepath.Join(t.TempDir(), "cfg", "config.json"), fileSaver(t))
	return NewService(store, NewServer(), NewHTTPServer(NewServer()))
}

func TestServiceOpenPathSwapsRecordsAndHooks(t *testing.T) {
	s := newService(t)
	swapped := 0
	s.SetSwapHook(func() { swapped++ })

	path := writeBundleFile(t, "deck.bundle", "")
	res, err := s.OpenPath(path, "")
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	if !res.Info.Loaded || res.Info.Name != "deck.bundle" {
		t.Fatalf("bundle info = %+v, want loaded deck.bundle", res.Info)
	}
	if swapped != 1 {
		t.Fatalf("swap hook fired %d times, want 1", swapped)
	}
	if rec := s.Recents(); len(rec) != 1 || rec[0].Path != path || !rec[0].Exists {
		t.Fatalf("recents = %+v, want the opened path present", rec)
	}
}

func TestServiceOpenDialogUsesInjectedPicker(t *testing.T) {
	s := newService(t)
	path := writeBundleFile(t, "picked.bundle", "")
	s.SetOpenFunc(func() (string, error) { return path, nil })

	res, err := s.OpenDialog()
	if err != nil {
		t.Fatalf("OpenDialog: %v", err)
	}
	if !res.Info.Loaded || res.Info.Name != "picked.bundle" {
		t.Fatalf("dialog open = %+v", res.Info)
	}
}

func TestServiceOpenDialogCancelKeepsCurrent(t *testing.T) {
	s := newService(t)
	s.SetOpenFunc(func() (string, error) { return "", nil })
	res, err := s.OpenDialog()
	if err != nil {
		t.Fatalf("OpenDialog cancel: %v", err)
	}
	if res.Info.Loaded {
		t.Fatalf("cancelled dialog loaded a bundle: %+v", res.Info)
	}
}

func TestServiceSetConfigTogglesLANServer(t *testing.T) {
	s := newService(t)

	cfg := s.GetConfig()
	cfg.ServeHTTP = true
	cfg.HTTPPort = 0 // ephemeral for the test
	applied, err := s.SetConfig(cfg)
	if err != nil {
		t.Fatalf("SetConfig on: %v", err)
	}
	if !applied.ServeHTTP {
		t.Fatal("ServeHTTP not persisted")
	}
	if st := s.ServerStatus(); !st.Running || len(st.URLs) == 0 {
		t.Fatalf("server not running after enable: %+v", st)
	}

	cfg = s.GetConfig()
	cfg.ServeHTTP = false
	if _, err := s.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig off: %v", err)
	}
	if st := s.ServerStatus(); st.Running {
		t.Fatalf("server still running after disable: %+v", st)
	}
}

func TestServiceRecentsFlagsMissing(t *testing.T) {
	s := newService(t)
	path := writeBundleFile(t, "present.bundle", "")
	if _, err := s.OpenPath(path, ""); err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	// Record a bogus path directly so it survives as "missing".
	if err := s.store.AddRecent("/no/such/bundle.bundle"); err != nil {
		t.Fatalf("AddRecent: %v", err)
	}
	rec := s.Recents()
	byPath := map[string]bool{}
	for _, r := range rec {
		byPath[r.Path] = r.Exists
	}
	if !byPath[path] {
		t.Errorf("present bundle flagged missing")
	}
	if byPath["/no/such/bundle.bundle"] {
		t.Errorf("bogus bundle flagged as existing")
	}
}

func TestServiceOpenBareZipRejected(t *testing.T) {
	s := newService(t)
	path := writeZipFile(t, "legacy.zip")
	// The Viewer opens .bundle files only; a bare zip has no container.
	if _, err := s.OpenPath(path, ""); err == nil {
		t.Fatal("a bare zip must be rejected, not served")
	}
}

func TestServiceOpenBytes(t *testing.T) {
	s := newService(t)
	swapped := 0
	s.SetSwapHook(func() { swapped++ })

	zb := makeZip(t, map[string]string{"index.html": "<h1>DROPPED</h1>"})
	packed, err := bundle.Pack(bundle.Manifest{Title: "Dropped"}, zb, "")
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	b64 := base64.StdEncoding.EncodeToString(packed)

	res, err := s.OpenBytes("dropped.bundle", b64, "")
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	if !res.Info.Loaded || res.Info.Name != "dropped.bundle" {
		t.Fatalf("info = %+v, want loaded dropped.bundle", res.Info)
	}
	if swapped != 1 {
		t.Fatalf("swap hook fired %d times, want 1", swapped)
	}
}

func TestServiceOpenBytesBadBase64(t *testing.T) {
	s := newService(t)
	if _, err := s.OpenBytes("x.zip", "!!!not base64!!!", ""); err == nil {
		t.Fatal("OpenBytes with invalid base64 = nil error, want failure")
	}
}

func TestServiceOpenEncryptedBundleFlow(t *testing.T) {
	s := newService(t)
	swapped := 0
	s.SetSwapHook(func() { swapped++ })
	path := writeBundleFile(t, "locked.bundle", "s3cret")

	// No password: not loaded, prompt requested, manifest surfaced for the UI.
	res, err := s.OpenPath(path, "")
	if err != nil {
		t.Fatalf("OpenPath no-pw: %v", err)
	}
	if !res.NeedsPassword || res.Info.Loaded {
		t.Fatalf("want NeedsPassword and not loaded, got %+v", res)
	}
	if !res.Info.Encrypted || res.Info.Title != "Packed Pack" || res.Path != path {
		t.Fatalf("manifest not surfaced for unlock prompt: %+v", res)
	}
	if swapped != 0 {
		t.Fatalf("nothing should have swapped yet, got %d", swapped)
	}

	// Wrong password: flagged, still not loaded.
	res, err = s.OpenPath(path, "nope")
	if err != nil {
		t.Fatalf("OpenPath wrong-pw: %v", err)
	}
	if !res.WrongPassword || res.Info.Loaded {
		t.Fatalf("want WrongPassword and not loaded, got %+v", res)
	}

	// Correct password: loaded and serving, title carried through.
	res, err = s.OpenPath(path, "s3cret")
	if err != nil {
		t.Fatalf("OpenPath correct-pw: %v", err)
	}
	if !res.Info.Loaded || res.Info.Title != "Packed Pack" {
		t.Fatalf("want loaded pack with title, got %+v", res.Info)
	}
	if swapped != 1 {
		t.Fatalf("swap hook fired %d times, want 1", swapped)
	}
	if s.Current().Title != "Packed Pack" || !s.Current().Encrypted {
		t.Fatalf("Current lost manifest: %+v", s.Current())
	}
}

func TestServiceOpenPlainBundleLoadsDirectly(t *testing.T) {
	s := newService(t)
	path := writeBundleFile(t, "open.bundle", "") // no password

	res, err := s.OpenPath(path, "")
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	if !res.Info.Loaded || res.NeedsPassword || res.WrongPassword {
		t.Fatalf("plain bundle should load directly, got %+v", res)
	}
	if res.Info.Encrypted || res.Info.Description != "a test pack" {
		t.Fatalf("plain bundle manifest wrong: %+v", res.Info)
	}
}

func TestServiceMessagesFollowConfigLanguage(t *testing.T) {
	s := newService(t)
	cfg := s.GetConfig()
	cfg.Language = "nl"
	if _, err := s.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if s.EffectiveLanguage() != "nl" {
		t.Fatalf("effective language = %q, want nl", s.EffectiveLanguage())
	}
	if msg := s.Messages(""); msg["settings.title"] != "Instellingen" {
		t.Fatalf("nl messages not returned: %q", msg["settings.title"])
	}
}
