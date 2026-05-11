package integrity

import (
	"fmt"
	"sort"
	"time"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// TemplateLoader is the narrow surface the analyzer needs from the
// template module: load a template by its filename (e.g. "basic.yaml").
type TemplateLoader interface {
	LoadTemplate(name string) (*template.Template, error)
}

// StorageReader is the narrow surface the analyzer needs from the
// storage module: list the forms under a template, load each one.
// Both signatures match storage.Manager exactly so the composition
// root can inject *storage.Manager directly.
type StorageReader interface {
	ListForms(templateFilename string) ([]string, error)
	LoadForm(templateFilename, datafile string) *storage.Form
}

// Manager owns the analyze logic. Stateless aside from its
// collaborators; safe to share across goroutines.
type Manager struct {
	templates TemplateLoader
	storage   StorageReader
	writer    StorageWriter
	now       func() time.Time
}

// NewManager builds the manager. now defaults to time.Now.
func NewManager(tpl TemplateLoader, sto StorageReader) *Manager {
	return &Manager{templates: tpl, storage: sto, now: time.Now}
}

// AnalyzeTemplate inspects every form under templateFilename and returns
// a Report listing every structural drift. Returning an error means the
// scan itself couldn't start (unknown template, list failure); a
// per-form parse error becomes IssueUnreadable inside the report rather
// than a hard error.
func (m *Manager) AnalyzeTemplate(templateFilename string) (Report, error) {
	tpl, err := m.templates.LoadTemplate(templateFilename)
	if err != nil {
		return Report{}, fmt.Errorf("integrity: load template %q: %w", templateFilename, err)
	}

	filenames, err := m.storage.ListForms(templateFilename)
	if err != nil {
		return Report{}, fmt.Errorf("integrity: list forms for %q: %w", templateFilename, err)
	}

	report := Report{
		Template:  templateFilename,
		FormCount: len(filenames),
		ScannedAt: m.now(),
	}

	for _, fn := range filenames {
		f := m.storage.LoadForm(templateFilename, fn)
		var issues []Issue
		if f == nil {
			issues = []Issue{{Kind: IssueUnreadable, Detail: "form file could not be parsed"}}
		} else {
			issues = analyzeForm(tpl, f)
		}
		if len(issues) > 0 {
			report.Forms = append(report.Forms, FormReport{Filename: fn, Issues: issues})
			report.IssueCount += len(issues)
		}
	}

	sort.Slice(report.Forms, func(i, j int) bool {
		return report.Forms[i].Filename < report.Forms[j].Filename
	})

	return report, nil
}

// analyzeForm runs every check against one loaded form. Returns the
// flattened issue list (already in deterministic order).
func analyzeForm(tpl *template.Template, f *storage.Form) []Issue {
	var out []Issue
	out = append(out, checkMeta(tpl, f.Meta)...)
	out = append(out, checkData(tpl.Fields, f.Data, "")...)
	return out
}

// ─── meta checks ───────────────────────────────────────────────────────

// checkMeta validates the meta block independently of the data map.
// All checks here are tolerant of empty: an un-set timestamp is OK,
// only a *bad* one is an issue. Same for flag_state.
func checkMeta(tpl *template.Template, meta storage.FormMeta) []Issue {
	var out []Issue

	// Created / Updated must be parseable RFC3339 (storage writes
	// RFC3339Nano; both Nano and second-precision parse via that layout).
	if meta.Created != "" && !parseableTimestamp(meta.Created) {
		out = append(out, Issue{
			Kind:   IssueMetaBadFormat,
			Path:   "meta.created",
			Detail: fmt.Sprintf("not RFC3339: %q", meta.Created),
		})
	}
	if meta.Updated != "" && !parseableTimestamp(meta.Updated) {
		out = append(out, Issue{
			Kind:   IssueMetaBadFormat,
			Path:   "meta.updated",
			Detail: fmt.Sprintf("not RFC3339: %q", meta.Updated),
		})
	}

	// ID is required when the template declares a guid field. Otherwise
	// blank is acceptable.
	if meta.ID == "" && templateHasGuid(tpl) {
		out = append(out, Issue{
			Kind:   IssueMetaMissing,
			Path:   "meta.id",
			Detail: "template declares a guid field; meta.id must be set",
		})
	}

	// flag_state must reference a declared FlagDefinition when set.
	// Empty flag_state is always OK (legacy `flagged: true` paths).
	if meta.FlagState != "" && !flagDefined(tpl, meta.FlagState) {
		out = append(out, Issue{
			Kind: IssueMetaBadFormat,
			Path: "meta.flag_state",
			Detail: fmt.Sprintf(
				"flag_state %q is not declared in template.flag_definitions",
				meta.FlagState,
			),
		})
	}

	return out
}

func parseableTimestamp(s string) bool {
	if _, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return true
	}
	_, err := time.Parse(time.RFC3339, s)
	return err == nil
}

