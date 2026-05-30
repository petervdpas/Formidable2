package journal

import (
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// fs is the filesystem surface the journal needs.
type fs interface {
	ResolvePath(segments ...string) string
	EnsureDirectory(path string) error
	FileExists(path string) bool
	LoadFile(path string) (string, error)
	SaveFile(path string, content string) error
	AppendFile(path string, content string) error
	ListFiles(dir string) ([]string, error)
	WalkFiles(dir string) ([]string, error)
}

// Manager owns the in-memory cursor + per-backend pending state and
// the on-disk log/cursor files. Methods are safe for concurrent use.
type Manager struct {
	fs      fs
	log     *slog.Logger
	emitter EventEmitter

	mu            sync.RWMutex
	contextFolder string
	backend       string
	cursors       CursorMap
	// pending[backend][path] = latest op for that path since cursor[backend].Ts
	pending map[string]map[string]string
	nowFn   func() time.Time
}

// NewManager constructs a journal. emitter may be nil (events silenced).
// log may be nil (uses slog.Default).
func NewManager(filesystem fs, log *slog.Logger, emitter EventEmitter) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{
		fs:      filesystem,
		log:     log,
		emitter: emitter,
		cursors: CursorMap{},
		pending: map[string]map[string]string{},
		nowFn:   time.Now,
	}
}

// SetNowFn injects a clock for tests.
func (m *Manager) SetNowFn(fn func() time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nowFn = fn
}

// Configure (re)points the journal at a context folder + active backend.
// On change, the cursor file is read (or seeded) and the in-memory pending
// state is rebuilt from the existing log.
func (m *Manager) Configure(contextFolder, backend string) error {
	m.mu.Lock()
	m.contextFolder = strings.TrimSpace(contextFolder)
	m.backend = strings.ToLower(strings.TrimSpace(backend))
	m.cursors = CursorMap{}
	m.pending = map[string]map[string]string{}
	ctx := m.contextFolder
	m.mu.Unlock()

	if ctx == "" {
		return nil
	}

	if err := m.fs.EnsureDirectory(ctx); err != nil {
		m.log.Warn("journal: ensure context dir failed", "err", err, "ctx", ctx)
	}

	if err := m.ensureCursorFile(); err != nil {
		m.log.Warn("journal: ensure cursor file failed", "err", err)
	}

	m.ensureGitignorePatterns()
	m.sweepStaleStash()

	cursors, err := m.loadCursors()
	if err != nil {
		m.log.Warn("journal: load cursors failed", "err", err)
		cursors = CursorMap{}
	}

	pending, err := m.rebuildPending(cursors)
	if err != nil {
		m.log.Warn("journal: rebuild pending failed", "err", err)
		pending = map[string]map[string]string{}
	}

	m.mu.Lock()
	m.cursors = cursors
	m.pending = pending
	m.mu.Unlock()
	return nil
}

// Init writes a baseline entry per existing tracked file (templates/*.yaml
// and everything under storage/). Skipped when the log already exists,
// when no context is configured, or when there are no tracked files.
func (m *Manager) Init() InitResult {
	m.mu.RLock()
	ctx := m.contextFolder
	m.mu.RUnlock()
	if ctx == "" {
		return InitResult{Created: false, Reason: "no-context"}
	}
	logPath := filepath.Join(ctx, logFileName)
	if m.fs.FileExists(logPath) {
		return InitResult{Created: false, Reason: "exists"}
	}

	files := m.collectTrackedFiles()
	if len(files) == 0 {
		return InitResult{Created: false, Reason: "empty"}
	}

	now := m.nowFn().UTC().Format(time.RFC3339Nano)
	var sb strings.Builder
	for _, rel := range files {
		entry := Entry{Ts: now, Op: OpBaseline, Path: rel}
		if size, err := os.Stat(filepath.Join(ctx, rel)); err == nil {
			entry.Bytes = size.Size()
		}
		raw, _ := json.Marshal(entry)
		sb.Write(raw)
		sb.WriteByte('\n')
	}
	if err := m.fs.AppendFile(logPath, sb.String()); err != nil {
		return InitResult{Created: false, Reason: err.Error()}
	}
	return InitResult{Created: true, Entries: len(files)}
}

