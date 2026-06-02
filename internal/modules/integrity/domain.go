package integrity

import (
	"fmt"
	"sort"
	"time"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// TemplateLoader loads a template by its filename (e.g. "basic.yaml").
type TemplateLoader interface {
	LoadTemplate(name string) (*template.Template, error)
}

// StorageReader lists the forms under a template and loads each one.
type StorageReader interface {
	ListForms(templateFilename string) ([]string, error)
	LoadForm(templateFilename, datafile string) *storage.Form
}

// RawFormReader runs the guid-sync check against the un-sanitized form; LoadForm mirrors guid onto meta.id and would hide drift.
type RawFormReader interface {
	LoadFormRaw(templateFilename, datafile string) *storage.Form
}

// Manager owns the analyze logic; stateless aside from collaborators, safe to share across goroutines.
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

// AnalyzeTemplate inspects every form under templateFilename, returning a Report of structural drift.
// An error means the scan couldn't start; a per-form parse error becomes IssueUnreadable in the report.
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
		Template:       templateFilename,
		FormCount:      len(filenames),
		ScannedAt:      m.now(),
		TemplateIssues: checkFacetDefaults(tpl),
	}

	for _, fn := range filenames {
		f := m.storage.LoadForm(templateFilename, fn)
		var issues []Issue
		if f == nil {
			issues = []Issue{{Kind: IssueUnreadable, Detail: "form file could not be parsed"}}
		} else {
			issues = analyzeForm(tpl, f)
			issues = append(issues, m.guidSyncIssues(templateFilename, fn, tpl, f.Meta, f.Data)...)
			issues = append(issues, m.facetSeedingIssues(templateFilename, fn, tpl)...)
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

// checkFacetDefaults is a template-level check: a facet bound to a virtual field (Type "facet" with a
// FacetKey) that declares no default cannot be auto-filled, so it is surfaced once for the author. A facet
// with no virtual field is intentionally unsettable from the form and is not reported.
func checkFacetDefaults(tpl *template.Template) []Issue {
	var out []Issue
	for _, f := range tpl.Fields {
		if f.Type != "facet" || f.FacetKey == "" {
			continue
		}
		if def, ok := f.Default.(string); ok && def != "" {
			continue
		}
		out = append(out, Issue{
			Kind:   IssueFacetNoDefault,
			Path:   fmt.Sprintf("facets.%s", f.FacetKey),
			Detail: fmt.Sprintf("facet field %q has no default; forms without an explicit selection stay empty", f.Key),
		})
	}
	return out
}

// facetSeedingIssues flags facets the form is missing on disk that the template defaults. It reads the
// RAW form (no seeding) so a sanitized load can't hide the gap; without a RawFormReader it cannot tell.
func (m *Manager) facetSeedingIssues(templateFilename, fn string, tpl *template.Template) []Issue {
	raw, ok := m.storage.(RawFormReader)
	if !ok {
		return nil
	}
	rf := raw.LoadFormRaw(templateFilename, fn)
	if rf == nil {
		return nil
	}
	return checkFacetSeeding(tpl, rf.Meta.Facets)
}

// checkFacetSeeding reports each facet field that declares a default but whose key is absent from the
// on-disk facets. An entry that exists (even an explicit set:false) is left alone, deliberate state wins.
func checkFacetSeeding(tpl *template.Template, disk map[string]storage.FacetState) []Issue {
	var out []Issue
	for _, f := range tpl.Fields {
		if f.Type != "facet" || f.FacetKey == "" {
			continue
		}
		def, ok := f.Default.(string)
		if !ok || def == "" {
			continue
		}
		if _, present := disk[f.FacetKey]; present {
			continue
		}
		out = append(out, Issue{
			Kind:    IssueFacetUnseeded,
			Path:    fmt.Sprintf("meta.facets.%s", f.FacetKey),
			Suggest: def,
			Detail:  fmt.Sprintf("facet %q is absent on disk; defaults to %q", f.FacetKey, def),
		})
	}
	return out
}

// analyzeForm runs every check against one loaded form, returning the issue list in deterministic order.
func analyzeForm(tpl *template.Template, f *storage.Form) []Issue {
	var out []Issue
	out = append(out, checkMeta(tpl, f.Meta)...)
	out = append(out, checkData(tpl.Fields, f.Data, "")...)
	return out
}

// guidSyncIssues runs the guid-sync check against the raw form when LoadFormRaw is available, else the
// sanitized meta/data; raw is needed because LoadForm mirrors guid onto meta.id and would hide real drift.
func (m *Manager) guidSyncIssues(templateFilename, fn string, tpl *template.Template, meta storage.FormMeta, data map[string]any) []Issue {
	if raw, ok := m.storage.(RawFormReader); ok {
		if rf := raw.LoadFormRaw(templateFilename, fn); rf != nil {
			return checkGuidSync(tpl, rf.Meta, rf.Data)
		}
	}
	return checkGuidSync(tpl, meta, data)
}

// checkGuidSync flags guid fields whose data value drifts from meta.id; Suggest carries the canonical value
// (field value when set, else meta.id). Only runs when meta.id is set (blank is the separate meta_missing issue).
func checkGuidSync(tpl *template.Template, meta storage.FormMeta, data map[string]any) []Issue {
	if meta.ID == "" || data == nil {
		return nil
	}
	var out []Issue
	for _, f := range tpl.Fields {
		if f.Type != "guid" {
			continue
		}
		cur, _ := data[f.Key].(string)
		if cur == meta.ID {
			continue
		}
		canonical := cur
		if canonical == "" {
			canonical = meta.ID
		}
		out = append(out, Issue{
			Kind:    IssueGuidUnsynced,
			Path:    f.Key,
			Value:   cur,
			Suggest: canonical,
			Detail:  "guid field and meta.id disagree",
		})
	}
	return out
}

// checkMeta validates the meta block independently of data; empty is tolerated, only a bad value is an issue.
func checkMeta(tpl *template.Template, meta storage.FormMeta) []Issue {
	var out []Issue

	if meta.Created.At != "" && !parseableTimestamp(meta.Created.At) {
		out = append(out, Issue{
			Kind:   IssueMetaBadFormat,
			Path:   "meta.created",
			Detail: fmt.Sprintf("not RFC3339: %q", meta.Created.At),
		})
	}
	if meta.Updated.At != "" && !parseableTimestamp(meta.Updated.At) {
		out = append(out, Issue{
			Kind:   IssueMetaBadFormat,
			Path:   "meta.updated",
			Detail: fmt.Sprintf("not RFC3339: %q", meta.Updated.At),
		})
	}

	// ID is required only when the template declares a guid field.
	if meta.ID == "" && templateHasGuid(tpl) {
		out = append(out, Issue{
			Kind:   IssueMetaMissing,
			Path:   "meta.id",
			Detail: "template declares a guid field; meta.id must be set",
		})
	}

	// Each facet's non-empty Selected must be a declared option; unknown facet keys are also drift.
	for key, state := range meta.Facets {
		f := findFacet(tpl, key)
		if f == nil {
			out = append(out, Issue{
				Kind:   IssueMetaBadFormat,
				Path:   fmt.Sprintf("meta.facets.%s", key),
				Detail: fmt.Sprintf("facet key %q is not declared on the template", key),
			})
			continue
		}
		if state.Selected != "" && !facetHasOption(f, state.Selected) {
			out = append(out, Issue{
				Kind:   IssueMetaBadFormat,
				Path:   fmt.Sprintf("meta.facets.%s.selected", key),
				Detail: fmt.Sprintf("selected %q is not a declared option of facet %q", state.Selected, key),
			})
		}
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

func findFacet(tpl *template.Template, key string) *template.Facet {
	for i := range tpl.Facets {
		if tpl.Facets[i].Key == key {
			return &tpl.Facets[i]
		}
	}
	return nil
}

func facetHasOption(f *template.Facet, label string) bool {
	for _, o := range f.Options {
		if o.Label == label {
			return true
		}
	}
	return false
}

// checkData walks a data map against the field list, recursing into loops; pathPrefix prefixes nested issue paths.
// Loop handling mirrors storage.Sanitize: a loopstart claims data[key] as []any of items.
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
		// Virtual fields store their value outside data; mark expected (so a stray legacy value isn't flagged
		// extra) but skip the per-field check (so its absence isn't flagged missing).
		if template.IsVirtualFieldType(f.Type) {
			expected[f.Key] = struct{}{}
			continue
		}
		expected[f.Key] = struct{}{}
		out = append(out, checkField(f, data, pathPrefix)...)
	}

	// Orphan keys: data keys with no matching field, sorted for deterministic output.
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

// matchLoopstop returns the matching loopstop index, honouring nested pairs at any depth; falls back to the last field.
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
	out := checkValueType(f.Type, v, path)
	if f.Type == "table" {
		out = append(out, checkTableCells(f, v, path)...)
	}
	return out
}

// checkTableCells validates each cell against its column type (from f.Options, positional). Date columns
// infer the dominant entry format rather than guess per-value; number/bool columns are checked per-cell.
func checkTableCells(f template.Field, v any, path string) []Issue {
	rows, ok := v.([]any)
	if !ok {
		return nil // non-array shape already flagged by checkValueType
	}
	var out []Issue
	for ci := 0; ci < len(f.Options); ci++ {
		colType := columnType(f.Options[ci])
		if colType == "date" {
			out = append(out, checkDateColumn(rows, ci, path)...)
			continue
		}
		for ri, raw := range rows {
			cells, ok := raw.([]any)
			if !ok || ci >= len(cells) {
				continue
			}
			cellPath := fmt.Sprintf("%s[%d][%d]", path, ri, ci)
			out = append(out, checkCell(colType, cells[ci], cellPath)...)
		}
	}
	return out
}

// checkDateColumn infers a date column's dominant format, then flags conforming non-ISO cells as bad_date_format
// (with the ISO in Suggest) and everything else (non-string, non-conforming, or undecidable) as date_anomaly.
func checkDateColumn(rows []any, ci int, path string) []Issue {
	var values []string
	for _, raw := range rows {
		cells, ok := raw.([]any)
		if !ok || ci >= len(cells) {
			continue
		}
		if s, ok := cells[ci].(string); ok {
			values = append(values, s)
		}
	}
	layout, decided := inferColumnDateLayout(values)

	var out []Issue
	for ri, raw := range rows {
		cells, ok := raw.([]any)
		if !ok || ci >= len(cells) {
			continue
		}
		cell := cells[ci]
		if cell == nil {
			continue
		}
		cellPath := fmt.Sprintf("%s[%d][%d]", path, ri, ci)
		s, ok := cell.(string)
		if !ok {
			out = append(out, Issue{
				Kind:   IssueDateAnomaly,
				Path:   cellPath,
				Detail: fmt.Sprintf("expected a date string, got %T", cell),
				Value:  fmt.Sprintf("%v", cell),
			})
			continue
		}
		if s == "" || isISODate(s) {
			continue
		}
		if !decided {
			out = append(out, Issue{
				Kind:   IssueDateAnomaly,
				Path:   cellPath,
				Detail: "column date format undecidable",
				Value:  s,
			})
			continue
		}
		if t, ok := parsesDateExactly(layout, s); ok {
			iso := t.Format(isoDate)
			out = append(out, Issue{
				Kind:    IssueBadDateFormat,
				Path:    cellPath,
				Detail:  fmt.Sprintf("reformat to %s", iso),
				Value:   s,
				Suggest: iso,
			})
			continue
		}
		out = append(out, Issue{
			Kind:   IssueDateAnomaly,
			Path:   cellPath,
			Detail: fmt.Sprintf("does not match the column format (%s)", layout),
			Value:  s,
		})
	}
	return out
}

// checkCell validates one non-date table cell; empty cells (nil or "") are always accepted (short-row padding / defaults).
func checkCell(colType string, cell any, path string) []Issue {
	if cell == nil {
		return nil
	}
	if s, ok := cell.(string); ok && s == "" {
		return nil
	}
	switch colType {
	case "number":
		if !isNumeric(cell) {
			return []Issue{{
				Kind:   IssueTypeMismatch,
				Path:   path,
				Detail: fmt.Sprintf("expected number, got %T", cell),
				Value:  fmt.Sprintf("%v", cell),
			}}
		}
	case "bool":
		if _, ok := cell.(bool); !ok {
			return []Issue{{
				Kind:   IssueTypeMismatch,
				Path:   path,
				Detail: fmt.Sprintf("expected bool, got %T", cell),
				Value:  fmt.Sprintf("%v", cell),
			}}
		}
	}
	return nil
}

const isoDate = "2006-01-02"

// dateCandidateLayouts are the non-ISO input formats; order only breaks ties (disambiguation comes from single-layout parses).
var dateCandidateLayouts = []string{
	"02-01-2006", // DD-MM-YYYY
	"01-02-2006", // MM-DD-YYYY
	"02/01/2006", // DD/MM/YYYY
	"01/02/2006", // MM/DD/YYYY
	"02.01.2006", // DD.MM.YYYY (common in NL/DE)
	"01.02.2006", // MM.DD.YYYY
	"2006/01/02", // YYYY/MM/DD
}

// isISODate reports whether s is exactly a YYYY-MM-DD date.
func isISODate(s string) bool {
	t, err := time.Parse(isoDate, s)
	return err == nil && t.Format(isoDate) == s
}

// parsesDateExactly parses s under layout and confirms reformatting reproduces s, rejecting Go's lenient matches.
func parsesDateExactly(layout, s string) (time.Time, bool) {
	t, err := time.Parse(layout, s)
	if err != nil || t.Format(layout) != s {
		return time.Time{}, false
	}
	return t, true
}

// inferColumnDateLayout picks the dominant layout by majority vote, where only single-layout parses vote
// (ambiguous values abstain); returns ("", false) when nothing is decisive so the caller treats all as anomalies.
func inferColumnDateLayout(values []string) (string, bool) {
	votes := map[string]int{}
	for _, v := range values {
		if v == "" || isISODate(v) {
			continue
		}
		matched := ""
		count := 0
		for _, l := range dateCandidateLayouts {
			if _, ok := parsesDateExactly(l, v); ok {
				matched = l
				count++
			}
		}
		if count == 1 {
			votes[matched]++
		}
	}
	best, bestN := "", 0
	for _, l := range dateCandidateLayouts { // stable for ties
		if votes[l] > bestN {
			best, bestN = l, votes[l]
		}
	}
	if bestN == 0 {
		return "", false
	}
	return best, true
}

// columnType reads the declared type off a table column option; absent/garbled defaults to string.
func columnType(opt any) string {
	if m, ok := opt.(map[string]any); ok {
		if t, ok := m["type"].(string); ok && t != "" {
			return t
		}
	}
	return "string"
}

// checkLoopValue validates data[loopKey] is a []any of objects, then recurses into each against the loop body.
func checkLoopValue(loopKey string, inner []template.Field, raw any, pathPrefix string) []Issue {
	path := pathPrefix + loopKey
	if raw == nil {
		// Sanitize defaults missing loops to []any{}; an explicit nil is recoverable but worth flagging.
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

// checkValueType verifies v's Go type matches the field type; empty/nil is always allowed (defaulted values).
func checkValueType(fieldType string, v any, path string) []Issue {
	if v == nil {
		return nil
	}
	switch fieldType {
	case "text", "textarea", "mermaid", "dropdown", "radio",
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
		// Canonically a {href, text} map, but a bare string is accepted (legacy/CSV carry the raw href).
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
		// "" is the legacy unset sentinel for an array field; treat like nil. A non-empty string is real drift.
		if s, ok := v.(string); ok && s == "" {
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

	// Unknown field type: permissive. Template validation owns the bogus-type check.
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
