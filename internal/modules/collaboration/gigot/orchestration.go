package gigot

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PushLocal walks the context folder, diffs against the track-record, and commits changed files.
// On first sync (no ledger) it seeds from /tree so we don't blindly re-push files the remote already has.
// Blob SHAs come from raw bytes via GitBlobSha; no .git/ is touched.
func (m *Manager) PushLocal(conn Connection, contextFolder string, message string) (*PushResult, error) {
	if err := validateConn(conn, true); err != nil {
		return nil, err
	}
	if contextFolder == "" {
		return nil, ErrMissingContext
	}

	local, err := CollectFormidableFiles(contextFolder)
	if err != nil {
		return nil, err
	}
	if len(local) == 0 {
		return nil, ErrEmptyContext
	}

	record := m.ReadTrackRecord(contextFolder)
	firstSync := record.Version == ""
	if firstSync {
		seeded, err := m.seedRecordFromTree(conn)
		if err != nil {
			return nil, err
		}
		if seeded != nil {
			record = *seeded
		}
	}

	diff := DiffAgainstRecord(local, record)
	if len(diff.Changed) == 0 && len(diff.Deleted) == 0 {
		record.LastSync = time.Now().UTC().Format(time.RFC3339)
		if err := m.WriteTrackRecord(contextFolder, record); err != nil {
			return nil, err
		}
		return &PushResult{
			Version: record.Version,
			Noop:    true,
			Scanned: len(local),
		}, nil
	}

	// The ledger version is the base our diff was computed against, so it is
	// the correct parent_version. Sending the live head instead (the old
	// "parent must equal HEAD" contract) makes the server fast-forward-overlay
	// our changes, silently clobbering a concurrent edit. With the true base,
	// the server 3-way merges against it (git + per-field record merge) or
	// returns a 409 we surface.
	if record.Version == "" {
		return nil, ErrNoParentVersion
	}

	changes := make([]Change, 0, len(diff.Changed)+len(diff.Deleted))
	for _, f := range diff.Changed {
		changes = append(changes, Change{
			Op:         "put",
			Path:       f.Path,
			ContentB64: base64.StdEncoding.EncodeToString(f.Bytes),
		})
	}
	for _, p := range diff.Deleted {
		changes = append(changes, Change{Op: "delete", Path: p})
	}

	req := CommitRequest{
		ParentVersion: record.Version,
		Changes:       changes,
		Message:       chooseCommitMessage(message, conn, changes),
	}
	if conn.Author != nil && (conn.Author.Name != "" || conn.Author.Email != "") {
		req.Author = conn.Author
	}

	resp, err := m.Commit(conn, req)
	if err != nil {
		var he *HTTPError
		if errors.As(err, &he) && he.Status == http.StatusConflict {
			if version, paths, ok := parseCommitConflict(he.Body); ok {
				// The server refused to reconcile our base with HEAD. Nothing
				// landed; leave the ledger untouched and surface the paths.
				return &PushResult{Version: version, Scanned: len(local), Conflicts: paths}, nil
			}
		}
		return nil, err
	}

	reconcileLedger(&record, diff, resp)
	record.LastSync = time.Now().UTC().Format(time.RFC3339)
	if err := m.WriteTrackRecord(contextFolder, record); err != nil {
		return nil, err
	}

	// A server-side merge rewrites records to canonical JSON, so the stored
	// blob differs from our on-disk bytes. Pull that authoritative content back
	// so disk matches the ledger; otherwise the record stays "pending" forever
	// (its local hash never equals the merged blob).
	if err := m.syncRewrittenRecords(conn, contextFolder, diff, resp); err != nil {
		return nil, err
	}

	return &PushResult{
		Version: record.Version,
		Pushed:  len(diff.Changed),
		Deleted: len(diff.Deleted),
		Scanned: len(local),
		Noop:    false,
	}, nil
}

// syncRewrittenRecords pulls back any pushed record whose server blob differs
// from what we sent (the server canonicalized it during a merge), so the local
// file matches the ledger. Records the server stored verbatim (blob == our sha)
// are skipped, so a plain fast-forward push does no extra fetches.
func (m *Manager) syncRewrittenRecords(conn Connection, contextFolder string, diff DiffResult, resp *CommitResponse) error {
	if resp == nil || len(resp.Changes) == 0 {
		return nil
	}
	localSha := make(map[string]string, len(diff.Changed))
	for _, f := range diff.Changed {
		localSha[f.Path] = f.Sha
	}
	for _, c := range resp.Changes {
		if c.Op == "delete" || c.Op == "deleted" || c.Blob == "" {
			continue
		}
		if c.Blob == localSha[c.Path] {
			continue
		}
		content, err := m.fetchServerRecord(conn, c.Path)
		if err != nil {
			return err
		}
		if err := m.writeContextRecord(contextFolder, c.Path, content); err != nil {
			return err
		}
	}
	return nil
}

