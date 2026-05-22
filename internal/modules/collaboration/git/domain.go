package git

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"gopkg.in/yaml.v3"

	"github.com/petervdpas/formidable2/internal/modules/collaboration/recmerge"
)

type Manager struct {
	log *slog.Logger
}

func NewManager() *Manager { return &Manager{log: slog.Default()} }

// WithLogger swaps the Manager's logger. Used by app wiring to route
// open/auth failures into the in-app log workspace so users without
// devtools (Windows builds) can see why a repo failed to open.
func (m *Manager) WithLogger(log *slog.Logger) *Manager {
	if log != nil {
		m.log = log
	}
	return m
}

func (m *Manager) open(path string) (*gogit.Repository, error) {
	return gogit.PlainOpenWithOptions(path, &gogit.PlainOpenOptions{DetectDotGit: true})
}

// IsGitRepo reports whether <path> sits inside a git worktree.
// Errors collapse to false for the caller's bool return, but the
// underlying go-git error is logged at warn level so the user can
// diagnose unsupported-extension and path-resolution failures (e.g.
// VSCode-cloned repos on Windows) from the in-app log.
// ErrRepositoryNotExists is the boring "really not a repo" case and
// is logged at debug only.
func (m *Manager) IsGitRepo(path string) bool {
	_, err := m.open(path)
	if err == nil {
		return true
	}
	if errors.Is(err, gogit.ErrRepositoryNotExists) {
		m.log.Debug("git: IsGitRepo: path is not a repository", "path", path)
	} else {
		m.log.Warn("git: IsGitRepo: open failed", "path", path, "err", err.Error())
	}
	return false
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
// both Staged and Modified - same as the porcelain output.
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
	// before any commit) is not an error - leave Branch empty.
	head, err := r.Head()
	switch {
	case err == nil:
		if head.Name().IsBranch() {
			out.Branch = head.Name().Short()
			if cfg, _ := r.Config(); cfg != nil {
				if br, ok := cfg.Branches[out.Branch]; ok && br.Remote != "" && br.Merge != "" {
					// Tracking is reported as the remote-tracking
					// ref the upstream maps to - same name pattern
					// the JS gitManager exposed.
					out.Tracking = "refs/remotes/" + br.Remote + "/" + br.Merge.Short()
				}
			}
		} else {
			// HEAD points at a hash, not a branch.
			out.Detached = true
		}
	case errors.Is(err, plumbing.ErrReferenceNotFound):
		// Newborn repo, no commits yet - no branch to report.
	default:
		return nil, fmt.Errorf("git: head: %w", err)
	}

	// Ahead / behind counts vs the tracking ref. Cheap when the
	// repo's small; on huge histories this still bounds at the
	// number of commits in the symmetric difference, which is
	// usually tiny in practice for an active workspace.
	if out.Tracking != "" {
		if trackRef, terr := r.Reference(plumbing.ReferenceName(out.Tracking), true); terr == nil {
			ahead, behind, abErr := aheadBehind(r, head.Hash(), trackRef.Hash())
			if abErr == nil {
				out.Ahead = ahead
				out.Behind = behind
			}
		}
	}

	for f, st := range s {
		// Conflicts (both sides modified, etc.) - go-git surfaces
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
// tracking branches are not included here - they belong on a
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
// not an error - returns an empty slice, matching the UI's "no
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

// CommitChanges returns the file-level diff between the named commit
// and its first parent. Each entry is {path, status} where status is
// "A" (added), "M" (modified), "D" (deleted), or "R" (renamed).
//
// A root commit (no parents) treats every file as added. Merge
// commits use the FIRST parent for the diff - the standard "what
// did this commit change relative to the mainline" interpretation
// that matches `git show <hash>` output.
//
// Pure go-git: object.Tree.Diff handles the per-entry walk; we
// translate object.Change.Action into the single-letter status.
func (m *Manager) CommitChanges(path, hash string) ([]ChangeFile, error) {
	r, err := m.open(path)
	if err != nil {
		return nil, fmt.Errorf("git: changes: open: %w", err)
	}
	if strings.TrimSpace(hash) == "" {
		return nil, errors.New("git: changes: hash required")
	}
	commit, err := r.CommitObject(plumbing.NewHash(hash))
	if err != nil {
		return nil, fmt.Errorf("git: changes: commit %q: %w", hash, err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("git: changes: tree: %w", err)
	}

	// Root commit: no parent → every file is added.
	if commit.NumParents() == 0 {
		out := []ChangeFile{}
		_ = tree.Files().ForEach(func(f *object.File) error {
			out = append(out, ChangeFile{Path: f.Name, Status: "A"})
			return nil
		})
		sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
		return out, nil
	}

	parent, err := commit.Parent(0)
	if err != nil {
		return nil, fmt.Errorf("git: changes: parent: %w", err)
	}
	parentTree, err := parent.Tree()
	if err != nil {
		return nil, fmt.Errorf("git: changes: parent tree: %w", err)
	}

	changes, err := parentTree.Diff(tree)
	if err != nil {
		return nil, fmt.Errorf("git: changes: diff: %w", err)
	}
	out := make([]ChangeFile, 0, len(changes))
	for _, c := range changes {
		action, _ := c.Action()
		var p string
		switch {
		case c.To.Name != "" && c.From.Name != "" && c.To.Name != c.From.Name:
			p = c.To.Name
		case c.To.Name != "":
			p = c.To.Name
		default:
			p = c.From.Name
		}
		status := "M"
		switch action {
		case 1: // Insert
			status = "A"
		case 2: // Delete
			status = "D"
		case 3: // Modify
			status = "M"
		}
		// Rename detection: same blob hash on both sides but different
		// paths is a rename. Action would be Modify or Insert/Delete
		// pair - we collapse to "R" when the names differ AND the
		// blob is identical.
		if c.From.Name != "" && c.To.Name != "" && c.From.Name != c.To.Name {
			if !c.From.TreeEntry.Hash.IsZero() && c.From.TreeEntry.Hash == c.To.TreeEntry.Hash {
				status = "R"
			}
		}
		out = append(out, ChangeFile{Path: p, Status: status})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// LogGraph returns the same commits Log produces but with each entry
// enriched with parent hashes (for drawing the DAG edges) and any
// branch / HEAD refs that point at it (for the row's ref pills).
//
// limit <= 0 means "all". An empty repo (no HEAD yet) yields an empty
// slice and no error - matches Log's behavior.
func (m *Manager) LogGraph(path string, limit int) ([]GraphCommit, error) {
	r, err := m.open(path)
	if err != nil {
		return nil, fmt.Errorf("git: log-graph: open: %w", err)
	}
	head, err := r.Head()
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return []GraphCommit{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("git: log-graph: head: %w", err)
	}

	// Build a hash → []ref-name map once; the iter loop can then
	// attach refs to each commit in O(1). HEAD is recorded under its
	// own bucket so the frontend can highlight the current checkout.
	refsByHash := map[string][]string{}
	if head.Name().IsBranch() {
		hh := head.Hash().String()
		refsByHash[hh] = append(refsByHash[hh], "HEAD -> "+head.Name().Short())
	} else {
		// Detached HEAD - still useful to mark its position.
		refsByHash[head.Hash().String()] = append(refsByHash[head.Hash().String()], "HEAD")
	}
	branches, berr := r.Branches()
	if berr == nil {
		_ = branches.ForEach(func(ref *plumbing.Reference) error {
			h := ref.Hash().String()
			name := ref.Name().Short()
			// Skip the bare branch name when HEAD points at it - the
			// "HEAD -> name" pill already covers it.
			if head.Name().IsBranch() && head.Name().Short() == name {
				return nil
			}
			refsByHash[h] = append(refsByHash[h], name)
			return nil
		})
	}

	iter, err := r.Log(&gogit.LogOptions{From: head.Hash()})
	if err != nil {
		return nil, fmt.Errorf("git: log-graph: log: %w", err)
	}
	defer iter.Close()

	out := []GraphCommit{}
	count := 0
	if err := iter.ForEach(func(c *object.Commit) error {
		if limit > 0 && count >= limit {
			return errStopIter
		}
		gc := toGraphCommit(c)
		gc.Refs = append(gc.Refs, refsByHash[gc.Hash]...)
		out = append(out, gc)
		count++
		return nil
	}); err != nil && !errors.Is(err, errStopIter) {
		return nil, fmt.Errorf("git: log-graph: iterate: %w", err)
	}
	return out, nil
}

// toGraphCommit fills hash/short/author/email/time/subject and the
// parent-hash list. Refs is left nil - LogGraph attaches them after
// resolving the per-hash ref map.
func toGraphCommit(c *object.Commit) GraphCommit {
	subject := c.Message
	if i := strings.IndexByte(subject, '\n'); i >= 0 {
		subject = subject[:i]
	}
	hash := c.Hash.String()
	short := hash
	if len(short) > 7 {
		short = short[:7]
	}
	parents := make([]string, 0, c.NumParents())
	for _, p := range c.ParentHashes {
		parents = append(parents, p.String())
	}
	return GraphCommit{
		Hash:    hash,
		Short:   short,
		Author:  c.Author.Name,
		Email:   c.Author.Email,
		Time:    c.Author.When.Format(time.RFC3339),
		Subject: strings.TrimSpace(subject),
		Parents: parents,
	}
}

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

// Sentinel - keep config import alive for tests that need
// CreateRemote via go-git's config types. Inlined here so a future
// call site (e.g. SetRemote) doesn't trigger an "unused import"
// dance during partial implementation.
var _ = config.RemoteConfig{}

// Clone fetches a remote repository into opts.Dest. URL + Dest are
// required; opts.Branch picks the initial checkout (empty = remote
// HEAD); opts.PAT enables HTTP basic auth (transient - never
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
	// usual case for a fresh clone). Detached HEAD leaves it empty -
	// the UI then keeps git_branch unchanged rather than writing "".
	branch := ""
	if head.Name().IsBranch() {
		branch = head.Name().Short()
	}
	return &CloneResult{Dest: opts.Dest, Head: head.Hash().String(), Branch: branch}, nil
}

// Commit stages every change in the worktree (modified, untracked,
// deleted) and creates a commit on the current branch with the given
// author/email/message.
//
// Refuses on:
//   - empty message / empty author / empty email - these are UI bugs
//     and silently no-op'ing leaves the user wondering why nothing
//     happened.
//   - a clean worktree - go-git would happily make an empty commit
//     otherwise; explicit error lets the UI greylist the button.
//   - detached HEAD - committing detached needs explicit branch
//     decisions we don't surface yet.
func (m *Manager) Commit(opts CommitOptions) (*CommitResult, error) {
	if strings.TrimSpace(opts.Message) == "" {
		return nil, errors.New("git: commit: message required")
	}
	if strings.TrimSpace(opts.Author) == "" || strings.TrimSpace(opts.Email) == "" {
		return nil, errors.New("git: commit: author and email required")
	}

	r, err := m.open(opts.Path)
	if err != nil {
		return nil, fmt.Errorf("git: commit: open: %w", err)
	}

	// Detached-HEAD guard. Empty repo (no HEAD yet) is allowed - the
	// first commit on a fresh init lands on the default branch.
	head, err := r.Head()
	if err == nil {
		if !head.Name().IsBranch() {
			return nil, errors.New("git: commit: HEAD is detached")
		}
	} else if !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return nil, fmt.Errorf("git: commit: head: %w", err)
	}

	wt, err := r.Worktree()
	if err != nil {
		return nil, fmt.Errorf("git: commit: worktree: %w", err)
	}

	st, err := wt.Status()
	if err != nil {
		return nil, fmt.Errorf("git: commit: status: %w", err)
	}
	if st.IsClean() {
		return nil, errors.New("git: commit: nothing to commit")
	}

	// Stage every change. Untracked / modified → Add; deleted →
	// Remove. Per-file iteration (vs AddWithOptions{All: true}) keeps
	// us robust across go-git versions where AddGlob/AddWithOptions
	// has shifted in behavior, and lets us be explicit about deletes.
	for f, fs := range st {
		if fs.Worktree == gogit.Deleted {
			if _, err := wt.Remove(f); err != nil {
				return nil, fmt.Errorf("git: commit: remove %q: %w", f, err)
			}
			continue
		}
		if _, err := wt.Add(f); err != nil {
			return nil, fmt.Errorf("git: commit: add %q: %w", f, err)
		}
	}

	h, err := wt.Commit(opts.Message, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  opts.Author,
			Email: opts.Email,
			When:  time.Now(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("git: commit: %w", err)
	}
	hash := h.String()
	short := hash
	if len(short) > 7 {
		short = short[:7]
	}
	return &CommitResult{Hash: hash, Short: short}, nil
}

// aheadBehind walks both sides of the local branch ↔ tracking ref
// pair and returns (ahead, behind). "Ahead" is the count of commits
// reachable from headHash but not from trackHash; "behind" is the
// reverse. We materialise both reachable sets and take the
// symmetric difference - costs O(history) memory in the worst case,
// but in an active workspace the divergence is small.
func aheadBehind(r *gogit.Repository, headHash, trackHash plumbing.Hash) (int, int, error) {
	headSet := make(map[plumbing.Hash]struct{})
	if err := walkReachable(r, headHash, headSet); err != nil {
		return 0, 0, err
	}
	trackSet := make(map[plumbing.Hash]struct{})
	if err := walkReachable(r, trackHash, trackSet); err != nil {
		return 0, 0, err
	}
	ahead := 0
	for h := range headSet {
		if _, ok := trackSet[h]; !ok {
			ahead++
		}
	}
	behind := 0
	for h := range trackSet {
		if _, ok := headSet[h]; !ok {
			behind++
		}
	}
	return ahead, behind, nil
}

func walkReachable(r *gogit.Repository, from plumbing.Hash, into map[plumbing.Hash]struct{}) error {
	if from == plumbing.ZeroHash {
		return nil
	}
	iter, err := r.Log(&gogit.LogOptions{From: from})
	if err != nil {
		return err
	}
	defer iter.Close()
	return iter.ForEach(func(c *object.Commit) error {
		into[c.Hash] = struct{}{}
		return nil
	})
}

// Push sends the current branch's HEAD to the named remote (default
// "origin"). Returns AlreadyUpToDate=true when go-git reports there
// was nothing to send - that's an info-level outcome, not an error,
// so the UI can surface it as "you're current."
//
// Refuses on detached HEAD: the ref to push is computed from
// head.Name().Short(), which has no sensible default for headless
// checkouts. Empty PAT means anonymous (works for public read or
// pre-authed transports like SSH; HTTPS pushes to private repos
// will return a 401 the caller surfaces).
func (m *Manager) Push(opts PushOptions) (*PushResult, error) {
	if strings.TrimSpace(opts.Path) == "" {
		return nil, errors.New("git: push: path required")
	}
	r, err := m.open(opts.Path)
	if err != nil {
		return nil, fmt.Errorf("git: push: open: %w", err)
	}
	head, err := r.Head()
	if err != nil {
		return nil, fmt.Errorf("git: push: head: %w", err)
	}
	if !head.Name().IsBranch() {
		return nil, errors.New("git: push: HEAD is detached")
	}

	remote := opts.Remote
	if remote == "" {
		remote = "origin"
	}

	pushOpts := &gogit.PushOptions{RemoteName: remote}
	if opts.PAT != "" {
		pushOpts.Auth = &githttp.BasicAuth{
			Username: "x-access-token",
			Password: opts.PAT,
		}
	}

	err = r.Push(pushOpts)
	if errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		return &PushResult{AlreadyUpToDate: true, NewHead: head.Hash().String()}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("git: push: %w", err)
	}
	return &PushResult{AlreadyUpToDate: false, NewHead: head.Hash().String()}, nil
}

// Pull fetches from the named remote (default "origin") and merges
// the upstream branch into the current local branch. Refuses on
// detached HEAD; on a non-fast-forward merge with conflicts the
// caller gets a wrapped error from go-git's worktree pull. Empty
// PAT means anonymous (works for public repos and SSH).
//
// AlreadyUpToDate=true mirrors Push/Fetch - info, not error.
func (m *Manager) Pull(opts PullOptions) (*PullResult, error) {
	if strings.TrimSpace(opts.Path) == "" {
		return nil, errors.New("git: pull: path required")
	}
	r, err := m.open(opts.Path)
	if err != nil {
		return nil, fmt.Errorf("git: pull: open: %w", err)
	}
	head, err := r.Head()
	if err != nil {
		return nil, fmt.Errorf("git: pull: head: %w", err)
	}
	if !head.Name().IsBranch() {
		return nil, errors.New("git: pull: HEAD is detached")
	}

	wt, err := r.Worktree()
	if err != nil {
		return nil, fmt.Errorf("git: pull: worktree: %w", err)
	}

	remote := opts.Remote
	if remote == "" {
		remote = "origin"
	}

	pullOpts := &gogit.PullOptions{
		RemoteName:    remote,
		ReferenceName: head.Name(),
	}
	if opts.PAT != "" {
		pullOpts.Auth = &githttp.BasicAuth{
			Username: "x-access-token",
			Password: opts.PAT,
		}
	}

	err = wt.Pull(pullOpts)
	if errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		return &PullResult{AlreadyUpToDate: true, NewHead: head.Hash().String()}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("git: pull: %w", err)
	}
	newHead, herr := r.Head()
	if herr != nil {
		return nil, fmt.Errorf("git: pull: head after merge: %w", herr)
	}
	return &PullResult{AlreadyUpToDate: false, NewHead: newHead.Hash().String()}, nil
}

// stashSubdir is the worktree-relative directory PullWithStash uses to
// snapshot pending file contents before resetting them. Sits under the
// context root and is matched by journal/types.go's `.changes.*`
// gitignore patterns, so it never gets committed.
const stashSubdir = ".changes.stash"

// PullWithStash performs a journal-aware auto-stash + pull + restore.
// The pending manifest is the authoritative list of files Formidable
// has touched since the last sync - narrower than `git status`, which
// would also catch external edits we don't own.
//
// Flow:
//  1. Snapshot each pending file's content into <repoRoot>/.changes.stash/
//     and capture the pre-pull HEAD blob hash for conflict detection.
//  2. Reset those paths to HEAD (`git checkout HEAD -- <path>`) so the
//     worktree is clean and pull can fast-forward.
//  3. Run the normal Pull.
//  4. For each snapshot: if the post-pull HEAD blob hash differs from
//     the pre-pull hash, the file moved under us → conflict. Otherwise
//     restore the stashed content via atomic write.
//  5. On clean restore (no conflicts), remove the stash directory. On
//     conflicts, leave it for manual recovery and surface the list.
//
// On pull failure (network, auth, divergent), the worktree paths
// already reset to HEAD stay reset - the stash directory remains so
// the user can manually recover. We surface the pull error and the
// stash directory path to the caller.
func (m *Manager) PullWithStash(opts PullWithStashOptions) (*StashedPullResult, error) {
	if strings.TrimSpace(opts.Path) == "" {
		return nil, errors.New("git: pull-with-stash: path required")
	}
	r, err := m.open(opts.Path)
	if err != nil {
		return nil, fmt.Errorf("git: pull-with-stash: open: %w", err)
	}
	head, err := r.Head()
	if err != nil {
		return nil, fmt.Errorf("git: pull-with-stash: head: %w", err)
	}
	if !head.Name().IsBranch() {
		return nil, errors.New("git: pull-with-stash: HEAD is detached")
	}
	wt, err := r.Worktree()
	if err != nil {
		return nil, fmt.Errorf("git: pull-with-stash: worktree: %w", err)
	}
	repoRoot := wt.Filesystem.Root()
	stashRoot := filepath.Join(repoRoot, stashSubdir)

	// Defensive sweep: if a previous PullWithStash crashed mid-flow
	// (or this is the first run after a pre-fix codebase upgrade),
	// the leftover .changes.stash/ would otherwise mix stale content
	// into our fresh snapshot. Always start from a clean slate. Best-
	// effort: a removal error means the snapshot writes happen on
	// top of whatever's there, which the journal pending list will
	// eventually re-cover anyway.
	_ = os.RemoveAll(stashRoot)

	// Phase 1: snapshot. We keep entries even when there's nothing to
	// stash so the pull-only branch (no pending) is reachable in one
	// path; an empty Pending slice produces an empty entries list.
	entries, err := m.stashSnapshot(r, head, repoRoot, stashRoot, opts.Pending)
	if err != nil {
		return nil, fmt.Errorf("git: pull-with-stash: snapshot: %w", err)
	}

	// Phase 2: reset only the snapshotted paths. We don't touch the
	// rest of the worktree - files that aren't in the journal pending
	// set don't get clobbered.
	if err := m.stashReset(r, wt, entries); err != nil {
		// Reset failed - undo what we can: rewrite stashed content
		// back so the user's changes aren't lost. Surface the error.
		_, _ = m.stashRestoreOnFailure(repoRoot, entries)
		_ = os.RemoveAll(stashRoot)
		return nil, fmt.Errorf("git: pull-with-stash: reset: %w", err)
	}

	// Phase 3: regular Pull on the now-clean worktree.
	pullRes, pullErr := m.Pull(opts.PullOptions)
	if pullErr != nil {
		// Pull failed (network, auth, divergent, ...). Worktree paths
		// are reset; restore stashed content so user data isn't lost,
		// then clean up the stash dir. Surface the pull error.
		restored, _ := m.stashRestoreOnFailure(repoRoot, entries)
		_ = os.RemoveAll(stashRoot)
		return &StashedPullResult{
			Pull:     pullRes,
			Stashed:  stashedPathList(entries),
			Restored: restored,
		}, fmt.Errorf("git: pull-with-stash: %w", pullErr)
	}

	// Phase 4: restore + merge. Re-open repo + HEAD so we read
	// post-pull tree. For each entry, compare pre-pull and post-pull
	// blob hashes. Same hash → pull didn't touch, restore stash
	// content. Different hash → try structured merge for .meta.json;
	// fall back to "pull wins, drop user version" otherwise. Stash
	// directory is always removed at the end - the Overridden list is
	// the only signal that something was lost.
	r2, err := m.open(opts.Path)
	if err != nil {
		return nil, fmt.Errorf("git: pull-with-stash: reopen: %w", err)
	}
	head2, err := r2.Head()
	if err != nil {
		return nil, fmt.Errorf("git: pull-with-stash: head after pull: %w", err)
	}
	restored, autoMerged, overridden, err := m.stashMergeOrOverride(r2, head2, repoRoot, entries)
	if err != nil {
		return nil, fmt.Errorf("git: pull-with-stash: restore: %w", err)
	}

	// Stash dir always trashed: clean restore, auto-merge, and override
	// all converged on a final on-disk state we want to keep.
	_ = os.RemoveAll(stashRoot)

	return &StashedPullResult{
		Pull:       pullRes,
		Stashed:    stashedPathList(entries),
		Restored:   restored,
		AutoMerged: autoMerged,
		Overridden: overridden,
	}, nil
}

// stashSnapshot captures pending file contents to <repoRoot>/.changes.stash/
// mirroring the worktree layout. For each pending entry it also records
// the pre-pull HEAD blob hash so the restore step can detect whether
// pull moved the path out from under the stash.
//
// Pending is the journal's view of "files Formidable mutated since
// the last sync". The journal is conservative - it logs every write
// through system.SaveFile, including no-op saves and writes that were
// later locally committed (the cursor only advances on sync, not on
// commit). So the manifest can include "stale" entries: paths the
// journal claims are dirty but where on-disk content already matches
// HEAD. Stashing those would be a double waste - pure clutter, plus
// it'd trip a false-positive conflict if pull happens to touch one
// (post-pull blob hash differs from pre-pull, but the user has nothing
// to restore).
//
// We filter at snapshot: a pending entry is kept only when on-disk
// content actually differs from the HEAD blob. Real working-tree dirt
// gets stashed; stale-but-clean entries are silently skipped.
func (m *Manager) stashSnapshot(r *gogit.Repository, head *plumbing.Reference, repoRoot, stashRoot string, pending []StashPathPending) ([]StashEntry, error) {
	if len(pending) == 0 {
		return []StashEntry{}, nil
	}

	headTree, err := treeOf(r, head.Hash())
	if err != nil {
		return nil, fmt.Errorf("head tree: %w", err)
	}

	entries := make([]StashEntry, 0, len(pending))
	for _, p := range pending {
		clean := filepath.ToSlash(filepath.Clean(p.Path))
		if clean == "" || clean == "." {
			continue
		}
		if strings.HasPrefix(clean, "../") || clean == ".." || filepath.IsAbs(clean) {
			continue
		}
		op := p.Op
		switch op {
		case "create", "update", "delete":
			// supported
		default:
			continue
		}

		// Record pre-pull HEAD blob hash for conflict detection later.
		oldHash := blobHashAt(headTree, clean)

		keep, content, err := shouldStash(r, repoRoot, clean, op, oldHash)
		if err != nil {
			return nil, fmt.Errorf("filter %q: %w", clean, err)
		}
		if !keep {
			continue
		}

		// For delete-ops there's no file content to snapshot; the
		// marker plus oldHash is enough to drive restore semantics.
		if op == "delete" {
			entries = append(entries, StashEntry{
				Path: clean, Op: op, OldHash: oldHash,
			})
			continue
		}

		// create / update: stash the captured content under
		// .changes.stash/<clean>.
		stashPath := filepath.Join(stashRoot, clean)
		if err := os.MkdirAll(filepath.Dir(stashPath), 0o755); err != nil {
			return nil, fmt.Errorf("mkdir stash %q: %w", clean, err)
		}
		if err := atomicWriteBytes(stashPath, content, 0o644); err != nil {
			return nil, fmt.Errorf("write stash %q: %w", clean, err)
		}
		entries = append(entries, StashEntry{
			Path:     clean,
			Op:       op,
			Bytes:    int64(len(content)),
			OldHash:  oldHash,
			StashRef: clean,
		})
	}
	return entries, nil
}

// shouldStash decides whether a journal-pending entry actually needs
// snapshotting. The journal logs every write through system.SaveFile;
// it doesn't know whether a write left the file content-equal to HEAD
// (committed-but-unpushed paths sit in pending until the next sync,
// even though their disk state matches HEAD). Stashing those would
// pollute .changes.stash and risk false-positive conflicts if pull
// happens to advance HEAD on one of them.
//
// Returns (keep, content, err):
//   - keep == false → the journal entry is stale; skip the snapshot.
//   - keep == true, op=="delete" → marker-only stash; content is nil.
//   - keep == true, op∈{create,update} → content holds the worktree
//     bytes the caller writes into .changes.stash/.
//
// Decision matrix:
//   delete + file present → stale (file came back). Skip.
//   delete + file absent + oldHash != "" → real delete. Keep.
//   delete + file absent + oldHash == "" → nothing to delete. Skip.
//   create/update + file absent → nothing to capture. Skip.
//   create/update + oldHash == "" → brand-new file (not in HEAD). Keep.
//   create/update + disk == HEAD blob → stale write. Skip.
//   create/update + disk != HEAD blob → real edit. Keep.
func shouldStash(r *gogit.Repository, repoRoot, path, op, oldHash string) (bool, []byte, error) {
	full := filepath.Join(repoRoot, path)
	disk, readErr := os.ReadFile(full)
	diskPresent := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		return false, nil, fmt.Errorf("read worktree: %w", readErr)
	}

	if op == "delete" {
		if diskPresent || oldHash == "" {
			return false, nil, nil
		}
		return true, nil, nil
	}

	// op is create or update.
	if !diskPresent {
		return false, nil, nil
	}
	if oldHash == "" {
		// Untracked-in-HEAD: any disk content is a real new file.
		return true, disk, nil
	}
	blob, err := r.BlobObject(plumbing.NewHash(oldHash))
	if err != nil {
		// Couldn't resolve HEAD blob - be conservative and stash.
		return true, disk, nil
	}
	reader, err := blob.Reader()
	if err != nil {
		return true, disk, nil
	}
	headContent, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		return true, disk, nil
	}
	if bytes.Equal(disk, headContent) {
		return false, nil, nil
	}
	return true, disk, nil
}