func templateHasGuid(tpl *template.Template) bool {
	for _, f := range tpl.Fields {
		if f.Type == "guid" {
			return true
		}
	}
	return false
}

func flagDefined(tpl *template.Template, label string) bool {
	for _, fd := range tpl.FlagDefinitions {
		if fd.Label == label {
			return true
		}
	}
	return false
}

// ─── data checks ──────────────────────────────────────────────────────

// checkData walks a flat-or-nested data map against the field list.
// pathPrefix is empty for top-level data; loop-recursive calls pass
// e.g. "items[0]." so issue paths read "items[0].name".
//
// The walk mirrors storage.Sanitize's loop handling: a loopstart claims
// data[key] as []any of items, the fields between it and the matching
// loopstop are the inner fields, and looper/loopstop markers are skipped.
func checkData(fields []template.Field, data map[string]any, pathPrefix string) []Issue {
	var out []Issue

	expected := map[string]struct{}{}
	skip := map[string]bool{}

	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if f.Type == "loopstart" {
			expected[f.Key] = struct{}{}
			endIdx := matchLoopstop(fields, i+1, f.Key)
			inner := fields[i+1 : endIdx]
			out = append(out, checkLoopValue(f.Key, inner, data[f.Key], pathPrefix)...)
			for _, inf := range inner {
				skip[inf.Key] = true
			}
			i = endIdx
			continue
		}
		if f.Type == "looper" || f.Type == "loopstop" {
			continue
		}
		if skip[f.Key] {
			continue
		}
		expected[f.Key] = struct{}{}
		out = append(out, checkField(f, data, pathPrefix)...)
	}

	// Extra/orphan keys: anything in data that isn't a top-level
	// expected key. Walk in a stable order so issue lists are
	// deterministic.
	extras := make([]string, 0, len(data))
	for k := range data {
		if _, ok := expected[k]; !ok {
			extras = append(extras, k)
		}
	}
	sort.Strings(extras)
	for _, k := range extras {
		out = append(out, Issue{
			Kind: IssueExtraField,
			Path: pathPrefix + k,
			Detail: fmt.Sprintf(
				"key %q has no matching field in the template",
				k,
			),
		})
	}

	return out
}

// matchLoopstop scans forward from start looking for the loopstop whose
// Key matches loopKey, honouring nested loopstart/loopstop pairs. The
// template module bounds nesting at depth 2, but this walker doesn't
// rely on that — it works at any depth. Returns the index of the
// closing loopstop, or len(fields)-1 if none found (matches how
// storage.skipLoop degrades on a malformed template).
func matchLoopstop(fields []template.Field, start int, loopKey string) int {
	depth := 0
	for i := start; i < len(fields); i++ {
		switch fields[i].Type {
		case "loopstart":
			depth++
		case "loopstop":
			if depth == 0 && fields[i].Key == loopKey {
				return i
			}
			depth--
		}
	}
	return len(fields) - 1
}

// checkField runs the per-field checks for a non-loop top-level field.
func checkField(f template.Field, data map[string]any, pathPrefix string) []Issue {
	path := pathPrefix + f.Key
	v, ok := data[f.Key]
	if !ok {
		return []Issue{{
			Kind:   IssueMissingField,
			Path:   path,
			Detail: fmt.Sprintf("template field %q (%s) has no entry in data", f.Key, f.Type),
		}}
	}
	return checkValueType(f.Type, v, path)
}

