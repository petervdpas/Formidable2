// Package gigot owns the GiGot remote-sync backend. Sibling of the
// git package: where git speaks git-protocol over HTTPS/SSH, gigot
// speaks a JSON-over-HTTP API to a GiGot server, authenticated by a
// long-lived subscription bearer issued at subscription time.
//
// What this package is responsible for:
//   - Talking to one GiGot server via its REST surface (health/me/
//     context/formidable/head/tree/files/log/commits/destinations).
//   - Walking the active context folder and diffing it against the
//     client-side track-record at .formidable/sync.json before push.
//   - Pulling templates/ + storage/ + the allowlisted root files
//     from the server's HEAD and refreshing the track-record after.
//   - Notifying the journal (RecordSync / RecordRemoteSeen) when bytes
//     actually moved.
//
// What this package is NOT:
//   - Not the git backend. The subscription bearer here is not a git
//     HTTPS PAT; see the feedback memory on credential separation.
//   - Not a server. The scaffold marker (.formidable/context.json)
//     is written by GiGot during repo creation, not by this client.
package gigot

// Connection is the per-call addressing + auth bundle the Manager
// uses for any HTTP op. BaseURL is the gigot server's origin
// ("https://gigot.example"); RepoName is the per-server repo handle;
// Token is the GiGot subscription bearer. Author is optional — when
// populated, gigot uses it as the git author on commits so the audit
// trail shows the real team member rather than the subscription's
// service account.
type Connection struct {
	BaseURL  string
	Token    string
	RepoName string
	Author   *Author
}

// Author identifies who made a commit. Empty fields are dropped at
// request-encode time so partial Author values don't fail server-side
// validation.
type Author struct {
	Name  string
	Email string
}

// HealthResponse is the parsed body of GET /api/health. Optional —
// vanilla gigot returns it for liveness checks; gated deployments may
// require auth even for /health.
type HealthResponse struct {
	OK      bool   `json:"ok"`
	Version string `json:"version,omitempty"`
}

// MeResponse is the parsed body of GET /api/me. Carries the caller's
// account plus the single subscription their token represents. Used
// by account-picker flows that don't yet know which repo to target.
type MeResponse struct {
	User         User         `json:"user"`
	Subscription Subscription `json:"subscription"`
}

// User identifies the gigot-side account a token belongs to. Provider
// is the OAuth issuer ("github", "google", ...) when present.
type User struct {
	Username string `json:"username"`
	Provider string `json:"provider,omitempty"`
	Role     string `json:"role,omitempty"`
}

// Subscription is the per-token capability bundle: which repo, what
// abilities. Mirrors gigot's TokenEntry shape server-side.
type Subscription struct {
	Repo      string   `json:"repo"`
	Abilities []string `json:"abilities,omitempty"`
}

// RepoContextResponse is the parsed body of GET /api/repos/{repo}/context.
// Single-read bootstrap: who am I, what can I do here, what does this
// repo offer. Renders the connect modal off this response.
type RepoContextResponse struct {
	User         User         `json:"user"`
	Subscription Subscription `json:"subscription"`
	Repo         RepoContext  `json:"repo"`
}

// RepoContext is the repo-side half of /context: HEAD version + default
// branch, the empty-repo flag, the is_formidable hint (set when the
// server detects a .formidable/context.json marker), and a count of
// mirror destinations so the UI can render a badge without a second call.
type RepoContext struct {
	Head             string `json:"head,omitempty"`
	DefaultBranch    string `json:"default_branch,omitempty"`
	Empty            bool   `json:"empty"`
	IsFormidable     bool   `json:"is_formidable"`
	DestinationCount int    `json:"destination_count"`
}

// RepoFormidableResponse mirrors gigot's same-named type at
// internal/server/handler_repo_formidable.go. Client-side copy so the
// gigot module doesn't import server code; align field-for-field.
type RepoFormidableResponse struct {
	MarkerPresent bool                  `json:"marker_present"`
	Marker        *FormidableMarkerView `json:"marker,omitempty"`
	Templates     []FormidableTemplate  `json:"templates"`
	Storage       []FormidableStorage   `json:"storage"`
}

// FormidableMarkerView mirrors gigot's marker payload. Lets the
// client detect scaffold-version mismatches without re-parsing
// .formidable/context.json.
type FormidableMarkerView struct {
	Version      int    `json:"version"`
	ScaffoldedBy string `json:"scaffolded_by,omitempty"`
	ScaffoldedAt string `json:"scaffolded_at,omitempty"`
}

