package config

// Service is the Wails-bound surface of the config module — what the
// Vue SPA can actually call. Deliberately narrow:
//
//   - Profile config read/write (the Settings UI)
//   - Profile management (the Profiles workspace)
//
// The richer Manager surface (VirtualStructure scan, per-template
// storage info, cache invalidation hooks) is intentionally NOT exposed
// here. Those primitives are for backend-internal consumers — most
// notably the future opt-in internal HTTP server (wiki view + REST
// collections API + OpenAPI), which will use Manager directly.
//
// The frontend covers what little it needs of the file tree through
// the focused per-module services (template.Service.ListTemplates,
// storage.Service.ListForms, etc.). Adding a VFS hop on top would
// duplicate scanning and create two ways to ask the same question.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// ─── Reads ───────────────────────────────────────────────────────────

func (s *Service) LoadUserConfig() (*Config, error) { return s.m.LoadUserConfig() }

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

// ListEnabledTemplates returns the YAML filenames the active profile is
// allowed to pick from. Empty EnabledTemplates → every template on disk
// (the "opt-in not used" default). The backend self-heals against the
// live templates folder before returning, so deleted templates don't
// leak through into the picker.
func (s *Service) ListEnabledTemplates() ([]string, error) {
	return s.m.ListEnabledTemplates()
}

// IsTemplateEnabled is the single-filename gate the frontend can use
// when it already knows the name (e.g. validating that a stored
// SelectedTemplate is still allowed). Empty filename is never enabled.
func (s *Service) IsTemplateEnabled(filename string) bool {
	return s.m.IsTemplateEnabled(filename)
}
