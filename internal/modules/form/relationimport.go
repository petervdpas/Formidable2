package form

import (
	"errors"
	"sort"
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

		values := view.Values
		if values == nil {
			values = map[string]any{}
		}
		values[fieldKey] = ids
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
