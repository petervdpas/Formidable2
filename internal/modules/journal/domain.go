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

// fs is the narrow filesystem surface the journal needs.
// *system.Manager satisfies it.
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
// In local-only mode (backend == "" or "none") the journal is inert — we
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
// backend's cursor, and clears its pending set.
func (m *Manager) RecordSync(rec SyncRecord) {
	m.mu.RLock()
	ctx := m.contextFolder
	m.mu.RUnlock()
	if ctx == "" || rec.Backend == "" {
		return
	}

	now := m.nowFn().UTC().Format(time.RFC3339Nano)
	entry := Entry{
		Ts:      now,
		Op:      OpSync,
		Backend: rec.Backend,
		Version: rec.Version,
		Pushed:  rec.Pushed,
		Pulled:  rec.Pulled,
	}
	if err := m.appendEntry(entry); err != nil {
		m.log.Warn("journal: append sync failed", "err", err, "backend", rec.Backend)
		return
	}

	m.mu.Lock()
	m.cursors[rec.Backend] = Cursor{Ts: now, Version: rec.Version}
	delete(m.pending, rec.Backend)
	cursorsCopy := cloneCursors(m.cursors)
	m.mu.Unlock()

	if err := m.saveCursors(cursorsCopy); err != nil {
		m.log.Warn("journal: save cursors failed", "err", err)
	}

	m.emit(EventChanged, entry)
}

// RecordRemoteSeen advances only the version of the cursor (no journal
// entry, no pending change). Called after a successful pull so the
// head-probe poller can short-circuit.
func (m *Manager) RecordRemoteSeen(backend, version string) {
	m.mu.RLock()
	ctx := m.contextFolder
	m.mu.RUnlock()
	if ctx == "" || backend == "" || version == "" {
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

func (m *Manager) loadCursors() (CursorMap, error) {
	cursorPath := filepath.Join(m.contextFolderLocked(), cursorFileName)
	if !m.fs.FileExists(cursorPath) {
		return CursorMap{}, nil
	}
	raw, err := m.fs.LoadFile(cursorPath)
	if err != nil {
		return CursorMap{}, err
	}
	return sanitizeCursor([]byte(raw)), nil
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
// Tolerates the legacy shape where the value was just the sync ts string.
func sanitizeCursor(raw []byte) CursorMap {
	out := CursorMap{}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return out
	}
	for k, v := range generic {
		if !knownBackends[k] {
			continue
		}
		switch val := v.(type) {
		case string:
			if val != "" {
				out[k] = Cursor{Ts: val}
			}
		case map[string]any:
			ts, _ := val["ts"].(string)
			ver, _ := val["version"].(string)
			if ts != "" || ver != "" {
				out[k] = Cursor{Ts: ts, Version: ver}
			}
		}
	}
	return out
}

func cloneCursors(in CursorMap) CursorMap {
	out := make(CursorMap, len(in))
	maps.Copy(out, in)
	return out
}

// ErrNoContext is returned by callers that want to signal the journal isn't ready.
var ErrNoContext = errors.New("journal: no context folder configured")
