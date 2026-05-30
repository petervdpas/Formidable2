package app

import (
	"database/sql"
	"fmt"

	"github.com/petervdpas/formidable2/internal/modules/datacore"
	"github.com/petervdpas/formidable2/internal/modules/index"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// columnNamer maps a table field's positional column to its value-key, the
// reverse of stat's ColumnResolver. The adapter depends on this narrow seam
// (not the whole template manager) so the column paths are unit-testable with
// a fake; templateColumnNamer is the production implementation.
type columnNamer interface {
	ColumnKey(tplFile, fieldKey string, col int) (string, bool)
}

// datacoreStatIndex satisfies stat.Index with the datacore tensor behind it,
// so stat can compute over perspectives instead of EAV aggregates. It is the
// drop-in for *index.Manager on the stat seam: same interface, same result
// shapes (index.Bucket / CrossCell / StatRawRow), different engine. Read-only,
// additive, and not wired anywhere yet (the swap is a later, flag-gated step).
//
// The one real translation is the column language: stat.Index addresses a
// table column positionally (fieldKey, col), datacore addresses it by name
// (table, columnKey). The columnNamer reverses the template's column options to
// bridge the two, and a Follow on the table reaches the rows.
type datacoreStatIndex struct {
	dc   *datacore.Service
	cols columnNamer
}

func newDatacoreStatIndex(dc *datacore.Service, cols columnNamer) *datacoreStatIndex {
	return &datacoreStatIndex{dc: dc, cols: cols}
}

// resolveColumn turns a (fieldKey, col) pair into the datacore (follow, field)
// pair: a scalar field follows nothing and reads the field; a table column
// follows the table and reads the named column. ok=false means the column
// index does not exist, which the index answers with an empty result rather
// than a failure, so the drop-in does the same.
func (a *datacoreStatIndex) resolveColumn(tplFile, fieldKey string, col *int) (follow, field string, ok bool) {
	if col == nil {
		return "", fieldKey, true
	}
	colKey, found := a.cols.ColumnKey(tplFile, fieldKey, *col)
	if !found {
		return "", "", false
	}
	return fieldKey, colKey, true
}

func (a *datacoreStatIndex) TotalForms(tplFile string) (int, error) {
	return a.dc.Count(tplFile, "")
}

func (a *datacoreStatIndex) ValueDistribution(tplFile, fieldKey string, col *int) ([]index.Bucket, error) {
	follow, field, ok := a.resolveColumn(tplFile, fieldKey, col)
	if !ok {
		return nil, nil
	}
	bs, err := a.dc.Distribution(tplFile, follow, field)
	if err != nil {
		return nil, err
	}
	return bucketsToIndex(bs), nil
}

func (a *datacoreStatIndex) NumericValues(tplFile, fieldKey string, col *int) ([]float64, error) {
	follow, field, ok := a.resolveColumn(tplFile, fieldKey, col)
	if !ok {
		return nil, nil
	}
	agg, err := a.dc.Aggregate(tplFile, follow, field)
	if err != nil {
		return nil, err
	}
	return agg.Values, nil
}

func (a *datacoreStatIndex) FacetDistribution(tplFile, facetKey string) ([]index.Bucket, error) {
	bs, err := a.dc.Distribution(tplFile, "", facetField(facetKey))
	if err != nil {
		return nil, err
	}
	return bucketsToIndex(bs), nil
}

func (a *datacoreStatIndex) FacetCross(tplFile, keyA, keyB string) ([]index.CrossCell, error) {
	ct, err := a.dc.Cross(tplFile, "", facetField(keyA), facetField(keyB))
	if err != nil {
		return nil, err
	}
	out := make([]index.CrossCell, len(ct.Cells))
	for i, c := range ct.Cells {
		out[i] = index.CrossCell{A: c.Row, B: c.Col, Count: c.Count}
	}
	return out, nil
}

func (a *datacoreStatIndex) DateSeries(tplFile, fieldKey string, col *int, period string) ([]index.Bucket, error) {
	follow, field, ok := a.resolveColumn(tplFile, fieldKey, col)
	if !ok {
		return nil, nil
	}
	s, err := a.dc.DateSeries(tplFile, follow, field, period)
	if err != nil {
		return nil, err
	}
	out := make([]index.Bucket, len(s.Buckets))
	for i, b := range s.Buckets {
		out[i] = index.Bucket{Label: b.Value, Count: b.Count}
	}
	return out, nil
}

func (a *datacoreStatIndex) AggregateRaw(tplFile string, dims []index.AggDim, nums []index.AggNum, filters []index.AggFilter) ([]index.StatRawRow, error) {
	gdims, err := a.toGridDims(tplFile, dims)
	if err != nil {
		return nil, err
	}
	gnums, err := a.toGridNums(tplFile, nums)
	if err != nil {
		return nil, err
	}
	gfilters, err := toGridFilters(filters)
	if err != nil {
		return nil, err
	}
	rows, err := a.dc.AggregateRaw(tplFile, gdims, gnums, gfilters)
	if err != nil {
		return nil, err
	}
	out := make([]index.StatRawRow, len(rows))
	for i, r := range rows {
		nums := make([]sql.NullFloat64, len(r.Nums))
		for j, n := range r.Nums {
			nums[j] = sql.NullFloat64{Float64: n.Value, Valid: n.OK}
		}
		out[i] = index.StatRawRow{Form: r.Form, Dims: r.Dims, Nums: nums}
	}
	return out, nil
}

func (a *datacoreStatIndex) toGridDims(tplFile string, dims []index.AggDim) ([]datacore.GridDim, error) {
	out := make([]datacore.GridDim, len(dims))
	for i, d := range dims {
		if d.Kind == "facet" {
			out[i] = datacore.GridDim{Field: facetField(d.Key), DateWidth: d.DateWidth}
			continue
		}
		field, table, err := a.gridField(tplFile, d.Key, d.Col)
		if err != nil {
			return nil, err
		}
		out[i] = datacore.GridDim{Field: field, Table: table, DateWidth: d.DateWidth}
	}
	return out, nil
}

func (a *datacoreStatIndex) toGridNums(tplFile string, nums []index.AggNum) ([]datacore.GridNum, error) {
	out := make([]datacore.GridNum, len(nums))
	for i, n := range nums {
		field, table, err := a.gridField(tplFile, n.Key, n.Col)
		if err != nil {
			return nil, err
		}
		out[i] = datacore.GridNum{Field: field, Table: table}
	}
	return out, nil
}

// gridField maps a (key, col) source to datacore's (field, table): a scalar
// reads the field with no table, a table column reads the named column off its
// table. An unknown column is an error here (unlike the distribution path's
// empty result) because a malformed dim would otherwise silently drop a whole
// grid axis, which a parity test must catch loudly.
func (a *datacoreStatIndex) gridField(tplFile, key string, col *int) (field, table string, err error) {
	if col == nil {
		return key, "", nil
	}
	colKey, ok := a.cols.ColumnKey(tplFile, key, *col)
	if !ok {
		return "", "", fmt.Errorf("datacore stat: no column %d in table %q of %q", *col, key, tplFile)
	}
	return colKey, key, nil
}

// toGridFilters translates the field and facet filters stat emits. Table-column
// filters (Col set) are not yet a datacore root-level filter, so they error
// rather than silently under-filter; that path is out of scope for the first
// cut and is surfaced, not swallowed.
func toGridFilters(filters []index.AggFilter) ([]datacore.GridFilter, error) {
	out := make([]datacore.GridFilter, 0, len(filters))
	for _, f := range filters {
		if f.Col != nil {
			return nil, fmt.Errorf("datacore stat: table-column filters not supported yet (table %q col %d)", f.Key, *f.Col)
		}
		field := f.Key
		if f.Kind == "facet" {
			field = facetField(f.Key)
		}
		out = append(out, datacore.GridFilter{Field: field, Op: f.Op, Value: f.Value})
	}
	return out, nil
}

func bucketsToIndex(bs []datacore.Bucket) []index.Bucket {
	out := make([]index.Bucket, len(bs))
	for i, b := range bs {
		out[i] = index.Bucket{Label: b.Value, Count: b.Count}
	}
	return out
}

// facetField namespaces a facet key the way the datacore loader does, so a
// facet distribution/cross reads the "facet:<key>" cells the tensor carries.
func facetField(key string) string { return "facet:" + key }

// templateColumnNamer is the production columnNamer: it resolves a column key
// through the live template manager. A template that fails to load yields
// ok=false, which the distribution path treats as an empty result.
type templateColumnNamer struct{ tpl *template.Manager }

func (n templateColumnNamer) ColumnKey(tplFile, fieldKey string, col int) (string, bool) {
	t, err := n.tpl.LoadTemplate(tplFile)
	if err != nil {
		return "", false
	}
	return columnKeyIn(t, fieldKey, col)
}

// columnKeyIn is the reverse of columnIndexIn: the value-key of the table
// field's column at position col, or ok=false when out of range. It bridges
// stat.Index's positional column to datacore's named column.
func columnKeyIn(t *template.Template, fieldKey string, col int) (string, bool) {
	for _, f := range t.Fields {
		if f.Key != fieldKey {
			continue
		}
		if col < 0 || col >= len(f.Options) {
			return "", false
		}
		if m, ok := f.Options[col].(map[string]any); ok {
			if v, _ := m["value"].(string); v != "" {
				return v, true
			}
		}
		return "", false
	}
	return "", false
}