// RecordOp appends a file mutation. absPath must lie under the configured
// context folder and under templates/ or storage/; otherwise the call is a
// silent no-op (mirrors the JS version's path filtering).
//
// In local-only mode (backend == "" or "none") the journal is inert - we
// neither write to disk nor track in-memory pending. Switch the backend
// to git or gigot to start accumulating.
func (m *Manager) RecordOp(op, absPath string, meta map[string]any) {
	m.mu.RLock()
	ctx := m.contextFolder
	backend := m.backend
	m.mu.RUnlock()
	if ctx == "" || backend == "" || backend == BackendNone {
		return
	}
	rel, ok := relPosixUnder(ctx, absPath)
	if !ok {
		return
	}
	if !isTrackedRel(rel) {
		return
	}
	if op != OpCreate && op != OpUpdate && op != OpDelete {
		return
	}

	now := m.nowFn().UTC().Format(time.RFC3339Nano)
	entry := Entry{Ts: now, Op: op, Path: rel}
	if meta != nil {
		if b, ok := meta["bytes"]; ok {
			switch v := b.(type) {
			case int:
				entry.Bytes = int64(v)
			case int64:
				entry.Bytes = v
			case float64:
				entry.Bytes = int64(v)
			}
		}
	}

	if err := m.appendEntry(entry); err != nil {
		m.log.Warn("journal: append failed", "err", err, "op", op, "path", rel)
		return
	}

	m.mu.Lock()
	m.applyEntryToPendingLocked(entry)
	m.mu.Unlock()

	m.emit(EventChanged, entry)
}

// RecordSync appends a sync marker for the given backend, advances that
// backend's cursor, and clears its pending set. Primitive signature so
// *Manager satisfies the Recorder interface directly - sync backends
// pass their tuple straight through without an adapter struct.
func (m *Manager) RecordSync(backend, version string, pushed, pulled int) {
	m.mu.RLock()
	ctx := m.contextFolder
	m.mu.RUnlock()
	if ctx == "" || backend == "" {
		return
	}
	// Tightening: refuse unknown backends on the write side so the
	// cursor map can't temporarily hold entries that parseLine would
	// drop on the next rebuild.
	if !knownBackends[backend] {
		return
	}

	now := m.nowFn().UTC().Format(time.RFC3339Nano)
	entry := Entry{
		Ts:      now,
		Op:      OpSync,
		Backend: backend,
		Version: version,
		Pushed:  pushed,
		Pulled:  pulled,
	}
	if err := m.appendEntry(entry); err != nil {
		m.log.Warn("journal: append sync failed", "err", err, "backend", backend)
		return
	}

	m.mu.Lock()
	m.cursors[backend] = Cursor{Ts: now, Version: version}
	delete(m.pending, backend)
	cursorsCopy := cloneCursors(m.cursors)
	m.mu.Unlock()

	if err := m.saveCursors(cursorsCopy); err != nil {
		m.log.Warn("journal: save cursors failed", "err", err)
	}

	m.emit(EventChanged, entry)
}

// RecordRemoteSeen advances only the version of the cursor (no journal
// entry, no pending change). Called after a successful pull so the
// head-probe poller can short-circuit. Emits journal:changed so
// frontend pollers see the post-pull cursor update - symmetric with
// RecordOp/RecordSync, which both emit on success.
func (m *Manager) RecordRemoteSeen(backend, version string) {
	m.mu.RLock()
	ctx := m.contextFolder
	m.mu.RUnlock()
	if ctx == "" || backend == "" || version == "" {
		return
	}
	if !knownBackends[backend] {
		return
	}

	m.mu.Lock()
	cur, ok := m.cursors[backend]
	if !ok {
		cur = Cursor{}
	}
	cur.Version = version
	m.cursors[backend] = cur
	cursorsCopy := cloneCursors(m.cursors)
	m.mu.Unlock()

	if err := m.saveCursors(cursorsCopy); err != nil {
		m.log.Warn("journal: save cursors (remote-seen) failed", "err", err)
	}

	// No log entry (pull is inbound), but the cursor moved - emit so
	// pollers refresh. Entry shape mirrors what callers can already
	// receive from RecordSync's "sync" event minus pushed/pulled.
	m.emit(EventChanged, Entry{
		Ts:      m.nowFn().UTC().Format(time.RFC3339Nano),
		Op:      OpSync,
		Backend: backend,
		Version: version,
	})
}

