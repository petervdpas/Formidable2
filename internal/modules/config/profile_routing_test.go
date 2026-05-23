package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/system"
)

// TestUpdate_WritesActiveProfileNotUserJSON guarantees config writes land
// in the profile .boot.json points at - NOT user.json - after a switch.
// This is the "why is user.json changing while I'm on tools.json" guard.
func TestUpdate_WritesActiveProfileNotUserJSON(t *testing.T) {
	m, _, root := newTestManager(t)

	// Capture user.json's bytes right after first-run seeding.
	userPath := filepath.Join(root, "config", "user.json")
	before, err := os.ReadFile(userPath)
	if err != nil {
		t.Fatalf("read seeded user.json: %v", err)
	}

	// Switch active profile to tools.json, then mutate config.
	if _, err := m.SwitchUserProfile("tools.json"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	if _, err := m.UpdateUserConfig(map[string]any{"context_ribbon": "profiles"}); err != nil {
		t.Fatalf("update: %v", err)
	}

	// tools.json must carry the change.
	toolsRaw, err := os.ReadFile(filepath.Join(root, "config", "tools.json"))
	if err != nil {
		t.Fatalf("read tools.json: %v", err)
	}
	if !strings.Contains(string(toolsRaw), `"context_ribbon": "profiles"`) {
		t.Errorf("tools.json should carry the update, got:\n%s", toolsRaw)
	}

	// user.json must be byte-for-byte unchanged.
	after, err := os.ReadFile(userPath)
	if err != nil {
		t.Fatalf("re-read user.json: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("user.json was modified while tools.json was active!\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestStartup_WritesBootProfileNotUserJSON guarantees that on boot, with
// .boot.json already pointing at tools.json, a fresh manager routes
// writes to tools.json - never user.json - without an explicit switch.
func TestStartup_WritesBootProfileNotUserJSON(t *testing.T) {
	root := t.TempDir()
	cfgDir := filepath.Join(root, "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Pre-seed: boot points at tools.json; a stale user.json also exists.
	if err := os.WriteFile(filepath.Join(cfgDir, ".boot.json"), []byte(`{"active_profile":"tools.json"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "tools.json"), []byte(`{"theme":"dark","context_ribbon":"storage"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	userSeed := []byte(`{"theme":"light","context_ribbon":"storage"}`)
	userPath := filepath.Join(cfgDir, "user.json")
	if err := os.WriteFile(userPath, userSeed, 0o644); err != nil {
		t.Fatal(err)
	}

	sys := system.NewManager(root, nil)
	m, err := NewManager(sys, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if got := m.CurrentProfileFilename(); got != "tools.json" {
		t.Fatalf("active profile = %q, want tools.json", got)
	}
	if _, err := m.UpdateUserConfig(map[string]any{"context_ribbon": "profiles"}); err != nil {
		t.Fatalf("update: %v", err)
	}

	after, _ := os.ReadFile(userPath)
	if string(after) != string(userSeed) {
		t.Errorf("user.json changed on a tools.json-active boot!\ngot:\n%s", after)
	}
}
