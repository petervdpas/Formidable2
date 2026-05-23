package config

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// profileFilenameRE is the canonical validation rule for profile
// filenames, mirroring the frontend's FILENAME_RE in
// frontend/src/composables/useProfiles.ts. Keep both in sync.
var profileFilenameRE = regexp.MustCompile(`^[a-z0-9-]+\.json$`)

// IsValidProfileFilename reports whether name is a syntactically valid
// profile filename: lowercase ASCII letters / digits / hyphens, ending
// in ".json". Source of truth for create + import paths.
func IsValidProfileFilename(name string) bool {
	return profileFilenameRE.MatchString(name)
}

// profiles.go owns multi-profile management. Mirrors `Formidable/controls/
// configManager.js` switchUserProfile + listAvailableProfiles +
// getCurrentProfileFilename + exportUserProfile + importUserProfile +
// deleteUserProfile + normalizeProfileFilename.

// CurrentProfileFilename returns the basename of the active profile
// JSON (e.g. "user.json"). Empty string when not yet initialized.
func (m *Manager) CurrentProfileFilename() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.configPath == "" {
		return ""
	}
	return filepath.Base(m.configPath)
}

// GitSelfCloned reports the active profile's "cloned outside
// Formidable" flag. False when no config is loaded yet, so callers
// don't need to special-case the early-boot window.
func (m *Manager) GitSelfCloned() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cached == nil {
		return false
	}
	return m.cached.GitSelfCloned
}

// IoCollectionOnly reports whether CSV Import/Export should be limited
// to templates with enable_collection: true. False (default) means the
// Storage workspace's Data menu is available for every template; true
// restores the old Formidable rule. Same uncached-safe contract as
// GitSelfCloned.
func (m *Manager) IoCollectionOnly() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cached == nil {
		return false
	}
	return m.cached.IoCollectionOnly
}

// GigotBaseURL returns the active profile's GiGot server origin
// ("https://gigot.example") or "" when unset / no profile cached.
func (m *Manager) GigotBaseURL() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cached == nil {
		return ""
	}
	return m.cached.GigotBaseURL
}

// GigotRepoName returns the active profile's GiGot repo handle or ""
// when unset / no profile cached.
func (m *Manager) GigotRepoName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cached == nil {
		return ""
	}
	return m.cached.GigotRepoName
}

// AuthorName returns the active profile's git/gigot author name, used
// to stamp commits on the server-side audit trail. "" when unset.
func (m *Manager) AuthorName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cached == nil {
		return ""
	}
	return m.cached.AuthorName
}

// AuthorEmail mirrors AuthorName for the email half of the author
// identity. "" when unset.
func (m *Manager) AuthorEmail() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cached == nil {
		return ""
	}
	return m.cached.AuthorEmail
}

// ContextFolder returns the active profile's context-folder path
// (where Formidable's templates/, storage/, and .formidable/ live).
// "" when unset / no profile cached.
func (m *Manager) ContextFolder() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cached == nil {
		return ""
	}
	return m.cached.ContextFolder
}

// SwitchUserProfile points .boot.json at profileFilename, swaps the
// active config path, and reloads. Held under updateMu so a concurrent
// UpdateUserConfig can't read the old profile and persist its merge
// into the new file.
//
// This is also the create entry point (switching to a missing filename
// seeds defaults into it), so the name must pass IsValidProfileFilename:
// lowercase [a-z0-9-]+\.json. Reserved dot-prefixed names like
// .boot.json or .pdf-state.json fail this check.
func (m *Manager) SwitchUserProfile(profileFilename string) (*Config, error) {
	if strings.HasPrefix(profileFilename, ".") {
		return nil, fmt.Errorf("profile filename cannot start with '.': %q", profileFilename)
	}
	if !IsValidProfileFilename(profileFilename) {
		return nil, fmt.Errorf("invalid profile filename %q: must match [a-z0-9-]+\\.json", profileFilename)
	}

	m.updateMu.Lock()
	boot := BootConfig{ActiveProfile: profileFilename}
	if err := m.writeJSON(m.bootRelPath(), boot); err != nil {
		m.updateMu.Unlock()
		return nil, err
	}
	m.setConfigPath(profileFilename)
	if err := m.ensureUserConfigFile(); err != nil {
		m.updateMu.Unlock()
		return nil, err
	}
	m.updateMu.Unlock()

	// A freshly-created profile starts scoped to all templates (see
	// SeedEnabledTemplatesIfUnset); an already-configured one is a no-op.
	// Done after releasing updateMu because the seed persists through
	// UpdateUserConfig, which takes the same lock.
	if err := m.SeedEnabledTemplatesIfUnset(); err != nil {
		return nil, err
	}
	return m.LoadUserConfig()
}

