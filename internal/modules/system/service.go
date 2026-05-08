package system

// Service is the api layer over Manager. Methods map 1:1 with the old
// Electron `window.api.system.*` IPC group (camelCase'd → PascalCase).
//
// Wails-only by design: this surface includes raw filesystem and command
// execution and must NOT be exposed via the internal HTTP server. There
// is no handlers.go in this module on purpose.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

func (s *Service) GetAppRoot() string                                   { return s.m.AppRoot() }
func (s *Service) ResolvePath(segments []string) string                 { return s.m.ResolvePath(segments...) }
func (s *Service) ResolveAbsolutePath(p string) (string, error)         { return s.m.ResolveAbsolutePath(p) }
func (s *Service) MakeAppRootRelative(p string) string                  { return s.m.MakeAppRootRelative(p) }
func (s *Service) EnsureDirectory(path string) error                    { return s.m.EnsureDirectory(path) }
func (s *Service) FileExists(path string) bool                          { return s.m.FileExists(path) }
func (s *Service) LoadFile(path string) (string, error)                 { return s.m.LoadFile(path) }
func (s *Service) SaveFile(path string, content string) error           { return s.m.SaveFile(path, content) }
func (s *Service) DeleteFile(path string) error                         { return s.m.DeleteFile(path) }
func (s *Service) EmptyFolder(path string) error                        { return s.m.EmptyFolder(path) }
func (s *Service) CopyFile(from, to string, overwrite bool) error       { return s.m.CopyFile(from, to, overwrite) }
func (s *Service) CopyFolder(from, to string, overwrite bool) error     { return s.m.CopyFolder(from, to, overwrite) }
func (s *Service) ExecuteCommand(cmdline string) (string, error)        { return s.m.ExecuteCommand(cmdline) }
func (s *Service) OpenExternal(target string) error                     { return s.m.OpenExternal(target) }

func (s *Service) ProxyFetchRemote(url string, opts FetchOptions) (*FetchResult, error) {
	return s.m.ProxyFetchRemote(url, opts)
}
