package gigot

import (
	"github.com/petervdpas/formidable2/internal/modules/journal"
)

// Service is the Wails-bound surface of the gigot backend. Wraps
// Manager and adds two cross-cutting concerns:
//   - when a call needs a Connection, the Service builds it from the
//     active profile (BaseURL + RepoName + Author) and the OS keychain
//     (the subscription bearer);
//   - on success it notifies the journal so Pending(gigot) and the
//     cursor stay accurate.
//
// The keychain is not exposed over the Wails bridge — see
// credential.Service. The Service is the only layer allowed to read
// the subscription bearer; Manager stays transport-neutral.
type Service struct {
	m       *Manager
	creds   CredentialReader
	profile ProfileReader
	cfg     ConfigReader
	jrnl    journal.Journal
}

// CredentialReader resolves the GiGot subscription bearer for a
// keychain account. Empty string + nil error means "no entry"; the
// Service treats that as a misconfigured profile rather than an
// anonymous attempt (gigot has no anonymous mode).
type CredentialReader interface {
	Get(account string) (string, error)
}

// ProfileReader yields the active profile filename. Used to compose
// the canonical credential account `<profile>:gigot:<repoName>`.
// Returning "" disables keychain auto-resolve.
type ProfileReader interface {
	CurrentProfileFilename() string
}

// ConfigReader yields the gigot-related fields off the active profile.
// Implemented by config.Manager. Author may return empty strings — the
// Service drops the Author block on outbound CommitRequests in that
// case so the server falls back to the subscription's default identity.
type ConfigReader interface {
	GigotBaseURL() string
	GigotRepoName() string
	AuthorName() string
	AuthorEmail() string
	ContextFolder() string
}

// journalBackend is the Service's identity in the journal cursor map.
const journalBackend = journal.BackendGigot

// NewService wires the gigot Service to its dependencies. Any of
// creds/profile/cfg/jrnl may be nil — calls that depend on the missing
// dep will surface a configuration error from validateConn or skip
// the journal hop. Mirrors NewService in the git package.
func NewService(m *Manager, creds CredentialReader, profile ProfileReader, cfg ConfigReader, jrnl journal.Journal) *Service {
	return &Service{m: m, creds: creds, profile: profile, cfg: cfg, jrnl: jrnl}
}

// resolveConnection builds a per-call Connection from the active
// profile + keychain. requireRepo lets non-scoped calls (Ping/Me)
// proceed without a RepoName configured. Returns the partially-built
// Connection alongside any validation error so the caller can decide
// whether to attempt the request anyway (e.g. Ping tolerates a
// missing repo).
func (s *Service) resolveConnection(requireRepo bool) (Connection, error) {
	conn := Connection{}
	if s.cfg != nil {
		conn.BaseURL = s.cfg.GigotBaseURL()
		conn.RepoName = s.cfg.GigotRepoName()
		if name, email := s.cfg.AuthorName(), s.cfg.AuthorEmail(); name != "" || email != "" {
			conn.Author = &Author{Name: name, Email: email}
		}
	}
	conn.Token = s.resolveToken(conn.RepoName)
	return conn, validateConn(conn, requireRepo)
}

// resolveToken looks up the stored subscription bearer for the active
// profile + repo. Empty + nil-error treated as "no token configured" —
// the caller's validateConn will surface ErrMissingToken so the user
// sees a clear "set your subscription token" rather than a server
// 401 buried in HTTP plumbing.
func (s *Service) resolveToken(repoName string) string {
	if s.creds == nil || s.profile == nil {
		return ""
	}
	profile := s.profile.CurrentProfileFilename()
	if profile == "" || repoName == "" {
		return ""
	}
	secret, err := s.creds.Get(profile + ":gigot:" + repoName)
	if err != nil {
		return ""
	}
	return secret
}

// resolveContextFolder yields the active profile's context folder for
// orchestration ops (PushLocal / PullLocal / Sync). Returning "" lets
// the underlying Manager method surface ErrMissingContext consistently.
func (s *Service) resolveContextFolder() string {
	if s.cfg == nil {
		return ""
	}
	return s.cfg.ContextFolder()
}

// Ping issues GET /api/health against the configured server. Tolerates
// a missing RepoName since /health is repo-agnostic.
func (s *Service) Ping() (*HealthResponse, error) {
	conn, err := s.resolveConnection(false)
	if err != nil {
		return nil, err
	}
	return s.m.Ping(conn)
}

// Me issues GET /api/me — bearer-aware self-introspection. Also
// repo-agnostic.
func (s *Service) Me() (*MeResponse, error) {
	conn, err := s.resolveConnection(false)
	if err != nil {
		return nil, err
	}
	return s.m.Me(conn)
}

