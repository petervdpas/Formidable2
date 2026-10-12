package config

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/system"
)

// hookFS runs a one-shot callback right after a LoadFile has read its bytes,
// which is the window where LoadUserConfig holds no lock: it has the old
// content in hand but has not cached it yet.
type hookFS struct {
	*system.Manager
	mu        sync.Mutex
	afterLoad func()
}

func (h *hookFS) arm(fn func()) {
	h.mu.Lock()
	h.afterLoad = fn
	h.mu.Unlock()
}

func (h *hookFS) LoadFile(path string) (string, error) {
	out, err := h.Manager.LoadFile(path)
	h.mu.Lock()
	fn := h.afterLoad
	h.afterLoad = nil
	h.mu.Unlock()
	if fn != nil {
		fn()
	}
	return out, err
}

func newHookedManager(t *testing.T) (*Manager, *hookFS) {
	t.Helper()
	fs := &hookFS{Manager: system.NewManager(t.TempDir(), nil)}
	m, err := NewManager(fs, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m, fs
}

func TestLoadUserConfig_ProfileSwitchDuringLoadDoesNotCacheTheOldProfile(t *testing.T) {
	m, fs := newHookedManager(t)
	seed := defaultConfig()
	seed.ProfileName = "P-b"
	buf, _ := json.Marshal(seed)
	if err := fs.SaveFile(filepath.Join("config", "b.json"), string(buf)); err != nil {
		t.Fatal(err)
	}
	m.InvalidateConfigCache()

	fs.arm(func() {
		if _, err := m.SwitchUserProfile("b.json"); err != nil {
			t.Errorf("switch: %v", err)
		}
	})
	got, err := m.LoadUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	if m.CurrentProfileFilename() != "b.json" {
		t.Fatalf("active profile = %q", m.CurrentProfileFilename())
	}
	if got.ProfileName != "P-b" {
		t.Errorf("load returned %q, want the profile that is active now", got.ProfileName)
	}
	if again, _ := m.LoadUserConfig(); again.ProfileName != "P-b" {
		t.Errorf("cache holds %q under b.json, want P-b", again.ProfileName)
	}
}

func TestLoadUserConfig_UpdateDuringLoadIsNotOverwrittenByTheStaleRead(t *testing.T) {
	m, fs := newHookedManager(t)
	m.InvalidateConfigCache()

	fs.arm(func() {
		if _, err := m.UpdateUserConfig(map[string]any{"author_name": "fresh"}); err != nil {
			t.Errorf("update: %v", err)
		}
	})
	got, err := m.LoadUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.AuthorName != "fresh" {
		t.Errorf("load returned author %q, want the value written meanwhile", got.AuthorName)
	}
	if again, _ := m.LoadUserConfig(); again.AuthorName != "fresh" {
		t.Errorf("cache holds author %q, want fresh (disk has fresh)", again.AuthorName)
	}
}
