// Package journal is Formidable's append-only change journal. It records mutations under a context
// folder's templates/ and storage/ trees plus per-backend sync markers, and tracks the pending-changes
// set in memory so reads are O(1). Every recorded mutation emits "journal:changed".
package journal

const (
	OpCreate   = "create"
	OpUpdate   = "update"
	OpDelete   = "delete"
	OpBaseline = "baseline"
	OpSync     = "sync"

	BackendGit   = "git"
	BackendGigot = "gigot"
	BackendNone  = "none"

	logFileName    = ".changes.log"
	cursorFileName = ".changes.cursor"

	// stashDirName is PullWithStash's transient dir; swept here so all .changes.* concerns live in one module.
	stashDirName = ".changes.stash"

	templatesDir = "templates"
	storageDir   = "storage"

	// findGitMaxDepth caps the upward .git walk so detached mounts can't walk to the FS root forever.
	findGitMaxDepth = 10
)

// gitignorePatterns exclude the journal's own .changes.* files; narrow on purpose so arbitrary user *.log files survive.
var gitignorePatterns = []string{
	".changes.*",
	"**/.changes.*",
}

// knownBackends validates sync entries from disk.
var knownBackends = map[string]bool{
	BackendGit:   true,
	BackendGigot: true,
}

// orderedSyncBackends is the canonical display order, exposed via Service.ListBackends; keep in sync with knownBackends.
var orderedSyncBackends = []string{
	BackendGit,
	BackendGigot,
}

// Entry is one journal record (sync entries fill Backend/Version/...; file-op entries fill Path/Bytes). JSONL on disk.
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

// Cursor is the per-backend sync watermark: Ts marks "everything older has synced"; Version is the remote id at that ts.
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

// PendingResult is the shape Pending() returns; Count always equals len(Paths).
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

// Recorder is the surface sync backends (git, gigot) call after a Push or Pull.
type Recorder interface {
	RecordSync(backend, version string, pushed, pulled int)
	RecordRemoteSeen(backend, version string)
}

// Reader is the journal's read-only state surface.
type Reader interface {
	Pending(backend string) PendingResult
}

// Journal combines Recorder + Reader for callers that need both.
type Journal interface {
	Recorder
	Reader
}

const (
	EventChanged = "journal:changed"
)