// RecentEntries returns up to <limit> most-recent log entries, newest
// first. limit <= 0 means "all". When no context is configured or
// the log file doesn't exist yet, returns an empty slice (not an
// error) - matches the inert-mode contract Pending uses.
//
// Used by the journal-feed UI: the on-disk log is the canonical
// chronological view of every mutation + sync that's gone through
// the system, so a one-shot read is sufficient for a "show me the
// recent activity" panel. The feed subscribes to `journal:changed`
// to know when to re-poll.
func (m *Manager) RecentEntries(limit int) []Entry {
	ctx := m.contextFolderLocked()
	if ctx == "" {
		return []Entry{}
	}
	logPath := filepath.Join(ctx, logFileName)
	if !m.fs.FileExists(logPath) {
		return []Entry{}
	}
	raw, err := m.fs.LoadFile(logPath)
	if err != nil {
		m.log.Warn("journal: read log for recent entries failed", "err", err)
		return []Entry{}
	}
	all := make([]Entry, 0, 256)
	for line := range strings.SplitSeq(raw, "\n") {
		entry, err := parseLine(line)
		if err != nil || entry == nil {
			continue
		}
		all = append(all, *entry)
	}
	// Reverse in-place: parseLine yielded oldest-first; the feed
	// wants newest-first.
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all
}

// Pending returns the pending changes for the given backend.
// An empty/unknown backend returns an empty result.
func (m *Manager) Pending(backend string) PendingResult {
	if backend == "" || backend == BackendNone {
		return PendingResult{Count: 0, Paths: []PendingChange{}}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	bucket := m.pending[backend]
	out := make([]PendingChange, 0, len(bucket))
	for path, op := range bucket {
		out = append(out, PendingChange{Path: path, Op: op})
	}
	return PendingResult{Count: len(out), Paths: out}
}

// ReadCursor returns a copy of the cursor map.
func (m *Manager) ReadCursor() CursorMap {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneCursors(m.cursors)
}

// ContextFolder returns the currently configured context folder. Used by tests.
func (m *Manager) ContextFolder() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.contextFolder
}

// ─────────────────────────────────────────────────────────────────────
// Internals
// ─────────────────────────────────────────────────────────────────────

func (m *Manager) ensureCursorFile() error {
	cursorPath := filepath.Join(m.contextFolderLocked(), cursorFileName)
	if m.fs.FileExists(cursorPath) {
		return nil
	}
	return m.fs.SaveFile(cursorPath, "{}\n")
}

// ensureGitignorePatterns adds the journal-exclusion patterns to the
// nearest .gitignore so .changes.log and .changes.cursor never get
// committed when the user points context at a synced repo. Skipped
// when no backend is active (local-only mode has no version-control
// concern). Resolution order:
//
//  1. <context>/.gitignore exists → patch it (most contexts that opt
//     into sync will already have one for their own purposes).
//  2. Else walk up from <context> looking for an enclosing .git → patch
//     (or create) <repoRoot>/.gitignore so a context inside a larger
//     repo still keeps the journal local.
//  3. Else → no-op (no git in scope; nothing to keep clean).
//
// Idempotent: existing patterns are detected line-by-line and skipped.
// Atomic write via fs.SaveFile (tmp+fsync+rename).
//
// Best-effort: read/write failures log a warning and return - Configure
// must not block on a permission glitch in someone else's repo root.
func (m *Manager) ensureGitignorePatterns() {
	ctx := m.contextFolderLocked()
	if ctx == "" {
		return
	}
	m.mu.RLock()
	backend := m.backend
	m.mu.RUnlock()
	if backend == "" || backend == BackendNone {
		return
	}

	target, ok := m.resolveGitignoreTarget(ctx)
	if !ok {
		return
	}

	body := ""
	if m.fs.FileExists(target) {
		loaded, err := m.fs.LoadFile(target)
		if err != nil {
			m.log.Warn("journal: gitignore read failed", "err", err, "path", target)
			return
		}
		body = loaded
	}

	present := map[string]bool{}
	for line := range strings.SplitSeq(body, "\n") {
		present[strings.TrimSpace(line)] = true
	}
	missing := make([]string, 0, len(gitignorePatterns))
	for _, p := range gitignorePatterns {
		if !present[p] {
			missing = append(missing, p)
		}
	}
	if len(missing) == 0 {
		return
	}

	sep := ""
	if body != "" && !strings.HasSuffix(body, "\n") {
		sep = "\n"
	}
	out := body + sep + strings.Join(missing, "\n") + "\n"
	if err := m.fs.SaveFile(target, out); err != nil {
		m.log.Warn("journal: gitignore write failed", "err", err, "path", target)
	}
}

