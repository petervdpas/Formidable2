package relation

import (
	"slices"
	"sort"
	"strings"
)

// ReconcileReport is the outcome of a self-heal pass.
type ReconcileReport struct {
	// Created lists relation counterparts that were missing and got recreated from the surviving
	// side (with that side's edges reversed onto the new half).
	Created []Counterpart `json:"created"`
	// EdgesHealed counts reversed edges added to bring the two sides' edge sets back into agreement.
	EdgesHealed int `json:"edges_healed"`
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

// Reconcile scans every relation file and makes the graph symmetric, in full: for each declared half
// it ensures the counterpart exists on the other side with the flipped cardinality (recreating it,
// with the surviving side's edges reversed, if missing), and for pairs present on both sides it fills
// in any missing reversed edges so both sides hold the same links. This is the self-heal: a lost or
// half-deleted side, declaration or edges, is rebuilt from the survivor. It never deletes (deletion
// is the AddEdge/RemoveEdge/SetRelations job, which touches both halves). When both halves exist but
// their cardinalities disagree it reports a conflict and leaves them alone. Tolerant: a missing
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
	declAdditions := map[string][]Relation{}        // template -> new relation halves (with reversed edges)
	edgeAdditions := map[string]map[string][]Edge{} // template -> to -> reversed edges to add
	seen := map[[2]string]bool{}

	addEdge := func(tpl, to string, e Edge) {
		if edgeAdditions[tpl] == nil {
			edgeAdditions[tpl] = map[string][]Edge{}
		}
		edgeAdditions[tpl][to] = append(edgeAdditions[tpl][to], e)
		report.EdgesHealed++
	}

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
			if !hasB {
				want := ra.Cardinality.inverse()
				declAdditions[b] = append(declAdditions[b], Relation{
					To: a, Cardinality: want, Inverse: !ra.Inverse, Edges: reverseEdges(ra.Edges),
				})
				report.Created = append(report.Created, Counterpart{Template: b, To: a, Cardinality: want})
				continue
			}
			if rb.Cardinality != ra.Cardinality.inverse() {
				report.Conflicts = append(report.Conflicts, Conflict{
					A: a, ACardinality: ra.Cardinality, B: b, BCardinality: rb.Cardinality,
				})
			}
			// Edge heal both ways: each side should hold the reverse of the other's edges.
			for _, e := range ra.Edges {
				rev := Edge{From: e.To, To: e.From}
				if !slices.Contains(rb.Edges, rev) {
					addEdge(b, a, rev)
				}
			}
			for _, e := range rb.Edges {
				rev := Edge{From: e.To, To: e.From}
				if !slices.Contains(ra.Edges, rev) {
					addEdge(a, b, rev)
				}
			}
		}
	}

	// Apply new relation halves (each already carries its reversed edges).
	for _, tpl := range sortedKeys(declAdditions) {
		rels, err := m.getRelationsLocked(tpl)
		if err != nil {
			return ReconcileReport{}, err
		}
		rels = append(rels, declAdditions[tpl]...)
		if err := m.saveRelationsLocked(tpl, rels); err != nil {
			return ReconcileReport{}, err
		}
	}
	// Fill missing reversed edges into existing halves.
	for _, tpl := range sortedEdgeKeys(edgeAdditions) {
		rels, err := m.getRelationsLocked(tpl)
		if err != nil {
			return ReconcileReport{}, err
		}
		changed := false
		for to, edges := range edgeAdditions[tpl] {
			k := relationIndex(rels, to)
			if k < 0 {
				continue
			}
			for _, e := range edges {
				if !slices.Contains(rels[k].Edges, e) {
					rels[k].Edges = append(rels[k].Edges, e)
					changed = true
				}
			}
		}
		if changed {
			if err := m.saveRelationsLocked(tpl, rels); err != nil {
				return ReconcileReport{}, err
			}
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

func reverseEdges(edges []Edge) []Edge {
	if len(edges) == 0 {
		return nil
	}
	out := make([]Edge, len(edges))
	for i, e := range edges {
		out[i] = Edge{From: e.To, To: e.From}
	}
	return out
}

func sortedKeys(m map[string][]Relation) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedEdgeKeys(m map[string]map[string][]Edge) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
