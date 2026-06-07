// Package datacore is a sparse rank-3 data tensor over form data.
//
// Its three modes are I (identity: which record), F (form: which field),
// and M (meaning: which context). A coordinate (i, f, m) addresses a cell
// holding a value or a reference to another identity. Storage is
// coordinate-list (COO) sparse: parallel columns of interned mode indices
// plus the value, so present cells iterate in storage order and a
// projection is a filter over columns rather than a pointer walk.
//
// Reading is done through perspectives: slices and reductions over the
// modes (project a field, scope a context, follow a reference, reduce
// along identity). Query and statistics are both perspectives over the
// same substrate; see design/datacore-perspectives.md.
//
// The module is self-contained: it ingests a plain Record and imports no
// other module, so it can be exercised and benchmarked in isolation.
package datacore

import "iter"

// Universal is the default meaning: a value that holds in every context.
// The M mode exists for context-varying values; ingest writes Universal
// and richer contexts attach later without changing the shape.
const Universal = "universal"

// sym is an interned mode label; the zero value is the empty/void label.
type sym uint32

// axis interns the labels of one mode, assigning each a stable sym.
type axis struct {
	id   map[string]sym
	name []string
}

func newAxis() *axis { return &axis{id: map[string]sym{"": 0}, name: []string{""}} }

func (a *axis) intern(label string) sym {
	if s, ok := a.id[label]; ok {
		return s
	}
	s := sym(len(a.name))
	a.id[label] = s
	a.name = append(a.name, label)
	return s
}

func (a *axis) lookup(label string) (sym, bool) { s, ok := a.id[label]; return s, ok }
func (a *axis) label(s sym) string              { return a.name[s] }

// Cell is one present coordinate. A value cell has empty Ref; a reference
// cell has empty Val and Ref set to the target identity label.
type Cell struct {
	I, F, M string
	Val     string
	Ref     string
}

// Tensor is a sparse rank-3 data tensor in coordinate-list form.
type Tensor struct {
	iax, fax, max *axis
	is, fs, ms    []sym
	val           []string
	ref           []sym // 0 = value cell; else target identity sym

	rootList []sym // top-level identities (records), in ingest order
	rootSet  map[sym]bool
	satSet   map[sym]bool   // satellite records: present as ref targets, not roots
	labels   map[sym]string // optional display label per identity
}

func New() *Tensor {
	return &Tensor{iax: newAxis(), fax: newAxis(), max: newAxis(), rootSet: map[sym]bool{}, satSet: map[sym]bool{}, labels: map[sym]string{}}
}

// isRecord reports whether s is a record identity (a root or a satellite), as
// opposed to a table/loop row. Both roots and satellites are records reached by
// the graph and Follow; only rows are sub-identities.
func (t *Tensor) isRecord(s sym) bool { return t.rootSet[s] || t.satSet[s] }

func (t *Tensor) nodeLabel(s sym) string {
	if l := t.labels[s]; l != "" {
		return l
	}
	return t.iax.label(s)
}

// markRoot records an identity as a top-level record, not a sub-identity
// reached by reference (a table or loop row). The default perspective
// reduces over roots, so loop rows don't inflate a form-level count while
// staying reachable by Follow.
func (t *Tensor) markRoot(id string) {
	s := t.iax.intern(id)
	if !t.rootSet[s] {
		t.rootSet[s] = true
		t.rootList = append(t.rootList, s)
	}
}

func (t *Tensor) Put(i, f, m, val string) {
	t.is = append(t.is, t.iax.intern(i))
	t.fs = append(t.fs, t.fax.intern(f))
	t.ms = append(t.ms, t.max.intern(m))
	t.val = append(t.val, val)
	t.ref = append(t.ref, 0)
}

func (t *Tensor) PutRef(i, f, m, target string) {
	t.is = append(t.is, t.iax.intern(i))
	t.fs = append(t.fs, t.fax.intern(f))
	t.ms = append(t.ms, t.max.intern(m))
	t.val = append(t.val, "")
	t.ref = append(t.ref, t.iax.intern(target))
}

// Len is the number of present cells (the count of nonzero coordinates).
func (t *Tensor) Len() int { return len(t.is) }

// Cells yields every present cell in storage order.
func (t *Tensor) Cells() iter.Seq[Cell] {
	return func(yield func(Cell) bool) {
		for k := range t.is {
			if !yield(t.cellAt(k)) {
				return
			}
		}
	}
}

func (t *Tensor) cellAt(k int) Cell {
	c := Cell{
		I:   t.iax.label(t.is[k]),
		F:   t.fax.label(t.fs[k]),
		M:   t.max.label(t.ms[k]),
		Val: t.val[k],
	}
	if t.ref[k] != 0 {
		c.Ref = t.iax.label(t.ref[k])
	}
	return c
}

// at is the point query for the (i, f, m) fiber: a full scan today, the
// place a value index attaches later.
func (t *Tensor) at(i, f, m sym) (string, sym, bool) {
	for k := range t.is {
		if t.is[k] == i && t.fs[k] == f && t.ms[k] == m {
			return t.val[k], t.ref[k], true
		}
	}
	return "", 0, false
}

// refsFrom returns every identity that i references under field f, in
// storage order. Edges are structural: read regardless of meaning scope.
func (t *Tensor) refsFrom(i, f sym) []sym {
	var out []sym
	for k := range t.is {
		if t.is[k] == i && t.fs[k] == f && t.ref[k] != 0 {
			out = append(out, t.ref[k])
		}
	}
	return out
}
