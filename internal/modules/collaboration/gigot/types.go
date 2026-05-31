// Package gigot owns the GiGot remote-sync backend, a sibling of the git package that speaks JSON-over-HTTP
// to a GiGot server, authenticated by a long-lived subscription bearer.
//
// The subscription bearer is NOT a git HTTPS PAT (different secret, different lifecycle; see the credential-separation memory).
// This is a client, not a server: the scaffold marker (.formidable/context.json) is written by GiGot at repo creation.
package gigot

// Connection is the per-call addressing + auth bundle for any HTTP op; Token is the GiGot subscription bearer.
// A populated Author becomes the git author on commits, so the audit trail shows the team member, not the service account.
type Connection struct {
	BaseURL  string
	Token    string
	RepoName string
	Author   *Author
}

// Author identifies who made a commit; empty fields are dropped at encode time to pass server validation.
type Author struct {
	Name  string
	Email string
}

// HealthResponse is the parsed body of GET /api/health.
type HealthResponse struct {
	OK      bool   `json:"ok"`
	Version string `json:"version,omitempty"`
}

// MeResponse is the parsed body of GET /api/me.
type MeResponse struct {
	User         User         `json:"user"`
	Subscription Subscription `json:"subscription"`
}

// User identifies the gigot-side account a token belongs to.
type User struct {
	Username string `json:"username"`
	Provider string `json:"provider,omitempty"`
	Role     string `json:"role,omitempty"`
}

// Subscription is the per-token capability bundle, mirroring gigot's TokenEntry.
type Subscription struct {
	Repo      string   `json:"repo"`
	Abilities []string `json:"abilities,omitempty"`
}

// RepoContextResponse is the parsed body of GET /api/repos/{repo}/context.
type RepoContextResponse struct {
	User         User         `json:"user"`
	Subscription Subscription `json:"subscription"`
	Repo         RepoContext  `json:"repo"`
}

// RepoContext is the repo-side half of /context; IsFormidable is set when the server finds a .formidable/context.json marker.
type RepoContext struct {
	Head             string `json:"head,omitempty"`
	DefaultBranch    string `json:"default_branch,omitempty"`
	Empty            bool   `json:"empty"`
	IsFormidable     bool   `json:"is_formidable"`
	DestinationCount int    `json:"destination_count"`
}

// RepoFormidableResponse mirrors gigot's server type at internal/server/handler_repo_formidable.go; align field-for-field.
type RepoFormidableResponse struct {
	MarkerPresent bool                  `json:"marker_present"`
	Marker        *FormidableMarkerView `json:"marker,omitempty"`
	Templates     []FormidableTemplate  `json:"templates"`
	Storage       []FormidableStorage   `json:"storage"`
}

// FormidableMarkerView mirrors gigot's marker payload, letting the client detect scaffold-version mismatches.
type FormidableMarkerView struct {
	Version      int    `json:"version"`
	ScaffoldedBy string `json:"scaffolded_by,omitempty"`
	ScaffoldedAt string `json:"scaffolded_at,omitempty"`
}

// FormidableTemplate is one templates/ entry at HEAD; Path is repo-relative for /files/{path} fetches.
type FormidableTemplate struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// FormidableStorage is one storage/ template dir; Files counts .meta.json records only (images/ excluded).
type FormidableStorage struct {
	Template string `json:"template"`
	Files    int    `json:"files"`
}

// HeadResponse is the parsed body of GET /api/repos/{repo}/head; Version is the parent_version for the next commit.
type HeadResponse struct {
	Version       string `json:"version"`
	DefaultBranch string `json:"default_branch,omitempty"`
}

// TreeResponse is the parsed body of GET /api/repos/{repo}/tree: a recursive snapshot at Version (paths + SHAs, no content).
type TreeResponse struct {
	Version string      `json:"version"`
	Files   []TreeEntry `json:"files"`
}

