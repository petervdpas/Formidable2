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

// WithLogger swaps the Manager's logger so open/auth failures reach the in-app log (Windows has no devtools).
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

// RepoRoot returns the absolute worktree root for <path>.
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

// Status snapshots the repo's current branch + worktree state into named buckets.
// A file modified in both index and worktree appears in both Staged and Modified, as porcelain does.
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

	head, err := r.Head()
	switch {
	case err == nil:
		if head.Name().IsBranch() {
			out.Branch = head.Name().Short()
			if cfg, _ := r.Config(); cfg != nil {
				if br, ok := cfg.Branches[out.Branch]; ok && br.Remote != "" && br.Merge != "" {
					out.Tracking = "refs/remotes/" + br.Remote + "/" + br.Merge.Short()
				}
			}
		} else {
			out.Detached = true
		}
	case errors.Is(err, plumbing.ErrReferenceNotFound):
		// Newborn repo, no commits yet.
	default:
		return nil, fmt.Errorf("git: head: %w", err)
	}

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

func isConflict(staging, worktree gogit.StatusCode) bool {
	type pair struct{ s, w gogit.StatusCode }
	conflicts := map[pair]bool{
		{gogit.Deleted, gogit.Deleted}:                       true,
		{gogit.Added, gogit.UpdatedButUnmerged}:              true,
		{gogit.UpdatedButUnmerged, gogit.Deleted}:            true,
		{gogit.UpdatedButUnmerged, gogit.Added}:              true,
		{gogit.Deleted, gogit.UpdatedButUnmerged}:            true,
		{gogit.Added, gogit.Added}:                           true,
		{gogit.UpdatedButUnmerged, gogit.UpdatedButUnmerged}: true,
	}
	return conflicts[pair{staging, worktree}]
}

// Branches lists local branches plus the active one, sorted.
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

// Log returns up to <limit> commits walking back from HEAD newest first; limit <= 0 means all.
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

// CommitChanges returns the file-level diff (status A/M/D/R) between the named commit and its first parent.
// Root commits treat every file as added; merge commits diff against the first parent, matching `git show`.
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
		case 1:
			status = "A"
		case 2:
			status = "D"
		case 3:
			status = "M"
		}
		// Same blob, different path is a rename: collapse to "R".
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

