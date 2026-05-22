package git

import (
	"github.com/petervdpas/formidable2/internal/modules/journal"
)

// Service is the Wails-bound surface of the Git collaboration
// backend. Wraps Manager and adds two cross-cutting concerns:
//   - when the frontend calls Push/Pull/Fetch without a PAT, the
//     Service auto-resolves it from the OS keychain via the injected
//     CredentialReader;
//   - on success it informs the journal (via journal.Recorder) so
//     Pending(git) and the cursor stay accurate.
//
// The keychain is intentionally NOT exposed to the frontend (see
// the credential.Service comment) so secrets never round-trip
// through the Wails bridge. That makes the Service the only layer
// allowed to read the PAT - Manager itself stays transport-neutral
// and unaware of credential storage.
type Service struct {
	m       *Manager
	creds   CredentialReader
	profile ProfileReader
	jrnl    journal.Journal
	flags   FlagReader
	sys     Sysgit
}

// FlagReader exposes per-profile toggles that affect transport
// selection. Today: GitSelfCloned. Implemented by config.Manager.
type FlagReader interface {
	GitSelfCloned() bool
}

// Sysgit is the system-git transport surface the Service shells out
// to in self-cloned mode. *sysgit.Runner satisfies it; tests inject
// fakes. Available() is checked before every dispatch so a missing
// binary degrades to the go-git fallback path.
type Sysgit interface {
	Available() bool
	Fetch(workdir, remote string) error
	Push(workdir, remote string) (alreadyUpToDate bool, err error)
	Pull(workdir, remote string) (alreadyUpToDate bool, err error)
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
// `<profile>:git:<remote_url>` - same format the frontend
// useCredentialAccount composable produces. Returning "" disables
// keychain auto-resolve (no profile yet → no scoped secret to look up).
type ProfileReader interface {
	CurrentProfileFilename() string
}

// journalBackend is the Service's identity in the journal cursor map.
// Same string as journal.BackendGit.
const journalBackend = journal.BackendGit

func NewService(m *Manager, creds CredentialReader, profile ProfileReader, jrnl journal.Journal) *Service {
	return &Service{m: m, creds: creds, profile: profile, jrnl: jrnl}
}

// AttachSysgit enables the "cloned outside Formidable" transport
// path: when flags.GitSelfCloned() is true, Fetch/Push/Pull shell out
// to the system git binary so the user's credential helper handles
// auth. Both args may be nil - that just keeps the go-git fallback
// in force.
//
// This is a package-level function, NOT a method on Service, because
// the Wails binding generator walks every exported method of a bound
// service and rejects interface-typed parameters (they're not
// JSON-serializable across the bridge). A *Service return type would
// also double Service as both service AND model, producing duplicate
// "Service" exports in the generated index.ts.
func AttachSysgit(s *Service, flags FlagReader, runner Sysgit) {
	if s == nil {
		return
	}
	s.flags = flags
	s.sys = runner
}

// useSysgit decides whether a network op should shell out. Toggle on
// AND binary available is the only path that returns true.
func (s *Service) useSysgit() bool {
	if s.flags == nil || s.sys == nil {
		return false
	}
	if !s.flags.GitSelfCloned() {
		return false
	}
	return s.sys.Available()
}

func (s *Service) IsGitRepo(path string) bool                       { return s.m.IsGitRepo(path) }
func (s *Service) RepoRoot(path string) (string, error)             { return s.m.RepoRoot(path) }
func (s *Service) Status(path string) (*Status, error)              { return s.m.Status(path) }
func (s *Service) Branches(path string) (*Branches, error)          { return s.m.Branches(path) }
func (s *Service) Log(path string, limit int) ([]Commit, error)     { return s.m.Log(path, limit) }
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

// Fetch refreshes remote-tracking refs. Two transport paths:
//
//   - Self-cloned mode (toggle on + system git on PATH): shell out
//     via sysgit so the user's credential helper resolves auth - no
//     PAT round-trip through Formidable's keychain.
//   - Default: go-git with PAT auto-filled from the keychain entry
//     for the repo's "origin" URL.
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

// Push sends commits to the named remote. Two transport paths
// (same shape as Fetch). Journal recording is identical regardless
// of path: AlreadyUpToDate → remote-seen; advancing → sync marker.
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

// pushViaSysgit shells out to system git, then re-reads HEAD through
// go-git so the journal cursor advances to the post-push commit. The
// HEAD read is best-effort - a HEAD failure after a successful push
// means we skip the journal update but still return the push success
// to the caller.
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

// Pull fetches + merges the upstream branch. Same dual-transport
// shape; same journal-recording semantics regardless of path.
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

// headHash resolves the worktree HEAD for journal recording. Returns
// "" on any failure - caller treats empty as "skip the journal hop"
// since recording an unknown version would corrupt the cursor.
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

// PullWithStash is the journal-aware auto-stash variant of Pull. The
// Service reads the journal's pending set for the git backend and
// passes it to Manager.PullWithStash; the Manager snapshots, resets,
// pulls, and restores. Same RecordRemoteSeen behavior as Pull on
// success - the underlying inner Pull's NewHead is what we record.
//
// The pending list includes only paths the journal knows are dirty -
// strictly narrower than `git status`, so external edits in unrelated
// files don't get stashed. When the journal has no pending changes,
// the call degrades to a plain pull.
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
