package datacore

import (
	"iter"
	"sort"
)

// A Perspective is a view of the tensor: a meaning scope plus a working set
// of identities. Narrow with Where, step references with Follow, then read a
// shape (Project, Distribution, Cross, Count). Query and statistics are both
// spelled in this algebra.
type Perspective struct {
	t     *Tensor
	scope sym
	ids   []sym // nil = every identity in the tensor
}

// View opens a perspective at Universal meaning over all identities.
func (t *Tensor) View() *Perspective {
	return &Perspective{t: t, scope: t.max.intern(Universal)}
}

// Scope fixes the M mode: subsequent reads see values in this context.
func (p *Perspective) Scope(m string) *Perspective {
	p.scope = p.t.max.intern(m)
	return p
}

// Pred tests a cell value during selection.
type Pred func(string) bool

// Where keeps identities whose field value at the current scope passes pred.
func (p *Perspective) Where(field string, pred Pred) *Perspective {
	f, ok := p.t.fax.lookup(field)
	if !ok {
		p.ids = []sym{}
		return p
	}
	kept := make([]sym, 0)
	for _, i := range p.identities() {
		if v, _, ok := p.t.at(i, f, p.scope); ok && pred(v) {
			kept = append(kept, i)
		}
	}
	p.ids = kept
	return p
}

// Follow moves the working set one hop: from the current identities to the
// identities they reference under field (record -> table rows, or a
// cross-record link). Targets are deduplicated, keeping first-seen order.
func (p *Perspective) Follow(field string) *Perspective {
	f, ok := p.t.fax.lookup(field)
	if !ok {
		p.ids = []sym{}
		return p
	}
	seen := map[sym]bool{}
	out := make([]sym, 0)
	for _, i := range p.identities() {
		for _, tgt := range p.t.refsFrom(i, f) {
			if !seen[tgt] {
				seen[tgt] = true
				out = append(out, tgt)
			}
		}
	}
	p.ids = out
	return p
}

// Node is one identity reached by Unfold: the identity label, its depth in
// hops from the seed that reached it, and the path of identity labels from
// that seed to here (Path[0] is the seed, the last entry is this node).
type Node struct {
	ID    string
	Depth int
	Path  []string
}

// Unfold walks field recursively from the working set, breadth-first, yielding
// each identity the first time it is reached. Bounded by maxDepth (hops from a
// seed; <= 0 yields nothing) and visited-guarded so cycles terminate. Seeds
// themselves are not yielded, only identities reached by a hop. Lazy: breaking
// the range stops exploration, so a wide closure is never fully materialized.
func (p *Perspective) Unfold(field string, maxDepth int) iter.Seq[Node] {
	return func(yield func(Node) bool) {
		f, ok := p.t.fax.lookup(field)
		if !ok || maxDepth <= 0 {
			return
		}
		type frontier struct {
			id   sym
			path []string
		}
		seeds := p.identities()
		visited := make(map[sym]bool, len(seeds))
		queue := make([]frontier, 0, len(seeds))
		for _, s := range seeds {
			visited[s] = true
			queue = append(queue, frontier{id: s, path: []string{p.t.iax.label(s)}})
		}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			if len(cur.path)-1 >= maxDepth {
				continue
			}
			for _, tgt := range p.t.refsFrom(cur.id, f) {
				if visited[tgt] {
					continue
				}
				visited[tgt] = true
				path := append(append([]string{}, cur.path...), p.t.iax.label(tgt))
				if !yield(Node{ID: p.t.iax.label(tgt), Depth: len(path) - 1, Path: path}) {
					return
				}
				queue = append(queue, frontier{id: tgt, path: path})
			}
		}
	}
}

// Row is one projected record: its identity label and the values of the
// requested fields in order. A missing coordinate reads as blank.
type Row struct {
	ID    string
	Cells []string
}

// Project reads the requested fields for every identity in the working set,
// sorted by identity for a stable result.
func (p *Perspective) Project(fields ...string) []Row {
	fs := make([]sym, len(fields))
	for j, f := range fields {
		fs[j], _ = p.t.fax.lookup(f)
	}
	ids := p.identities()
	rows := make([]Row, 0, len(ids))
	for _, i := range ids {
		r := Row{ID: p.t.iax.label(i), Cells: make([]string, len(fs))}
		for j, f := range fs {
			if v, _, ok := p.t.at(i, f, p.scope); ok {
				r.Cells[j] = v
			}
		}
		rows = append(rows, r)
	}
	sort.Slice(rows, func(a, b int) bool { return rows[a].ID < rows[b].ID })
	return rows
}

