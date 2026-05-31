package gigot

import (
	"errors"

	"github.com/petervdpas/formidable2/internal/event"
	"github.com/petervdpas/formidable2/internal/modules/journal"
	"github.com/petervdpas/formidable2/internal/optrack"
)

// Service is the Wails-bound surface of the gigot backend. It builds each Connection from the active profile +
// keychain subscription bearer and records journal hops on success.
//
// The subscription bearer never crosses the Wails bridge: the Service is the only layer that reads it, keeping Manager transport-neutral.
type Service struct {
	m        *Manager
	creds    CredentialReader
	profile  ProfileReader
	cfg      ConfigReader
	jrnl     journal.Journal
	progress ProgressFunc
	emit     event.Emitter
	ops      *optrack.Registry
}

// CredentialReader resolves the GiGot subscription bearer (NOT a git PAT) for a keychain account; empty + nil means "no entry".
type CredentialReader interface {
	Get(account string) (string, error)
}

// ProfileReader yields the active profile filename; "" disables keychain auto-resolve.
type ProfileReader interface {
	CurrentProfileFilename() string
}

// ConfigReader yields the gigot fields off the active profile; empty Author makes the server use the subscription's default identity.
type ConfigReader interface {
	GigotBaseURL() string
	GigotRepoName() string
	AuthorName() string
	AuthorEmail() string
	ContextFolder() string
}

const journalBackend = journal.BackendGigot

// NewService wires the gigot Service; any of creds/profile/cfg/jrnl may be nil (missing deps surface a config error or skip the journal hop).
func NewService(m *Manager, creds CredentialReader, profile ProfileReader, cfg ConfigReader, jrnl journal.Journal) *Service {
	return &Service{m: m, creds: creds, profile: profile, cfg: cfg, jrnl: jrnl}
}

// AttachProgress installs the emit function for PullLocal/Reclone SyncProgress events.
// Package-level, not a method: the Wails binding generator rejects interface-typed params on bound methods (see git.AttachSysgit).
func AttachProgress(s *Service, emit func(name string, data any)) {
	if s == nil || emit == nil {
		return
	}
	s.progress = func(p SyncProgress) {
		emit(EventSyncProgress, p)
	}
}

// AttachEmitter installs the transport so PullLocal/Reclone announce context:reloaded
// when they change files, keeping the backend the single source of truth.
func AttachEmitter(s *Service, emit event.Emitter) {
	if s == nil {
		return
	}
	s.emit = emit
}

// AttachOps installs the shared op registry so long ops register their state
// (guarding "cannot run twice" and letting the frontend resume on reload).
func AttachOps(s *Service, ops *optrack.Registry) {
	if s == nil {
		return
	}
	s.ops = ops
}

// progressCB records progress into the op handle (for the registry/frontend) and forwards
// to the SyncProgress emit. A nil handle is a safe no-op.
func (s *Service) progressCB(h *optrack.Handle) ProgressFunc {
	return func(p SyncProgress) {
		h.Note(p.Current, p.Total, p.Path)
		if s.progress != nil {
			s.progress(p)
		}
	}
}

// resolveConnection builds a per-call Connection from the active profile + keychain; requireRepo lets Ping/Me proceed without a RepoName.
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

// resolveToken reads the keychain subscription bearer for the active profile + repo; "" lets validateConn surface ErrMissingToken instead of a buried server 401.
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

// resolveContextFolder yields the active profile's context folder; "" lets the Manager surface ErrMissingContext.
func (s *Service) resolveContextFolder() string {
	if s.cfg == nil {
		return ""
	}
	return s.cfg.ContextFolder()
}

func (s *Service) Ping() (*HealthResponse, error) {
	conn, err := s.resolveConnection(false)
	if err != nil {
		return nil, err
	}
	return s.m.Ping(conn)
}

func (s *Service) Me() (*MeResponse, error) {
	conn, err := s.resolveConnection(false)
	if err != nil {
		return nil, err
	}
	return s.m.Me(conn)
}

func (s *Service) Context() (*RepoContextResponse, error) {
	conn, err := s.resolveConnection(true)
	if err != nil {
		return nil, err
	}
	return s.m.Context(conn)
}

func (s *Service) Formidable() (*RepoFormidableResponse, error) {
	conn, err := s.resolveConnection(true)
	if err != nil {
		return nil, err
	}
	return s.m.Formidable(conn)
}

