package git

// Service is the Wails-bound surface of the Git collaboration
// backend. Wraps Manager and adds one cross-cutting concern: when
// the frontend calls Push or Fetch without a PAT, the Service
// auto-resolves it from the OS keychain via the injected
// CredentialReader.
//
// The keychain is intentionally NOT exposed to the frontend (see
// the credential.Service comment) so secrets never round-trip
// through the Wails bridge. That makes the Service the only layer
// allowed to read the PAT — Manager itself stays transport-neutral
// and unaware of credential storage.
type Service struct {
	m       *Manager
	creds   CredentialReader
	profile ProfileReader
}

// CredentialReader resolves a stored secret for an HTTPS auth account.
// Empty string + nil error means "no entry"; the Service treats that
// as an anonymous attempt and lets the remote's 401 surface as the
// caller-visible error.
type CredentialReader interface {
	Get(account string) (string, error)
}

// ProfileReader yields the active profile filename. The Service uses
// it to compose the canonical credential account
// `<profile>:git:<remote_url>` — same format the frontend
// useCredentialAccount composable produces. Returning "" disables
// keychain auto-resolve (no profile yet → no scoped secret to look up).
type ProfileReader interface {
	CurrentProfileFilename() string
}

func NewService(m *Manager, creds CredentialReader, profile ProfileReader) *Service {
	return &Service{m: m, creds: creds, profile: profile}
}

func (s *Service) IsGitRepo(path string) bool                       { return s.m.IsGitRepo(path) }
func (s *Service) RepoRoot(path string) (string, error)             { return s.m.RepoRoot(path) }
func (s *Service) Status(path string) (*Status, error)              { return s.m.Status(path) }
func (s *Service) Branches(path string) (*Branches, error)          { return s.m.Branches(path) }
func (s *Service) Log(path string, limit int) ([]Commit, error)     { return s.m.Log(path, limit) }
func (s *Service) RemoteInfo(path string) (*RemoteInfo, error)      { return s.m.RemoteInfo(path) }
func (s *Service) Clone(opts CloneOptions) (*CloneResult, error)    { return s.m.Clone(opts) }
func (s *Service) Commit(opts CommitOptions) (*CommitResult, error) { return s.m.Commit(opts) }
func (s *Service) Discard(opts DiscardOptions) error                { return s.m.Discard(opts) }

// Fetch refreshes remote-tracking refs. When opts.PAT is empty, the
// Service auto-fills it from the keychain entry for the repo's
// "origin" URL — frontend doesn't need to (and can't) read the
// secret itself.
func (s *Service) Fetch(opts FetchOptions) (*FetchResult, error) {
	if opts.PAT == "" {
		opts.PAT = s.resolvePAT(opts.Path)
	}
	return s.m.Fetch(opts)
}

// Push sends commits to the named remote. Same keychain auto-fill
// behavior as Fetch.
func (s *Service) Push(opts PushOptions) (*PushResult, error) {
	if opts.PAT == "" {
		opts.PAT = s.resolvePAT(opts.Path)
	}
	return s.m.Push(opts)
}

// resolvePAT looks up the stored token for the repo's "origin"
// remote, scoped by active profile. Any failure (missing creds dep,
// no profile, no origin, keychain miss / error) collapses to "" so
// the Push/Fetch attempt proceeds anonymously.
func (s *Service) resolvePAT(path string) string {
	if s.creds == nil || s.profile == nil {
		return ""
	}
	profile := s.profile.CurrentProfileFilename()
	if profile == "" {
		return ""
	}
	info, err := s.m.RemoteInfo(path)
	if err != nil || info == nil {
		return ""
	}
	var url string
	for _, r := range info.Remotes {
		if r.Name == "origin" && len(r.URLs) > 0 {
			url = r.URLs[0]
			break
		}
	}
	if url == "" {
		return ""
	}
	secret, err := s.creds.Get(profile + ":git:" + url)
	if err != nil {
		return ""
	}
	return secret
}