// resolveGitignoreTarget picks which .gitignore the patterns go into.
// Returns ("", false) when there's no git in scope (neither a .gitignore
// in the context nor an enclosing .git anywhere up to findGitMaxDepth).
func (m *Manager) resolveGitignoreTarget(ctx string) (string, bool) {
	contextGitignore := filepath.Join(ctx, ".gitignore")
	if m.fs.FileExists(contextGitignore) {
		return contextGitignore, true
	}
	repoRoot := m.findGitRepoRoot(ctx)
	if repoRoot == "" {
		return "", false
	}
	return filepath.Join(repoRoot, ".gitignore"), true
}

// sweepStaleStash removes any leftover .changes.stash/ directory
// inside the context folder. Owned-by-git but family-managed-here:
// the journal already curates .changes.log + .changes.cursor +
// .gitignore patches, so taking responsibility for the transient
// .changes.stash artifact keeps every "files starting with .changes"
// concern in one module.
//
// Called from Configure (boot / context switch). PullWithStash also
// removes its own stash dir at the end of every run, but a process
// crash mid-pull or a pre-fix codebase can leave one behind. Best-
// effort: any RemoveAll error is logged at warn but does not block
// boot - the user can always delete it by hand and the next pull
// will sweep it on its own start.
func (m *Manager) sweepStaleStash() {
	ctx := m.contextFolderLocked()
	if ctx == "" {
		return
	}
	stashPath := filepath.Join(ctx, stashDirName)
	if !m.fs.FileExists(stashPath) {
		return
	}
	if err := os.RemoveAll(stashPath); err != nil {
		m.log.Warn("journal: stale stash sweep failed",
			"err", err, "path", stashPath)
	}
}

// findGitRepoRoot walks up from start (inclusive), returning the
// absolute path of the first ancestor containing a .git entry, or ""
// if none within findGitMaxDepth levels. Uses fs.FileExists which
// returns true for both regular files (worktree pointer) and
// directories (the standard repo layout).
func (m *Manager) findGitRepoRoot(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for range findGitMaxDepth {
		if m.fs.FileExists(filepath.Join(dir, ".git")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
	return ""
}

func (m *Manager) loadCursors() (CursorMap, error) {
	cursorPath := filepath.Join(m.contextFolderLocked(), cursorFileName)
	if !m.fs.FileExists(cursorPath) {
		return CursorMap{}, nil
	}
	raw, err := m.fs.LoadFile(cursorPath)
	if err != nil {
		return CursorMap{}, err
	}
	cursors, wasLegacy := sanitizeCursor([]byte(raw))
	if wasLegacy {
		// Migrate the on-disk file to the modern object-shaped form so
		// subsequent loads don't keep re-parsing the legacy string-form.
		// Best-effort: a write failure logs a warning but does not block
		// the in-memory state - readers still get correct values either
		// way (sanitizeCursor handled the translation already).
		if err := m.saveCursors(cursors); err != nil {
			m.log.Warn("journal: cursor migration write failed", "err", err)
		}
	}
	return cursors, nil
}

func (m *Manager) saveCursors(cursors CursorMap) error {
	bytes, err := json.Marshal(cursors)
	if err != nil {
		return err
	}
	cursorPath := filepath.Join(m.contextFolderLocked(), cursorFileName)
	return m.fs.SaveFile(cursorPath, string(bytes)+"\n")
}

func (m *Manager) appendEntry(e Entry) error {
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	logPath := filepath.Join(m.contextFolderLocked(), logFileName)
	return m.fs.AppendFile(logPath, string(raw)+"\n")
}

// rebuildPending walks the on-disk journal once and computes the
// pending set for each known backend, applying their respective cursors.
func (m *Manager) rebuildPending(cursors CursorMap) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	for backend := range knownBackends {
		out[backend] = map[string]string{}
	}

	logPath := filepath.Join(m.contextFolderLocked(), logFileName)
	if !m.fs.FileExists(logPath) {
		return out, nil
	}
	raw, err := m.fs.LoadFile(logPath)
	if err != nil {
		return out, err
	}

	for line := range strings.SplitSeq(raw, "\n") {
		entry, err := parseLine(line)
		if err != nil || entry == nil {
			continue
		}
		if entry.Op == OpSync || entry.Op == OpBaseline {
			continue
		}
		for backend := range knownBackends {
			cur := cursors[backend]
			if entry.Ts <= cur.Ts && cur.Ts != "" {
				continue
			}
			out[backend][entry.Path] = entry.Op
		}
	}
	return out, nil
}

// applyEntryToPendingLocked updates per-backend pending after a fresh
// RecordOp. Caller must hold m.mu (write lock).
func (m *Manager) applyEntryToPendingLocked(e Entry) {
	for backend := range knownBackends {
		bucket, ok := m.pending[backend]
		if !ok {
			bucket = map[string]string{}
			m.pending[backend] = bucket
		}
		bucket[e.Path] = e.Op
	}
}

func (m *Manager) collectTrackedFiles() []string {
	ctx := m.contextFolderLocked()
	out := []string{}

	// templates/*.yaml (non-recursive)
	templatesPath := filepath.Join(ctx, templatesDir)
	if files, err := m.fs.ListFiles(templatesPath); err == nil {
		for _, f := range files {
			if strings.HasSuffix(f, ".yaml") {
				out = append(out, templatesDir+"/"+f)
			}
		}
	}

	// storage/** (recursive)
	storagePath := filepath.Join(ctx, storageDir)
	if files, err := m.fs.WalkFiles(storagePath); err == nil {
		for _, abs := range files {
			rel, ok := relPosixUnder(ctx, abs)
			if ok {
				out = append(out, rel)
			}
		}
	}

	return out
}

// contextFolderLocked is a cheap accessor that handles the read-lock for
// callers that already verified context is set.
func (m *Manager) contextFolderLocked() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.contextFolder
}

