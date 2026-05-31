package gigot

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Conflict resolution leans entirely on the server's merge engine: it never
// reimplements the per-field merge on the client. The flow is two-phase.
//
//  1. Neutralize every conflicting field to the server's value and push with
//     the ledger base as parent. The server 3-way merges base/theirs/ours, so
//     all the disjoint edits on both sides land and the neutralized fields are
//     a clean no-op. This is the whole "take theirs" outcome in one push.
//  2. For fields the user chose to keep ("mine"), fetch the just-merged record
//     and fast-forward our value on top (parent = new head, so the server
//     applies the bytes verbatim). This preserves the other side's changes to
//     every other field while asserting our value on the ones we kept.

// ConflictValues fetches the server's current value for each conflicting field
// and pairs it with ours from disk, so the resolver UI can show them side by
// side. Read-only: touches neither disk nor the ledger.
func (m *Manager) ConflictValues(c Connection, contextFolder string, conflicts []PathConflict) ([]ConflictFieldValue, error) {
	if contextFolder == "" {
		return nil, ErrMissingContext
	}
	out := []ConflictFieldValue{}
	for _, pc := range conflicts {
		yours, err := m.readContextRecord(contextFolder, pc.Path)
		if err != nil {
			return nil, err
		}
		theirs, err := m.fetchServerRecord(c, pc.Path)
		if err != nil {
			return nil, err
		}
		for _, fc := range pc.Fields {
			yv, _, err := getRecordField(yours, fc.Scope, fc.Key)
			if err != nil {
				return nil, err
			}
			tv, _, err := getRecordField(theirs, fc.Scope, fc.Key)
			if err != nil {
				return nil, err
			}
			out = append(out, ConflictFieldValue{
				Path:   pc.Path,
				Scope:  fc.Scope,
				Key:    fc.Key,
				Yours:  string(yv),
				Theirs: string(tv),
			})
		}
	}
	return out, nil
}

// ResolveConflicts applies the user's per-field picks and re-pushes. See the
// file comment for the two-phase strategy. A push that conflicts again (a third
// party raced) is returned via PushResult.Conflicts, not an error.
func (m *Manager) ResolveConflicts(c Connection, contextFolder, message string, resolutions []FieldResolution) (*PushResult, error) {
	if contextFolder == "" {
		return nil, ErrMissingContext
	}
	byPath := map[string][]FieldResolution{}
	order := []string{}
	for _, r := range resolutions {
		if _, seen := byPath[r.Path]; !seen {
			order = append(order, r.Path)
		}
		byPath[r.Path] = append(byPath[r.Path], r)
	}

	// Phase 1: capture our kept values, neutralize every conflict to theirs.
	type keptField struct {
		scope, key string
		value      json.RawMessage
	}
	kept := map[string][]keptField{}
	for _, path := range order {
		fields := byPath[path]
		yours, err := m.readContextRecord(contextFolder, path)
		if err != nil {
			return nil, err
		}
		theirs, err := m.fetchServerRecord(c, path)
		if err != nil {
			return nil, err
		}
		for _, f := range fields {
			if f.Side != "mine" {
				continue
			}
			v, ok, err := getRecordField(yours, f.Scope, f.Key)
			if err != nil {
				return nil, err
			}
			if ok {
				kept[path] = append(kept[path], keptField{f.Scope, f.Key, v})
			}
		}
		neutralized, err := copyFields(yours, theirs, fields)
		if err != nil {
			return nil, err
		}
		if err := m.writeContextRecord(contextFolder, path, neutralized); err != nil {
			return nil, err
		}
	}

	res, err := m.PushLocal(c, contextFolder, message)
	if err != nil {
		return res, err
	}
	if len(res.Conflicts) > 0 {
		return res, nil
	}

	// Phase 2: sync resolved records to the server's merged content, then
	// fast-forward our kept values on top.
	hasMine := false
	for _, path := range order {
		merged, err := m.fetchServerRecord(c, path)
		if err != nil {
			return nil, err
		}
		for _, kf := range kept[path] {
			merged, err = setRecordField(merged, kf.scope, kf.key, kf.value)
			if err != nil {
				return nil, err
			}
			hasMine = true
		}
		if err := m.writeContextRecord(contextFolder, path, merged); err != nil {
			return nil, err
		}
	}
	if !hasMine {
		return res, nil
	}
	return m.PushLocal(c, contextFolder, message)
}

// readContextRecord reads a managed record's raw bytes from the context folder.
func (m *Manager) readContextRecord(contextFolder, repoRelPath string) ([]byte, error) {
	return os.ReadFile(filepath.Join(contextFolder, filepath.FromSlash(repoRelPath)))
}

// writeContextRecord writes a record back through the atomic filesystem (falling
// back to a plain write only when no Filesystem is wired, e.g. bare tests).
func (m *Manager) writeContextRecord(contextFolder, repoRelPath string, content []byte) error {
	target := filepath.Join(contextFolder, filepath.FromSlash(repoRelPath))
	if m.fs == nil {
		return os.WriteFile(target, content, 0o644)
	}
	return m.fs.SaveFile(target, string(content))
}

// fetchServerRecord pulls a single record's current HEAD bytes from the server.
func (m *Manager) fetchServerRecord(c Connection, repoRelPath string) ([]byte, error) {
	resp, err := m.GetFile(c, repoRelPath)
	if err != nil {
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(resp.ContentB64)
	if err != nil {
		return nil, fmt.Errorf("gigot: decode %s: %w", repoRelPath, err)
	}
	return raw, nil
}
