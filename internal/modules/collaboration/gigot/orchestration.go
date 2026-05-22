package gigot

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PushLocal walks the active context folder, diffs against the
// track-record, and commits changed files to the server. On first
// sync (no ledger), seeds the ledger from /tree so we don't blindly
// re-push files the remote already has. Steady state is /head +
// /commits only.
//
// message is the user-supplied commit subject. When blank (or
// whitespace-only), buildCommitMessage falls back to an auto-generated
// "<who>: sync N file(s)" audit string so back-end audit trails always
// have *something* useful even on programmatic pushes.
//
// The on-disk state is not assumed to be a git working tree - blob
// SHAs are computed from raw bytes via GitBlobSha; no .git/ is touched.
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

	head, err := m.Head(conn)
	if err != nil {
		if isHTTPStatus(err, http.StatusConflict) {
			return nil, ErrNoParentVersion
		}
		return nil, err
	}
	if head.Version == "" {
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
		ParentVersion: head.Version,
		Changes:       changes,
		Message:       chooseCommitMessage(message, conn, changes),
	}
	if conn.Author != nil && (conn.Author.Name != "" || conn.Author.Email != "") {
		req.Author = conn.Author
	}

	resp, err := m.Commit(conn, req)
	if err != nil {
		return nil, err
	}

	reconcileLedger(&record, diff, resp)
	record.LastSync = time.Now().UTC().Format(time.RFC3339)
	if err := m.WriteTrackRecord(contextFolder, record); err != nil {
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

// PullLocal fetches the server's tree and writes any blob whose local
// SHA differs. Files the prior ledger remembered but that vanished
// from the new tree are deleted locally. The ledger is rebuilt from
// the authoritative tree, so the server wins any disagreement.
//
// Delegates to PullLocalWithProgress with a nil callback - keeps the
// no-progress callsite simple while the with-callback form drives the
// frontend's per-file progress bar.
func (m *Manager) PullLocal(conn Connection, contextFolder string) (*PullResult, error) {
	return m.PullLocalWithProgress(conn, contextFolder, nil)
}

// PullLocalWithProgress is the progress-instrumented form of PullLocal.
// The callback receives SyncProgress events at: Start (before any
// HTTP), Tree (after /tree, with Total set), Delete (once per
// vanished managed path), Fetch (once per managed tree entry,
// regardless of SHA-match short-circuit), and Done (at completion).
// A nil callback degrades to plain PullLocal behaviour.
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

// Reclone wipes every gigot-managed path under contextFolder
// (templates/, storage/, allowlisted root files, the ledger) and then
// pulls a fresh copy from the server. Use when the user wants a
// guaranteed-clean slate keyed to the server's HEAD - local-only edits
// in managed paths are LOST, by design. Non-managed files in the
// context folder (notes, user data, .formidable/context.json marker)
// are preserved.
//
// On a context folder that's already empty, Reclone behaves like an
// initial clone - wipe is a no-op and the pull populates everything.
func (m *Manager) Reclone(conn Connection, contextFolder string) (*PullResult, error) {
	return m.RecloneWithProgress(conn, contextFolder, nil)
}

// RecloneWithProgress is the progress-instrumented form of Reclone.
// Emits PhaseWipe before the destructive sweep, then delegates to
// PullLocalWithProgress for the fetch half - so consumers see the
// full Start → Wipe → Tree → Delete/Fetch* → Done sequence.
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

// Sync runs PushLocal then PullLocal. A push error aborts before pull
// so unpushed local changes aren't overwritten - symmetric with the
// git Service's sync behaviour. message threads through to the push
// half; the pull half is read-only.
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

// seedRecordFromTree fetches the server tree and returns a fresh
// track-record populated with every reported blob's SHA. Used on
// first sync so a freshly cloned context doesn't blindly re-push
// files the remote already has. A 409 (empty repo) returns (nil, nil)
// so the caller can proceed with the empty ledger and let the commit
// path surface ErrNoParentVersion.
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

// managedTreeEntries filters a server tree to only Formidable-managed
// paths. Anything outside templates/ + storage/ + the root allowlist
// stays untouched on disk (and untouched in pull's ledger rebuild
// keeps it tracked for accurate "deletion since" semantics).
func managedTreeEntries(all []TreeEntry) []TreeEntry {
	out := make([]TreeEntry, 0, len(all))
	for _, e := range all {
		if IsFormidablePath(e.Path) {
			out = append(out, e)
		}
	}
	return out
}

// reconcileLedger updates the ledger after a successful commit. When
// the server echoes a changes[] list, use it as the authoritative
// post-merge state (handles F1 auto-merge where server-resolved
// content differs from what we sent). Otherwise fall back to the
// SHAs the diff computed locally.
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

// chooseCommitMessage picks the commit subject sent to gigot. A
// non-blank user-supplied message wins verbatim; whitespace-only or
// empty falls back to the auto-generated audit string so the server's
// audit log never carries a literal empty message.
func chooseCommitMessage(userMessage string, conn Connection, changes []Change) string {
	trimmed := strings.TrimSpace(userMessage)
	if trimmed != "" {
		return userMessage
	}
	return buildCommitMessage(conn, changes)
}

// buildCommitMessage produces an audit-friendly commit subject for a
// gigot push: "<who>: sync N files" followed by a bulleted list of
// up to 20 paths. Mirrors the format the gigot server's audit log
// understands and matches the prior client's output.
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

// isHTTPStatus reports whether err is a *HTTPError with the given code.
func isHTTPStatus(err error, code int) bool {
	var he *HTTPError
	if !errors.As(err, &he) {
		return false
	}
	return he.Status == code
}