// checkLoopValue validates that data[loopKey] is a []any of map[string]any
// entries, then recurses into each entry against the loop body.
func checkLoopValue(loopKey string, inner []template.Field, raw any, pathPrefix string) []Issue {
	path := pathPrefix + loopKey
	if raw == nil {
		// Sanitize defaults missing loops to []any{} — an explicit nil
		// is recoverable but worth flagging as a missing entry.
		return []Issue{{
			Kind:   IssueMissingField,
			Path:   path,
			Detail: fmt.Sprintf("loop %q has no entry in data", loopKey),
		}}
	}
	arr, ok := raw.([]any)
	if !ok {
		return []Issue{{
			Kind:   IssueTypeMismatch,
			Path:   path,
			Detail: fmt.Sprintf("loop %q must be an array, got %T", loopKey, raw),
		}}
	}
	var out []Issue
	for idx, item := range arr {
		itemPath := fmt.Sprintf("%s[%d]", path, idx)
		m, ok := item.(map[string]any)
		if !ok {
			out = append(out, Issue{
				Kind:   IssueTypeMismatch,
				Path:   itemPath,
				Detail: fmt.Sprintf("loop item must be an object, got %T", item),
			})
			continue
		}
		out = append(out, checkData(inner, m, itemPath+".")...)
	}
	return out
}

// checkValueType verifies v's Go type matches the declared field type.
// Returns a single issue or nothing. The "empty" sentinel (zero value
// for that type, or empty string) is always allowed — sanitize uses
// type defaults for unset values, and the analyzer should not flag a
// freshly defaulted form.
func checkValueType(fieldType string, v any, path string) []Issue {
	// nil is treated as "empty" — the field exists in data but is unset.
	if v == nil {
		return nil
	}
	switch fieldType {
	case "text", "textarea", "dropdown", "radio",
		"file-path", "folder-path", "image", "guid":
		if _, ok := v.(string); ok {
			return nil
		}
		return []Issue{{
			Kind:   IssueTypeMismatch,
			Path:   path,
			Detail: fmt.Sprintf("expected string, got %T", v),
		}}

	case "link":
		// link is `{href, text}` map canonically — FormFieldLink.vue
		// builds it from a free-form URL or a formidable:// pair. A
		// bare string is also accepted because legacy forms (and CSV
		// imports) carry the raw href without the wrapper; the field
		// component normalises on read.
		switch v.(type) {
		case string, map[string]any:
			return nil
		}
		return []Issue{{
			Kind:   IssueTypeMismatch,
			Path:   path,
			Detail: fmt.Sprintf("expected link object or string, got %T", v),
		}}

	case "date":
		s, ok := v.(string)
		if !ok {
			return []Issue{{
				Kind:   IssueTypeMismatch,
				Path:   path,
				Detail: fmt.Sprintf("expected ISO date string, got %T", v),
			}}
		}
		if s == "" {
			return nil
		}
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return []Issue{{
				Kind:   IssueBadDateFormat,
				Path:   path,
				Detail: fmt.Sprintf("not YYYY-MM-DD: %q", s),
			}}
		}
		return nil

	case "boolean":
		if _, ok := v.(bool); ok {
			return nil
		}
		return []Issue{{
			Kind:   IssueTypeMismatch,
			Path:   path,
			Detail: fmt.Sprintf("expected bool, got %T", v),
		}}

	case "number", "range":
		if isNumeric(v) {
			return nil
		}
		return []Issue{{
			Kind:   IssueTypeMismatch,
			Path:   path,
			Detail: fmt.Sprintf("expected number, got %T", v),
		}}

	case "tags", "multioption", "list", "table":
		if _, ok := v.([]any); ok {
			return nil
		}
		return []Issue{{
			Kind:   IssueTypeMismatch,
			Path:   path,
			Detail: fmt.Sprintf("expected array, got %T", v),
		}}

	case "api":
		if _, ok := v.(map[string]any); ok {
			return nil
		}
		return []Issue{{
			Kind:   IssueTypeMismatch,
			Path:   path,
			Detail: fmt.Sprintf("expected object, got %T", v),
		}}
	}

	// Unknown field type — be permissive. Template validation owns the
	// "field type is bogus" check; we don't want to double-report it as
	// a data issue.
	return nil
}

func isNumeric(v any) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return true
	}
	return false
}
