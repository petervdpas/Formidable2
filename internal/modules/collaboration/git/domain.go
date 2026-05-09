package git

import (
	"errors"
	"fmt"
	"io"
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

// Commit stages every change in the worktree (modified, untracked,
// deleted) and creates a commit on the current branch with the given
// author/email/message.
//
// Refuses on:
//   - empty message / empty author / empty email — these are UI bugs
//     and silently no-op'ing leaves the user wondering why nothing
//     happened.
//   - a clean worktree — go-git would happily make an empty commit
//     otherwise; explicit error lets the UI greylist the button.
//   - detached HEAD — committing detached needs explicit branch
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

	// Detached-HEAD guard. Empty repo (no HEAD yet) is allowed — the
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
// symmetric difference — costs O(history) memory in the worst case,
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
// was nothing to send — that's an info-level outcome, not an error,
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
// AlreadyUpToDate=true mirrors Push/Fetch — info, not error.
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
// gone, raced with a manual delete) are not an error — the desired
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
	// means "this file isn't tracked at HEAD" — either untracked or
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

	// File not in HEAD — drop it. wt.Remove handles the staged-add
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