// TreeEntry is one blob in the tree response; Blob is the git blob SHA1 matching GitBlobSha.
type TreeEntry struct {
	Path string `json:"path"`
	Blob string `json:"blob"`
	Size int64  `json:"size,omitempty"`
}

// FileResponse is the parsed body of GET /api/repos/{repo}/files/{path}; ContentB64 is the base64-std raw blob.
type FileResponse struct {
	Path       string `json:"path"`
	ContentB64 string `json:"content_b64"`
	Blob       string `json:"blob,omitempty"`
	Size       int64  `json:"size,omitempty"`
}

// LogEntry is one RepoLogResponse row; Changes is populated only when Log is called with withChanges=true.
type LogEntry struct {
	Hash    string       `json:"hash"`
	Parents []string     `json:"parents,omitempty"`
	Refs    []string     `json:"refs,omitempty"`
	Author  string       `json:"author"`
	Email   string       `json:"email,omitempty"`
	Date    string       `json:"date"`
	Message string       `json:"message"`
	Changes []ChangeFile `json:"changes,omitempty"`
}

// ChangeFile is one per-path entry in a commit's changes list; Status is a single-letter code A/M/D/R.
type ChangeFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

// RepoLogResponse is the wrapped body of GET /api/repos/{repo}/log; the commit trail is in Entries.
type RepoLogResponse struct {
	Name    string     `json:"name"`
	Entries []LogEntry `json:"entries"`
	Count   int        `json:"count"`
}

// CommitRequest is the body of POST /api/repos/{repo}/commits. ParentVersion is the base our changes were computed against
// (the ledger version), NOT the live HEAD: the server fast-forwards when it equals HEAD, 3-way merges when it is an ancestor,
// and returns 409 when it cannot reconcile.
type CommitRequest struct {
	ParentVersion string   `json:"parent_version"`
	Changes       []Change `json:"changes"`
	Message       string   `json:"message"`
	Author        *Author  `json:"author,omitempty"`
}

// Change is one put/delete op; ContentB64 is required for put, and Blob is set on the response side after the server hashes post-merge content.
type Change struct {
	Op         string `json:"op"`
	Path       string `json:"path"`
	ContentB64 string `json:"content_b64,omitempty"`
	Blob       string `json:"blob,omitempty"`
}

// CommitResponse: Version is the new HEAD; Changes echoes the post-merge content the server wrote (differs from what we sent on F1 auto-merge).
type CommitResponse struct {
	Version string   `json:"version"`
	Changes []Change `json:"changes,omitempty"`
}

// Destination is one mirror-sync target; RemoteStatus is the server's mirror view: "in_sync", "diverged", "error", or empty (never checked / invalidated).
type Destination struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	URL            string `json:"url"`
	Auto           bool   `json:"auto"`
	LastSyncStatus string `json:"last_sync_status,omitempty"`
	LastSyncAt     string `json:"last_sync_at,omitempty"`
	LastSyncError  string `json:"last_sync_error,omitempty"`
	RemoteStatus   string `json:"remote_status,omitempty"`
}

// TrackRecord is the client-side ledger at <context>/.formidable/sync.json: the last agreed server version plus each managed file's blob SHA.
// Each team member keeps their own copy; deliberately not shared.
type TrackRecord struct {
	Version  string            `json:"version,omitempty"`
	LastSync string            `json:"lastSync,omitempty"`
	Files    map[string]string `json:"files"`
}

// LocalFile is one context-folder walker entry: repo-relative path, on-disk bytes, and precomputed git blob SHA.
type LocalFile struct {
	Path  string
	Bytes []byte
	Sha   string
}

// PushResult is PushLocal's success envelope; Noop means no bytes moved.
type PushResult struct {
	Version string `json:"version"`
	Pushed  int    `json:"pushed"`
	Deleted int    `json:"deleted"`
	Scanned int    `json:"scanned"`
	Noop    bool   `json:"noop"`
	// Conflicts is non-empty when the server refused the commit because our base
	// could not be reconciled with HEAD: nothing was pushed and the ledger is
	// untouched. Surfaced to the user instead of clobbering or erroring opaquely.
	Conflicts []PathConflict `json:"conflicts,omitempty"`
}

