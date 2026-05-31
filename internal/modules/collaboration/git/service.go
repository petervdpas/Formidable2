package git

import (
	"github.com/petervdpas/formidable2/internal/modules/journal"
)

// Service is the Wails-bound surface of the Git backend. It auto-resolves a missing PAT from the keychain
// and records journal sync hops on success.
//
// The PAT never round-trips through the Wails bridge: the Service is the only layer allowed to read it,
// keeping Manager transport-neutral and unaware of credential storage.
type Service struct {
	m       *Manager
	creds   CredentialReader
	profile ProfileReader
	jrnl    journal.Journal
	flags   FlagReader
	sys     Sysgit
}

// FlagReader exposes per-profile toggles that affect transport selection.
type FlagReader interface {
	GitSelfCloned() bool
}

// Sysgit is the system-git transport for self-cloned mode; Available() gates dispatch so a missing binary falls back to go-git.
type Sysgit interface {
	Available() bool
	Fetch(workdir, remote string) error
	Push(workdir, remote string) (alreadyUpToDate bool, err error)
	Pull(workdir, remote string) (alreadyUpToDate bool, err error)
}

// CredentialReader resolves a stored secret for an HTTPS auth account.
// Empty string + nil error means "no entry", treated as an anonymous attempt.
type CredentialReader interface {
	Get(account string) (string, error)
}

// ProfileReader yields the active profile filename. Returning ""
// disables keychain auto-resolve.
type ProfileReader interface {
	CurrentProfileFilename() string
}

const journalBackend = journal.BackendGit

func NewService(m *Manager, creds CredentialReader, profile ProfileReader, jrnl journal.Journal) *Service {
	return &Service{m: m, creds: creds, profile: profile, jrnl: jrnl}
}

// AttachSysgit enables the self-cloned transport: when GitSelfCloned() is true, Fetch/Push/Pull shell out to
// system git so the user's credential helper handles auth. Nil args keep the go-git fallback.
//
// Package-level, not a method: the Wails binding generator rejects interface-typed params on bound methods,
// and a *Service return would double Service as both service and model in the generated index.ts.
func AttachSysgit(s *Service, flags FlagReader, runner Sysgit) {
	if s == nil {
		return
	}
	s.flags = flags
	s.sys = runner
}

func (s *Service) useSysgit() bool {
	if s.flags == nil || s.sys == nil {
		return false
	}
	if !s.flags.GitSelfCloned() {
		return false
	}
	return s.sys.Available()
}

func (s *Service) IsGitRepo(path string) bool                   { return s.m.IsGitRepo(path) }
func (s *Service) RepoRoot(path string) (string, error)         { return s.m.RepoRoot(path) }
func (s *Service) Status(path string) (*Status, error)          { return s.m.Status(path) }
func (s *Service) Branches(path string) (*Branches, error)      { return s.m.Branches(path) }
func (s *Service) Log(path string, limit int) ([]Commit, error) { return s.m.Log(path, limit) }
func (s *Service) LogGraph(path string, limit int) ([]GraphCommit, error) {
	return s.m.LogGraph(path, limit)
}
func (s *Service) CommitChanges(path, hash string) ([]ChangeFile, error) {
	return s.m.CommitChanges(path, hash)
}
func (s *Service) RemoteInfo(path string) (*RemoteInfo, error)      { return s.m.RemoteInfo(path) }
func (s *Service) Clone(opts CloneOptions) (*CloneResult, error)    { return s.m.Clone(opts) }
func (s *Service) Commit(opts CommitOptions) (*CommitResult, error) { return s.m.Commit(opts) }
func (s *Service) Discard(opts DiscardOptions) error                { return s.m.Discard(opts) }

// Fetch refreshes remote-tracking refs via sysgit (credential helper resolves auth) or go-git with keychain PAT.
func (s *Service) Fetch(opts FetchOptions) (*FetchResult, error) {
	if s.useSysgit() {
		if err := s.sys.Fetch(opts.Path, opts.Remote); err != nil {
			return nil, err
		}
		return &FetchResult{AlreadyUpToDate: false}, nil
	}
	if opts.PAT == "" {
		opts.PAT = s.resolvePAT(opts.Path)
	}
	return s.m.Fetch(opts)
}

