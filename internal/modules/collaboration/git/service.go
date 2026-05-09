package git

import "github.com/petervdpas/formidable2/internal/modules/journal"

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
// allowed to read the PAT — Manager itself stays transport-neutral
// and unaware of credential storage.
type Service struct {
	m       *Manager
	creds   CredentialReader
	profile ProfileReader
	jrnl    journal.Journal
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

// journalBackend is the Service's identity in the journal cursor map.
// Same string as journal.BackendGit.
const journalBackend = journal.BackendGit

func NewService(m *Manager, creds CredentialReader, profile ProfileReader, jrnl journal.Journal) *Service {
	return &Service{m: m, creds: creds, profile: profile, jrnl: jrnl}
}

func (s *Service) IsGitRepo(path string) bool                       { return s.m.IsGitRepo(path) }
func (s *Service) RepoRoot(path string) (string, error)             { return s.m.RepoRoot(path) }
func (s *Service) Status(path string) (*Status, error)              { return s.m.Status(path) }
func (s *Service) Branches(path string) (*Branches, error)          { return s.m.Branches(path) }
func (s *Service) Log(path string, limit int) ([]Commit, error)     { return s.m.Log(path, limit) }
func (s *Service) LogGraph(path string, limit int) ([]GraphCommit, error) {
	return s.m.LogGraph(path, limit)
}
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
// behavior as Fetch. On success, informs the journal: an advancing
// push records a sync marker (pending clears for git); an
// already-up-to-date push records a remote-seen update only (we now
// know the remote head, but no outbound sync happened).
func (s *Service) Push(opts PushOptions) (*PushResult, error) {
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

// Pull fetches + merges the upstream branch. Same keychain auto-fill
// behavior as Fetch / Push. On success (including already-up-to-date),
// informs the journal that the remote head is at NewHead — pull is
// inbound, so no sync marker is appended; only the cursor's version
// updates.
func (s *Service) Pull(opts PullOptions) (*PullResult, error) {
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

// PullWithStash is the journal-aware auto-stash variant of Pull. The
// Service reads the journal's pending set for the git backend and
// passes it to Manager.PullWithStash; the Manager snapshots, resets,
// pulls, and restores. Same RecordRemoteSeen behavior as Pull on
// success — the underlying inner Pull's NewHead is what we record.
//
// The pending list includes only paths the journal knows are dirty —
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