// FormidableTemplate is one entry under templates/ at HEAD. Path is
// repo-relative so the client can fetch via /files/{path} without
// re-deriving it.
type FormidableTemplate struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// FormidableStorage is one template-directory under storage/ that
// holds at least one record. Files counts .meta.json records only —
// images/ and other non-record paths are excluded.
type FormidableStorage struct {
	Template string `json:"template"`
	Files    int    `json:"files"`
}

// HeadResponse is the parsed body of GET /api/repos/{repo}/head.
// Version is the HEAD commit hash, used as parent_version for the
// next commit.
type HeadResponse struct {
	Version       string `json:"version"`
	DefaultBranch string `json:"default_branch,omitempty"`
}

// TreeResponse is the parsed body of GET /api/repos/{repo}/tree. A
// recursive snapshot at Version — every blob's path + git SHA1, no
// content. Sized so the client can warn before pulling very large blobs.
type TreeResponse struct {
	Version string      `json:"version"`
	Files   []TreeEntry `json:"files"`
}

// TreeEntry is one blob in the tree response. Blob is the git blob
// SHA1 (matches the value gitBlobSha computes locally); Size is the
// blob's byte length when the server reports it.
type TreeEntry struct {
	Path string `json:"path"`
	Blob string `json:"blob"`
	Size int64  `json:"size,omitempty"`
}

// FileResponse is the parsed body of GET /api/repos/{repo}/files/{path}.
// ContentB64 is the raw blob, base64-standard encoded.
type FileResponse struct {
	Path       string `json:"path"`
	ContentB64 string `json:"content_b64"`
	Blob       string `json:"blob,omitempty"`
	Size       int64  `json:"size,omitempty"`
}

// LogEntry is one row in a RepoLogResponse. Date is RFC3339 in the
// commit author's stored offset. Parents and Refs are populated
// unconditionally on the server side so graph-style UIs can render
// branch pills + parent edges without a second request. Changes is
// populated only when Log is called with withChanges=true (the server
// adds one extra diff-tree call per commit) — omitted from JSON when
// nil so the lean shape stays cheap for graph-only callers.
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

// ChangeFile is one per-path entry in a commit's changes list. Status
// uses git's standard single-letter codes: A=added, M=modified,
// D=deleted, R=renamed.
type ChangeFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

// RepoLogResponse is the wrapped body returned by
// GET /api/repos/{repo}/log. Name + Count are envelope metadata; the
// commit trail itself is in Entries.
type RepoLogResponse struct {
	Name    string     `json:"name"`
	Entries []LogEntry `json:"entries"`
	Count   int        `json:"count"`
}

// CommitRequest is the body of POST /api/repos/{repo}/commits.
// ParentVersion must equal current HEAD or the server returns 409.
// Changes is a non-empty list of put/delete ops. Author, when set,
// overrides the subscription's default identity in the resulting
// commit.
type CommitRequest struct {
	ParentVersion string   `json:"parent_version"`
	Changes       []Change `json:"changes"`
	Message       string   `json:"message"`
	Author        *Author  `json:"author,omitempty"`
}

// Change is one op in a CommitRequest (and one row in a CommitResponse's
// echoed changes list). Op is "put" or "delete"; ContentB64 is required
// for put and omitted for delete; Blob is set on the response side after
// the server hashes the post-merge content.
type Change struct {
	Op         string `json:"op"`
	Path       string `json:"path"`
	ContentB64 string `json:"content_b64,omitempty"`
	Blob       string `json:"blob,omitempty"`
}

// CommitResponse is the body returned by POST /api/repos/{repo}/commits.
// Version is the new HEAD; Changes (when present) echoes the
// post-merge content the server actually wrote — important on F1
// auto-merge where server-resolved bytes differ from what we sent.
type CommitResponse struct {
	Version string   `json:"version"`
	Changes []Change `json:"changes,omitempty"`
}

// Destination is one mirror-sync target attached to a repo. Status
// fields carry the last attempt's outcome so the UI renders a badge
// without polling each destination.
type Destination struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	URL            string `json:"url"`
	Auto           bool   `json:"auto"`
	LastSyncStatus string `json:"last_sync_status,omitempty"`
	LastSyncAt     string `json:"last_sync_at,omitempty"`
	LastSyncError  string `json:"last_sync_error,omitempty"`
}