// PullLocal fetches the server tree and writes any blob whose local SHA differs, deleting vanished managed paths.
// The ledger is rebuilt from the authoritative tree, so the server wins any disagreement.
func (m *Manager) PullLocal(conn Connection, contextFolder string) (*PullResult, error) {
	return m.PullLocalWithProgress(conn, contextFolder, nil)
}

// PullLocalWithProgress is the progress-instrumented PullLocal, emitting Start, Tree, Delete, Fetch, Done events; nil cb degrades to PullLocal.
func (m *Manager) PullLocalWithProgress(conn Connection, contextFolder string, cb ProgressFunc) (*PullResult, error) {
	if err := validateConn(conn, true); err != nil {
		return nil, err
	}
	if contextFolder == "" {
		return nil, ErrMissingContext
	}
	emit := func(p SyncProgress) {
		if cb != nil {
			cb(p)
		}
	}
	emit(SyncProgress{Phase: PhaseStart})

	oldRecord := m.ReadTrackRecord(contextFolder)

	tree, err := m.Tree(conn)
	if err != nil {
		return nil, err
	}

	managed := managedTreeEntries(tree.Files)
	newPaths := make(map[string]struct{}, len(tree.Files))
	for _, e := range tree.Files {
		newPaths[e.Path] = struct{}{}
	}

	pendingDeletes := []string{}
	for p := range oldRecord.Files {
		if !IsFormidablePath(p) {
			continue
		}
		if _, stillThere := newPaths[p]; stillThere {
			continue
		}
		pendingDeletes = append(pendingDeletes, p)
	}

	total := len(pendingDeletes) + len(managed)
	emit(SyncProgress{Phase: PhaseTree, Current: 0, Total: total})

	current := 0
	deleted := 0
	for _, p := range pendingDeletes {
		current++
		abs := filepath.Join(contextFolder, filepath.FromSlash(p))
		if err := os.Remove(abs); err == nil {
			deleted++
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("gigot: remove %s: %w", p, err)
		}
		emit(SyncProgress{Phase: PhaseDelete, Current: current, Total: total, Path: p})
	}

	written := 0
	for _, entry := range managed {
		current++
		abs := filepath.Join(contextFolder, filepath.FromSlash(entry.Path))
		skip := false
		if buf, err := os.ReadFile(abs); err == nil {
			if GitBlobSha(buf) == entry.Blob {
				skip = true
			}
		}
		if !skip {
			fileResp, err := m.GetFile(conn, entry.Path)
			if err != nil {
				return nil, err
			}
			raw, err := base64.StdEncoding.DecodeString(fileResp.ContentB64)
			if err != nil {
				return nil, fmt.Errorf("gigot: decode %s: %w", entry.Path, err)
			}
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(abs, raw, 0o644); err != nil {
				return nil, fmt.Errorf("gigot: write %s: %w", entry.Path, err)
			}
			written++
		}
		emit(SyncProgress{Phase: PhaseFetch, Current: current, Total: total, Path: entry.Path})
	}

	record := EmptyTrackRecord()
	record.Version = tree.Version
	record.LastSync = time.Now().UTC().Format(time.RFC3339)
	for _, e := range tree.Files {
		record.Files[e.Path] = e.Blob
	}
	if err := m.WriteTrackRecord(contextFolder, record); err != nil {
		return nil, err
	}

	emit(SyncProgress{Phase: PhaseDone, Current: total, Total: total})
	return &PullResult{
		Version: tree.Version,
		Files:   written,
		Deleted: deleted,
	}, nil
}

// Reclone wipes every gigot-managed path then pulls fresh from the server. Local-only edits in managed paths are LOST by design; non-managed files are preserved.
func (m *Manager) Reclone(conn Connection, contextFolder string) (*PullResult, error) {
	return m.RecloneWithProgress(conn, contextFolder, nil)
}

// RecloneWithProgress is the progress-instrumented Reclone, emitting PhaseWipe before the destructive sweep then delegating to PullLocalWithProgress.
func (m *Manager) RecloneWithProgress(conn Connection, contextFolder string, cb ProgressFunc) (*PullResult, error) {
	if err := validateConn(conn, true); err != nil {
		return nil, err
	}
	if contextFolder == "" {
		return nil, ErrMissingContext
	}
	if cb != nil {
		cb(SyncProgress{Phase: PhaseWipe})
	}
	if err := wipeManagedContent(contextFolder); err != nil {
		return nil, err
	}
	return m.PullLocalWithProgress(conn, contextFolder, cb)
}

// Sync runs PushLocal then PullLocal; a push error aborts before pull so unpushed local changes aren't overwritten.
func (m *Manager) Sync(conn Connection, contextFolder string, message string) (*SyncResult, error) {
	if err := validateConn(conn, true); err != nil {
		return nil, err
	}
	if contextFolder == "" {
		return nil, ErrMissingContext
	}

	push, err := m.PushLocal(conn, contextFolder, message)
	if err != nil {
		return nil, err
	}
	pull, err := m.PullLocal(conn, contextFolder)
	if err != nil {
		return nil, err
	}

	version := pull.Version
	if version == "" {
		version = push.Version
	}
	return &SyncResult{
		Version:       version,
		Pushed:        push.Pushed,
		PushedDeleted: push.Deleted,
		Pulled:        pull.Files,
		PulledDeleted: pull.Deleted,
		Noop:          push.Noop && pull.Files == 0 && pull.Deleted == 0,
	}, nil
}

