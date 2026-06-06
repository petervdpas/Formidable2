package relation

import (
	"sort"
	"strings"
)

// ReconcileReport is the outcome of a self-heal pass.
type ReconcileReport struct {
	// Created lists counterparts that were missing and got recreated from the surviving side.
	Created []Counterpart `json:"created"`
	// Conflicts lists pairs present on both sides whose cardinalities disagree. These are left
	// untouched: with no owner there is no safe way to pick a winner, so they are surfaced for the
	// user to resolve (doctor-style: heal the unambiguous, flag the outliers).
	Conflicts []Conflict `json:"conflicts"`
}

// Counterpart is one recreated relation half (written into Template, pointing at To).
type Counterpart struct {
	Template    string      `json:"template"`
	To          string      `json:"to"`
	Cardinality Cardinality `json:"cardinality"`
}

// Conflict is a pair stored on both sides whose cardinalities are not each other's inverse.
type Conflict struct {
	A            string      `json:"a"`
	ACardinality Cardinality `json:"a_cardinality"`
	B            string      `json:"b"`
	BCardinality Cardinality `json:"b_cardinality"`
}

// Reconcile scans every relation file and makes the graph symmetric: for each declared half it
// ensures the counterpart exists on the other side with the flipped cardinality, recreating it if
// missing (this is the self-heal: a lost or half-deleted side is rebuilt from the survivor). It
// never deletes (deletion is SetRelations' job, which removes both halves). When both halves exist
// but their cardinalities disagree it reports a conflict and leaves them alone. Tolerant: a missing
// relations dir means nothing to do.
func (m *Manager) Reconcile() (ReconcileReport, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	files, err := m.fs.ListFiles(m.dir)
	if err != nil {
		return ReconcileReport{}, nil
	}

	// Snapshot the stored graph: template -> (to -> relation half).
	graph := map[string]map[string]Relation{}
	for _, f := range files {
		if !strings.HasSuffix(f, ".yaml") {
			continue
		}
		rels, err := m.getRelationsLocked(f)
		if err != nil {
			return ReconcileReport{}, err
		}
		tos := make(map[string]Relation, len(rels))
		for _, r := range rels {
			tos[r.To] = r
		}
		graph[f] = tos
	}

	var report ReconcileReport
	additions := map[string][]Relation{} // template -> counterparts to append
	seen := map[[2]string]bool{}

	for a := range graph {
		for b, ra := range graph[a] {
			if a == b {
				continue // self-relation is its own counterpart
			}
			key := pairKey(a, b)
			if seen[key] {
				continue
			}
			seen[key] = true

			rb, hasB := graph[b][a]
			switch {
			case !hasB:
				want := ra.Cardinality.inverse()
				additions[b] = append(additions[b], Relation{To: a, Cardinality: want, Inverse: !ra.Inverse})
				report.Created = append(report.Created, Counterpart{Template: b, To: a, Cardinality: want})
			case rb.Cardinality != ra.Cardinality.inverse():
				report.Conflicts = append(report.Conflicts, Conflict{
					A: a, ACardinality: ra.Cardinality, B: b, BCardinality: rb.Cardinality,
				})
			}
		}
	}

	// Apply additions in a stable order, preserving each file's existing entries (and their edges).
	targets := make([]string, 0, len(additions))
	for tpl := range additions {
		targets = append(targets, tpl)
	}
	sort.Strings(targets)
	for _, tpl := range targets {
		rels, err := m.getRelationsLocked(tpl)
		if err != nil {
			return ReconcileReport{}, err
		}
		rels = append(rels, additions[tpl]...)
		if err := m.saveRelationsLocked(tpl, rels); err != nil {
			return ReconcileReport{}, err
		}
	}
	return report, nil
}

// pairKey is an order-independent key for an unordered template pair.
func pairKey(a, b string) [2]string {
	if a <= b {
		return [2]string{a, b}
	}
	return [2]string{b, a}
}
