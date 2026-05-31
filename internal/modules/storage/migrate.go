package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// MigrateTemplateMeta rewrites legacy meta shape into the AuditEntry pair across one template's forms.
// Migration is structural, not authorship: the legacy author fills both Created and Updated (SaveFormExact,
// not SaveForm, so the migrator isn't stamped). Already-new files are skipped untouched; per-file errors don't abort.
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
		// LoadForm's Sanitize migrates the legacy keys; SaveFormExact writes verbatim so the profile isn't stamped.
		datafile := strings.TrimSuffix(filename, formExt)
		f := m.LoadForm(templateFilename, datafile)
		if f == nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: load returned nil", filename))
			continue
		}
		sr := m.SaveFormExact(context.Background(), templateFilename, datafile, *f)
		if !sr.Success {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: save: %s", filename, sr.Error))
			continue
		}
		res.Migrated++
	}
	return res, nil
}

// needsMetaMigration reports a legacy marker in the meta block: flat author_name/author_email keys,
// or string-typed created/updated. Non-envelope JSON errors so the caller can flag the file.
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

// isJSONString reports whether the raw value is a quoted string; audit blocks are objects, so a string is a legacy marker.
func isJSONString(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return len(trimmed) > 0 && trimmed[0] == '"'
}
