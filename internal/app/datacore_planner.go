package app

import (
	"sort"

	"github.com/petervdpas/formidable2/internal/modules/datacore"
	"github.com/petervdpas/formidable2/internal/modules/index"
)

// datacoreIndexPlanner is the planner seam between datacore and the SQLite
// index. Given a narrowing predicate it returns the datafile names that match,
// so the tensor ingests only those forms instead of every one. This is the
// "index narrows, datacore computes" split: the index does what an EAV store
// is fast at (find/filter/FTS), the tensor does what it is fast at (compute).
//
// Each condition is pushed to its natural index path and the results are
// intersected (the predicate is an AND): a full-text Search hits the FTS5
// index, a Facet condition filters the indexed facet rows, a field Equals hits
// form_values. A predicate that constrains nothing the index can answer
// returns narrowed=false, so datacore falls back to a full build (correct,
// just unaccelerated).
type datacoreIndexPlanner struct {
	idx *index.Manager
}

func newDatacoreIndexPlanner(idx *index.Manager) *datacoreIndexPlanner {
	return &datacoreIndexPlanner{idx: idx}
}

func (p *datacoreIndexPlanner) Plan(template string, pred datacore.Predicate) ([]string, bool, error) {
	if p.idx == nil || pred.Empty() {
		return nil, false, nil
	}

	// set is nil until the first condition constrains it; after that it is the
	// running intersection. A nil set at the end means no condition was
	// pushable, so we decline rather than claim a (wrong) "everything".
	var set map[string]bool
	intersect := func(match []string) {
		next := make(map[string]bool, len(match))
		for _, f := range match {
			if set == nil || set[f] {
				next[f] = true
			}
		}
		set = next
	}

	if pred.Search != "" {
		rows, err := p.idx.SearchForms(template, pred.Search, index.QueryOpts{})
		if err != nil {
			return nil, false, err
		}
		intersect(filenamesOf(rows))
	}

	if len(pred.Facets) > 0 {
		rows, err := p.idx.ListForms(template, index.QueryOpts{})
		if err != nil {
			return nil, false, err
		}
		for key, want := range pred.Facets {
			match := make([]string, 0)
			for _, r := range rows {
				if facetSelected(r, key, want) {
					match = append(match, r.Filename)
				}
			}
			intersect(match)
		}
	}

	for key, want := range pred.Equals {
		match, err := p.idx.FormsWithValue(template, key, want)
		if err != nil {
			return nil, false, err
		}
		intersect(match)
	}

	if set == nil {
		return nil, false, nil
	}

	ids := make([]string, 0, len(set))
	for f := range set {
		ids = append(ids, f)
	}
	sort.Strings(ids)
	return ids, true, nil
}

func filenamesOf(rows []index.FormRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Filename
	}
	return out
}

func facetSelected(r index.FormRow, key, want string) bool {
	for _, f := range r.Facets {
		if f.Key == key {
			return f.Set && f.Selected == want
		}
	}
	return false
}
