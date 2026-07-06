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

	"github.com/petervdpas/formidable2/internal/event"
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
	emitter event.Emitter

	mu            sync.RWMutex
	contextFolder string
	backend       string
	cursors       CursorMap
	// pending[backend][path] = latest op for that path since cursor[backend].Ts
	pending map[string]map[string]string
	nowFn   func() time.Time
}

// NewManager constructs a journal; emitter may be nil (events silenced), log may be nil.
func NewManager(filesystem fs, log *slog.Logger, emitter event.Emitter) *Manager {
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

// Configure (re)points the journal at a context folder + backend, seeding the cursor and rebuilding pending state.
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

// Init writes a baseline entry per tracked file; skipped when the log exists, no context is set, or nothing is tracked.
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

// RecordOp appends a file mutation; absPath outside the tracked trees
// (templates/, storage/, relations/) or root files (README.md, .gitignore) is a
// silent no-op.
// In local-only mode (backend "" or "none") the journal is inert.
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

// RecordRevert appends a revert marker for absPath and drops it from every backend's pending set.
// A discard reverts the worktree file to its committed state, so a stale pending create/update/delete
// no longer reflects reality. The marker makes the clear durable: rebuildPending replays it as a
// deletion, so a restart can't resurrect the entry from the earlier op. Same inert contract as RecordOp:
// no-op outside the context, for untracked paths, or when journaling is off.
func (m *Manager) RecordRevert(absPath string) {
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

	now := m.nowFn().UTC().Format(time.RFC3339Nano)
	entry := Entry{Ts: now, Op: OpRevert, Path: rel}
	if err := m.appendEntry(entry); err != nil {
		m.log.Warn("journal: append revert failed", "err", err, "path", rel)
		return
	}

	m.mu.Lock()
	m.applyEntryToPendingLocked(entry)
	m.mu.Unlock()

	m.emit(EventChanged, entry)
}

// RecordSync appends a sync marker, advances the backend's cursor, and clears its pending set.
func (m *Manager) RecordSync(backend, version string, pushed, pulled int) {
	m.mu.RLock()
	ctx := m.contextFolder
	m.mu.RUnlock()
	if ctx == "" || backend == "" {
		return
	}
	// Refuse unknown backends on write so the cursor map can't hold entries parseLine would later drop.
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

// RecordRemoteSeen advances only the cursor version after a pull (no log entry), emitting journal:changed so pollers refresh.
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

	// No log entry (inbound pull), but the cursor moved; emit so pollers refresh.
	m.emit(EventChanged, Entry{
		Ts:      m.nowFn().UTC().Format(time.RFC3339Nano),
		Op:      OpSync,
		Backend: backend,
		Version: version,
	})
}

// RecentEntries returns up to limit most-recent entries, newest first (limit <= 0 means all);
// empty slice when no context or log file.
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
	// Reverse: parseLine yields oldest-first, the feed wants newest-first.
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all
}

// Pending returns the pending changes for the backend (empty for unknown/none).
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

// ContextFolder returns the configured context folder.
func (m *Manager) ContextFolder() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.contextFolder
}

func (m *Manager) ensureCursorFile() error {
	cursorPath := filepath.Join(m.contextFolderLocked(), cursorFileName)
	if m.fs.FileExists(cursorPath) {
		return nil
	}
	return m.fs.SaveFile(cursorPath, "{}\n")
}

// ensureGitignorePatterns adds the .changes.* exclusion patterns to the nearest .gitignore (context, else
// enclosing repo root, else no-op) so the journal stays local in a synced repo. Idempotent and best-effort.
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

// resolveGitignoreTarget picks the .gitignore to patch; ("", false) when no git is in scope.
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

// sweepStaleStash removes a leftover .changes.stash/ dir (from a crash mid-pull); best-effort, doesn't block boot.
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

// findGitRepoRoot returns the first ancestor (from start, inclusive) containing a .git entry, or "" within findGitMaxDepth.
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
		// Persist the modern object form so later loads stop re-parsing legacy; best-effort.
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

// rebuildPending computes each backend's pending set from the on-disk journal, applying their cursors.
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
			if entry.Op == OpRevert {
				delete(out[backend], entry.Path)
				continue
			}
			out[backend][entry.Path] = entry.Op
		}
	}
	return out, nil
}

// applyEntryToPendingLocked updates per-backend pending after a RecordOp; caller holds m.mu (write).
func (m *Manager) applyEntryToPendingLocked(e Entry) {
	for backend := range knownBackends {
		bucket, ok := m.pending[backend]
		if !ok {
			bucket = map[string]string{}
			m.pending[backend] = bucket
		}
		if e.Op == OpRevert {
			delete(bucket, e.Path)
			continue
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

	// storage/** and relations/** (recursive)
	for _, dir := range []string{storageDir, relationsDir} {
		if files, err := m.fs.WalkFiles(filepath.Join(ctx, dir)); err == nil {
			for _, abs := range files {
				if rel, ok := relPosixUnder(ctx, abs); ok {
					out = append(out, rel)
				}
			}
		}
	}

	// Context-root files that travel with the repo (README.md, .gitignore).
	for name := range rootTrackedFiles {
		if m.fs.FileExists(filepath.Join(ctx, name)) {
			out = append(out, name)
		}
	}

	return out
}

// contextFolderLocked reads contextFolder under the read lock.
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

func isTrackedRel(rel string) bool {
	return rel == templatesDir ||
		rel == storageDir ||
		rel == relationsDir ||
		rootTrackedFiles[rel] ||
		strings.HasPrefix(rel, templatesDir+"/") ||
		strings.HasPrefix(rel, storageDir+"/") ||
		strings.HasPrefix(rel, relationsDir+"/")
}

// relPosixUnder returns absPath relative to base with forward slashes; ("", false) when it escapes base.
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

// parseLine parses one JSONL line; empty/malformed lines return (nil, nil) so a corrupt log still scans.
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
	case OpCreate, OpUpdate, OpDelete, OpBaseline, OpRevert:
		if e.Path == "" {
			return nil, nil
		}
	default:
		return nil, nil
	}
	return &e, nil
}

// sanitizeCursor normalises the on-disk JSON to a CursorMap, tolerating the legacy ts-string value;
// the bool reports whether a legacy entry was seen so loadCursors can rewrite the file.
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

var (
	_ Recorder = (*Manager)(nil)
	_ Reader   = (*Manager)(nil)
	_ Journal  = (*Manager)(nil)
)
