package git

import (
	"sort"
	"strconv"
	"strings"
)

// syncBackend is the set of git operations whose result depends on WHICH tool
// drives the working copy: go-git in-process, or the system git binary in
// self-cloned mode. Everything here either touches credentials, the index
// stat-data, or the worktree, which is exactly where the two tools disagree.
//
// Selecting one implementation per repo (not per call, scattered) keeps the two
// transports fully isolated: a repo is never half-driven by both, which is the
// class of bug where a go-git discard left the worktree phantom-dirty to the
// binary that then ran the pull.
//
// Pure local reads that both tools agree on (Log, Branches, RemoteInfo,
// CommitChanges, IsGitRepo, RepoRoot, Commit) stay on the Manager directly:
// they read commit objects and refs, not the racy index, so go-git reads them
// correctly even on a binary-managed repo, and a porcelain parser would add
// surface for no behavioral gain.
type syncBackend interface {
	Status(path string) (*Status, error)
	Discard(opts DiscardOptions) error
	Fetch(opts FetchOptions) (*FetchResult, error)
	Push(opts PushOptions) (*PushResult, error)
	Pull(opts PullOptions) (*PullResult, error)
}

// gogitBackend drives the repo with the in-process go-git Manager and resolves
// the keychain PAT for HTTPS auth. This is the default transport.
type gogitBackend struct {
	m       *Manager
	creds   CredentialReader
	profile ProfileReader
}

func (b *gogitBackend) Status(path string) (*Status, error) { return b.m.Status(path) }
func (b *gogitBackend) Discard(opts DiscardOptions) error   { return b.m.Discard(opts) }

func (b *gogitBackend) Fetch(opts FetchOptions) (*FetchResult, error) {
	if opts.PAT == "" {
		opts.PAT = b.resolvePAT(opts.Path)
	}
	return b.m.Fetch(opts)
}

func (b *gogitBackend) Push(opts PushOptions) (*PushResult, error) {
	if opts.PAT == "" {
		opts.PAT = b.resolvePAT(opts.Path)
	}
	return b.m.Push(opts)
}

func (b *gogitBackend) Pull(opts PullOptions) (*PullResult, error) {
	if opts.PAT == "" {
		opts.PAT = b.resolvePAT(opts.Path)
	}
	return b.m.Pull(opts)
}

// resolvePAT reads the keychain token for the origin remote (profile-scoped);
// any failure collapses to "" (anonymous attempt). go-git-only: the sysgit
// backend leaves auth to the user's credential helper.
func (b *gogitBackend) resolvePAT(path string) string {
	if b.creds == nil || b.profile == nil {
		return ""
	}
	profile := b.profile.CurrentProfileFilename()
	if profile == "" {
		return ""
	}
	info, err := b.m.RemoteInfo(path)
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
	secret, err := b.creds.Get(profile + ":git:" + url)
	if err != nil {
		return ""
	}
	return secret
}

// sysgitBackend drives the repo with the system git binary so the user's
// credential helper handles auth and the index stat-data stays in the binary's
// format. It never touches go-git, so the two transports share no state.
type sysgitBackend struct {
	run Sysgit
}

func (b *sysgitBackend) Status(path string) (*Status, error) {
	raw, err := b.run.StatusPorcelain(path)
	if err != nil {
		return nil, err
	}
	return parseStatusPorcelain(raw), nil
}

func (b *sysgitBackend) Discard(opts DiscardOptions) error {
	return b.run.Restore(opts.Path, opts.File)
}

func (b *sysgitBackend) Fetch(opts FetchOptions) (*FetchResult, error) {
	if err := b.run.Fetch(opts.Path, opts.Remote); err != nil {
		return nil, err
	}
	return &FetchResult{AlreadyUpToDate: false}, nil
}

func (b *sysgitBackend) Push(opts PushOptions) (*PushResult, error) {
	upToDate, err := b.run.Push(opts.Path, opts.Remote)
	if err != nil {
		return nil, err
	}
	head, _ := b.run.HeadHash(opts.Path)
	return &PushResult{AlreadyUpToDate: upToDate, NewHead: head}, nil
}

func (b *sysgitBackend) Pull(opts PullOptions) (*PullResult, error) {
	upToDate, err := b.run.Pull(opts.Path, opts.Remote)
	if err != nil {
		return nil, err
	}
	head, _ := b.run.HeadHash(opts.Path)
	return &PullResult{AlreadyUpToDate: upToDate, NewHead: head}, nil
}

// parseStatusPorcelain turns `git status --porcelain=v1 --branch` output into a
// Status with the same field classification as the go-git Manager.Status, so
// the frontend reads an identical shape regardless of transport.
func parseStatusPorcelain(raw string) *Status {
	out := &Status{
		Modified:   []string{},
		Untracked:  []string{},
		Staged:     []string{},
		Deleted:    []string{},
		Renamed:    []string{},
		Conflicted: []string{},
	}
	for _, line := range strings.Split(raw, "\n") {
		if line == "" {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "## "); ok {
			parseBranchHeader(rest, out)
			continue
		}
		if len(line) < 3 {
			continue
		}
		xy, path := line[:2], line[3:]
		if xy == "??" {
			out.Untracked = append(out.Untracked, path)
			continue
		}
		x, y := xy[0], xy[1]
		// Unmerged combinations are conflicts; skip the normal classification.
		if x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D') {
			out.Conflicted = append(out.Conflicted, path)
			continue
		}
		switch x {
		case 'A', 'M', 'C':
			out.Staged = append(out.Staged, path)
		case 'D':
			out.Staged = append(out.Staged, path)
			out.Deleted = append(out.Deleted, path)
		case 'R':
			renamed := path
			if i := strings.Index(path, " -> "); i >= 0 {
				renamed = path[i+4:]
			}
			out.Staged = append(out.Staged, renamed)
			out.Renamed = append(out.Renamed, renamed)
		}
		switch y {
		case 'M':
			out.Modified = append(out.Modified, path)
		case 'D':
			out.Deleted = append(out.Deleted, path)
		}
	}
	for _, slice := range [][]string{
		out.Modified, out.Untracked, out.Staged,
		out.Deleted, out.Renamed, out.Conflicted,
	} {
		sort.Strings(slice)
	}
	out.Clean = len(out.Modified) == 0 && len(out.Untracked) == 0 &&
		len(out.Staged) == 0 && len(out.Deleted) == 0 &&
		len(out.Renamed) == 0 && len(out.Conflicted) == 0
	return out
}

func parseBranchHeader(rest string, out *Status) {
	if branch, ok := strings.CutPrefix(rest, "No commits yet on "); ok {
		out.Branch = strings.TrimSpace(branch)
		return
	}
	if strings.HasPrefix(rest, "HEAD (no branch)") {
		out.Detached = true
		return
	}
	if i := strings.Index(rest, " ["); i >= 0 {
		parseAheadBehind(rest[i+2:], out)
		rest = rest[:i]
	}
	if i := strings.Index(rest, "..."); i >= 0 {
		out.Branch = rest[:i]
		out.Tracking = "refs/remotes/" + rest[i+3:]
		return
	}
	out.Branch = strings.TrimSpace(rest)
}

func parseAheadBehind(s string, out *Status) {
	s = strings.TrimSuffix(strings.TrimSpace(s), "]")
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if v, ok := strings.CutPrefix(part, "ahead "); ok {
			out.Ahead, _ = strconv.Atoi(strings.TrimSpace(v))
		} else if v, ok := strings.CutPrefix(part, "behind "); ok {
			out.Behind, _ = strconv.Atoi(strings.TrimSpace(v))
		}
	}
}
