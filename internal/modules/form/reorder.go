package form

import (
	"errors"
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// ReorderResult reports what a sequence reorder wrote. Normalized is true when
// no gap was left to slot the moved record so the whole collection was
// re-spread to step spacing; Written lists the datafiles whose sequence value
// actually changed (records already at their target are never rewritten).
type ReorderResult struct {
	Normalized bool     `json:"normalized"`
	Written    []string `json:"written"`
}

// SequenceOrder returns the collection's datafiles ordered by their sequence
// value (ascending; a record missing a value sorts last, filename tie-break).
// The studio list renders a presentation template in this order, since the
// index can't ORDER BY a data field.
func (m *Manager) SequenceOrder(templateName string) ([]string, error) {
	_, seq, err := m.sequenceContext(templateName)
	if err != nil {
		return nil, err
	}
	files, err := m.storage.ListForms(templateName)
	if err != nil {
		return nil, fmt.Errorf("form: sequence order: %w", err)
	}
	type row struct {
		file string
		val  int
		ok   bool
	}
	rows := make([]row, 0, len(files))
	for _, f := range files {
		v, ok := m.sequenceOf(templateName, f, seq.Key)
		rows = append(rows, row{file: f, val: v, ok: ok})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.ok != b.ok {
			return a.ok // a valued record sorts before an unvalued one
		}
		if a.ok && a.val != b.val {
			return a.val < b.val
		}
		return a.file < b.file
	})
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.file
	}
	return out, nil
}

// ReorderSequence moves movedDatafile to its position in orderedDatafiles and
// persists the new order with a minimal write: only the moved record gets a
// fresh sequence value, the midpoint between its new neighbours. When no
// integer slot exists between them the whole collection is renumbered to step
// spacing (Normalized=true), still skipping records already at their target.
func (m *Manager) ReorderSequence(templateName, movedDatafile string, orderedDatafiles []string) (ReorderResult, error) {
	_, seq, err := m.sequenceContext(templateName)
	if err != nil {
		return ReorderResult{}, err
	}
	step := template.SequenceStep(*seq)

	i := indexOf(orderedDatafiles, movedDatafile)
	if i < 0 {
		return ReorderResult{}, fmt.Errorf("form: reorder: %q not in the given order", movedDatafile)
	}
	if len(orderedDatafiles) <= 1 {
		return ReorderResult{}, nil // nothing to order
	}

	prev, hasPrev := 0, false
	next, hasNext := 0, false
	if i > 0 {
		prev, hasPrev = m.sequenceOf(templateName, orderedDatafiles[i-1], seq.Key)
	}
	if i < len(orderedDatafiles)-1 {
		next, hasNext = m.sequenceOf(templateName, orderedDatafiles[i+1], seq.Key)
	}

	newVal, ok := sequenceMidpoint(hasPrev, prev, hasNext, next, step)
	if !ok {
		return m.normalizeSequence(templateName, orderedDatafiles, seq.Key, step)
	}
	if err := m.rewriteSequence(templateName, movedDatafile, seq.Key, newVal); err != nil {
		return ReorderResult{}, err
	}
	return ReorderResult{Written: []string{movedDatafile}}, nil
}

// NormalizeSequence re-spreads every record to clean step spacing (10, 20, 30…)
// in current sequence order: the manual cleanup for when many minimal-write
// moves have shrunk the gaps. Records already at their target are not rewritten.
func (m *Manager) NormalizeSequence(templateName string) (ReorderResult, error) {
	ordered, err := m.SequenceOrder(templateName)
	if err != nil {
		return ReorderResult{}, err
	}
	_, seq, err := m.sequenceContext(templateName)
	if err != nil {
		return ReorderResult{}, err
	}
	return m.normalizeSequence(templateName, ordered, seq.Key, template.SequenceStep(*seq))
}

func (m *Manager) normalizeSequence(templateName string, ordered []string, seqKey string, step int) (ReorderResult, error) {
	written := []string{}
	for idx, df := range ordered {
		want := (idx + 1) * step
		if cur, ok := m.sequenceOf(templateName, df, seqKey); ok && cur == want {
			continue
		}
		if err := m.rewriteSequence(templateName, df, seqKey, want); err != nil {
			return ReorderResult{}, err
		}
		written = append(written, df)
	}
	return ReorderResult{Normalized: true, Written: written}, nil
}

// sequenceContext loads the template and its sole sequence field, enforcing the
// ladder: collection on and a sequence field present.
func (m *Manager) sequenceContext(templateName string) (*template.Template, *template.Field, error) {
	tpl, err := m.templates.LoadTemplate(templateName)
	if err != nil {
		return nil, nil, fmt.Errorf("form: load template %q: %w", templateName, err)
	}
	if !tpl.EnableCollection {
		return nil, nil, errors.New("form: sequence ops need a collection template")
	}
	for i := range tpl.Fields {
		if tpl.Fields[i].Type == "sequence" {
			return tpl, &tpl.Fields[i], nil
		}
	}
	return nil, nil, errors.New("form: template has no sequence field")
}

func (m *Manager) sequenceOf(templateName, datafile, seqKey string) (int, bool) {
	loaded := m.storage.LoadForm(templateName, datafile)
	if loaded == nil {
		return 0, false
	}
	v, ok := loaded.Data[seqKey]
	if !ok {
		return 0, false
	}
	return toInt(v)
}

// rewriteSequence persists one record's new sequence value, leaving every other
// record's file untouched. Goes through SaveValues so meta, facets, and edges
// are preserved exactly as a normal save would.
func (m *Manager) rewriteSequence(templateName, datafile, seqKey string, val int) error {
	loaded := m.storage.LoadForm(templateName, datafile)
	if loaded == nil {
		return fmt.Errorf("form: reorder: record %q not found", datafile)
	}
	data := make(map[string]any, len(loaded.Data))
	maps.Copy(data, loaded.Data)
	data[seqKey] = val
	if _, err := m.SaveValues(templateName, SavePayload{
		Datafile: datafile,
		Values:   data,
		Meta:     loaded.Meta,
	}); err != nil {
		return err
	}
	return nil
}

// sequenceMidpoint returns the value to give a record dropped between prev and
// next. ok=false means there is no integer slot (adjacent values), so the
// caller must renumber.
func sequenceMidpoint(hasPrev bool, prev int, hasNext bool, next int, step int) (int, bool) {
	switch {
	case hasPrev && hasNext:
		if next-prev >= 2 {
			return prev + (next-prev)/2, true
		}
		return 0, false
	case hasPrev:
		return prev + step, true
	case hasNext:
		return next - step, true
	default:
		return 0, false
	}
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

func toInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	case float32:
		return int(x), true
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(x)); err == nil {
			return n, true
		}
	}
	return 0, false
}
