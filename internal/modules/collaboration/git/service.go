package git

// Service is the Wails-bound surface of the Git collaboration
// backend. Mirrors Manager; the split exists so future cross-cutting
// concerns (auth, telemetry, request scoping) live in one obvious
// place without bleeding into the domain layer.
//
// Phase 1 is intentionally read-only — the Collaboration → Current
// Service overview can render meaningful state against any local
// repo without us having to make decisions about credential storage
// or push-time conflict resolution yet.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

func (s *Service) IsGitRepo(path string) bool                  { return s.m.IsGitRepo(path) }
func (s *Service) RepoRoot(path string) (string, error)        { return s.m.RepoRoot(path) }
func (s *Service) Status(path string) (*Status, error)         { return s.m.Status(path) }
func (s *Service) Branches(path string) (*Branches, error)     { return s.m.Branches(path) }
func (s *Service) Log(path string, limit int) ([]Commit, error) { return s.m.Log(path, limit) }
func (s *Service) RemoteInfo(path string) (*RemoteInfo, error) { return s.m.RemoteInfo(path) }
func (s *Service) Clone(opts CloneOptions) (*CloneResult, error)    { return s.m.Clone(opts) }
func (s *Service) Commit(opts CommitOptions) (*CommitResult, error) { return s.m.Commit(opts) }
func (s *Service) Discard(opts DiscardOptions) error                { return s.m.Discard(opts) }