// Bucket is one value of a field and the number of identities carrying it.
type Bucket struct {
	Value string
	Count int
}

// Distribution reduces along the I mode: per distinct value of field at the
// current scope, the count of identities carrying it. Blank values are skipped
// (absence is not a category, matching the index). Buckets sorted by value.
func (p *Perspective) Distribution(field string) []Bucket {
	counts := map[string]int{}
	if f, ok := p.t.fax.lookup(field); ok {
		for _, i := range p.identities() {
			if v, _, ok := p.t.at(i, f, p.scope); ok && v != "" {
				counts[v]++
			}
		}
	}
	out := make([]Bucket, 0, len(counts))
	for v, n := range counts {
		out = append(out, Bucket{Value: v, Count: n})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Value < out[b].Value })
	return out
}

// CrossCell is one cell of a cross-tab: the count of identities carrying both
// the Row value (of the row field) and the Col value (of the column field).
type CrossCell struct {
	Row   string
	Col   string
	Count int
}

// CrossTab is a rank-2 contingency: counts binned by two fields. Rows and
// Cols are the sorted distinct axis values; Cells holds the nonzero cells
// sorted by row then col, so a caller can render a dense grid (zeros for
// absent cells) from the axes.
type CrossTab struct {
	Rows  []string
	Cols  []string
	Cells []CrossCell
}

// Count returns the cell count at (row, col), zero if absent.
func (c CrossTab) Count(row, col string) int {
	for _, cell := range c.Cells {
		if cell.Row == row && cell.Col == col {
			return cell.Count
		}
	}
	return 0
}

// Cross reduces along the I mode into a rank-2 contingency: per (rowField,
// colField) value pair, the count of identities carrying both at the current
// scope. Identities missing either value are dropped (complete-case). The row
// and column margins are the two rank-1 Distributions.
func (p *Perspective) Cross(rowField, colField string) CrossTab {
	rf, rok := p.t.fax.lookup(rowField)
	cf, cok := p.t.fax.lookup(colField)
	var ct CrossTab
	if !rok || !cok {
		return ct
	}
	counts := map[[2]string]int{}
	rowSet, colSet := map[string]bool{}, map[string]bool{}
	for _, i := range p.identities() {
		rv, _, ok := p.t.at(i, rf, p.scope)
		if !ok || rv == "" {
			continue
		}
		cv, _, ok := p.t.at(i, cf, p.scope)
		if !ok || cv == "" {
			continue
		}
		counts[[2]string{rv, cv}]++
		rowSet[rv], colSet[cv] = true, true
	}
	ct.Rows, ct.Cols = sortedKeys(rowSet), sortedKeys(colSet)
	for k, n := range counts {
		ct.Cells = append(ct.Cells, CrossCell{Row: k[0], Col: k[1], Count: n})
	}
	sort.Slice(ct.Cells, func(a, b int) bool {
		if ct.Cells[a].Row != ct.Cells[b].Row {
			return ct.Cells[a].Row < ct.Cells[b].Row
		}
		return ct.Cells[a].Col < ct.Cells[b].Col
	})
	return ct
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Count is the size of the working set.
func (p *Perspective) Count() int { return len(p.identities()) }

// identities returns the working set. By default it is the root identities
// (records) in ingest order, so loop rows reached by Follow don't inflate a
// form-level reduction. When no roots are marked (raw Put usage without
// Ingest), it falls back to every distinct identity in first-seen order, but
// never satellites: a primary template with zero records that pulls in
// cross-template satellites must still reduce to nothing, not to the satellites.
func (p *Perspective) identities() []sym {
	if p.ids != nil {
		return p.ids
	}
	if len(p.t.rootList) > 0 {
		return p.t.rootList
	}
	seen := map[sym]bool{}
	out := make([]sym, 0)
	for k := range p.t.is {
		if s := p.t.is[k]; !seen[s] && !p.t.satSet[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
