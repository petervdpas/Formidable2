// Package git owns the Git collaboration backend. Pure go-git by default (no system git needed);
// self-cloned mode shells out to system git so the user's credential helper handles auth.
package git

// Status is a JSON-friendly snapshot of a repository's working tree + HEAD position.
type Status struct {
	// Branch is the current local branch, empty when detached.
	Branch string `json:"branch"`
	// Tracking is the upstream ref (e.g. "refs/remotes/origin/main"), or "" if none.
	Tracking string `json:"tracking"`
	Detached bool   `json:"detached"`
	Clean    bool   `json:"clean"`
	Ahead    int    `json:"ahead"`
	// Behind reflects the last-known Tracking state; call Fetch first to refresh it.
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
	// Current is the active branch, "" when detached.
	Current string   `json:"current"`
	Locals  []string `json:"locals"`
}

// Commit is a JSON-friendly view of a git commit; Time is RFC3339 in the author's stored offset.
type Commit struct {
	Hash    string `json:"hash"`
	Short   string `json:"short"`
	Author  string `json:"author"`
	Email   string `json:"email"`
	Time    string `json:"time"`
	Subject string `json:"subject"`
}

// ChangeFile is one CommitChanges row; Status is a single-letter code A/M/D/R (R is exact-content rename only).
type ChangeFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

// GraphCommit is a Commit enriched with parent hashes (graph edges) and Refs (branch tips plus "HEAD") at this commit.
type GraphCommit struct {
	Hash    string   `json:"hash"`
	Short   string   `json:"short"`
	Author  string   `json:"author"`
	Email   string   `json:"email"`
	Time    string   `json:"time"`
	Subject string   `json:"subject"`
	Parents []string `json:"parents"`
	Refs    []string `json:"refs"`
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

// CloneOptions describes a clone request. URL and Dest are required; empty Branch uses remote HEAD.
// PAT is the HTTP Basic password (username "x-access-token"); empty means anonymous.
// PAT is transient: never persisted by the manager, discarded by the frontend once the response returns.
type CloneOptions struct {
	URL    string `json:"url"`
	Dest   string `json:"dest"`
	Branch string `json:"branch"`
	PAT    string `json:"pat"`
}

// CloneResult is the success envelope: worktree, HEAD commit, and branch (empty on a detached-HEAD clone).
type CloneResult struct {
	Dest   string `json:"dest"`
	Head   string `json:"head"`
	Branch string `json:"branch"`
}

// CommitOptions describes a commit request; it stages every worktree change before committing.
type CommitOptions struct {
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
	Author  string `json:"author"`
	Email   string `json:"email"`
}

// CommitResult is the success envelope: the new commit's full hash plus a 7-char short form.
type CommitResult struct {
	Hash  string `json:"hash"`
	Short string `json:"short"`
}

// FetchOptions describes a fetch request; empty Remote defaults to "origin", PAT is transient (see CloneOptions).
type FetchOptions struct {
	Path   string `json:"path,omitempty"`
	Remote string `json:"remote"`
	PAT    string `json:"pat"`
}

// FetchResult signals whether the remote-tracking refs moved.
type FetchResult struct {
	AlreadyUpToDate bool `json:"already_up_to_date"`
}

// PullOptions describes a pull (fetch + merge, no rebase); empty Remote defaults to "origin", PAT is transient.
type PullOptions struct {
	Path   string `json:"path,omitempty"`
	Remote string `json:"remote"`
	PAT    string `json:"pat"`
}

// PullResult: NewHead is the post-pull local HEAD hash, recorded as the journal cursor version.
type PullResult struct {
	AlreadyUpToDate bool   `json:"already_up_to_date"`
	NewHead         string `json:"new_head"`
}

// PushOptions describes a push of the current branch HEAD; empty Remote defaults to "origin", PAT is transient.
type PushOptions struct {
	Path   string `json:"path,omitempty"`
	Remote string `json:"remote"`
	PAT    string `json:"pat"`
}

// PushResult: NewHead is the local HEAD hash now on the remote, recorded as the post-sync cursor version.
type PushResult struct {
	AlreadyUpToDate bool   `json:"already_up_to_date"`
	NewHead         string `json:"new_head"`
}

// StashEntry is one path captured by PullWithStash before the worktree reset; Op drives the restore (write vs re-delete).
type StashEntry struct {
	Path     string `json:"path"`                // worktree-relative, posix slashes
	Op       string `json:"op"`                  // create | update | delete
	Bytes    int64  `json:"bytes"`               // size of stashed content (0 for delete)
	OldHash  string `json:"old_hash"`            // pre-pull HEAD blob hash, "" if absent
	StashRef string `json:"stash_ref,omitempty"` // path under .changes.stash/, "" for delete
}

// StashedPullResult is the outcome of PullWithStash: Restored re-applied cleanly, AutoMerged via recmerge,
// Overridden where pull won and the local change was dropped (with remote authorship captured).
//
// Policy: pull always wins on disk; auto-merge meta.json when recmerge reconciles, else drop the user's change.
// .changes.stash is always cleaned up; the Overridden list is the only signal that something was lost.
type StashedPullResult struct {
	Pull       *PullResult      `json:"pull"`
	Stashed    []string         `json:"stashed"`
	Restored   []string         `json:"restored"`
	AutoMerged []string         `json:"auto_merged"`
	Overridden []OverriddenPath `json:"overridden"`
}

// OverriddenPath names a path where the local change was dropped for pull's content, plus the remote commit's authorship.
type OverriddenPath struct {
	Path   string `json:"path"`
	Author string `json:"author"`
	Email  string `json:"email"`
	Time   string `json:"time"`
	Commit string `json:"commit"`
}

// StashPathPending is one journal-pending path, declared locally so the git signature avoids importing journal types.
type StashPathPending struct {
	Path string `json:"path"`
	Op   string `json:"op"`
}

// PullWithStashOptions extends PullOptions with the journal-derived stash manifest of dirty-since-sync paths.
type PullWithStashOptions struct {
	PullOptions
	Pending []StashPathPending `json:"pending"`
}

// DiscardOptions targets a single worktree file to discard its local change.
// File is worktree-relative; path-traversal segments ("..") are rejected.
type DiscardOptions struct {
	Path string `json:"path,omitempty"`
	File string `json:"file"`
}
