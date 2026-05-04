package config

// Service is the api layer over Manager. Methods map 1:1 with the old
// Electron `window.api.config.*` IPC group (camelCase'd → PascalCase).
//
// Wails-only: no handlers.go in this module. Raw config is too sensitive
// for the loopback HTTP API surface.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// Reads ----------------------------------------------------------------

func (s *Service) LoadUserConfig() (*Config, error) { return s.m.LoadUserConfig() }

func (s *Service) GetVirtualStructure() (*VirtualStructure, error) {
	return s.m.GetVirtualStructure()
}

func (s *Service) GetContextPath() (string, error)       { return s.m.GetContextPath() }
func (s *Service) GetTemplatesFolder() (string, error)   { return s.m.GetContextTemplatesPath() }
func (s *Service) GetStorageFolder() (string, error)     { return s.m.GetContextStoragePath() }

func (s *Service) GetTemplateStorageInfo(templateFilename string) *TemplateStorageFolder {
	return s.m.GetTemplateStorageInfo(templateFilename)
}
func (s *Service) GetTemplateStorageFolder(templateFilename string) string {
	return s.m.GetTemplateStoragePath(templateFilename)
}
func (s *Service) GetTemplateMetaFiles(templateFilename string) []string {
	return s.m.GetTemplateMetaFiles(templateFilename)
}
func (s *Service) GetTemplateImageFiles(templateFilename string) []string {
	return s.m.GetTemplateImageFiles(templateFilename)
}
func (s *Service) GetSingleTemplateEntry(templateName string) *SingleTemplateEntry {
	return s.m.GetSingleTemplateEntry(templateName)
}

// Writes ---------------------------------------------------------------

func (s *Service) UpdateUserConfig(partial map[string]any) (*Config, error) {
	return s.m.UpdateUserConfig(partial)
}

func (s *Service) InvalidateConfigCache()  { s.m.InvalidateConfigCache() }
func (s *Service) DirtyVirtualStructure()  { s.m.DirtyVirtualStructure() }

// Profiles -------------------------------------------------------------

func (s *Service) SwitchUserProfile(profileFilename string) (*Config, error) {
	return s.m.SwitchUserProfile(profileFilename)
}

func (s *Service) ListUserProfiles() ([]ProfileEntry, error) {
	return s.m.ListAvailableProfiles()
}

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
