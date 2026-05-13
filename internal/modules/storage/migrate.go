package storage

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// MigrateTemplateMeta walks every form under the given template's
// storage folder and rewrites legacy meta shape (flat author_name /
// author_email + string-typed created / updated) into the new
// AuditEntry pair. Files already in the new shape are skipped — the
// disk file is untouched, mtime preserved, no git churn.
//
// Migration is structural, not authorship: the legacy author appears
// in BOTH Created and Updated (since we can't reconstruct historical
// update authorship), and the active profile's identity is NOT
// stamped. SaveFormExact is used rather than SaveForm so the new
// Updated block reflects the legacy author rather than the migrator.
//
// Missing template folder → zero counts, nil error. Per-file failures
// (corrupt JSON, unreadable file) land in Errors and don't abort the
// rest of the pass.
func (m *Manager) MigrateTemplateMeta(templateFilename string) (MigrateResult, error) {
	var res MigrateResult
	files, err := m.ListForms(templateFilename)
	if err != nil {
		return res, fmt.Errorf("storage: migrate: list %q: %w", templateFilename, err)
	}
	res.Total = len(files)

	dir := m.templateDir(templateFilename)
	for _, filename := range files {
		full := filepath.Join(dir, filename)
		raw, err := m.fs.LoadFile(full)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: read: %v", filename, err))
			continue
		}
		needs, err := needsMetaMigration([]byte(raw))
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: parse: %v", filename, err))
			continue
		}
		if !needs {
			res.Skipped++
			continue
		}
		// LoadForm runs Sanitize which migrates the legacy keys into
		// the AuditEntry pair (using the legacy author for both Created
		// and Updated). SaveFormExact writes that result verbatim, so
		// the active profile is NOT stamped into Updated.
		datafile := strings.TrimSuffix(filename, formExt)
		f := m.LoadForm(templateFilename, datafile)
		if f == nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: load returned nil", filename))
			continue
		}
		sr := m.SaveFormExact(templateFilename, datafile, *f)
		if !sr.Success {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: save: %s", filename, sr.Error))
			continue
		}
		res.Migrated++
	}
	return res, nil
}

// needsMetaMigration inspects the raw bytes of a `.meta.json` file and
// returns true when at least one legacy marker is present in its meta
// block. Markers: flat `author_name` / `author_email` keys, or string
// (rather than object) values for `created` / `updated`. Cheap parse —
// we only decode the meta sub-object as map[string]any.
//
// Anything that isn't an envelope-shaped {meta:{...}, data:...} JSON
// returns an error so the caller can flag the file. Missing meta
// (empty meta map or no meta key) → not legacy, doesn't need migration
// (Sanitize will fill defaults on next save anyway).
func needsMetaMigration(raw []byte) (bool, error) {
	var top struct {
		Meta map[string]json.RawMessage `json:"meta"`
	}
	if err := json.Unmarshal(raw, &top); err != nil {
		return false, err
	}
	if top.Meta == nil {
		return false, nil
	}
	if _, ok := top.Meta["author_name"]; ok {
		return true, nil
	}
	if _, ok := top.Meta["author_email"]; ok {
		return true, nil
	}
	if v, ok := top.Meta["created"]; ok && isJSONString(v) {
		return true, nil
	}
	if v, ok := top.Meta["updated"]; ok && isJSONString(v) {
		return true, nil
	}
	return false, nil
}

// isJSONString returns true when the raw JSON value is a quoted string
// (i.e. starts with `"`). New-shape audit blocks are objects, so a
// string-typed created/updated is a reliable legacy marker.
func isJSONString(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return len(trimmed) > 0 && trimmed[0] == '"'
}