// Context issues GET /api/repos/{repo}/context — the per-repo bootstrap.
func (s *Service) Context() (*RepoContextResponse, error) {
	conn, err := s.resolveConnection(true)
	if err != nil {
		return nil, err
	}
	return s.m.Context(conn)
}

// Formidable issues GET /api/repos/{repo}/formidable — the Formidable-
// shape bootstrap (marker + templates + storage summary).
func (s *Service) Formidable() (*RepoFormidableResponse, error) {
	conn, err := s.resolveConnection(true)
	if err != nil {
		return nil, err
	}
	return s.m.Formidable(conn)
}

// Head issues GET /api/repos/{repo}/head — current HEAD version.
func (s *Service) Head() (*HeadResponse, error) {
	conn, err := s.resolveConnection(true)
	if err != nil {
		return nil, err
	}
	return s.m.Head(conn)
}

// Tree issues GET /api/repos/{repo}/tree — recursive file listing.
func (s *Service) Tree() (*TreeResponse, error) {
	conn, err := s.resolveConnection(true)
	if err != nil {
		return nil, err
	}
	return s.m.Tree(conn)
}

// GetFile issues GET /api/repos/{repo}/files/{path}.
func (s *Service) GetFile(repoRelPath string) (*FileResponse, error) {
	conn, err := s.resolveConnection(true)
	if err != nil {
		return nil, err
	}
	return s.m.GetFile(conn, repoRelPath)
}

// Log issues GET /api/repos/{repo}/log[?limit=N&with_changes=1].
// limit<=0 collapses to the server's default page size. withChanges=true
// asks the server to attach each commit's per-path file changes — the
// audit-trail view; leave it false for cheap graph rendering.
func (s *Service) Log(limit int, withChanges bool) (*RepoLogResponse, error) {
	conn, err := s.resolveConnection(true)
	if err != nil {
		return nil, err
	}
	return s.m.Log(conn, limit, withChanges)
}

// Destinations issues GET /api/repos/{repo}/destinations.
func (s *Service) Destinations() ([]Destination, error) {
	conn, err := s.resolveConnection(true)
	if err != nil {
		return nil, err
	}
	return s.m.Destinations(conn)
}

// DestinationSync issues POST /api/repos/{repo}/destinations/{id}/sync.
func (s *Service) DestinationSync(destinationID string) (*Destination, error) {
	conn, err := s.resolveConnection(true)
	if err != nil {
		return nil, err
	}
	return s.m.DestinationSync(conn, destinationID)
}

// PushLocal walks the active context folder, diffs against the track-
// record, and commits changed files to the server. On success records
// a journal sync entry so Pending(gigot) reflects the post-push state.
func (s *Service) PushLocal() (*PushResult, error) {
	conn, err := s.resolveConnection(true)
	if err != nil {
		return nil, err
	}
	res, err := s.m.PushLocal(conn, s.resolveContextFolder())
	if err != nil {
		return res, err
	}
	if s.jrnl != nil && res != nil && (res.Pushed > 0 || res.Deleted > 0) {
		s.jrnl.RecordSync(journalBackend, res.Version, res.Pushed, 0)
	}
	return res, nil
}

// PullLocal fetches the server's tree and writes changed files to disk.
// On success records a remote-seen entry so the head-probe poller can
// short-circuit next tick.
func (s *Service) PullLocal() (*PullResult, error) {
	conn, err := s.resolveConnection(true)
	if err != nil {
		return nil, err
	}
	res, err := s.m.PullLocal(conn, s.resolveContextFolder())
	if err != nil {
		return res, err
	}
	if s.jrnl != nil && res != nil && res.Version != "" {
		s.jrnl.RecordRemoteSeen(journalBackend, res.Version)
	}
	return res, nil
}

// Sync runs PushLocal then PullLocal at the Service layer so each
// half emits its own journal entry via the wrapper methods. A push
// failure aborts before pull to preserve unpushed local changes —
// symmetric with the git Service and with Manager.Sync.
func (s *Service) Sync() (*SyncResult, error) {
	if _, err := s.resolveConnection(true); err != nil {
		return nil, err
	}
	push, err := s.PushLocal()
	if err != nil {
		return nil, err
	}
	pull, err := s.PullLocal()
	if err != nil {
		return nil, err
	}
	version := pull.Version
	if version == "" {
		version = push.Version
	}
	return &SyncResult{
		Version:       version,
		Pushed:        push.Pushed,
		PushedDeleted: push.Deleted,
		Pulled:        pull.Files,
		PulledDeleted: pull.Deleted,
		Noop:          push.Noop && pull.Files == 0 && pull.Deleted == 0,
	}, nil
}
