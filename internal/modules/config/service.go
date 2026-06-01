package config

// Service is the Wails-bound surface of the config module: profile config
// read/write plus profile management. The richer Manager surface (VFS scan,
// per-template storage info, cache hooks) is intentionally NOT exposed here;
// those are backend-internal, for the future opt-in internal HTTP server.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// ─── Reads ───────────────────────────────────────────────────────────

func (s *Service) LoadUserConfig() (*Config, error) { return s.m.LoadUserConfig() }

// GetRemoteRootPath returns the active backend's working folder, resolved the
// one shared way (none/git/gigot all go through ResolvePath against AppRoot).
// The git frontend uses this instead of resolving git_root itself, so all three
// backends resolve their root identically.
func (s *Service) GetRemoteRootPath() (string, error) { return s.m.GetRemoteRootPath() }

// ─── Writes ──────────────────────────────────────────────────────────

func (s *Service) UpdateUserConfig(partial map[string]any) (*Config, error) {
	return s.m.UpdateUserConfig(partial)
}

// ─── Profiles ────────────────────────────────────────────────────────

func (s *Service) SwitchUserProfile(profileFilename string) (*Config, error) {
	return s.m.SwitchUserProfile(profileFilename)
}

func (s *Service) ListUserProfiles() ([]ProfileEntry, error) {
	return s.m.ListAvailableProfiles()
}

func (s *Service) HasUserProfiles() bool { return s.m.HasUserProfiles() }

func (s *Service) CurrentProfileFilename() string { return s.m.CurrentProfileFilename() }

func (s *Service) ExportUserProfile(profileFilename, targetPath string, overwrite bool) ProfileResult {
	return s.m.ExportUserProfile(profileFilename, targetPath, overwrite)
}

func (s *Service) ImportUserProfile(sourcePath, profileFilename string, overwrite bool) ProfileResult {
	return s.m.ImportUserProfile(sourcePath, profileFilename, overwrite)
}

func (s *Service) DeleteUserProfile(profileFilename string) ProfileResult {
	return s.m.DeleteUserProfile(profileFilename)
}

// ─── Enabled templates (per-profile curation) ────────────────────────

// ListEnabledTemplates returns the YAML filenames the active profile may pick
// from. Empty EnabledTemplates means every template on disk. Self-heals against
// the live templates folder so deleted templates don't leak into the picker.
func (s *Service) ListEnabledTemplates() ([]string, error) {
	return s.m.ListEnabledTemplates()
}

// IsTemplateEnabled reports whether filename is in the active profile's scope.
// Empty filename is never enabled.
func (s *Service) IsTemplateEnabled(filename string) bool {
	return s.m.IsTemplateEnabled(filename)
}

// SetTemplateEnabled toggles one template in the active profile's scope and
// returns the resulting EnabledTemplates slice.
func (s *Service) SetTemplateEnabled(filename string, on bool) ([]string, error) {
	return s.m.SetTemplateEnabled(filename, on)
}