// HasUserProfiles reports whether at least one user profile exists
// under config/ (.boot.json excluded). Used by the ribbon to ghost
// workspaces that require a profile to be meaningful (Settings).
// Errors collapse to false - an unreadable config dir is treated
// as "no profiles available".
func (m *Manager) HasUserProfiles() bool {
	profiles, err := m.ListAvailableProfiles()
	if err != nil {
		return false
	}
	return len(profiles) > 0
}

// ListAvailableProfiles enumerates *.json under config/ except
// dot-prefixed files (.boot.json, .pdf-state.json, etc.), returning
// {value, display} entries for the picker. Dot-files are reserved
// for module-private state - only plain user.json-style profiles
// belong in the picker.
// Display falls back from profile_name → author_name → "(unnamed)".
func (m *Manager) ListAvailableProfiles() ([]ProfileEntry, error) {
	files, err := m.fs.ListFiles(configDirName)
	if err != nil {
		return nil, err
	}
	out := make([]ProfileEntry, 0, len(files))
	for _, f := range files {
		if !strings.HasSuffix(strings.ToLower(f), ".json") {
			continue
		}
		if strings.HasPrefix(f, ".") {
			continue
		}
		display := profileDisplayName(m, f)
		out = append(out, ProfileEntry{Value: f, Display: display})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	return out, nil
}

func profileDisplayName(m *Manager, filename string) string {
	raw, err := m.fs.LoadFile(filepath.Join(configDirName, filename))
	if err != nil {
		return "(unknown)"
	}
	cfg, _, err := parseUserConfig(raw)
	if err != nil {
		return "(unknown)"
	}
	if name := strings.TrimSpace(cfg.ProfileName); name != "" {
		return name
	}
	if name := strings.TrimSpace(cfg.AuthorName); name != "" {
		return name
	}
	return "(unnamed)"
}

// ExportUserProfile copies a profile JSON from config/<filename> to an
// arbitrary target path. Returns ProfileResult with .Code populated for
// the structured error cases (matches the JS contract one-to-one so
// frontend modal handlers don't need branching).
func (m *Manager) ExportUserProfile(profileFilename, targetPath string, overwrite bool) ProfileResult {
	if profileFilename == "" || targetPath == "" {
		return ProfileResult{Success: false, Error: "Missing profileFilename or targetPath."}
	}
	source := m.fs.ResolvePath(configDirName, profileFilename)
	if !m.fs.FileExists(source) {
		return ProfileResult{
			Success: false,
			Error:   "Profile file not found: " + profileFilename,
			Code:    "not_found",
		}
	}
	if err := m.fs.EnsureDirectory(filepath.Dir(targetPath)); err != nil {
		return ProfileResult{Success: false, Error: err.Error(), Code: "ensure_target_dir_failed"}
	}
	if err := m.fs.CopyFile(source, targetPath, overwrite); err != nil {
		return ProfileResult{Success: false, Error: err.Error(), Code: "copy_failed"}
	}
	return ProfileResult{
		Success:         true,
		ProfileFilename: profileFilename,
		SourcePath:      source,
		TargetPath:      targetPath,
	}
}

// ImportUserProfile copies a JSON file into config/, normalising the
// destination filename and re-saving it through parseUserConfig so any
// missing fields are filled in (mirrors the JS sanitize-on-import).
//
// If profileFilename is empty, the basename of sourcePath is normalised
// (lowercased, slugified, .json suffix). .boot.json is rejected so the
// boot pointer can't be overwritten via the profile UI.
func (m *Manager) ImportUserProfile(sourcePath, profileFilename string, overwrite bool) ProfileResult {
	if sourcePath == "" {
		return ProfileResult{Success: false, Error: "Missing sourcePath."}
	}
	if !m.fs.FileExists(sourcePath) {
		return ProfileResult{
			Success: false,
			Error:   "Source file not found: " + sourcePath,
			Code:    "not_found",
		}
	}

	final := profileFilename
	if final == "" {
		final = normalizeProfileFilename(filepath.Base(sourcePath))
	}
	if final == "" {
		return ProfileResult{
			Success: false,
			Error:   "Unable to derive a valid profile filename.",
			Code:    "invalid_name",
		}
	}
	if strings.HasPrefix(final, ".") {
		return ProfileResult{
			Success: false,
			Error:   "Dot-prefixed filenames are reserved (e.g. " + final + ").",
			Code:    "boot_forbidden",
		}
	}
	if !IsValidProfileFilename(final) {
		return ProfileResult{
			Success: false,
			Error:   "Invalid profile filename " + final + " (must match [a-z0-9-]+.json).",
			Code:    "invalid_name",
		}
	}

	target := m.fs.ResolvePath(configDirName, final)
	if m.fs.FileExists(target) && !overwrite {
		return ProfileResult{
			Success:    false,
			Error:      "Profile already exists: " + final,
			Code:       "exists",
			Filename:   final,
			TargetPath: target,
		}
	}
	if err := m.fs.CopyFile(sourcePath, target, overwrite); err != nil {
		return ProfileResult{Success: false, Error: err.Error(), Code: "copy_failed"}
	}

	// Sanitize-on-import: read back, parse with defaults, rewrite. If
	// the source was malformed, undo the copy and report invalid_config.
	raw, err := m.fs.LoadFile(target)
	if err != nil {
		_ = m.fs.DeleteFile(target)
		return ProfileResult{Success: false, Error: err.Error(), Code: "invalid_config", Filename: final, TargetPath: target}
	}
	cfg, _, err := parseUserConfig(raw)
	if err != nil {
		_ = m.fs.DeleteFile(target)
		return ProfileResult{Success: false, Error: err.Error(), Code: "invalid_config", Filename: final, TargetPath: target}
	}
	if err := m.writeJSON(target, cfg); err != nil {
		return ProfileResult{Success: false, Error: err.Error(), Code: "write_failed", Filename: final, TargetPath: target}
	}

	m.InvalidateConfigCache()
	return ProfileResult{
		Success:    true,
		Filename:   final,
		TargetPath: target,
	}
}

// DeleteUserProfile removes a profile JSON. .boot.json is rejected
// always, and the active profile is rejected to keep the manager in a
// loadable state - switch first, then delete.
func (m *Manager) DeleteUserProfile(profileFilename string) ProfileResult {
	if profileFilename == "" {
		return ProfileResult{Success: false, Error: "Missing profileFilename.", Code: "missing_filename"}
	}
	if profileFilename == bootFileName {
		return ProfileResult{Success: false, Error: ".boot.json cannot be deleted.", Code: "boot_forbidden"}
	}
	if profileFilename == m.CurrentProfileFilename() {
		return ProfileResult{Success: false, Error: "The active profile cannot be deleted.", Code: "active_profile"}
	}
	target := m.fs.ResolvePath(configDirName, profileFilename)
	if !m.fs.FileExists(target) {
		return ProfileResult{
			Success: false,
			Error:   "Profile file not found: " + profileFilename,
			Code:    "not_found",
		}
	}
	if err := m.fs.DeleteFile(target); err != nil {
		return ProfileResult{Success: false, Error: err.Error(), Code: "delete_failed"}
	}
	return ProfileResult{Success: true, Filename: profileFilename}
}

// normalizeProfileFilename slugifies an arbitrary string into a valid
// profile filename: lowercase, only [a-z0-9-], hyphens collapsed and
// trimmed, .json suffix appended. Returns "" if the result would be
// empty (caller must treat that as an error).
//
// Cases covered by TestNormalizeProfileFilename - keep it in sync.
func normalizeProfileFilename(name string) string {
	if name == "" {
		return ""
	}
	base := filepath.Base(name)
	lower := strings.ToLower(base)
	stem := strings.TrimSuffix(lower, ".json")

	var b strings.Builder
	b.Grow(len(stem))
	for _, r := range stem {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	cleaned := b.String()
	for strings.Contains(cleaned, "--") {
		cleaned = strings.ReplaceAll(cleaned, "--", "-")
	}
	cleaned = strings.Trim(cleaned, "-")
	if cleaned == "" {
		return ""
	}
	return cleaned + ".json"
}
