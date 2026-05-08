package git

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

// Manager is the read-only entry point into a Git repository.
// Stateless for now; repo opens are cheap with go-git and we'd rather
// pay that cost per-call than maintain an invalidation policy.
// Network/auth-bearing ops will arrive in a later iteration.
type Manager struct{}

// NewManager constructs the read-only manager. No options yet —
// kept as a constructor so we don't have to change call sites when
// state (e.g. credential cache) lands.
func NewManager() *Manager { return &Manager{} }

// open returns the repository whose worktree contains <path>. We
// use DetectDotGit so callers can pass any path inside the worktree
// (e.g. a subfolder) and we'll walk up to the actual repo root.
func (m *Manager) open(path string) (*gogit.Repository, error) {
	return gogit.PlainOpenWithOptions(path, &gogit.PlainOpenOptions{DetectDotGit: true})
}

// IsGitRepo reports whether <path> sits inside a git worktree.
// Errors collapse to false — the caller doesn't care why, only
// whether the path is git-managed.
func (m *Manager) IsGitRepo(path string) bool {
	_, err := m.open(path)
	return err == nil
}

// RepoRoot returns the absolute worktree root for <path>, or an
// error if <path> is not inside a repo. Useful for showing the
// user the actual checkout location and for normalizing paths
// before relative-path operations.
func (m *Manager) RepoRoot(path string) (string, error) {
	r, err := m.open(path)
	if err != nil {
		return "", fmt.Errorf("git: open: %w", err)
	}
	wt, err := r.Worktree()
	if err != nil {
		return "", fmt.Errorf("git: worktree: %w", err)
	}
	return wt.Filesystem.Root(), nil
}