// stashReset clears each snapshotted path so the worktree is clean
// before pull. go-git v5 has no per-file `git checkout HEAD -- <path>`
// - wt.Checkout operates on the whole worktree, which would clobber
// dirt outside the journal pending set. So we mirror the Discard()
// pattern: read the HEAD blob (or its absence) and rewrite/remove the
// worktree file by hand, then wt.Add (or wt.Remove) to refresh the
// index entry so go-git sees the file as clean.
//
// Delete-ops need the same treatment: a missing file vs a HEAD that
// has it would block pull with "unstaged changes". We restore the file
// from HEAD before pull; the restore step deletes it again post-pull
// (or detects a conflict if pull modified it).
func (m *Manager) stashReset(r *gogit.Repository, wt *gogit.Worktree, entries []StashEntry) error {
	for _, e := range entries {
		full := filepath.Join(wt.Filesystem.Root(), e.Path)

		if e.OldHash == "" {
			// Brand-new file (no HEAD entry). Drop it from the index
			// (if staged) and the worktree so pull sees a clean slate.
			// A delete-op against a path that wasn't in HEAD shouldn't
			// happen in practice (you can't delete a non-existent file),
			// but the same "clear the worktree" handling is correct
			// either way.
			if _, err := wt.Remove(e.Path); err != nil {
				if rmErr := os.Remove(full); rmErr != nil && !os.IsNotExist(rmErr) {
					return fmt.Errorf("clear new file %q: %w", e.Path, rmErr)
				}
			}
			continue
		}

		// Tracked file: rewrite worktree from HEAD blob, then re-add
		// so the index hash matches.
		hash := plumbing.NewHash(e.OldHash)
		blob, err := r.BlobObject(hash)
		if err != nil {
			return fmt.Errorf("blob %q: %w", e.Path, err)
		}
		reader, err := blob.Reader()
		if err != nil {
			return fmt.Errorf("blob reader %q: %w", e.Path, err)
		}
		content, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			return fmt.Errorf("read blob %q: %w", e.Path, err)
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return fmt.Errorf("mkdir reset %q: %w", e.Path, err)
		}
		if err := atomicWriteBytes(full, content, 0o644); err != nil {
			return fmt.Errorf("reset write %q: %w", e.Path, err)
		}
		if _, err := wt.Add(e.Path); err != nil {
			return fmt.Errorf("reset index %q: %w", e.Path, err)
		}
	}
	return nil
}

