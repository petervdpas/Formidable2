package config

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// boot.go owns the boot-pointer file that names the active profile.
// Mirrors `Formidable/controls/configManager.js` resolveBootProfile +
// setUserConfigPath + ensureConfigFile.

// bootRelPath is the boot pointer file, relative to AppRoot.
func (m *Manager) bootRelPath() string {
	return filepath.Join(configDirName, bootFileName)
}

// legacyBootRelPath is the pre-dotfile pointer path, kept only so
// installs that predate the rename get migrated on first read.
func (m *Manager) legacyBootRelPath() string {
	return filepath.Join(configDirName, legacyBootFileName)
}

// resolveBootProfile reads (or seeds and repairs) config/.boot.json and
// returns the active profile filename it points to.
//
// If .boot.json is missing but legacy config/boot.json exists → migrate
// (rewrite as .boot.json, remove the legacy file) so existing installs
// don't quietly reset to a fresh seed.
// If .boot.json is missing and there's no legacy → seed defaults.
// If .boot.json exists but is malformed or missing fields → repair.
// If both exist → .boot.json wins; legacy is removed as drift.
func (m *Manager) resolveBootProfile() (string, error) {
	if err := m.fs.EnsureDirectory(configDirName); err != nil {
		return "", fmt.Errorf("ensure config dir: %w", err)
	}

	bootPath := m.bootRelPath()
	legacyPath := m.legacyBootRelPath()

	if !m.fs.FileExists(bootPath) && m.fs.FileExists(legacyPath) {
		if err := m.migrateLegacyBoot(legacyPath, bootPath); err != nil {
			return "", fmt.Errorf("migrate legacy boot.json: %w", err)
		}
	}

	if !m.fs.FileExists(bootPath) {
		if err := m.writeJSON(bootPath, defaultBootConfig()); err != nil {
			return "", fmt.Errorf("seed .boot.json: %w", err)
		}
		return defaultBootConfig().ActiveProfile, nil
	}

	if m.fs.FileExists(legacyPath) {
		if err := m.fs.DeleteFile(legacyPath); err != nil {
			m.log.Warn("could not remove stale legacy boot.json", "path", legacyPath, "err", err)
		}
	}

	raw, err := m.fs.LoadFile(bootPath)
	if err != nil {
		return "", fmt.Errorf("read .boot.json: %w", err)
	}

	boot, changed := sanitizeBoot(raw)
	if changed {
		if err := m.writeJSON(bootPath, boot); err != nil {
			return "", fmt.Errorf("repair .boot.json: %w", err)
		}
	}
	return boot.ActiveProfile, nil
}

// migrateLegacyBoot reads the pre-dotfile pointer, sanitizes it, writes
// the result atomically to the new dotfile path, and removes the legacy
// file. Order matters: the new file must be on disk before the legacy
// is removed so a crash mid-migration leaves a recoverable state (next
// run sees the legacy still present and retries).
func (m *Manager) migrateLegacyBoot(legacyPath, newPath string) error {
	raw, err := m.fs.LoadFile(legacyPath)
	if err != nil {
		return fmt.Errorf("read legacy: %w", err)
	}
	boot, _ := sanitizeBoot(raw)
	if err := m.writeJSON(newPath, boot); err != nil {
		return fmt.Errorf("write new: %w", err)
	}
	if err := m.fs.DeleteFile(legacyPath); err != nil {
		return fmt.Errorf("remove legacy: %w", err)
	}
	m.log.Info("migrated boot pointer", "from", legacyPath, "to", newPath, "active_profile", boot.ActiveProfile)
	return nil
}

// sanitizeBoot mirrors `schemas/boot.schema.js` — fills missing fields
// with defaults. The bool indicates whether the input was actually
// amended (so callers can persist iff anything changed).
func sanitizeBoot(raw string) (BootConfig, bool) {
	def := defaultBootConfig()
	var probe map[string]any
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return def, true
	}
	got := def
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		return def, true
	}
	if got.ActiveProfile == "" {
		got.ActiveProfile = def.ActiveProfile
		return got, true
	}
	if _, ok := probe["active_profile"]; !ok {
		return got, true
	}
	return got, false
}

// setConfigPath records the absolute path to the active profile JSON
// and drops the cached config so the next access reloads from disk.
func (m *Manager) setConfigPath(profileFilename string) {
	abs := m.fs.ResolvePath(configDirName, profileFilename)
	m.mu.Lock()
	m.configPath = abs
	m.cached = nil
	m.virtualStructure = nil
	m.mu.Unlock()
}

// ensureUserConfigFile makes sure the active profile's JSON exists on
// disk. Called once during initialize() so listing/exporting works
// before the first LoadUserConfig.
func (m *Manager) ensureUserConfigFile() error {
	m.mu.RLock()
	path := m.configPath
	m.mu.RUnlock()
	if path == "" {
		return fmt.Errorf("ensureUserConfigFile: no active profile path")
	}
	if m.fs.FileExists(path) {
		return nil
	}
	def := defaultConfig()
	if err := m.writeJSON(path, def); err != nil {
		return fmt.Errorf("seed user config: %w", err)
	}
	return nil
}
