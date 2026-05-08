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