// TrackRecord is the client-side ledger persisted at
// <context>/.formidable/sync.json. It records the last server version
// the client agreed with plus a snapshot of "what was each managed
// file's git blob SHA1 at that moment." Lets pushLocal diff local
// content against the ledger without re-fetching /tree on every sync,
// and survives Formidable restarts. Each team member has their own
// local copy — deliberately not shared.
type TrackRecord struct {
	Version  string            `json:"version,omitempty"`
	LastSync string            `json:"lastSync,omitempty"`
	Files    map[string]string `json:"files"`
}

// LocalFile is one entry produced by the context-folder walker:
// repo-relative path plus the on-disk bytes and the git blob SHA1 of
// those bytes. Sha is precomputed so callers diff against the ledger
// without re-hashing.
type LocalFile struct {
	Path  string
	Bytes []byte
	Sha   string
}

// PushResult is what PushLocal returns on success: the post-commit
// version, how many files were put/deleted, total scanned, and a
// "no bytes moved" flag so the UI can collapse the result to a quiet
// "already in sync" toast.
type PushResult struct {
	Version string `json:"version"`
	Pushed  int    `json:"pushed"`
	Deleted int    `json:"deleted"`
	Scanned int    `json:"scanned"`
	Noop    bool   `json:"noop"`
}

// PullResult is what PullLocal returns on success: the tree's version,
// how many files were written, and how many were removed because they
// vanished from the server.
type PullResult struct {
	Version string `json:"version"`
	Files   int    `json:"files"`
	Deleted int    `json:"deleted"`
}

// SyncPhase enumerates the named steps PullLocal / Reclone walk
// through, in the order a progress consumer can expect to see them.
// String-valued so the Wails event payload stays JSON-friendly.
type SyncPhase string

const (
	// PhaseStart fires once at the very entry of a sync op, before
	// any HTTP. Total is 0 — the count isn't known yet. Useful for
	// the UI to flip an indeterminate spinner on before /tree returns.
	PhaseStart SyncPhase = "start"
	// PhaseWipe fires once before the local-wipe step of Reclone.
	// Plain PullLocal never emits this.
	PhaseWipe SyncPhase = "wipe"
	// PhaseTree fires once after /tree has returned and the work plan
	// is computed. Total is the sum of pending deletes + managed
	// entries to inspect/fetch — the count Current ramps toward.
	PhaseTree SyncPhase = "tree"
	// PhaseDelete fires once per locally-deleted path. Current is the
	// running count across delete+fetch events.
	PhaseDelete SyncPhase = "delete"
	// PhaseFetch fires once per managed tree entry inspected,
	// regardless of whether bytes were actually downloaded — SHA-match
	// short-circuits still count so the progress bar advances.
	PhaseFetch SyncPhase = "fetch"
	// PhaseDone fires once at completion. Current==Total. Consumers
	// hide their progress UI on this signal rather than racing the
	// Manager's return.
	PhaseDone SyncPhase = "done"
)

// SyncProgress is the payload carried by gigot:sync_progress events
// and by direct ProgressFunc callbacks. Path is non-empty only for
// per-file phases (delete / fetch).
type SyncProgress struct {
	Phase   SyncPhase `json:"phase"`
	Current int       `json:"current"`
	Total   int       `json:"total"`
	Path    string    `json:"path,omitempty"`
}

// ProgressFunc is the per-call callback shape Manager.PullLocalWithProgress
// and Manager.RecloneWithProgress accept. Implementations should treat
// the function as cheap-on-call but must NOT block — the Manager calls
// it inline between HTTP requests.
type ProgressFunc func(SyncProgress)

// EventSyncProgress is the Wails event name the Service emits when a
// progress callback fires. Registered in main.go alongside the other
// typed events so the frontend gets a typed subscription signature.
const EventSyncProgress = "gigot:sync_progress"

// SyncResult is what Sync returns on success — the combined push+pull
// outcome. Noop is true only when both halves were quiet.
type SyncResult struct {
	Version       string `json:"version"`
	Pushed        int    `json:"pushed"`
	PushedDeleted int    `json:"pushedDeleted"`
	Pulled        int    `json:"pulled"`
	PulledDeleted int    `json:"pulledDeleted"`
	Noop          bool   `json:"noop"`
}