// stashMergeOrOverride is phase 4 of PullWithStash. For each stashed
// entry, it picks one of three outcomes:
//
//   - Restored: pull didn't touch the path (pre/post HEAD blob hash
//     unchanged). The stashed content is written back. The user's edit
//     round-trips; no remote conflict to consider.
//   - AutoMerged: pull touched the path AND the path is a Formidable
//     record file (storage/<tpl>/<n>.meta.json) AND recmerge.Merge
//     reconciled cleanly. The merged JSON is written; both sides'
//     non-overlapping field edits survive.
//   - Overridden: pull touched the path AND either it's not a record
//     file (yaml templates, binaries) OR recmerge returned a
//     RecordConflict (immutable meta divergence). Pull's content stays
//     on disk (we do nothing - pull already left it there). The
//     post-pull commit's author/email/time for this path is captured
//     so the UI can tell the user who to coordinate with.
//
// The .changes.stash directory is removed by the caller after this
// function returns regardless of the outcome - the Overridden list is
// the only durable signal of what was lost.
func (m *Manager) stashMergeOrOverride(r *gogit.Repository, head *plumbing.Reference, repoRoot string, entries []StashEntry) ([]string, []string, []OverriddenPath, error) {
	headTree, err := treeOf(r, head.Hash())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("post-pull head tree: %w", err)
	}

	restored := []string{}
	autoMerged := []string{}
	overridden := []OverriddenPath{}
	stashRoot := filepath.Join(repoRoot, stashSubdir)

	for _, e := range entries {
		newHash := blobHashAt(headTree, e.Path)

		if e.OldHash == newHash {
			// Pull didn't touch this path → restore stash content.
			if err := applyStashEntry(repoRoot, stashRoot, e); err != nil {
				return nil, nil, nil, err
			}
			restored = append(restored, e.Path)
			continue
		}

		// Pull touched the path. Try a structured merge for record
		// files; everything else falls through to "pull wins".
		if e.Op != "delete" && e.StashRef != "" && recordPathMergeable(e) {
			merged, ok, err := tryRecordMerge(r, e, repoRoot, stashRoot, head)
			if err != nil {
				return nil, nil, nil, err
			}
			if ok {
				full := filepath.Join(repoRoot, e.Path)
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					return nil, nil, nil, fmt.Errorf("mkdir merge %q: %w", e.Path, err)
				}
				if err := atomicWriteBytes(full, merged, 0o644); err != nil {
					return nil, nil, nil, fmt.Errorf("write merge %q: %w", e.Path, err)
				}
				autoMerged = append(autoMerged, e.Path)
				continue
			}
		}

		// Override: pull wins, capture authorship for the UI. Prefer
		// the file's own author identity (records embed it in
		// meta.author_name/email; templates carry author_name/email
		// at the YAML root). Both are kept fresh by SaveTemplate /
		// the form save path - transport-agnostic, so this works
		// for git and gigot identically. Fall back to git log for
		// binaries and for files that don't carry the fields yet.
		over := lookupAuthorFromRecord(r, head, repoRoot, e.Path)
		if over.Author == "" && over.Email == "" {
			over = lookupAuthorFromTemplate(r, head, repoRoot, e.Path)
		}
		if over.Author == "" && over.Email == "" {
			gitOver, err := lookupAuthor(r, head.Hash(), e.Path)
			if err == nil {
				over = gitOver
			} else {
				over = OverriddenPath{Path: e.Path}
			}
		}
		overridden = append(overridden, over)
	}
	return restored, autoMerged, overridden, nil
}