func (m *Manager) emit(name string, data any) {
	if m.emitter == nil {
		return
	}
	m.emitter.Emit(name, data)
}

// ─────────────────────────────────────────────────────────────────────
// Pure helpers
// ─────────────────────────────────────────────────────────────────────

func isTrackedRel(rel string) bool {
	return rel == templatesDir ||
		rel == storageDir ||
		strings.HasPrefix(rel, templatesDir+"/") ||
		strings.HasPrefix(rel, storageDir+"/")
}

// relPosixUnder returns absPath relative to base, with forward slashes.
// Returns ("", false) when absPath escapes the base.
func relPosixUnder(base, absPath string) (string, bool) {
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", false
	}
	pathAbs, err := filepath.Abs(absPath)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(baseAbs, pathAbs)
	if err != nil {
		return "", false
	}
	if rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// parseLine parses one JSONL line into an Entry. Empty / malformed lines
// return (nil, nil) so callers can scan a partially-corrupted log.
func parseLine(line string) (*Entry, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, nil
	}
	var e Entry
	if err := json.Unmarshal([]byte(line), &e); err != nil {
		return nil, nil
	}
	if e.Ts == "" || e.Op == "" {
		return nil, nil
	}
	switch e.Op {
	case OpSync:
		if !knownBackends[e.Backend] {
			return nil, nil
		}
	case OpCreate, OpUpdate, OpDelete, OpBaseline:
		if e.Path == "" {
			return nil, nil
		}
	default:
		return nil, nil
	}
	return &e, nil
}

// sanitizeCursor accepts the on-disk JSON and normalises it to a CursorMap.
// Tolerates the legacy shape where the value was just the sync ts string;
// the second return reports whether any legacy entry was seen, so
// loadCursors can write the file back in modern object-form and stop
// paying the migration cost on every subsequent load.
func sanitizeCursor(raw []byte) (CursorMap, bool) {
	out := CursorMap{}
	wasLegacy := false
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return out, false
	}
	for k, v := range generic {
		if !knownBackends[k] {
			continue
		}
		switch val := v.(type) {
		case string:
			if val != "" {
				out[k] = Cursor{Ts: val}
				wasLegacy = true
			}
		case map[string]any:
			ts, _ := val["ts"].(string)
			ver, _ := val["version"].(string)
			if ts != "" || ver != "" {
				out[k] = Cursor{Ts: ts, Version: ver}
			}
		}
	}
	return out, wasLegacy
}

func cloneCursors(in CursorMap) CursorMap {
	out := make(CursorMap, len(in))
	maps.Copy(out, in)
	return out
}

// ErrNoContext is returned by callers that want to signal the journal isn't ready.
var ErrNoContext = errors.New("journal: no context folder configured")

// Compile-time assertions that *Manager satisfies the public journal
// interfaces. Anything that drifts the method signatures fails the
// build here, not at the distant call site in app.go.
var (
	_ Recorder = (*Manager)(nil)
	_ Reader   = (*Manager)(nil)
	_ Journal  = (*Manager)(nil)
)
