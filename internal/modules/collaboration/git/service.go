package git

import (
	"errors"

	"github.com/petervdpas/formidable2/internal/event"
	"github.com/petervdpas/formidable2/internal/modules/journal"
	"github.com/petervdpas/formidable2/internal/optrack"
)

// ErrNoContext is returned when no active context/root is configured, so the
// git ops can't resolve the working folder they operate on.
var ErrNoContext = errors.New("git: no context configured")

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
	emit    event.Emitter
	ops     *optrack.Registry
	root    RootReader
}

// RootReader resolves the active profile's working folder (the shared context).
// The Service operates on it, so the frontend never passes a path: the backend
// is steered by the active profile, not the other way round. config.Manager
// satisfies this via GetRemoteRootPath.
type RootReader interface {
	GetRemoteRootPath() (string, error)
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

// AttachEmitter installs the transport so data-changing ops announce context:reloaded
// for the frontend to reload off, keeping the backend the single source of truth.
// Package-level for the same binding-generator reason as AttachSysgit.
func AttachEmitter(s *Service, emit event.Emitter) {
	if s == nil {
		return
	}
	s.emit = emit
}

// AttachOps installs the shared op registry so the long git ops register their
// state (guarding "cannot run twice" and letting the frontend resume on reload).
// Package-level for the same binding-generator reason as AttachSysgit.
func AttachOps(s *Service, ops *optrack.Registry) {
	if s == nil {
		return
	}
	s.ops = ops
}

// AttachRoot installs the active-profile root resolver so the Service resolves
// its own working folder; the frontend then calls git ops with no path.
// Package-level for the same binding-generator reason as AttachSysgit.
func AttachRoot(s *Service, root RootReader) {
	if s == nil {
		return
	}
	s.root = root
}

// resolveRoot returns the active profile's working folder. Every bound op goes
// through here instead of taking a path, so the backend is the single source of
// truth for which folder it acts on.
func (s *Service) resolveRoot() (string, error) {
	if s.root == nil {
		return "", ErrNoContext
	}
	return s.root.GetRemoteRootPath()
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

// IsGitRepo reports whether the active context folder is a git repo. A missing
// context (no root) reads as not-a-repo rather than an error.
func (s *Service) IsGitRepo() bool {
	root, err := s.resolveRoot()
	if err != nil {
		return false
	}
	return s.m.IsGitRepo(root)
}

func (s *Service) RepoRoot() (string, error) {
	root, err := s.resolveRoot()
	if err != nil {
		return "", err
	}
	return s.m.RepoRoot(root)
}

func (s *Service) Status() (*Status, error) {
	root, err := s.resolveRoot()
	if err != nil {
		return nil, err
	}
	return s.m.Status(root)
}

func (s *Service) Branches() (*Branches, error) {
	root, err := s.resolveRoot()
	if err != nil {
		return nil, err
	}
	return s.m.Branches(root)
}

func (s *Service) Log(limit int) ([]Commit, error) {
	root, err := s.resolveRoot()
	if err != nil {
		return nil, err
	}
	return s.m.Log(root, limit)
}

func (s *Service) LogGraph(limit int) ([]GraphCommit, error) {
	root, err := s.resolveRoot()
	if err != nil {
		return nil, err
	}
	return s.m.LogGraph(root, limit)
}

func (s *Service) CommitChanges(hash string) ([]ChangeFile, error) {
	root, err := s.resolveRoot()
	if err != nil {
		return nil, err
	}
	return s.m.CommitChanges(root, hash)
}

func (s *Service) RemoteInfo() (*RemoteInfo, error) {
	root, err := s.resolveRoot()
	if err != nil {
		return nil, err
	}
	return s.m.RemoteInfo(root)
}

// Commit can run long on a large worktree, so it is tracked and guarded against
// a concurrent second commit; the guard releases when it ends.
func (s *Service) Commit(opts CommitOptions) (*CommitResult, error) {
	_, release, err := optrack.Guard(s.ops, "git:commit")
	if err != nil {
		return nil, err
	}
	defer release()
	if opts.Path == "" {
		if opts.Path, err = s.resolveRoot(); err != nil {
			return nil, err
		}
	}
	return s.m.Commit(opts)
}

// Clone brings a whole new working tree in, so it announces context:reloaded on
// success; tracked and guarded against a concurrent second clone.
func (s *Service) Clone(opts CloneOptions) (*CloneResult, error) {
	_, release, err := optrack.Guard(s.ops, "git:clone")
	if err != nil {
		return nil, err
	}
	defer release()
	res, err := s.m.Clone(opts)
	if err == nil {
		event.Emit(s.emit, "context:reloaded", nil)
	}
	return res, err
}

// Discard reverts the working tree, so it announces context:reloaded on success.
func (s *Service) Discard(opts DiscardOptions) error {
	if opts.Path == "" {
		root, err := s.resolveRoot()
		if err != nil {
			return err
		}
		opts.Path = root
	}
	err := s.m.Discard(opts)
	if err == nil {
		event.Emit(s.emit, "context:reloaded", nil)
	}
	return err
}

// Fetch refreshes remote-tracking refs via sysgit (credential helper resolves auth) or go-git with keychain PAT.
func (s *Service) Fetch(opts FetchOptions) (*FetchResult, error) {
	if opts.Path == "" {
		root, err := s.resolveRoot()
		if err != nil {
			return nil, err
		}
		opts.Path = root
	}
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

// FetchStatus refreshes the remote-tracking refs then returns a fresh Status,
// so Behind reflects the real remote position. The status panel's Behind is
// only as fresh as the last fetch; a user who never pulls reads behind=0 even
// when the remote moved. The commit-time guard calls this to decide whether to
// offer "pull first" before a commit diverges the branch.
//
// Fetch is read-only against the worktree, so a dirty (about-to-commit) tree is
// safe here. A fetch failure (offline, bad auth) propagates: the caller treats
// any error as "cannot determine, do not block the commit".
func (s *Service) FetchStatus(opts FetchOptions) (*Status, error) {
	root := opts.Path
	if root == "" {
		var err error
		if root, err = s.resolveRoot(); err != nil {
			return nil, err
		}
	}
	if _, err := s.Fetch(opts); err != nil {
		return nil, err
	}
	return s.m.Status(root)
}

// Push sends commits to the remote; AlreadyUpToDate records remote-seen, advancing records a sync marker.
// Tracked and guarded against a concurrent second push (covers both transports).
func (s *Service) Push(opts PushOptions) (*PushResult, error) {
	_, release, err := optrack.Guard(s.ops, "git:push")
	if err != nil {
		return nil, err
	}
	defer release()
	if opts.Path == "" {
		if opts.Path, err = s.resolveRoot(); err != nil {
			return nil, err
		}
	}
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
// Tracked and guarded against a concurrent second pull (covers both transports).
func (s *Service) Pull(opts PullOptions) (*PullResult, error) {
	_, release, err := optrack.Guard(s.ops, "git:pull")
	if err != nil {
		return nil, err
	}
	defer release()
	if opts.Path == "" {
		if opts.Path, err = s.resolveRoot(); err != nil {
			return nil, err
		}
	}
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
	if !res.AlreadyUpToDate {
		event.Emit(s.emit, "context:reloaded", nil)
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
	if !upToDate {
		event.Emit(s.emit, "context:reloaded", nil)
	}
	return &PullResult{AlreadyUpToDate: upToDate, NewHead: newHead}, nil
}

// sysgitStashPull is the stash-flow pull step for self-cloned mode: shell out
// to system git (credential helper handles auth) and re-read HEAD for the
// cursor. Unlike pullViaSysgit it records no journal/emit side effects;
// PullWithStash applies those once around the whole flow.
func (s *Service) sysgitStashPull(opts PullOptions) (*PullResult, error) {
	remote := opts.Remote
	if remote == "" {
		remote = "origin"
	}
	upToDate, err := s.sys.Pull(opts.Path, remote)
	if err != nil {
		return nil, err
	}
	return &PullResult{AlreadyUpToDate: upToDate, NewHead: s.headHash(opts.Path)}, nil
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
//
// The pull step honors self-cloned mode like Pull does: sysgit shells out so the credential helper authenticates,
// go-git uses the keychain PAT. Without this parity a sysgit user (who stores no PAT) hits "authentication required".
func (s *Service) PullWithStash(opts PullOptions) (*StashedPullResult, error) {
	_, release, err := optrack.Guard(s.ops, "git:pull")
	if err != nil {
		return nil, err
	}
	defer release()
	if opts.Path == "" {
		if opts.Path, err = s.resolveRoot(); err != nil {
			return nil, err
		}
	}

	pull := s.m.Pull
	if s.useSysgit() {
		pull = s.sysgitStashPull
	} else if opts.PAT == "" {
		opts.PAT = s.resolvePAT(opts.Path)
	}

	pending := []StashPathPending{}
	if s.jrnl != nil {
		pr := s.jrnl.Pending(journalBackend)
		for _, p := range pr.Paths {
			pending = append(pending, StashPathPending{Path: p.Path, Op: p.Op})
		}
	}
	res, err := s.m.pullWithStash(PullWithStashOptions{
		PullOptions: opts,
		Pending:     pending,
	}, pull)
	if err != nil {
		return res, err
	}
	if res != nil && res.Pull != nil && s.jrnl != nil && res.Pull.NewHead != "" {
		s.jrnl.RecordRemoteSeen(journalBackend, res.Pull.NewHead)
	}
	if res != nil && res.Pull != nil && !res.Pull.AlreadyUpToDate {
		event.Emit(s.emit, "context:reloaded", nil)
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
