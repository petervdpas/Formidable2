package datacore

import "strconv"

// Record is the plain input datacore ingests. It is decoupled from storage
// and template by design: anything that can shape a record into fields,
// tables, and facets can feed the tensor.
type Record struct {
	ID     string
	Fields map[string]string
	Tables map[string][]map[string]string
	Facets map[string]string
	// Links are record-to-record references: a field mapped to the
	// identities it points at. This is the FDRM cross-record edge, the
	// thing Unfold walks recursively.
	Links map[string][]string
}

// Ingest writes one record into the tensor.
//
// Scalar fields and facets land on the record identity at Universal meaning.
// Each table row becomes its own identity, so a row keeps a first-class
// identity instead of being flattened into the parent (the property an EAV
// index loses and the matrix has to rebuild from provenance). The record
// references each row under the table's field, so Follow walks record -> rows.
func (t *Tensor) Ingest(r Record) {
	t.markRoot(r.ID)
	for f, v := range r.Fields {
		t.Put(r.ID, f, Universal, v)
	}
	for k, v := range r.Facets {
		t.Put(r.ID, facetField(k), Universal, v)
	}
	for field, rows := range r.Tables {
		for n, row := range rows {
			rowID := r.ID + "#" + field + "#" + strconv.Itoa(n)
			for col, v := range row {
				t.Put(rowID, col, Universal, v)
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
// built rather than enumerated.
func facetField(key string) string { return "facet:" + key }