// seedRecordFromTree builds a ledger from the server tree so first sync doesn't re-push existing files.
// A 409 (empty repo) returns (nil, nil) so the caller proceeds with the empty ledger and hits ErrNoParentVersion at commit.
func (m *Manager) seedRecordFromTree(conn Connection) (*TrackRecord, error) {
	tree, err := m.Tree(conn)
	if err != nil {
		if isHTTPStatus(err, http.StatusConflict) {
			return nil, nil
		}
		return nil, err
	}
	rec := EmptyTrackRecord()
	rec.Version = tree.Version
	for _, e := range tree.Files {
		rec.Files[e.Path] = e.Blob
	}
	return &rec, nil
}

// managedTreeEntries filters a server tree to Formidable-managed paths only.
func managedTreeEntries(all []TreeEntry) []TreeEntry {
	out := make([]TreeEntry, 0, len(all))
	for _, e := range all {
		if IsFormidablePath(e.Path) {
			out = append(out, e)
		}
	}
	return out
}

// parseCommitConflict extracts conflicting paths from a server 409 body. It
// handles both the generic conflicts[] shape and the record record_conflicts[]
// shape (§10.6, per-field detail). Returns ok=false when the body is not a
// recognizable conflict (so the caller surfaces the raw error instead).
func parseCommitConflict(body string) (version string, paths []PathConflict, ok bool) {
	var raw struct {
		CurrentVersion string `json:"current_version"`
		Conflicts      []struct {
			Path string `json:"path"`
		} `json:"conflicts"`
		RecordConflicts []struct {
			Path   string          `json:"path"`
			Fields []FieldConflict `json:"field_conflicts"`
		} `json:"record_conflicts"`
	}
	if json.Unmarshal([]byte(body), &raw) != nil {
		return "", nil, false
	}
	for _, c := range raw.Conflicts {
		paths = append(paths, PathConflict{Path: c.Path})
	}
	for _, c := range raw.RecordConflicts {
		paths = append(paths, PathConflict{Path: c.Path, Fields: c.Fields})
	}
	if len(paths) == 0 {
		return "", nil, false
	}
	return raw.CurrentVersion, paths, true
}

// reconcileLedger updates the ledger after commit. A server-echoed changes[] is authoritative post-merge
// (handles F1 auto-merge where server content differs from what we sent); otherwise fall back to local diff SHAs.
func reconcileLedger(rec *TrackRecord, diff DiffResult, resp *CommitResponse) {
	if resp == nil {
		return
	}
	rec.Version = resp.Version
	if rec.Files == nil {
		rec.Files = map[string]string{}
	}
	if len(resp.Changes) > 0 {
		for _, c := range resp.Changes {
			switch c.Op {
			case "delete", "deleted":
				delete(rec.Files, c.Path)
			default:
				if c.Blob != "" {
					rec.Files[c.Path] = c.Blob
				}
			}
		}
		return
	}
	for _, f := range diff.Changed {
		rec.Files[f.Path] = f.Sha
	}
	for _, p := range diff.Deleted {
		delete(rec.Files, p)
	}
}

// chooseCommitMessage returns a non-blank user message verbatim, else the auto-generated audit string.
func chooseCommitMessage(userMessage string, conn Connection, changes []Change) string {
	trimmed := strings.TrimSpace(userMessage)
	if trimmed != "" {
		return userMessage
	}
	return buildCommitMessage(conn, changes)
}

// buildCommitMessage produces "<who>: sync N files" plus a bulleted list of up to 20 paths, matching gigot's audit format.
func buildCommitMessage(conn Connection, changes []Change) string {
	who := "Formidable"
	if conn.Author != nil && conn.Author.Name != "" {
		who = conn.Author.Name
	}
	count := len(changes)
	plural := ""
	if count != 1 {
		plural = "s"
	}
	header := fmt.Sprintf("%s: sync %d file%s", who, count, plural)
	const cap = 20
	shown := changes
	if len(shown) > cap {
		shown = shown[:cap]
	}
	var body strings.Builder
	for _, c := range shown {
		if c.Op == "delete" {
			body.WriteString("\n- [delete] ")
		} else {
			body.WriteString("\n- ")
		}
		body.WriteString(c.Path)
	}
	if len(changes) > len(shown) {
		fmt.Fprintf(&body, "\n…and %d more", len(changes)-len(shown))
	}
	return header + "\n" + body.String()
}

func isHTTPStatus(err error, code int) bool {
	var he *HTTPError
	if !errors.As(err, &he) {
		return false
	}
	return he.Status == code
}
