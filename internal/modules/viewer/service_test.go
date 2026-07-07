package viewer

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
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

func newService(t *testing.T) *Service {
	t.Helper()
	store := NewConfigStore(filepath.Join(t.TempDir(), "cfg", "config.json"), fileSaver(t))
	return NewService(store, NewServer(), NewHTTPServer(NewServer()))
}

func TestServiceOpenPathSwapsRecordsAndHooks(t *testing.T) {
	s := newService(t)
	swapped := 0
	s.SetSwapHook(func() { swapped++ })

	path := writeZipFile(t, "deck.zip")
	info, err := s.OpenPath(path)
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	if !info.Loaded || info.Name != "deck.zip" {
		t.Fatalf("bundle info = %+v, want loaded deck.zip", info)
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
	path := writeZipFile(t, "picked.zip")
	s.SetOpenFunc(func() (string, error) { return path, nil })

	info, err := s.OpenDialog()
	if err != nil {
		t.Fatalf("OpenDialog: %v", err)
	}
	if !info.Loaded || info.Name != "picked.zip" {
		t.Fatalf("dialog open = %+v", info)
	}
}

func TestServiceOpenDialogCancelKeepsCurrent(t *testing.T) {
	s := newService(t)
	s.SetOpenFunc(func() (string, error) { return "", nil })
	info, err := s.OpenDialog()
	if err != nil {
		t.Fatalf("OpenDialog cancel: %v", err)
	}
	if info.Loaded {
		t.Fatalf("cancelled dialog loaded a bundle: %+v", info)
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
	path := writeZipFile(t, "present.zip")
	if _, err := s.OpenPath(path); err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	// Record a bogus path directly so it survives as "missing".
	if err := s.store.AddRecent("/no/such/bundle.zip"); err != nil {
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
	if byPath["/no/such/bundle.zip"] {
		t.Errorf("bogus bundle flagged as existing")
	}
}

func TestServiceOpenBytes(t *testing.T) {
	s := newService(t)
	swapped := 0
	s.SetSwapHook(func() { swapped++ })

	zb := makeZip(t, map[string]string{"index.html": "<h1>DROPPED</h1>"})
	b64 := base64.StdEncoding.EncodeToString(zb)

	info, err := s.OpenBytes("dropped.zip", b64)
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	if !info.Loaded || info.Name != "dropped.zip" {
		t.Fatalf("info = %+v, want loaded dropped.zip", info)
	}
	if swapped != 1 {
		t.Fatalf("swap hook fired %d times, want 1", swapped)
	}
}

func TestServiceOpenBytesBadBase64(t *testing.T) {
	s := newService(t)
	if _, err := s.OpenBytes("x.zip", "!!!not base64!!!"); err == nil {
		t.Fatal("OpenBytes with invalid base64 = nil error, want failure")
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