// ConflictFieldValue carries both candidate values of one conflicting field so the resolver UI can show
// "yours" vs "theirs" side by side. Values are raw JSON strings (the field is atomic).
type ConflictFieldValue struct {
	Path   string `json:"path"`
	Scope  string `json:"scope"`
	Key    string `json:"key"`
	Yours  string `json:"yours"`
	Theirs string `json:"theirs"`
}

// FieldResolution is the user's pick for one conflicting field; Side is "mine" or "theirs".
type FieldResolution struct {
	Path  string `json:"path"`
	Scope string `json:"scope"`
	Key   string `json:"key"`
	Side  string `json:"side"`
}

// FieldConflict is one per-field conflict the server reported for a record path (e.g. an immutable meta field).
type FieldConflict struct {
	Scope  string `json:"scope"`
	Key    string `json:"key"`
	Reason string `json:"reason,omitempty"`
}

// PathConflict is one path the server refused to merge in a rejected commit.
// Fields is populated for record (meta.json) conflicts, empty for generic ones.
type PathConflict struct {
	Path   string          `json:"path"`
	Fields []FieldConflict `json:"field_conflicts,omitempty"`
}

// PullResult is PullLocal's success envelope: tree version, files written, files removed.
type PullResult struct {
	Version string `json:"version"`
	Files   int    `json:"files"`
	Deleted int    `json:"deleted"`
}

// SyncPhase enumerates the named steps PullLocal / Reclone emit, in order.
type SyncPhase string

const (
	// PhaseStart fires once at entry, before any HTTP (Total is 0, not yet known).
	PhaseStart SyncPhase = "start"
	// PhaseWipe fires before Reclone's local wipe; plain PullLocal never emits it.
	PhaseWipe SyncPhase = "wipe"
	// PhaseTree fires after /tree returns; Total is pending deletes + managed entries.
	PhaseTree SyncPhase = "tree"
	// PhaseDelete fires once per locally-deleted path.
	PhaseDelete SyncPhase = "delete"
	// PhaseFetch fires once per managed entry inspected, even when a SHA-match short-circuits the download.
	PhaseFetch SyncPhase = "fetch"
	// PhaseDone fires once at completion (Current==Total).
	PhaseDone SyncPhase = "done"
)

// SyncProgress is the payload for gigot:sync_progress events; Path is set only on per-file phases (delete/fetch).
type SyncProgress struct {
	Phase   SyncPhase `json:"phase"`
	Current int       `json:"current"`
	Total   int       `json:"total"`
	Path    string    `json:"path,omitempty"`
}

// ProgressFunc is the progress callback; the Manager calls it inline between HTTP requests, so it must NOT block.
type ProgressFunc func(SyncProgress)

// EventSyncProgress is the Wails event name the Service emits on progress.
const EventSyncProgress = "gigot:sync_progress"

// LedgerSummary is a purely-local snapshot of the ledger + on-disk diff (no HTTP) for the Sync UI's pending-state hints.
type LedgerSummary struct {
	Version  string   `json:"version"`
	LastSync string   `json:"lastSync"`
	Changed  []string `json:"changed"`
	Deleted  []string `json:"deleted"`
	Scanned  int      `json:"scanned"`
}

// SyncResult is Sync's combined push+pull outcome; Noop is true only when both halves were quiet.
type SyncResult struct {
	Version       string `json:"version"`
	Pushed        int    `json:"pushed"`
	PushedDeleted int    `json:"pushedDeleted"`
	Pulled        int    `json:"pulled"`
	PulledDeleted int    `json:"pulledDeleted"`
	Noop          bool   `json:"noop"`
}
