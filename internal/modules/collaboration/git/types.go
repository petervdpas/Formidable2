// Package git owns the Git collaboration backend. Pure go-git
// (github.com/go-git/go-git/v5) — no shell-out to a system git
// binary, so end users do not need git installed. Auth-bearing
// network ops (clone/pull/push) will arrive in a later iteration
// once credential storage is decided.
//
// Phase 1 (this file): read-only inspection — IsGitRepo, RepoRoot,
// Status, Branches, Log, RemoteInfo. Enough to render the
// Collaboration → Current Service overview against a real repo.
package git

// Status is a JSON-friendly snapshot of a repository's working tree
// + HEAD position. Mirrors the subset of `git status` that the
// Collaboration overview surfaces; richer info (per-file diff hunks
// etc.) lives in dedicated calls.
type Status struct {
	// Branch is the current local branch name (e.g. "main").
	// Empty when HEAD is detached.
	Branch string `json:"branch"`
	// Tracking is the configured upstream ref name (e.g.
	// "refs/remotes/origin/main"), or "" if none / detached.
	Tracking string `json:"tracking"`
	// Detached reports HEAD-not-on-a-branch.
	Detached bool `json:"detached"`
	// Clean is true when the worktree has no modifications,
	// untracked files, or staged changes.
	Clean bool `json:"clean"`
	// Ahead is the number of local commits on this branch that
	// aren't on Tracking. 0 when there's no tracking ref.
	Ahead int `json:"ahead"`
	// Behind is the number of remote commits on Tracking that
	// aren't on this branch. 0 when there's no tracking ref.
	// Reflects the last-known state of Tracking — call Fetch to
	// update the remote-tracking ref before reading this.
	Behind int `json:"behind"`

	Modified   []string `json:"modified"`
	Untracked  []string `json:"untracked"`
	Staged     []string `json:"staged"`
	Deleted    []string `json:"deleted"`
	Renamed    []string `json:"renamed"`
	Conflicted []string `json:"conflicted"`
}

// Branches summarizes local branches plus the active one.
type Branches struct {
	// Current is the active branch name; "" when detached.
	Current string   `json:"current"`
	Locals  []string `json:"locals"`
}

// Commit is a JSON-friendly view of a git commit. Time is RFC3339
// in the commit author's stored offset.
type Commit struct {
	Hash    string `json:"hash"`
	Short   string `json:"short"`
	Author  string `json:"author"`
	Email   string `json:"email"`
	Time    string `json:"time"`
	Subject string `json:"subject"`
}

// RemoteInfo wraps the configured remotes for the repo.
type RemoteInfo struct {
	Remotes []Remote `json:"remotes"`
}

// Remote is one configured remote with all its push/fetch URLs.
type Remote struct {
	Name string   `json:"name"`
	URLs []string `json:"urls"`
}

// CloneOptions describes a clone request. URL and Dest are required;
// Branch picks an initial checkout (empty = remote's default HEAD).
//
// PAT is the Personal Access Token used as the password in HTTP Basic
// auth (with username "x-access-token" — the GitHub-PAT convention,
// also accepted by Gitea/GitLab/Bitbucket as long as the username is
// non-empty). Empty PAT means anonymous (public repos / SSH).
//
// IMPORTANT: PAT is read-only at the call site and never persisted by
// the manager. The frontend keeps it transient — pasted into the
// clone form, sent over the Wails bridge once, and discarded as soon
// as the response returns. SSH-based auth lives in a follow-up.
type CloneOptions struct {
	URL    string `json:"url"`
	Dest   string `json:"dest"`
	Branch string `json:"branch"`
	PAT    string `json:"pat"`
}

// CloneResult is the success envelope: the worktree we cloned into,
// the commit HEAD now points at, and the branch HEAD now sits on.
// Branch is empty when the clone produced a detached HEAD (rare —
// happens when the requested ref isn't a branch). The frontend uses
// Dest to flip git_root and Branch to flip git_branch once a clone
// completes, so Current Service reflects what was actually fetched.
type CloneResult struct {
	Dest   string `json:"dest"`
	Head   string `json:"head"`
	Branch string `json:"branch"`
}

// CommitOptions describes a commit request. Path is any path inside
// the worktree; Author/Email come from the active profile's config.
//
// v1 stages every change in the worktree (modified, untracked,
// deleted) before committing — matching the "commit everything I
// touched in this session" mental model. Per-file selection arrives
// in a later iteration once the UI grows checkboxes.
type CommitOptions struct {
	Path    string `json:"path"`
	Message string `json:"message"`
	Author  string `json:"author"`
	Email   string `json:"email"`
}

// CommitResult is the success envelope for a commit: the new commit's
// full hash plus a 7-char short form for display.
type CommitResult struct {
	Hash  string `json:"hash"`
	Short string `json:"short"`
}

// FetchOptions describes a fetch request. Path is any path inside
// the worktree; Remote defaults to "origin" when empty. PAT is the
// HTTP Basic password (transient — never persisted by the manager,
// same convention as Clone).
type FetchOptions struct {
	Path   string `json:"path"`
	Remote string `json:"remote"`
	PAT    string `json:"pat"`
}

// FetchResult signals whether the remote-tracking refs actually
// moved. AlreadyUpToDate=true means there was nothing new to
// pull; the UI can collapse this to "you're current."
type FetchResult struct {
	AlreadyUpToDate bool `json:"already_up_to_date"`
}

// PullOptions describes a pull request — a fetch followed by a
// merge of the tracking ref into the current branch. Default merge
// strategy (no rebase). Path is any path inside the worktree;
// Remote defaults to "origin"; PAT is the HTTPS Basic password
// (transient, same as Clone).
type PullOptions struct {
	Path   string `json:"path"`
	Remote string `json:"remote"`
	PAT    string `json:"pat"`
}

// PullResult mirrors PushResult / FetchResult: AlreadyUpToDate=true
// means there were no new commits to merge.
type PullResult struct {
	AlreadyUpToDate bool `json:"already_up_to_date"`
}

// PushOptions describes a push request. The current branch's HEAD
// is pushed to the matching ref on Remote (default "origin"); we
// don't expose explicit refspecs in v1.
type PushOptions struct {
	Path   string `json:"path"`
	Remote string `json:"remote"`
	PAT    string `json:"pat"`
}

// PushResult signals whether the push actually advanced the remote.
// AlreadyUpToDate=true means the remote already had every commit;
// the UI surfaces this as info, not an error.
type PushResult struct {
	AlreadyUpToDate bool `json:"already_up_to_date"`
}

// DiscardOptions targets a single worktree file for "throw away the
// local change to this file." The semantics depend on the file's
// current status:
//   - tracked + modified  → worktree restored from HEAD's blob
//   - tracked + deleted   → file recreated from HEAD's blob
//   - staged add (no HEAD blob) → unstaged + removed from worktree
//   - untracked           → removed from worktree
//
// Path is any path inside the worktree; File is the worktree-relative
// path of the file to discard. Path-traversal segments ("..") are
// rejected so the frontend can pass values straight from Status()
// without re-validating.
type DiscardOptions struct {
	Path string `json:"path"`
	File string `json:"file"`
}