// lookupAuthorFromTemplate pulls author identity out of a Formidable
// template's YAML root (author_name / author_email keys, populated by
// template.SaveTemplate from the active profile's config when missing).
// Returns a populated OverriddenPath when the path matches
// templates/*.yaml and the YAML decodes with non-empty author fields;
// otherwise returns a zero value (caller falls back to git log).
// Mirrors lookupAuthorFromRecord - both pull author info from the file
// itself rather than walking git history.
func lookupAuthorFromTemplate(r *gogit.Repository, head *plumbing.Reference, _ string, path string) OverriddenPath {
	// Path shape: "templates/<name>.yaml" exactly two segments.
	if !isTemplatePath(path) {
		return OverriddenPath{}
	}
	headTree, err := treeOf(r, head.Hash())
	if err != nil {
		return OverriddenPath{}
	}
	entry, err := headTree.File(path)
	if err != nil {
		return OverriddenPath{}
	}
	content, err := readBlob(r, entry.Hash)
	if err != nil {
		return OverriddenPath{}
	}
	// Light-weight head-only parse: we only need author_name +
	// author_email. Full template.UnmarshalYAML would drag the
	// validate/normalize stack in. Instead, parse as a generic map.
	var head2 map[string]any
	if err := yaml.Unmarshal(content, &head2); err != nil {
		return OverriddenPath{}
	}
	name, _ := head2["author_name"].(string)
	email, _ := head2["author_email"].(string)
	if name == "" && email == "" {
		return OverriddenPath{}
	}
	return OverriddenPath{
		Path:   path,
		Author: name,
		Email:  email,
	}
}

