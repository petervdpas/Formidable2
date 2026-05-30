package datacore

// Predicate is a narrowing request the planner can satisfy from a fast store
// before the tensor is built. Equals matches scalar field values, Facets
// matches set facet options, Search is a full-text query. An empty Predicate
// narrows nothing, so the tensor is built from every record.
//
// A predicate is a row filter the planner pushes down to the index instead of
// the tensor selecting in memory: narrowing here is the same set as a Where on
// each named field, just decided by the fast store. The seam's contract is
// that the two agree (see the cross-check parity test).
type Predicate struct {
	Equals map[string]string // field key -> exact value
	Facets map[string]string // facet key -> selected option
	Search string            // full-text query
}

// Empty reports whether the predicate asks for any narrowing at all.
func (p Predicate) Empty() bool {
	return len(p.Equals) == 0 && len(p.Facets) == 0 && p.Search == ""
}

// Planner narrows the candidate identities for a template+predicate before the
// tensor is built. It is the seam to a fast store (the SQLite index): it
// returns the ids that match, so only those records are materialized instead
// of every one.
//
// narrowed=false means "not narrowed, load everything" - an empty predicate,
// no planner, or a predicate the store cannot push down. That keeps the result
// correct (just not accelerated): the caller falls back to the full build.
type Planner interface {
	Plan(template string, pred Predicate) (ids []string, narrowed bool, err error)
}

// SubsetLoader is a Loader that can materialize only a named subset of records.
// When the planner narrows, the service loads just the matching ids through
// this path. A Loader that doesn't implement it still works: loadSubset falls
// back to loading every record and keeping the id set, which is correct but
// reads everything (so the acceleration is lost, not the answer).
type SubsetLoader interface {
	Loader
	LoadSubset(ids []string) ([]Record, error)
}

// buildFromRecords ingests a materialized record slice into a fresh tensor.
func buildFromRecords(recs []Record) *Tensor {
	t := New()
	for _, r := range recs {
		t.Ingest(r)
	}
	return t
}

// loadSubset materializes just the named ids. A SubsetLoader does it directly;
// any other Loader is read in full and filtered by the id set so the seam
// works against fixtures and future loaders without forcing the interface.
func loadSubset(l Loader, ids []string) ([]Record, error) {
	if sl, ok := l.(SubsetLoader); ok {
		return sl.LoadSubset(ids)
	}
	all, err := l.Records()
	if err != nil {
		return nil, err
	}
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	out := make([]Record, 0, len(ids))
	for _, r := range all {
		if want[r.ID] {
			out = append(out, r)
		}
	}
	return out, nil
}

// buildNarrowed builds a tensor for the template, narrowed by pred when a
// planner is wired and the predicate is non-empty and pushable. With no
// planner, an empty predicate, or a planner that declines to narrow, it builds
// from every record - identical to Build. This is the planner seam: the index
// narrows which records exist, the tensor computes over them.
func buildNarrowed(loader Loader, planner Planner, template string, pred Predicate) (*Tensor, error) {
	if planner == nil || pred.Empty() {
		return Build(loader)
	}
	ids, narrowed, err := planner.Plan(template, pred)
	if err != nil {
		return nil, err
	}
	if !narrowed {
		return Build(loader)
	}
	recs, err := loadSubset(loader, ids)
	if err != nil {
		return nil, err
	}
	return buildFromRecords(recs), nil
}