func (s *Service) Head() (*HeadResponse, error) {
	conn, err := s.resolveConnection(true)
	if err != nil {
		return nil, err
	}
	return s.m.Head(conn)
}

func (s *Service) Tree() (*TreeResponse, error) {
	conn, err := s.resolveConnection(true)
	if err != nil {
		return nil, err
	}
	return s.m.Tree(conn)
}

func (s *Service) GetFile(repoRelPath string) (*FileResponse, error) {
	conn, err := s.resolveConnection(true)
	if err != nil {
		return nil, err
	}
	return s.m.GetFile(conn, repoRelPath)
}

func (s *Service) Log(limit int, withChanges bool) (*RepoLogResponse, error) {
	conn, err := s.resolveConnection(true)
	if err != nil {
		return nil, err
	}
	return s.m.Log(conn, limit, withChanges)
}

func (s *Service) Destinations() ([]Destination, error) {
	conn, err := s.resolveConnection(true)
	if err != nil {
		return nil, err
	}
	return s.m.Destinations(conn)
}

func (s *Service) DestinationSync(destinationID string) (*Destination, error) {
	conn, err := s.resolveConnection(true)
	if err != nil {
		return nil, err
	}
	return s.m.DestinationSync(conn, destinationID)
}

// LedgerSummary previews the ledger + pending diff with no HTTP, driving the Sync UI's "what would push/pull do" hints.
func (s *Service) LedgerSummary() (*LedgerSummary, error) {
	ctx := s.resolveContextFolder()
	if ctx == "" {
		return nil, ErrMissingContext
	}
	return s.m.LedgerSummary(ctx)
}

// PushLocal commits changed files in the active context folder, recording a journal sync entry on success.
func (s *Service) PushLocal(message string) (*PushResult, error) {
	conn, err := s.resolveConnection(true)
	if err != nil {
		return nil, err
	}
	res, err := s.m.PushLocal(conn, s.resolveContextFolder(), message)
	if err != nil {
		return res, err
	}
	if s.jrnl != nil && res != nil && (res.Pushed > 0 || res.Deleted > 0) {
		s.jrnl.RecordSync(journalBackend, res.Version, res.Pushed, 0)
	}
	return res, nil
}

// PullLocal fetches the server tree and writes changed files, emitting SyncProgress events and recording remote-seen on success.
func (s *Service) PullLocal() (*PullResult, error) {
	conn, err := s.resolveConnection(true)
	if err != nil {
		return nil, err
	}
	res, err := s.m.PullLocalWithProgress(conn, s.resolveContextFolder(), s.progress)
	if err != nil {
		return res, err
	}
	if s.jrnl != nil && res != nil && res.Version != "" {
		s.jrnl.RecordRemoteSeen(journalBackend, res.Version)
	}
	if res != nil && (res.Files > 0 || res.Deleted > 0) {
		event.Emit(s.emit, "context:reloaded", nil)
	}
	return res, nil
}

// Reclone wipes managed paths in the active context folder and pulls fresh; destructive (local-only edits dropped), records remote-seen on success.
func (s *Service) Reclone() (*PullResult, error) {
	var h *optrack.Handle
	if s.ops != nil {
		// Cannot run twice: reject a second reclone while one is tracked. The
		// defer releases on success, error, or panic, so no restart is needed.
		if h = s.ops.TryBegin("gigot:reclone"); h == nil {
			return nil, errors.New("gigot: a reclone is already running")
		}
		defer h.Done()
	}
	conn, err := s.resolveConnection(true)
	if err != nil {
		return nil, err
	}
	res, err := s.m.RecloneWithProgress(conn, s.resolveContextFolder(), s.progressCB(h))
	if err != nil {
		return res, err
	}
	if s.jrnl != nil && res != nil && res.Version != "" {
		s.jrnl.RecordRemoteSeen(journalBackend, res.Version)
	}
	if res != nil && (res.Files > 0 || res.Deleted > 0) {
		event.Emit(s.emit, "context:reloaded", nil)
	}
	return res, nil
}

// Sync runs PushLocal then PullLocal at the Service layer so each half records its own journal entry; a push failure aborts before pull.
func (s *Service) Sync(message string) (*SyncResult, error) {
	if _, err := s.resolveConnection(true); err != nil {
		return nil, err
	}
	push, err := s.PushLocal(message)
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