// Push sends commits to the remote; AlreadyUpToDate records remote-seen, advancing records a sync marker.
func (s *Service) Push(opts PushOptions) (*PushResult, error) {
	if s.useSysgit() {
		return s.pushViaSysgit(opts)
	}
	if opts.PAT == "" {
		opts.PAT = s.resolvePAT(opts.Path)
	}
	res, err := s.m.Push(opts)
	if err != nil || res == nil {
		return res, err
	}
	if s.jrnl != nil && res.NewHead != "" {
		if res.AlreadyUpToDate {
			s.jrnl.RecordRemoteSeen(journalBackend, res.NewHead)
		} else {
			s.jrnl.RecordSync(journalBackend, res.NewHead, 1, 0)
		}
	}
	return res, nil
}

// pushViaSysgit shells out to system git, then best-effort re-reads HEAD via go-git for the journal cursor.
func (s *Service) pushViaSysgit(opts PushOptions) (*PushResult, error) {
	upToDate, err := s.sys.Push(opts.Path, opts.Remote)
	if err != nil {
		return nil, err
	}
	newHead := s.headHash(opts.Path)
	if s.jrnl != nil && newHead != "" {
		if upToDate {
			s.jrnl.RecordRemoteSeen(journalBackend, newHead)
		} else {
			s.jrnl.RecordSync(journalBackend, newHead, 1, 0)
		}
	}
	return &PushResult{AlreadyUpToDate: upToDate, NewHead: newHead}, nil
}

// Pull fetches + merges the upstream branch via sysgit or go-git, recording remote-seen on success.
func (s *Service) Pull(opts PullOptions) (*PullResult, error) {
	if s.useSysgit() {
		return s.pullViaSysgit(opts)
	}
	if opts.PAT == "" {
		opts.PAT = s.resolvePAT(opts.Path)
	}
	res, err := s.m.Pull(opts)
	if err != nil || res == nil {
		return res, err
	}
	if s.jrnl != nil && res.NewHead != "" {
		s.jrnl.RecordRemoteSeen(journalBackend, res.NewHead)
	}
	return res, nil
}

func (s *Service) pullViaSysgit(opts PullOptions) (*PullResult, error) {
	upToDate, err := s.sys.Pull(opts.Path, opts.Remote)
	if err != nil {
		return nil, err
	}
	newHead := s.headHash(opts.Path)
	if s.jrnl != nil && newHead != "" {
		s.jrnl.RecordRemoteSeen(journalBackend, newHead)
	}
	return &PullResult{AlreadyUpToDate: upToDate, NewHead: newHead}, nil
}

// headHash resolves the worktree HEAD for journal recording; "" means skip the hop (an unknown version would corrupt the cursor).
func (s *Service) headHash(path string) string {
	r, err := s.m.open(path)
	if err != nil {
		return ""
	}
	h, err := r.Head()
	if err != nil {
		return ""
	}
	return h.Hash().String()
}

// PullWithStash is the journal-aware auto-stash variant of Pull. The pending set is narrower than `git status`
// (only journal-dirty paths), so external edits don't get stashed; no pending degrades to a plain pull.
func (s *Service) PullWithStash(opts PullOptions) (*StashedPullResult, error) {
	if opts.PAT == "" {
		opts.PAT = s.resolvePAT(opts.Path)
	}
	pending := []StashPathPending{}
	if s.jrnl != nil {
		pr := s.jrnl.Pending(journalBackend)
		for _, p := range pr.Paths {
			pending = append(pending, StashPathPending{Path: p.Path, Op: p.Op})
		}
	}
	res, err := s.m.PullWithStash(PullWithStashOptions{
		PullOptions: opts,
		Pending:     pending,
	})
	if err != nil {
		return res, err
	}
	if res != nil && res.Pull != nil && s.jrnl != nil && res.Pull.NewHead != "" {
		s.jrnl.RecordRemoteSeen(journalBackend, res.Pull.NewHead)
	}
	return res, nil
}

// resolvePAT reads the keychain token for the origin remote (profile-scoped); any failure collapses to "" (anonymous attempt).
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
