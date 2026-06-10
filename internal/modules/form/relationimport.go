package form

import (
	"errors"
	"sort"
	"strings"
)

var (
	errRecordResolverMissing = errors.New("form: record resolver not wired")
	errNotAPIField           = errors.New("form: field is not an api field")
	errFieldNotFound         = errors.New("form: field not found on template")
)

// EdgePair is one source-guid -> target-guid link parsed from an import sheet's
// two id columns.
type EdgePair struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ImportRelationResult reports the outcome of ImportRelationEdges: how many
// source records were touched, how many links were added, and how many rows
// were skipped because an endpoint record did not exist.
type ImportRelationResult struct {
	Records     int `json:"records"`
	Linked      int `json:"linked"`
	MissingFrom int `json:"missingFrom"`
	MissingTo   int `json:"missingTo"`
}

// recordResolver maps a (template, guid) to its datafile, true when the record
// exists. Injected so this package never imports dataprovider.
type recordResolver func(template, guid string) (datafile string, ok bool)

// SetRecordResolver wires the guid->datafile lookup ImportRelationEdges needs.
func (m *Manager) SetRecordResolver(r recordResolver) { m.resolveRecord = r }

// ImportRelationEdges is the relations pass of a multipass import: it writes the
// target ids of an api field onto the existing source records, then saves each
// through SaveValues so the reference-edge syncer mirrors them into the relation
// graph (and a later save won't drain them, since the ids now live in the
// record's data). fieldKey must name an api field on sourceTemplate. Pairs are
// grouped by From; each From record's api value becomes the union of its prior
// ids and the resolved To ids. Rows whose From or To record does not exist are
// skipped and counted. Idempotent: re-running unions, never duplicates.
func (m *Manager) ImportRelationEdges(sourceTemplate, fieldKey string, pairs []EdgePair) (ImportRelationResult, error) {
	var res ImportRelationResult
	if m.resolveRecord == nil {
		return res, errRecordResolverMissing
	}
	tpl, err := m.templates.LoadTemplate(sourceTemplate)
	if err != nil {
		return res, err
	}
	var target string
	found := false
	for _, f := range tpl.Fields {
		if f.Key == fieldKey {
			if f.Type != "api" {
				return res, errNotAPIField
			}
			target = f.Collection
			found = true
			break
		}
	}
	if !found {
		return res, errFieldNotFound
	}

	// Group To ids per From, preserving uniqueness. A missing To record drops
	// that id (counted once); a missing From record drops the whole group.
	grouped := map[string]map[string]bool{}
	order := []string{}
	for _, p := range pairs {
		if p.From == "" || p.To == "" {
			continue
		}
		if _, ok := m.resolveRecord(target, p.To); !ok {
			res.MissingTo++
			continue
		}
		set := grouped[p.From]
		if set == nil {
			set = map[string]bool{}
			grouped[p.From] = set
			order = append(order, p.From)
		}
		set[p.To] = true
	}

	for _, from := range order {
		datafile, ok := m.resolveRecord(sourceTemplate, from)
		if !ok {
			res.MissingFrom++
			continue
		}
		view, err := m.BuildView(sourceTemplate, datafile)
		if err != nil {
			res.MissingFrom++
			continue
		}
		existing := map[string]bool{}
		for _, id := range refIDs(view.Values[fieldKey]) {
			existing[id] = true
		}
		added := 0
		for id := range grouped[from] {
			if !existing[id] {
				existing[id] = true
				added++
			}
		}
		ids := make([]string, 0, len(existing))
		for id := range existing {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		// Store as []any of strings: that is the JSON-shaped to-many api value
		// storage.Sanitize/coerceAPIRef accepts. A []string falls through to
		// the default branch there and is dropped to nil.
		anyIDs := make([]any, len(ids))
		for i, s := range ids {
			anyIDs[i] = s
		}

		values := view.Values
		if values == nil {
			values = map[string]any{}
		}
		values[fieldKey] = anyIDs
		if _, err := m.SaveValues(sourceTemplate, SavePayload{
			Datafile: datafile,
			Values:   values,
			Meta:     view.Meta,
		}); err != nil {
			return res, err
		}
		res.Records++
		res.Linked += added
	}
	return res, nil
}

var (
	errRelationReaderMissing = errors.New("form: relation reader not wired")
	errRelationSyncDisabled  = errors.New("form: relation sync is disabled (enable it in config first)")
)

// relationSyncEnabled reports the config gate. The destructive sync overwrites
// api-field values from the relation graph, so it stays off unless the user opts
// in; this is the backend half of the guard (the menu item is also gated).
func (m *Manager) relationSyncEnabled() bool {
	return m.config != nil && m.config.FormDefaults().RelationSyncEnabled
}

// SyncRelationsToField makes an api field MIRROR the relation edges that exist
// for it: each host record's value is REPLACED with the exact set of to-ids in
// the field's relation (the entry whose target is the field's collection). This
// is the inverse of the normal field->edges sync, so a link removed elsewhere
// disappears from the field, and a record whose edges are now empty is cleared.
// Records already in agreement are left untouched. Gated by config: a no-op-guard
// error when relation sync is disabled. fieldKey must name an api field.
func (m *Manager) SyncRelationsToField(template, fieldKey string) (ImportRelationResult, error) {
	var res ImportRelationResult
	if !m.relationSyncEnabled() {
		return res, errRelationSyncDisabled
	}
	if m.relations == nil {
		return res, errRelationReaderMissing
	}
	tpl, err := m.templates.LoadTemplate(template)
	if err != nil {
		return res, err
	}
	target := ""
	found := false
	for _, f := range tpl.Fields {
		if f.Key == fieldKey {
			if f.Type != "api" {
				return res, errNotAPIField
			}
			target = f.Collection
			found = true
			break
		}
	}
	if !found {
		return res, errFieldNotFound
	}
	return m.replaceFieldFromEdges(template, fieldKey, target)
}

// replaceFieldFromEdges is the per-record mirror pass for one api field. It builds
// the desired to-ids per host guid from the relation edges (dropping ids whose
// target no longer exists), then walks every record and rewrites the field only
// where its current value differs from the desired set.
func (m *Manager) replaceFieldFromEdges(template, fieldKey, target string) (ImportRelationResult, error) {
	var res ImportRelationResult
	pairs, err := m.relations.RelationEdges(template, target)
	if err != nil {
		return res, err
	}
	desired := map[string]map[string]bool{}
	for _, p := range pairs {
		if p.From == "" || p.To == "" {
			continue
		}
		if m.resolveRecord != nil {
			if _, ok := m.resolveRecord(target, p.To); !ok {
				res.MissingTo++
				continue
			}
		}
		set := desired[p.From]
		if set == nil {
			set = map[string]bool{}
			desired[p.From] = set
		}
		set[p.To] = true
	}

	files, err := m.storage.ListForms(template)
	if err != nil {
		return res, err
	}
	for _, df := range files {
		form := m.storage.LoadForm(template, df)
		if form == nil || form.Meta.ID == "" {
			continue
		}
		want := desired[form.Meta.ID]
		have := map[string]bool{}
		for _, id := range refIDs(form.Data[fieldKey]) {
			have[id] = true
		}
		if sameStringSet(want, have) {
			continue
		}
		ids := make([]string, 0, len(want))
		for id := range want {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		anyIDs := make([]any, len(ids))
		for i, s := range ids {
			anyIDs[i] = s
		}
		values := form.Data
		if values == nil {
			values = map[string]any{}
		}
		values[fieldKey] = anyIDs
		if _, err := m.SaveValues(template, SavePayload{
			Datafile: df,
			Values:   values,
			Meta:     form.Meta,
		}); err != nil {
			return res, err
		}
		res.Records++
		res.Linked += len(ids)
	}
	return res, nil
}

// sameStringSet treats nil as empty.
func sameStringSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// SyncRelationsForTemplate back-fills every api field on the template from the
// relation edges that already exist for it, summing the per-field results. This is
// the one-click "Synchronize from relations" utility: each api field's value is
// brought into agreement with the edges (e.g. an inverse field added after the
// links). Idempotent. A field whose edges are empty contributes nothing.
func (m *Manager) SyncRelationsForTemplate(template string) (ImportRelationResult, error) {
	var total ImportRelationResult
	if !m.relationSyncEnabled() {
		return total, errRelationSyncDisabled
	}
	if m.relations == nil {
		return total, errRelationReaderMissing
	}
	tpl, err := m.templates.LoadTemplate(template)
	if err != nil {
		return total, err
	}
	for _, f := range tpl.Fields {
		if f.Type != "api" {
			continue
		}
		res, err := m.SyncRelationsToField(template, f.Key)
		if err != nil {
			return total, err
		}
		total.Records += res.Records
		total.Linked += res.Linked
		total.MissingFrom += res.MissingFrom
		total.MissingTo += res.MissingTo
	}
	return total, nil
}

// RelationField is one api-field a relations import can fill: its key, label,
// and target collection. Backend-sourced so the dialog's relation picker has one
// source of truth, mirroring how the records pass gets MappableFields from the
// backend instead of filtering the template in Vue.
type RelationField struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	Collection string `json:"collection"`
}

// RelationFields returns a template's api fields (the relation targets a
// relations import can populate), in declaration order.
func (m *Manager) RelationFields(template string) ([]RelationField, error) {
	tpl, err := m.templates.LoadTemplate(template)
	if err != nil {
		return nil, err
	}
	out := make([]RelationField, 0)
	for _, f := range tpl.Fields {
		if f.Type == "api" {
			out = append(out, RelationField{Key: f.Key, Label: f.Label, Collection: f.Collection})
		}
	}
	return out, nil
}

// ImportRelationsFromColumns extracts {from,to} pairs from a parsed sheet's two
// id columns and links them through fieldKey. The pair extraction lives backend
// side (not in the dialog) so the importer's contract is owned and tested in Go.
func (m *Manager) ImportRelationsFromColumns(template, fieldKey, fromColumn, toColumn string, headers []string, rows [][]string) (ImportRelationResult, error) {
	return m.ImportRelationEdges(template, fieldKey, buildEdgePairs(headers, rows, fromColumn, toColumn))
}

// buildEdgePairs reads the from/to id columns out of the parsed rows. Rows
// missing either id are dropped; unknown column names yield no pairs.
func buildEdgePairs(headers []string, rows [][]string, fromColumn, toColumn string) []EdgePair {
	fi, ti := -1, -1
	for i, h := range headers {
		if h == fromColumn {
			fi = i
		}
		if h == toColumn {
			ti = i
		}
	}
	if fi < 0 || ti < 0 {
		return nil
	}
	pairs := make([]EdgePair, 0, len(rows))
	for _, row := range rows {
		from, to := cellAt(row, fi), cellAt(row, ti)
		if from != "" && to != "" {
			pairs = append(pairs, EdgePair{From: from, To: to})
		}
	}
	return pairs
}

func cellAt(row []string, i int) string {
	if i >= 0 && i < len(row) {
		return strings.TrimSpace(row[i])
	}
	return ""
}

// refIDs pulls target ids from a stored api-field value: a bare id string or a
// list of id strings. Mirrors internal/app.referenceIDs (kept local so form
// imports nothing extra).
func refIDs(v any) []string {
	switch t := v.(type) {
	case string:
		if t != "" {
			return []string{t}
		}
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