// isTemplatePath returns true when path is exactly templates/<name>.yaml.
func isTemplatePath(path string) bool {
	if path == "" || !strings.HasSuffix(path, ".yaml") {
		return false
	}
	if !strings.HasPrefix(path, "templates/") {
		return false
	}
	rest := path[len("templates/"):]
	if rest == "" || strings.Contains(rest, "/") {
		return false
	}
	return true
}

// lookupAuthorFromRecord pulls author identity out of a Formidable
// record's meta envelope. Returns a populated OverriddenPath when the
// path resolves to a parseable record at HEAD with non-empty author
// fields; otherwise returns a zero value (caller falls back to git
// log). The post-pull HEAD content is the right source - it represents
// the version that won.
func lookupAuthorFromRecord(r *gogit.Repository, head *plumbing.Reference, repoRoot, path string) OverriddenPath {
	if !recmerge.IsRecordPath(path) {
		return OverriddenPath{}
	}
	headTree, err := treeOf(r, head.Hash())
	if err != nil {
		return OverriddenPath{}
	}
	entry, err := headTree.File(path)
	if err != nil {
		return OverriddenPath{}
	}
	content, err := readBlob(r, entry.Hash)
	if err != nil {
		return OverriddenPath{}
	}
	rec, err := recmerge.ParseRecord(content)
	if err != nil {
		return OverriddenPath{}
	}
	// New shape: meta.updated = {at, name, email}. Pull authorship from
	// the Updated block (last writer wins, matching git's committer
	// semantics). Fall back to the legacy flat author_name/email keys
	// for records written by old Formidable that haven't been re-saved
	// since the migration.
	name, email, updated := recordAuthorFromMeta(rec.Meta)
	if name == "" && email == "" {
		return OverriddenPath{}
	}
	return OverriddenPath{
		Path:   path,
		Author: name,
		Email:  email,
		Time:   updated,
	}
}

