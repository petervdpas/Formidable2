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

// CloneResult is the success envelope: the worktree we cloned into
// and the commit HEAD now points at. The frontend uses Dest to flip
// git_root once a clone completes.
type CloneResult struct {
	Dest string `json:"dest"`
	Head string `json:"head"`
}
