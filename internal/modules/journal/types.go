// Package journal is Formidable's append-only change journal. It
// records mutations under a context folder's templates/ and storage/
// trees plus per-backend (git, gigot) sync markers, and tracks the
// "pending changes" set in memory so reads are O(1).
//
// Lifts beyond the JS source:
//   - In-memory rolling pending state per backend (no full-log scan
//     on every Pending() call).
//   - Wails events: every recorded mutation emits "journal:changed"
//     so frontend pollers can subscribe instead of polling.
//   - Strongly typed Entry struct (vs. duck-typed JSON in JS).
//
// Wails-only: the journal is too internal for the loopback HTTP API.
package journal

const (
	// Op codes mirror the JS schema/changes.schema.js.
	OpCreate   = "create"
	OpUpdate   = "update"
	OpDelete   = "delete"
	OpBaseline = "baseline"
	OpSync     = "sync"

	// Known sync backends.
	BackendGit   = "git"
	BackendGigot = "gigot"
	BackendNone  = "none"

	// Filenames inside the context folder.
	logFileName    = ".changes.log"
	cursorFileName = ".changes.cursor"

	// Tracked top-level dirs under the context folder.
	templatesDir = "templates"
	storageDir   = "storage"
)

// knownBackends — used to validate sync entries from disk.
var knownBackends = map[string]bool{
	BackendGit:   true,
	BackendGigot: true,
}

// Entry is one journal record. Sync entries fill Backend/Version/...;
// file-op entries fill Path/Bytes. Encoded as JSONL on disk.
type Entry struct {
	Ts      string `json:"ts"`
	Op      string `json:"op"`
	Path    string `json:"path,omitempty"`
	Bytes   int64  `json:"bytes,omitempty"`
	Backend string `json:"backend,omitempty"`
	Version string `json:"version,omitempty"`
	Pushed  int    `json:"pushed,omitempty"`
	Pulled  int    `json:"pulled,omitempty"`
}

// Cursor is the per-backend sync watermark.
// Ts marks "everything older has reached this backend"; Version is the
// remote-side identifier (git commit hash, gigot version) at that ts.
type Cursor struct {
	Ts      string `json:"ts"`
	Version string `json:"version"`
}

// CursorMap is keyed by backend ("git", "gigot").
type CursorMap = map[string]Cursor

// PendingChange is one entry in PendingResult.Paths.
type PendingChange struct {
	Path string `json:"path"`
	Op   string `json:"op"`
}

// PendingResult is the shape returned by Pending() and exposed to the
// frontend. Count is always equal to len(Paths) — kept as a convenience
// so JS callers don't need to call paths.length.
type PendingResult struct {
	Count int             `json:"count"`
	Paths []PendingChange `json:"paths"`
}

// InitResult is the outcome of seeding the baseline.
type InitResult struct {
	Created bool   `json:"created"`
	Entries int    `json:"entries"`
	Reason  string `json:"reason"`
}

// SyncRecord is the input to RecordSync.
type SyncRecord struct {
	Backend string
	Version string
	Pushed  int
	Pulled  int
}

// EventEmitter is the interface the journal uses to publish change
// events. The composition root (internal/app) wires a Wails-backed
// implementation; tests inject a stub. Nil is allowed and silences emit.
type EventEmitter interface {
	Emit(name string, data any)
}

// Wails event names emitted by the journal.
const (
	EventChanged = "journal:changed"
)