func recordAuthorFromMeta(meta map[string]any) (name, email, updatedAt string) {
	if u, ok := meta["updated"].(map[string]any); ok {
		name, _ = u["name"].(string)
		email, _ = u["email"].(string)
		updatedAt, _ = u["at"].(string)
	}
	if name == "" {
		name, _ = meta["author_name"].(string)
	}
	if email == "" {
		email, _ = meta["author_email"].(string)
	}
	if updatedAt == "" {
		updatedAt, _ = meta["updated"].(string)
	}
	return
}

// applyStashEntry writes a single stashed entry to the worktree -
// content for create/update, removal for delete. Used for the
// "restore" branch where pull didn't touch the path.
func applyStashEntry(repoRoot, stashRoot string, e StashEntry) error {
	full := filepath.Join(repoRoot, e.Path)
	switch e.Op {
	case "delete":
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("re-delete %q: %w", e.Path, err)
		}
	case "create", "update":
		if e.StashRef == "" {
			return nil
		}
		stashPath := filepath.Join(stashRoot, e.StashRef)
		content, err := os.ReadFile(stashPath)
		if err != nil {
			return fmt.Errorf("read stash %q: %w", e.Path, err)
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return fmt.Errorf("mkdir restore %q: %w", e.Path, err)
		}
		if err := atomicWriteBytes(full, content, 0o644); err != nil {
			return fmt.Errorf("write restore %q: %w", e.Path, err)
		}
	}
	return nil
}