// Status snapshots the repo's current branch + worktree state.
// Status codes are normalised into named buckets (Modified,
// Untracked, Staged, Deleted, Renamed, Conflicted) so the
// frontend doesn't have to learn git's two-letter status grammar.
//
// A file modified in both the index and the worktree appears in
// both Staged and Modified — same as the porcelain output.
func (m *Manager) Status(path string) (*Status, error) {
	r, err := m.open(path)
	if err != nil {
		return nil, fmt.Errorf("git: open: %w", err)
	}
	wt, err := r.Worktree()
	if err != nil {
		return nil, fmt.Errorf("git: worktree: %w", err)
	}
	s, err := wt.Status()
	if err != nil {
		return nil, fmt.Errorf("git: status: %w", err)
	}

	out := &Status{
		Modified:   []string{},
		Untracked:  []string{},
		Staged:     []string{},
		Deleted:    []string{},
		Renamed:    []string{},
		Conflicted: []string{},
	}

	// Branch + tracking + detached. A missing HEAD (newborn repo
	// before any commit) is not an error — leave Branch empty.
	head, err := r.Head()
	switch {
	case err == nil:
		if head.Name().IsBranch() {
			out.Branch = head.Name().Short()
			if cfg, _ := r.Config(); cfg != nil {
				if br, ok := cfg.Branches[out.Branch]; ok && br.Remote != "" && br.Merge != "" {
					// Tracking is reported as the remote-tracking
					// ref the upstream maps to — same name pattern
					// the JS gitManager exposed.
					out.Tracking = "refs/remotes/" + br.Remote + "/" + br.Merge.Short()
				}
			}
		} else {
			// HEAD points at a hash, not a branch.
			out.Detached = true
		}
	case errors.Is(err, plumbing.ErrReferenceNotFound):
		// Newborn repo, no commits yet — no branch to report.
	default:
		return nil, fmt.Errorf("git: head: %w", err)
	}

	for f, st := range s {
		// Conflicts (both sides modified, etc.) — go-git surfaces
		// these via the worktree code 'U' / 'A' / 'D' pairs.
		if isConflict(st.Staging, st.Worktree) {
			out.Conflicted = append(out.Conflicted, f)
			continue
		}
		switch st.Staging {
		case gogit.Added, gogit.Modified, gogit.Copied:
			out.Staged = append(out.Staged, f)
		case gogit.Deleted:
			out.Staged = append(out.Staged, f)
			out.Deleted = append(out.Deleted, f)
		case gogit.Renamed:
			out.Staged = append(out.Staged, f)
			out.Renamed = append(out.Renamed, f)
		}
		switch st.Worktree {
		case gogit.Modified:
			out.Modified = append(out.Modified, f)
		case gogit.Deleted:
			out.Deleted = append(out.Deleted, f)
		case gogit.Untracked:
			out.Untracked = append(out.Untracked, f)
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
	return out, nil
}

// isConflict identifies the unmerged-file status code pairs that
// `git status --porcelain` would render as conflicts (DD, AU, UD,
// UA, DU, AA, UU). Mirrors the JS gitManager's UNMERGED_CODES set.
func isConflict(staging, worktree gogit.StatusCode) bool {
	type pair struct{ s, w gogit.StatusCode }
	conflicts := map[pair]bool{
		{gogit.Deleted, gogit.Deleted}:                 true,
		{gogit.Added, gogit.UpdatedButUnmerged}:        true,
		{gogit.UpdatedButUnmerged, gogit.Deleted}:      true,
		{gogit.UpdatedButUnmerged, gogit.Added}:        true,
		{gogit.Deleted, gogit.UpdatedButUnmerged}:      true,
		{gogit.Added, gogit.Added}:                     true,
		{gogit.UpdatedButUnmerged, gogit.UpdatedButUnmerged}: true,
	}
	return conflicts[pair{staging, worktree}]
}

// Branches lists local branches plus the active one. Sorted
// lexicographically so the UI can render a stable list. Remote-
// tracking branches are not included here — they belong on a
// separate call once the UI needs them.
func (m *Manager) Branches(path string) (*Branches, error) {
	r, err := m.open(path)
	if err != nil {
		return nil, fmt.Errorf("git: open: %w", err)
	}
	out := &Branches{Locals: []string{}}

	head, err := r.Head()
	if err == nil && head.Name().IsBranch() {
		out.Current = head.Name().Short()
	}

	iter, err := r.Branches()
	if err != nil {
		return nil, fmt.Errorf("git: branches: %w", err)
	}
	if err := iter.ForEach(func(ref *plumbing.Reference) error {
		out.Locals = append(out.Locals, ref.Name().Short())
		return nil
	}); err != nil {
		return nil, fmt.Errorf("git: iterate branches: %w", err)
	}
	sort.Strings(out.Locals)
	return out, nil
}

// Log returns up to <limit> commits walking back from HEAD, newest
// first. limit <= 0 means "all". An empty repo (no HEAD yet) is
// not an error — returns an empty slice, matching the UI's "no
// activity yet" expectation.
func (m *Manager) Log(path string, limit int) ([]Commit, error) {
	r, err := m.open(path)
	if err != nil {
		return nil, fmt.Errorf("git: open: %w", err)
	}
	head, err := r.Head()
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return []Commit{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("git: head: %w", err)
	}
	iter, err := r.Log(&gogit.LogOptions{From: head.Hash()})
	if err != nil {
		return nil, fmt.Errorf("git: log: %w", err)
	}
	defer iter.Close()

	out := []Commit{}
	count := 0
	if err := iter.ForEach(func(c *object.Commit) error {
		if limit > 0 && count >= limit {
			return errStopIter
		}
		out = append(out, toCommit(c))
		count++
		return nil
	}); err != nil && !errors.Is(err, errStopIter) {
		return nil, fmt.Errorf("git: iterate log: %w", err)
	}
	return out, nil
}

var errStopIter = errors.New("stop iteration")

func toCommit(c *object.Commit) Commit {
	subject := c.Message
	if i := strings.IndexByte(subject, '\n'); i >= 0 {
		subject = subject[:i]
	}
	hash := c.Hash.String()
	short := hash
	if len(short) > 7 {
		short = short[:7]
	}
	return Commit{
		Hash:    hash,
		Short:   short,
		Author:  c.Author.Name,
		Email:   c.Author.Email,
		Time:    c.Author.When.Format(time.RFC3339),
		Subject: strings.TrimSpace(subject),
	}
}

// RemoteInfo returns the configured remotes and their URLs. Order
// is the order go-git's Remotes() yields, which is config-file order
// (deterministic across calls because go-git reads the same map).
func (m *Manager) RemoteInfo(path string) (*RemoteInfo, error) {
	r, err := m.open(path)
	if err != nil {
		return nil, fmt.Errorf("git: open: %w", err)
	}
	remotes, err := r.Remotes()
	if err != nil {
		return nil, fmt.Errorf("git: remotes: %w", err)
	}
	out := &RemoteInfo{Remotes: make([]Remote, 0, len(remotes))}
	for _, rem := range remotes {
		cfg := rem.Config()
		out.Remotes = append(out.Remotes, Remote{
			Name: cfg.Name,
			URLs: append([]string(nil), cfg.URLs...),
		})
	}
	sort.Slice(out.Remotes, func(i, j int) bool {
		return out.Remotes[i].Name < out.Remotes[j].Name
	})
	return out, nil
}

// Sentinel — keep config import alive for tests that need
// CreateRemote via go-git's config types. Inlined here so a future
// call site (e.g. SetRemote) doesn't trigger an "unused import"
// dance during partial implementation.
var _ = config.RemoteConfig{}

// Clone fetches a remote repository into opts.Dest. URL + Dest are
// required; opts.Branch picks the initial checkout (empty = remote
// HEAD); opts.PAT enables HTTP basic auth (transient — never
// persisted by the manager).
//
// Refuses to clone into an existing non-empty directory. The
// frontend folder picker can hand us a fresh path; we surface a
// clear error rather than letting go-git's "repository already
// exists" message bubble up.
//
// SSH-based auth and clone progress streaming arrive in a later
// pass; this iteration covers HTTPS + PAT, which covers GitHub /
// GitLab / Gitea / Bitbucket the same way the JS version did.
func (m *Manager) Clone(opts CloneOptions) (*CloneResult, error) {
	if strings.TrimSpace(opts.URL) == "" {
		return nil, errors.New("git: clone: URL required")
	}
	if strings.TrimSpace(opts.Dest) == "" {
		return nil, errors.New("git: clone: destination required")
	}

	// Refuse to clone into a non-empty existing dir. A missing dir
	// is fine (go-git creates it), an empty existing dir is fine.
	if entries, err := os.ReadDir(opts.Dest); err == nil && len(entries) > 0 {
		return nil, fmt.Errorf("git: clone: destination not empty: %s", opts.Dest)
	}

	cloneOpts := &gogit.CloneOptions{URL: opts.URL}
	if opts.Branch != "" {
		cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(opts.Branch)
		cloneOpts.SingleBranch = true
	}
	if opts.PAT != "" {
		// "x-access-token" is a sentinel username many providers
		// accept (GitHub mandates a non-empty username; Gitea/GitLab
		// don't care what it is, only that the password is the PAT).
		cloneOpts.Auth = &githttp.BasicAuth{
			Username: "x-access-token",
			Password: opts.PAT,
		}
	}

	repo, err := gogit.PlainClone(opts.Dest, false, cloneOpts)
	if err != nil {
		return nil, fmt.Errorf("git: clone: %w", err)
	}
	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("git: clone: head: %w", err)
	}
	// Branch is the short name when HEAD points at a branch (the
	// usual case for a fresh clone). Detached HEAD leaves it empty —
	// the UI then keeps git_branch unchanged rather than writing "".
	branch := ""
	if head.Name().IsBranch() {
		branch = head.Name().Short()
	}
	return &CloneResult{Dest: opts.Dest, Head: head.Hash().String(), Branch: branch}, nil
}
