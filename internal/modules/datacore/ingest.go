package datacore

import "strconv"

// Record is the plain input datacore ingests, decoupled from storage and
// template: anything that can shape a record into fields, tables, and
// facets can feed the tensor.
type Record struct {
	ID     string
	Fields map[string]string
	Tables map[string][]map[string]string
	Facets map[string]string
	// Links are record-to-record references (the FDRM cross-record edge
	// Unfold walks recursively): a field mapped to the identities it points at.
	Links map[string][]string
	// Label is an optional display name for the record, used by the graph
	// view. Empty falls back to the ID.
	Label string
	// Color is an optional per-record node color (a CSS color string) used by
	// the graph view to tint this record's node. Empty leaves the node at its
	// default kind-based color. Carried per record because the loader sources
	// it from the record's template, so one template colors all its records.
	Color string
	// TableLabels gives an optional display label per table row (in row
	// order), keyed by table field, to label row nodes in the graph.
	TableLabels map[string][]string
	// Satellite marks a record present only as a reference target (a
	// cross-template relation's far side): its fields and tables are ingested
	// so Follow can reach them, but it is NOT a root, so root-based reductions
	// (Count, Distribution, Grid) over the primary template ignore it. Zero
	// value (false) is a normal root, so existing callers are unaffected.
	Satellite bool
}

// Ingest writes one record into the tensor.
//
// Scalar fields and facets land on the record identity at Universal meaning.
// Each table row becomes its own identity, so a row keeps a first-class
// identity instead of being flattened into the parent (the property an EAV
// index loses and has to rebuild from provenance). The record references each
// row under the table's field, so Follow walks record -> rows.
func (t *Tensor) Ingest(r Record) {
	if r.Satellite {
		t.satSet[t.iax.intern(r.ID)] = true // present as a reference target, not a root
	} else {
		t.markRoot(r.ID)
	}
	if r.Label != "" {
		t.labels[t.iax.intern(r.ID)] = r.Label
	}
	if r.Color != "" {
		t.colors[t.iax.intern(r.ID)] = r.Color
	}
	for f, v := range r.Fields {
		t.Put(r.ID, f, Universal, v)
	}
	for k, v := range r.Facets {
		t.Put(r.ID, facetField(k), Universal, v)
	}
	for field, rows := range r.Tables {
		labs := r.TableLabels[field]
		for n, row := range rows {
			rowID := r.ID + "#" + field + "#" + strconv.Itoa(n)
			for col, v := range row {
				t.Put(rowID, col, Universal, v)
			}
			if n < len(labs) && labs[n] != "" {
				t.labels[t.iax.intern(rowID)] = labs[n]
			}
			t.PutRef(r.ID, field, Universal, rowID)
		}
	}
	for field, targets := range r.Links {
		for _, tgt := range targets {
			t.PutRef(r.ID, field, Universal, tgt)
		}
	}
}

// facetField namespaces a facet key so it cannot collide with a field label.
// Facet keys are a legitimately open set (author-defined), so the prefix is
// built rather than enumerated from a fixed map.
func facetField(key string) string { return "facet:" + key }