// recordPathMergeable returns true when the entry's path is a
// Formidable record file (storage/<tpl>/<n>.meta.json) AND it's
// tracked on both pre- and post-pull HEAD (so we have a base + theirs
// to merge against). Add/delete shapes can't go through structured
// merge - they fall through to "pull wins".
func recordPathMergeable(e StashEntry) bool {
	if !recmerge.IsRecordPath(e.Path) {
		return false
	}
	if e.OldHash == "" {
		return false
	}
	return true
}

// tryRecordMerge runs recmerge.Merge against three blob inputs:
//   - BASE: the pre-pull HEAD blob (e.OldHash captured at snapshot).
//   - THEIRS: the post-pull HEAD blob.
//   - YOURS: the stashed content (the user's pre-pull worktree).
//
// Returns (merged, true, nil) on a clean per-field merge; (nil, false,
// nil) on RecordConflict (immutable-meta divergence); (nil, false,
// err) on transport/parse errors. The caller falls through to the
// "override" path on (nil, false, ...).
func tryRecordMerge(r *gogit.Repository, e StashEntry, repoRoot, stashRoot string, head *plumbing.Reference) ([]byte, bool, error) {
	baseBytes, err := readBlob(r, plumbing.NewHash(e.OldHash))
	if err != nil {
		return nil, false, nil
	}
	headTree, err := treeOf(r, head.Hash())
	if err != nil {
		return nil, false, fmt.Errorf("head tree: %w", err)
	}
	theirsEntry, err := headTree.File(e.Path)
	if err != nil {
		return nil, false, nil
	}
	theirsBytes, err := readBlob(r, theirsEntry.Hash)
	if err != nil {
		return nil, false, nil
	}
	yoursPath := filepath.Join(stashRoot, e.StashRef)
	yoursBytes, err := os.ReadFile(yoursPath)
	if err != nil {
		return nil, false, fmt.Errorf("read stash %q: %w", e.Path, err)
	}

	base, err := recmerge.ParseRecord(baseBytes)
	if err != nil {
		return nil, false, nil
	}
	theirs, err := recmerge.ParseRecord(theirsBytes)
	if err != nil {
		return nil, false, nil
	}
	yours, err := recmerge.ParseRecord(yoursBytes)
	if err != nil {
		return nil, false, nil
	}
	res, err := recmerge.Merge(e.Path, base, theirs, yours)
	if err != nil {
		return nil, false, err
	}
	if res.Conflict != nil {
		return nil, false, nil
	}
	return res.Merged, true, nil
}

// readBlob reads the bytes of a blob given its hash. Used by the
// record merger to materialise base + theirs content.
func readBlob(r *gogit.Repository, h plumbing.Hash) ([]byte, error) {
	blob, err := r.BlobObject(h)
	if err != nil {
		return nil, err
	}
	reader, err := blob.Reader()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

// lookupAuthor walks back from head to find the most recent commit
// that touched path, then returns the author/email/time for the UI's
// "contact this person" notification. Falls back to the head commit's
// author when path-bisecting walk fails - better than nothing.
func lookupAuthor(r *gogit.Repository, headHash plumbing.Hash, path string) (OverriddenPath, error) {
	iter, err := r.Log(&gogit.LogOptions{From: headHash})
	if err != nil {
		return OverriddenPath{Path: path}, err
	}
	defer iter.Close()

	var fallback OverriddenPath
	fallback.Path = path

	var found OverriddenPath
	foundIt := false
	if err := iter.ForEach(func(c *object.Commit) error {
		if !foundIt && fallback.Author == "" {
			fallback = OverriddenPath{
				Path:   path,
				Author: c.Author.Name,
				Email:  c.Author.Email,
				Time:   c.Author.When.Format(time.RFC3339),
				Commit: c.Hash.String(),
			}
		}
		// Match any commit that has this path in its tree differently
		// from its parent. For a root commit (no parent), any
		// containment counts.
		tree, err := c.Tree()
		if err != nil {
			return nil
		}
		_, treeErr := tree.File(path)
		var parentTree *object.Tree
		if c.NumParents() > 0 {
			parent, perr := c.Parent(0)
			if perr == nil {
				parentTree, _ = parent.Tree()
			}
		}
		var parentHash plumbing.Hash
		if parentTree != nil {
			if entry, err := parentTree.File(path); err == nil {
				parentHash = entry.Hash
			}
		}
		var thisHash plumbing.Hash
		if treeErr == nil {
			if entry, err := tree.File(path); err == nil {
				thisHash = entry.Hash
			}
		}
		if parentHash != thisHash {
			found = OverriddenPath{
				Path:   path,
				Author: c.Author.Name,
				Email:  c.Author.Email,
				Time:   c.Author.When.Format(time.RFC3339),
				Commit: c.Hash.String(),
			}
			foundIt = true
			return errStopIter
		}
		return nil
	}); err != nil && !errors.Is(err, errStopIter) {
		return fallback, err
	}
	if foundIt {
		return found, nil
	}
	return fallback, nil
}

// stashRestoreOnFailure writes stashed content back without conflict
// checking - used when pull never ran (or failed) and we want the
// user's pre-stash state back on disk. Best-effort: returns the paths
// that were successfully restored; ignores read errors and continues.
func (m *Manager) stashRestoreOnFailure(repoRoot string, entries []StashEntry) ([]string, error) {
	restored := []string{}
	stashRoot := filepath.Join(repoRoot, stashSubdir)
	for _, e := range entries {
		full := filepath.Join(repoRoot, e.Path)
		switch e.Op {
		case "delete":
			if err := os.Remove(full); err == nil || os.IsNotExist(err) {
				restored = append(restored, e.Path)
			}
		case "create", "update":
			if e.StashRef == "" {
				continue
			}
			stashPath := filepath.Join(stashRoot, e.StashRef)
			content, err := os.ReadFile(stashPath)
			if err != nil {
				continue
			}
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				continue
			}
			if err := atomicWriteBytes(full, content, 0o644); err != nil {
				continue
			}
			restored = append(restored, e.Path)
		}
	}
	return restored, nil
}