// LogGraph returns Log's commits enriched with parent hashes and the branch/HEAD refs pointing at each.
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

	refsByHash := map[string][]string{}
	if head.Name().IsBranch() {
		hh := head.Hash().String()
		refsByHash[hh] = append(refsByHash[hh], "HEAD -> "+head.Name().Short())
	} else {
		refsByHash[head.Hash().String()] = append(refsByHash[head.Hash().String()], "HEAD")
	}
	branches, berr := r.Branches()
	if berr == nil {
		_ = branches.ForEach(func(ref *plumbing.Reference) error {
			h := ref.Hash().String()
			name := ref.Name().Short()
			// HEAD -> name pill already covers the active branch.
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

// RemoteInfo returns the configured remotes and their URLs, sorted by name.
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

// Keeps the config import alive for a future call site (e.g. SetRemote).
var _ = config.RemoteConfig{}

// Clone fetches a remote into opts.Dest. opts.PAT enables HTTP basic auth (transient, never persisted).
// Refuses to clone into an existing non-empty directory.
func (m *Manager) Clone(opts CloneOptions) (*CloneResult, error) {
	if strings.TrimSpace(opts.URL) == "" {
		return nil, errors.New("git: clone: URL required")
	}
	if strings.TrimSpace(opts.Dest) == "" {
		return nil, errors.New("git: clone: destination required")
	}

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
	branch := ""
	if head.Name().IsBranch() {
		branch = head.Name().Short()
	}
	return &CloneResult{Dest: opts.Dest, Head: head.Hash().String(), Branch: branch}, nil
}

// Commit stages every worktree change and commits on the current branch.
// Refuses on empty message/author/email, a clean worktree, or a detached HEAD.
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

	// Empty repo (no HEAD yet) is allowed: first commit lands on the default branch.
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

	// Per-file Add/Remove (not AddWithOptions{All}) stays robust across go-git versions and is explicit about deletes.
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

// aheadBehind returns (ahead, behind) by symmetric difference of the two reachable sets.
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

// Push sends the current branch HEAD to the remote (default "origin"); refuses on detached HEAD.
// Empty PAT means anonymous; private HTTPS pushes then surface a 401 to the caller.
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

	// Push only the current branch, like `git push origin <branch>`
	// (push.default=simple). go-git's default refspec is
	// refs/heads/*:refs/heads/*, which would push every local branch; a
	// single stale sibling that can't fast-forward rejects the whole push.
	branchRef := head.Name().String()
	pushOpts := &gogit.PushOptions{
		RemoteName: remote,
		RefSpecs:   []config.RefSpec{config.RefSpec(branchRef + ":" + branchRef)},
	}
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

// Pull fetches the remote (default "origin") and merges upstream into the current branch; refuses on detached HEAD.
// Empty PAT means anonymous.
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

// stashSubdir is matched by journal/types.go's `.changes.*` gitignore patterns, so it never gets committed.
const stashSubdir = ".changes.stash"

// PullWithStash does a journal-aware auto-stash + pull + restore. The pending
// manifest is the authoritative set of files Formidable touched since the last
// sync, narrower than `git status` which would also catch external edits we don't own.
//
// Flow:
//  1. Snapshot each pending file into <repoRoot>/.changes.stash/ and capture its pre-pull HEAD blob hash.
//  2. Reset those paths to HEAD so the worktree is clean and pull can fast-forward.
//  3. Run the normal Pull.
//  4. Per snapshot: post-pull HEAD blob hash differs from pre-pull means the file moved under us (conflict); else restore the stash.
//  5. Clean restore removes the stash dir; conflicts leave it for manual recovery.
//
// On pull failure the reset paths stay reset and the stash dir remains for manual recovery.
func (m *Manager) PullWithStash(opts PullWithStashOptions) (*StashedPullResult, error) {
	return m.pullWithStash(opts, m.Pull)
}

// pullWithStash runs the stash orchestration with an injectable pull step, so
// self-cloned mode can route the fetch+merge through system git while reusing
// the journal-aware snapshot/reset/restore around it.
func (m *Manager) pullWithStash(opts PullWithStashOptions, pull func(PullOptions) (*PullResult, error)) (*StashedPullResult, error) {
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

	// Defensive sweep: a crashed prior run could leave stale .changes.stash/ content to mix into this snapshot.
	_ = os.RemoveAll(stashRoot)

	// Phase 1: snapshot.
	entries, err := m.stashSnapshot(r, head, repoRoot, stashRoot, opts.Pending)
	if err != nil {
		return nil, fmt.Errorf("git: pull-with-stash: snapshot: %w", err)
	}

	// Phase 2: reset only the snapshotted paths; files outside the journal pending set stay untouched.
	if err := m.stashReset(r, wt, entries); err != nil {
		// Undo: rewrite stashed content back so the user's changes aren't lost.
		_, _ = m.stashRestoreOnFailure(repoRoot, entries)
		_ = os.RemoveAll(stashRoot)
		return nil, fmt.Errorf("git: pull-with-stash: reset: %w", err)
	}

	// Phase 3: pull (go-git or sysgit, injected) on the now-clean worktree.
	pullRes, pullErr := pull(opts.PullOptions)
	if pullErr != nil {
		// Worktree paths are reset; restore stashed content so user data isn't lost.
		restored, _ := m.stashRestoreOnFailure(repoRoot, entries)
		_ = os.RemoveAll(stashRoot)
		return &StashedPullResult{
			Pull:     pullRes,
			Stashed:  stashedPathList(entries),
			Restored: restored,
		}, fmt.Errorf("git: pull-with-stash: %w", pullErr)
	}

	// Phase 4: restore + merge. Reopen so we read the post-pull tree.
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

	// Stash dir always removed: every outcome converged on a final on-disk state we keep.
	_ = os.RemoveAll(stashRoot)

	return &StashedPullResult{
		Pull:       pullRes,
		Stashed:    stashedPathList(entries),
		Restored:   restored,
		AutoMerged: autoMerged,
		Overridden: overridden,
	}, nil
}

// stashSnapshot captures pending contents to .changes.stash/ and records each pre-pull HEAD blob hash.
// The journal is conservative (cursor advances on sync, not commit), so pending may list paths whose
// disk already matches HEAD; stashing those would clutter and trip false-positive conflicts. We keep an
// entry only when on-disk content actually differs from its HEAD blob.
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

		oldHash := blobHashAt(headTree, clean)

		keep, content, err := shouldStash(r, repoRoot, clean, op, oldHash)
		if err != nil {
			return nil, fmt.Errorf("filter %q: %w", clean, err)
		}
		if !keep {
			continue
		}

		// Delete-ops have no content to snapshot; marker plus oldHash drives restore.
		if op == "delete" {
			entries = append(entries, StashEntry{
				Path: clean, Op: op, OldHash: oldHash,
			})
			continue
		}

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

// shouldStash decides whether a journal-pending entry actually needs snapshotting.
//
// Returns (keep, content, err):
//   - keep == false: stale entry, skip.
//   - keep == true, op=="delete": marker-only stash, content nil.
//   - keep == true, op in {create,update}: content holds the worktree bytes to stash.
//
// Decision matrix:
//
//	delete + file present → stale (file came back). Skip.
//	delete + file absent + oldHash != "" → real delete. Keep.
//	delete + file absent + oldHash == "" → nothing to delete. Skip.
//	create/update + file absent → nothing to capture. Skip.
//	create/update + oldHash == "" → brand-new file (not in HEAD). Keep.
//	create/update + disk == HEAD blob → stale write. Skip.
//	create/update + disk != HEAD blob → real edit. Keep.
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

	if !diskPresent {
		return false, nil, nil
	}
	if oldHash == "" {
		return true, disk, nil
	}
	blob, err := r.BlobObject(plumbing.NewHash(oldHash))
	if err != nil {
		// Couldn't resolve HEAD blob: be conservative and stash.
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

// stashReset clears each snapshotted path so the worktree is clean before pull.
// go-git v5 has no per-file checkout (wt.Checkout would clobber dirt outside the pending set),
// so we rewrite/remove each file by hand then wt.Add/Remove to refresh the index.
// Delete-ops restore the file from HEAD before pull, else a missing-vs-HEAD path blocks pull.
func (m *Manager) stashReset(r *gogit.Repository, wt *gogit.Worktree, entries []StashEntry) error {
	for _, e := range entries {
		full := filepath.Join(wt.Filesystem.Root(), e.Path)

		if e.OldHash == "" {
			// Brand-new file (no HEAD entry): drop it from index and worktree.
			if _, err := wt.Remove(e.Path); err != nil {
				if rmErr := os.Remove(full); rmErr != nil && !os.IsNotExist(rmErr) {
					return fmt.Errorf("clear new file %q: %w", e.Path, rmErr)
				}
			}
			continue
		}

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

// stashMergeOrOverride is phase 4 of PullWithStash, picking one outcome per stashed entry:
//
//   - Restored: pull didn't touch the path (HEAD blob hash unchanged); stashed content is written back.
//   - AutoMerged: pull touched a record file (storage/<tpl>/<n>.meta.json) and recmerge.Merge reconciled cleanly.
//   - Overridden: pull touched a non-record file or recmerge returned a RecordConflict; pull's content stays, authorship captured for the UI.
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
			if err := applyStashEntry(repoRoot, stashRoot, e); err != nil {
				return nil, nil, nil, err
			}
			restored = append(restored, e.Path)
			continue
		}

		// Pull touched the path: try a structured merge for record files, else pull wins.
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

		// Prefer the file's own author identity (transport-agnostic, so git and gigot match); fall back to git log.
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

// lookupAuthorFromTemplate reads author_name/author_email from a templates/*.yaml root, else a zero value.
func lookupAuthorFromTemplate(r *gogit.Repository, head *plumbing.Reference, _ string, path string) OverriddenPath {
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
	// Generic-map parse avoids dragging template.UnmarshalYAML's validate/normalize stack in.
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

// lookupAuthorFromRecord reads author identity from a record's post-pull HEAD meta envelope (the winning version).
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
	// meta.updated = {at, name, email} (last writer wins); legacy flat author_name/email is the fallback.
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

// applyStashEntry writes a stashed entry to the worktree: content for create/update, removal for delete.
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

// recordPathMergeable reports whether the entry is a record file tracked at HEAD (so base + theirs exist to merge).
func recordPathMergeable(e StashEntry) bool {
	if !recmerge.IsRecordPath(e.Path) {
		return false
	}
	if e.OldHash == "" {
		return false
	}
	return true
}

// tryRecordMerge runs recmerge.Merge with BASE=pre-pull HEAD blob, THEIRS=post-pull HEAD blob, YOURS=stashed content.
// Returns (merged, true, nil) on a clean merge; (nil, false, nil) on RecordConflict; (nil, false, err) on parse/transport errors.
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

// lookupAuthor returns author/email/time of the most recent commit touching path, falling back to HEAD's author.
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
		// Match a commit whose tree differs from its parent at path (root commit: any containment).
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

// stashRestoreOnFailure writes stashed content back without conflict checking, used when pull never ran or failed.
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

func stashedPathList(entries []StashEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Path)
	}
	return out
}

// atomicWriteBytes is a local tmp+fsync+rename copy. Restore writes go here (NOT system.SaveFile)
// so they bypass the journal: the snapshot's journal entry stays pending and rewriting the same content keeps that contract.
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

// Fetch updates the remote-tracking refs for the remote (default "origin") without touching the worktree.
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

// Discard throws away the local change to a single file:
//
//   - tracked + modified/deleted: restore worktree from HEAD blob, re-add to refresh the index.
//   - staged add or untracked (not in HEAD): remove from index and/or worktree.
//
// Path-traversal segments ("..") are rejected up-front. A missing file is not an error: the end state is already "discarded".
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

	// A nil headBlob means the file isn't tracked at HEAD (untracked or staged add): discard = remove.
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

	// wt.Remove handles staged-add; untracked files have no index entry, so fall back to os.Remove.
	if _, rmErr := wt.Remove(clean); rmErr != nil {
		if rmErr := os.Remove(fullPath); rmErr != nil && !os.IsNotExist(rmErr) {
			return fmt.Errorf("git: discard: remove: %w", rmErr)
		}
	}
	return nil
}
