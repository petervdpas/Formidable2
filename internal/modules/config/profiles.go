package config

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// profileFilenameRE validates profile filenames; mirrors the frontend's
// FILENAME_RE in useProfiles.ts, keep both in sync.
var profileFilenameRE = regexp.MustCompile(`^[a-z0-9-]+\.json$`)

// IsValidProfileFilename reports whether name is [a-z0-9-]+.json. Source of
// truth for the create + import paths.
func IsValidProfileFilename(name string) bool {
	return profileFilenameRE.MatchString(name)
}

// CurrentProfileFilename returns the active profile basename, or "" when
// not yet initialized.
func (m *Manager) CurrentProfileFilename() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.configPath == "" {
		return ""
	}
	return filepath.Base(m.configPath)
}

// GitSelfCloned reports the active profile's "cloned outside Formidable"
// flag; false when no config is loaded, so callers skip an early-boot case.
func (m *Manager) GitSelfCloned() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cached == nil {
		return false
	}
	return m.cached.Git.SelfCloned
}

// IoCollectionOnly reports whether CSV Import/Export is limited to
// collection-enabled templates; false (default) allows every template.
func (m *Manager) IoCollectionOnly() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cached == nil {
		return false
	}
	return m.cached.IoCollectionOnly
}

// GigotBaseURL returns the active profile's GiGot server origin, or "".
func (m *Manager) GigotBaseURL() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cached == nil {
		return ""
	}
	return m.cached.Gigot.BaseURL
}

// GigotRepoName returns the active profile's GiGot repo handle, or "".
func (m *Manager) GigotRepoName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cached == nil {
		return ""
	}
	return m.cached.Gigot.RepoName
}

// AuthorName returns the active profile's git/gigot author name, or "".
func (m *Manager) AuthorName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cached == nil {
		return ""
	}
	return m.cached.AuthorName
}

// AuthorEmail returns the email half of the author identity, or "".
func (m *Manager) AuthorEmail() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cached == nil {
		return ""
	}
	return m.cached.AuthorEmail
}

// ContextFolder returns the active profile's context-folder path (root of
// templates/, storage/, .formidable/), or "".
func (m *Manager) ContextFolder() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cached == nil {
		return ""
	}
	return m.cached.ContextFolder
}

// SwitchUserProfile points .boot.json at profileFilename, swaps the active
// config path, and reloads, held under updateMu against a concurrent merge
// into the wrong file. Also the create path: a missing filename is seeded
// with defaults, so the name must pass IsValidProfileFilename (dot-prefixed
// names are rejected).
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

	// After releasing updateMu: the seed persists via UpdateUserConfig,
	// which takes the same lock.
	if err := m.SeedEnabledTemplatesIfUnset(); err != nil {
		return nil, err
	}
	return m.LoadUserConfig()
}

// HasUserProfiles reports whether at least one user profile exists under
// config/ (dot-files excluded). Errors collapse to false.
func (m *Manager) HasUserProfiles() bool {
	profiles, err := m.ListAvailableProfiles()
	if err != nil {
		return false
	}
	return len(profiles) > 0
}

// ListAvailableProfiles returns {value, display} picker entries for the
// non-dot *.json under config/ (dot-files are module-private state). Display
// falls back profile_name, author_name, "(unnamed)".
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

// ExportUserProfile copies config/<filename> to targetPath, returning a
// ProfileResult with .Code set on the structured error cases.
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
// destination filename and re-saving through parseUserConfig to fill missing
// fields. An empty profileFilename slugifies the source basename; dot-prefixed
// names are rejected so the boot pointer can't be overwritten.
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

	// Read back, parse with defaults, rewrite; a malformed source undoes
	// the copy and reports invalid_config.
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

// DeleteUserProfile removes a profile JSON. .boot.json and the active
// profile are rejected (switch first) so the manager stays loadable.
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

// normalizeProfileFilename slugifies a string into a valid profile filename
// (lowercase [a-z0-9-], collapsed/trimmed hyphens, .json suffix), or ""
// when the result would be empty.
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