// treeOf returns the commit tree at hash, or an error if hash is zero
// (unborn HEAD - shouldn't happen here since we already checked
// IsBranch upstream).
func treeOf(r *gogit.Repository, h plumbing.Hash) (*object.Tree, error) {
	if h.IsZero() {
		return nil, errors.New("zero hash")
	}
	c, err := r.CommitObject(h)
	if err != nil {
		return nil, err
	}
	return c.Tree()
}

// blobHashAt returns the blob hash for path in tree, or "" if absent.
// Used as a "did this path's content change between trees" signal.
func blobHashAt(tree *object.Tree, path string) string {
	if tree == nil {
		return ""
	}
	entry, err := tree.File(path)
	if err != nil {
		return ""
	}
	return entry.Hash.String()
}

// stashedPathList projects a StashEntry slice to a flat path list for
// the JSON-friendly result envelope.
func stashedPathList(entries []StashEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Path)
	}
	return out
}

// atomicWriteBytes is the local copy of system.atomicWriteFile -
// inlined here so the git package doesn't have to import system just
// for the tmp+fsync+rename idiom. Restore writes go this way
// (NOT through system.SaveFile) so they bypass the journal: the
// journal entry that drove the snapshot is still pending, and writing
// the same content back doesn't change that contract.
func atomicWriteBytes(target string, content []byte, perm os.FileMode) error {
	dir := filepath.Dir(target)
	base := filepath.Base(target)
	f, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		return err
	}
	committed = true
	return nil
}

// Fetch updates the remote-tracking refs for the named remote
// (default "origin"). The local worktree isn't touched; ahead/behind
// counts in subsequent Status() calls reflect the new state of the
// remote-tracking ref. AlreadyUpToDate=true mirrors the Push contract.
func (m *Manager) Fetch(opts FetchOptions) (*FetchResult, error) {
	if strings.TrimSpace(opts.Path) == "" {
		return nil, errors.New("git: fetch: path required")
	}
	r, err := m.open(opts.Path)
	if err != nil {
		return nil, fmt.Errorf("git: fetch: open: %w", err)
	}

	remote := opts.Remote
	if remote == "" {
		remote = "origin"
	}

	fetchOpts := &gogit.FetchOptions{RemoteName: remote}
	if opts.PAT != "" {
		fetchOpts.Auth = &githttp.BasicAuth{
			Username: "x-access-token",
			Password: opts.PAT,
		}
	}

	err = r.Fetch(fetchOpts)
	if errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		return &FetchResult{AlreadyUpToDate: true}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("git: fetch: %w", err)
	}
	return &FetchResult{AlreadyUpToDate: false}, nil
}

// Discard throws away the local change to a single file. The right
// action depends on the file's current state:
//
//   - tracked + worktree-modified → restore worktree from HEAD blob,
//     then re-add so the index matches.
//   - tracked + worktree-deleted  → recreate the file from HEAD blob.
//   - staged add (file not in HEAD) → remove from index AND worktree.
//   - untracked                   → remove from worktree.
//
// Path-traversal segments ("..") are rejected up-front; File must be
// a clean relative path inside the worktree. Missing files (already
// gone, raced with a manual delete) are not an error - the desired
// end state is "discarded," and a missing untracked file already is.
func (m *Manager) Discard(opts DiscardOptions) error {
	if strings.TrimSpace(opts.File) == "" {
		return errors.New("git: discard: file required")
	}
	clean := filepath.Clean(opts.File)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return errors.New("git: discard: invalid file path")
	}

	r, err := m.open(opts.Path)
	if err != nil {
		return fmt.Errorf("git: discard: open: %w", err)
	}
	wt, err := r.Worktree()
	if err != nil {
		return fmt.Errorf("git: discard: worktree: %w", err)
	}

	// Look up the file's blob in HEAD if it's there. A nil headBlob
	// means "this file isn't tracked at HEAD" - either untracked or
	// a staged add. Either way: discard = remove.
	var headBlob *object.Blob
	if h, herr := r.Head(); herr == nil {
		if commit, cerr := r.CommitObject(h.Hash()); cerr == nil {
			if tree, terr := commit.Tree(); terr == nil {
				if entry, eerr := tree.File(clean); eerr == nil {
					if blob, berr := r.BlobObject(entry.Hash); berr == nil {
						headBlob = blob
					}
				}
			}
		}
	}

	fullPath := filepath.Join(wt.Filesystem.Root(), clean)

	if headBlob != nil {
		// Restore-from-HEAD path. Read the blob, write it to the
		// worktree, then Add() to refresh the index entry.
		reader, err := headBlob.Reader()
		if err != nil {
			return fmt.Errorf("git: discard: blob reader: %w", err)
		}
		content, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			return fmt.Errorf("git: discard: read blob: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return fmt.Errorf("git: discard: mkdir: %w", err)
		}
		if err := os.WriteFile(fullPath, content, 0o644); err != nil {
			return fmt.Errorf("git: discard: write: %w", err)
		}
		if _, err := wt.Add(clean); err != nil {
			return fmt.Errorf("git: discard: refresh index: %w", err)
		}
		return nil
	}

	// File not in HEAD - drop it. wt.Remove handles the staged-add
	// case (delete from worktree + index). For an untracked file,
	// wt.Remove fails because the index has no entry, so we fall
	// back to a plain os.Remove.
	if _, rmErr := wt.Remove(clean); rmErr != nil {
		if rmErr := os.Remove(fullPath); rmErr != nil && !os.IsNotExist(rmErr) {
			return fmt.Errorf("git: discard: remove: %w", rmErr)
		}
	}
	return nil
}
