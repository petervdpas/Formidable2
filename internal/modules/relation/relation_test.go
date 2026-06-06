package relation

import (
	"os"
	"path/filepath"
	"testing"
)

type memFS struct {
	files map[string]string
	dirs  map[string]bool
}

func newMemFS() *memFS { return &memFS{files: map[string]string{}, dirs: map[string]bool{}} }

func (f *memFS) FileExists(p string) bool       { _, ok := f.files[p]; return ok || f.dirs[p] }
func (f *memFS) EnsureDirectory(p string) error { f.dirs[p] = true; return nil }
func (f *memFS) SaveFile(p, c string) error     { f.files[p] = c; return nil }
func (f *memFS) ListFiles(dir string) ([]string, error) {
	var out []string
	for p := range f.files {
		if filepath.Dir(p) == dir {
			out = append(out, filepath.Base(p))
		}
	}
	return out, nil
}
func (f *memFS) LoadFile(p string) (string, error) {
	c, ok := f.files[p]
	if !ok {
		return "", os.ErrNotExist
	}
	return c, nil
}

type fakeCatalog struct {
	collections map[string]bool
	records     map[string]map[string]bool
}

func (c fakeCatalog) IsCollection(t string) bool { return c.collections[t] }
func (c fakeCatalog) RecordExists(t, id string) bool {
	r := c.records[t]
	return r != nil && r[id]
}

func fullCatalog() fakeCatalog {
	return fakeCatalog{
		collections: map[string]bool{"project.yaml": true, "person.yaml": true, "team.yaml": true},
		records: map[string]map[string]bool{
			"project.yaml": {"p1": true, "p2": true, "p3": true},
			"person.yaml":  {"u1": true, "u2": true},
		},
	}
}

func newMgr() *Manager { return NewManager(newMemFS(), "/ctx/relations", fullCatalog()) }

func TestCardinalities(t *testing.T) {
	got := Cardinalities()
	want := []Cardinality{OneToOne, OneToMany, ManyToOne, ManyToMany}
	if len(got) != len(want) {
		t.Fatalf("got %d cardinalities, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("cardinality[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCardinalityOptions_CarryLabelKeys(t *testing.T) {
	opts := CardinalityOptions()
	if len(opts) != len(Cardinalities()) {
		t.Fatalf("got %d options, want %d", len(opts), len(Cardinalities()))
	}
	defaults := 0
	for i, o := range opts {
		if o.Value != Cardinalities()[i] {
			t.Errorf("option[%d] value = %q, want %q", i, o.Value, Cardinalities()[i])
		}
		if o.LabelKey == "" {
			t.Errorf("option %q has no label key", o.Value)
		}
		if o.Default {
			defaults++
			if o.Value != defaultCardinality {
				t.Errorf("default flagged on %q, want %q", o.Value, defaultCardinality)
			}
		}
	}
	if defaults != 1 {
		t.Errorf("want exactly one default option, got %d", defaults)
	}
}

func TestCardinality_Inverse(t *testing.T) {
	cases := map[Cardinality]Cardinality{
		OneToOne:   OneToOne,
		OneToMany:  ManyToOne,
		ManyToOne:  OneToMany,
		ManyToMany: ManyToMany,
	}
	for in, want := range cases {
		if got := in.inverse(); got != want {
			t.Errorf("%q.inverse() = %q, want %q", in, got, want)
		}
	}
}

func TestSetRelations_MirrorsInverseToTarget(t *testing.T) {
	m := newMgr()
	if err := m.SetRelations("project.yaml", []Relation{{To: "person.yaml", Cardinality: OneToMany}}); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, _ := m.GetRelations("project.yaml")
	if len(got) != 1 || got[0].To != "person.yaml" || got[0].Cardinality != OneToMany {
		t.Fatalf("owner side wrong: %+v", got)
	}
	mirror, _ := m.GetRelations("person.yaml")
	if len(mirror) != 1 || mirror[0].To != "project.yaml" || mirror[0].Cardinality != ManyToOne {
		t.Fatalf("mirror should be project.yaml many-to-one: %+v", mirror)
	}
}

func TestSetRelations_SymmetricFromEitherSide(t *testing.T) {
	m := newMgr()
	// Author the many-to-one side; the non-inverse one-to-many counterpart must appear.
	if err := m.SetRelations("person.yaml", []Relation{{To: "project.yaml", Cardinality: ManyToOne}}); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, _ := m.GetRelations("project.yaml")
	if len(got) != 1 || got[0].To != "person.yaml" || got[0].Cardinality != OneToMany {
		t.Fatalf("expected one-to-many counterpart on project: %+v", got)
	}
}

func TestSetRelations_EditUpdatesMirror(t *testing.T) {
	m := newMgr()
	_ = m.SetRelations("project.yaml", []Relation{{To: "person.yaml", Cardinality: OneToMany}})
	_ = m.SetRelations("project.yaml", []Relation{{To: "person.yaml", Cardinality: ManyToMany}})
	mirror, _ := m.GetRelations("person.yaml")
	if len(mirror) != 1 || mirror[0].Cardinality != ManyToMany {
		t.Fatalf("mirror cardinality not updated: %+v", mirror)
	}
}

func TestSetRelations_RemoveDropsMirror(t *testing.T) {
	m := newMgr()
	_ = m.SetRelations("project.yaml", []Relation{{To: "person.yaml", Cardinality: OneToMany}})
	if err := m.SetRelations("project.yaml", nil); err != nil {
		t.Fatalf("clear: %v", err)
	}
	mirror, _ := m.GetRelations("person.yaml")
	if len(mirror) != 0 {
		t.Fatalf("counterpart should be removed: %+v", mirror)
	}
}

func TestSetRelations_MirrorPreservesTargetEntries(t *testing.T) {
	m := newMgr()
	// person owns a relation to team; that puts person -> team in person's file.
	_ = m.SetRelations("person.yaml", []Relation{{To: "team.yaml", Cardinality: OneToMany}})
	// project relates to person; the mirror lands in person's file alongside person -> team.
	_ = m.SetRelations("project.yaml", []Relation{{To: "person.yaml", Cardinality: ManyToMany}})
	got, _ := m.GetRelations("person.yaml")
	if relationIndex(got, "team.yaml") < 0 {
		t.Errorf("person -> team was lost: %+v", got)
	}
	if relationIndex(got, "project.yaml") < 0 {
		t.Errorf("mirror person -> project missing: %+v", got)
	}
}

func TestSetRelations_MirrorFlipsInverseFlag(t *testing.T) {
	m := newMgr()
	_ = m.SetRelations("project.yaml", []Relation{{To: "person.yaml", Cardinality: OneToMany, Inverse: false}})
	own, _ := m.GetRelations("project.yaml")
	if own[0].Inverse {
		t.Fatalf("authored half should stay non-inverse: %+v", own)
	}
	mirror, _ := m.GetRelations("person.yaml")
	if len(mirror) != 1 || !mirror[0].Inverse {
		t.Fatalf("counterpart of a non-inverse half should be inverse: %+v", mirror)
	}
}

func TestReconcile_RecreatedCounterpartFlipsInverse(t *testing.T) {
	fs := newMemFS()
	m := NewManager(fs, "/ctx/relations", fullCatalog())
	_ = m.saveRelationsLocked("project.yaml", []Relation{{To: "person.yaml", Cardinality: OneToMany, Inverse: false}})
	if _, err := m.Reconcile(); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	mirror, _ := m.GetRelations("person.yaml")
	if len(mirror) != 1 || !mirror[0].Inverse {
		t.Fatalf("recreated counterpart should be inverse: %+v", mirror)
	}
}

func TestSetRelations_SelfRelationNotClobbered(t *testing.T) {
	m := newMgr()
	if err := m.SetRelations("project.yaml", []Relation{{To: "project.yaml", Cardinality: OneToMany}}); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, _ := m.GetRelations("project.yaml")
	if len(got) != 1 || got[0].To != "project.yaml" || got[0].Cardinality != OneToMany {
		t.Fatalf("self-relation should be left intact, not flipped: %+v", got)
	}
}

func TestSelfRelation_InverseForcedOff(t *testing.T) {
	m := newMgr()
	if err := m.SetRelations("project.yaml", []Relation{{To: "project.yaml", Cardinality: OneToMany, Inverse: true}}); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, _ := m.GetRelations("project.yaml")
	if len(got) != 1 || got[0].Inverse {
		t.Fatalf("a self-relation has no other side, so it must never persist inverse: %+v", got)
	}
}

func TestSelfRelation_AddEdgeStoresSingleEdge(t *testing.T) {
	m := newMgr()
	if err := m.SetRelations("project.yaml", []Relation{{To: "project.yaml", Cardinality: ManyToMany}}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := m.AddEdge("project.yaml", "project.yaml", Edge{From: "p1", To: "p2"}); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	got, _ := m.GetRelations("project.yaml")
	if len(got) != 1 {
		t.Fatalf("self-relation should stay a single entry: %+v", got)
	}
	// The single edge is stored once. Reverse traversal reads it backward; it is never mirrored
	// into a second {p2,p1} edge in the same list.
	if len(got[0].Edges) != 1 || got[0].Edges[0] != (Edge{From: "p1", To: "p2"}) {
		t.Fatalf("self edge must be stored once, not mirrored: %+v", got[0].Edges)
	}
}

func TestSelfRelation_RejectsSelfLoopEdge(t *testing.T) {
	m := newMgr()
	_ = m.SetRelations("project.yaml", []Relation{{To: "project.yaml", Cardinality: ManyToMany}})
	if err := m.AddEdge("project.yaml", "project.yaml", Edge{From: "p1", To: "p1"}); err == nil {
		t.Error("a record must not link to itself")
	}
}

func TestSelfRelation_EnforcesCardinality(t *testing.T) {
	m := newMgr()
	_ = m.SetRelations("project.yaml", []Relation{{To: "project.yaml", Cardinality: ManyToOne}})
	add := func(from, to string) error {
		return m.AddEdge("project.yaml", "project.yaml", Edge{From: from, To: to})
	}
	if err := add("p1", "p2"); err != nil {
		t.Fatalf("first self edge: %v", err)
	}
	// many-to-one: one target per source -> p1 cannot point at a second target.
	if err := add("p1", "p3"); err == nil {
		t.Error("many-to-one self must reject a second target for the same source")
	}
	// a target may still be reached from many sources.
	if err := add("p3", "p2"); err != nil {
		t.Errorf("many-to-one self must allow a second source for the same target: %v", err)
	}
}

func TestSelfRelation_ManyToManyAllowsBothDirections(t *testing.T) {
	m := newMgr()
	_ = m.SetRelations("project.yaml", []Relation{{To: "project.yaml", Cardinality: ManyToMany}})
	if err := m.AddEdge("project.yaml", "project.yaml", Edge{From: "p1", To: "p2"}); err != nil {
		t.Fatalf("p1->p2: %v", err)
	}
	// p2->p1 is a distinct directed fact, not the auto-mirror of p1->p2.
	if err := m.AddEdge("project.yaml", "project.yaml", Edge{From: "p2", To: "p1"}); err != nil {
		t.Fatalf("p2->p1 should be a distinct edge: %v", err)
	}
	got, _ := m.GetRelations("project.yaml")
	if len(got[0].Edges) != 2 {
		t.Fatalf("both directed edges should be stored: %+v", got[0].Edges)
	}
}

func TestSelfRelation_RemoveEdge(t *testing.T) {
	m := newMgr()
	_ = m.SetRelations("project.yaml", []Relation{{To: "project.yaml", Cardinality: ManyToMany}})
	_ = m.AddEdge("project.yaml", "project.yaml", Edge{From: "p1", To: "p2"})
	if err := m.RemoveEdge("project.yaml", "project.yaml", Edge{From: "p1", To: "p2"}); err != nil {
		t.Fatalf("RemoveEdge: %v", err)
	}
	got, _ := m.GetRelations("project.yaml")
	if len(got) != 1 || len(got[0].Edges) != 0 {
		t.Fatalf("self edge should be cleanly removed, relation kept: %+v", got)
	}
}

func TestReconcile_LeavesSelfRelationUntouched(t *testing.T) {
	fs := newMemFS()
	m := NewManager(fs, "/ctx/relations", fullCatalog())
	// A self-relation with a single stored edge (no mirror, by design).
	_ = m.saveRelationsLocked("project.yaml", []Relation{
		{To: "project.yaml", Cardinality: ManyToOne, Edges: []Edge{{From: "p1", To: "p2"}}},
	})
	rep, err := m.Reconcile()
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	// Self has no counterpart to create/heal and no conflict to report.
	if len(rep.Created) != 0 || rep.EdgesHealed != 0 || len(rep.Conflicts) != 0 {
		t.Fatalf("self-relation should need no repair: %+v", rep)
	}
	got, _ := m.GetRelations("project.yaml")
	if len(got) != 1 || len(got[0].Edges) != 1 || got[0].Edges[0] != (Edge{From: "p1", To: "p2"}) {
		t.Fatalf("reconcile must not add a reversed edge or duplicate the self entry: %+v", got)
	}
}

func TestReconcile_RecreatesMissingCounterpart(t *testing.T) {
	fs := newMemFS()
	m := NewManager(fs, "/ctx/relations", fullCatalog())
	// Only one side on disk: a lost / half-deleted counterpart.
	if err := m.saveRelationsLocked("project.yaml", []Relation{{To: "person.yaml", Cardinality: OneToMany}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	rep, err := m.Reconcile()
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rep.Created) != 1 || rep.Created[0].Template != "person.yaml" || rep.Created[0].Cardinality != ManyToOne {
		t.Fatalf("expected person.yaml many-to-one recreated: %+v", rep.Created)
	}
	mirror, _ := m.GetRelations("person.yaml")
	if len(mirror) != 1 || mirror[0].To != "project.yaml" || mirror[0].Cardinality != ManyToOne {
		t.Fatalf("counterpart not recreated: %+v", mirror)
	}
}

func TestReconcile_ConsistentPairNoChange(t *testing.T) {
	m := newMgr()
	_ = m.SetRelations("project.yaml", []Relation{{To: "person.yaml", Cardinality: OneToMany}})
	rep, err := m.Reconcile()
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rep.Created) != 0 || len(rep.Conflicts) != 0 {
		t.Fatalf("a healthy pair should need no changes: %+v", rep)
	}
}

func TestReconcile_ReportsConflictWithoutChanging(t *testing.T) {
	fs := newMemFS()
	m := NewManager(fs, "/ctx/relations", fullCatalog())
	// Both halves present but cardinalities not each other's inverse.
	_ = m.saveRelationsLocked("project.yaml", []Relation{{To: "person.yaml", Cardinality: OneToMany}})
	_ = m.saveRelationsLocked("person.yaml", []Relation{{To: "project.yaml", Cardinality: OneToOne}})
	rep, err := m.Reconcile()
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rep.Conflicts) != 1 {
		t.Fatalf("expected one conflict: %+v", rep)
	}
	if len(rep.Created) != 0 {
		t.Fatalf("a conflict must not create anything: %+v", rep)
	}
	p, _ := m.GetRelations("person.yaml")
	if p[0].Cardinality != OneToOne {
		t.Fatalf("conflicting side must be left untouched: %+v", p)
	}
}

func TestReconcile_PreservesEdgesWhenAddingCounterpart(t *testing.T) {
	fs := newMemFS()
	m := NewManager(fs, "/ctx/relations", fullCatalog())
	// person already owns person->team with an edge.
	_ = m.saveRelationsLocked("person.yaml", []Relation{{To: "team.yaml", Cardinality: OneToMany, Edges: []Edge{{From: "u1", To: "t1"}}}})
	// project->person exists with no counterpart on person yet.
	_ = m.saveRelationsLocked("project.yaml", []Relation{{To: "person.yaml", Cardinality: ManyToMany}})
	if _, err := m.Reconcile(); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	got, _ := m.GetRelations("person.yaml")
	ti := relationIndex(got, "team.yaml")
	if ti < 0 || len(got[ti].Edges) != 1 {
		t.Fatalf("person->team edges must survive a counterpart add: %+v", got)
	}
	if relationIndex(got, "project.yaml") < 0 {
		t.Fatalf("person->project counterpart not added: %+v", got)
	}
}

func TestReconcile_EmptyTolerant(t *testing.T) {
	rep, err := newMgr().Reconcile()
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rep.Created) != 0 || len(rep.Conflicts) != 0 || rep.EdgesHealed != 0 {
		t.Fatalf("empty store should yield empty report: %+v", rep)
	}
}

func TestAddEdge_MirrorsReversedEdge(t *testing.T) {
	m := newMgr()
	_ = m.SetRelations("project.yaml", []Relation{{To: "person.yaml", Cardinality: ManyToMany}})
	if err := m.AddEdge("project.yaml", "person.yaml", Edge{From: "p1", To: "u1"}); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	src, _ := m.GetRelations("project.yaml")
	if i := relationIndex(src, "person.yaml"); i < 0 || len(src[i].Edges) != 1 || src[i].Edges[0] != (Edge{From: "p1", To: "u1"}) {
		t.Fatalf("source edge wrong: %+v", src)
	}
	mir, _ := m.GetRelations("person.yaml")
	j := relationIndex(mir, "project.yaml")
	if j < 0 || len(mir[j].Edges) != 1 || mir[j].Edges[0] != (Edge{From: "u1", To: "p1"}) {
		t.Fatalf("mirror edge should be reversed u1->p1: %+v", mir)
	}
}

func TestRemoveEdge_RemovesMirror(t *testing.T) {
	m := newMgr()
	_ = m.SetRelations("project.yaml", []Relation{{To: "person.yaml", Cardinality: ManyToMany}})
	_ = m.AddEdge("project.yaml", "person.yaml", Edge{From: "p1", To: "u1"})
	if err := m.RemoveEdge("project.yaml", "person.yaml", Edge{From: "p1", To: "u1"}); err != nil {
		t.Fatalf("RemoveEdge: %v", err)
	}
	mir, _ := m.GetRelations("person.yaml")
	j := relationIndex(mir, "project.yaml")
	if j < 0 || len(mir[j].Edges) != 0 {
		t.Fatalf("mirror edge should be removed: %+v", mir)
	}
}

func TestAddEdge_EnforcesCardinality(t *testing.T) {
	mk := func(c Cardinality) *Manager {
		m := newMgr()
		if err := m.SetRelations("project.yaml", []Relation{{To: "person.yaml", Cardinality: c}}); err != nil {
			t.Fatalf("setup %s: %v", c, err)
		}
		return m
	}
	add := func(m *Manager, from, to string) error {
		return m.AddEdge("project.yaml", "person.yaml", Edge{From: from, To: to})
	}

	// one-to-one: at most one per source AND per target.
	m := mk(OneToOne)
	if err := add(m, "p1", "u1"); err != nil {
		t.Fatalf("one-to-one first: %v", err)
	}
	if err := add(m, "p1", "u2"); err == nil {
		t.Error("one-to-one must reject a second target for the same source")
	}
	if err := add(m, "p2", "u1"); err == nil {
		t.Error("one-to-one must reject a second source for the same target")
	}

	// one-to-many: a source may link many targets, but a target only one source.
	m = mk(OneToMany)
	_ = add(m, "p1", "u1")
	if err := add(m, "p1", "u2"); err != nil {
		t.Errorf("one-to-many must allow a second target for the same source: %v", err)
	}
	if err := add(m, "p2", "u1"); err == nil {
		t.Error("one-to-many must reject a second source for the same target")
	}

	// many-to-one: a target may be linked from many sources, but a source links one target.
	m = mk(ManyToOne)
	_ = add(m, "p1", "u1")
	if err := add(m, "p2", "u1"); err != nil {
		t.Errorf("many-to-one must allow a second source for the same target: %v", err)
	}
	if err := add(m, "p1", "u2"); err == nil {
		t.Error("many-to-one must reject a second target for the same source")
	}

	// many-to-many: no limits.
	m = mk(ManyToMany)
	_ = add(m, "p1", "u1")
	if err := add(m, "p1", "u2"); err != nil {
		t.Errorf("many-to-many must allow: %v", err)
	}
	if err := add(m, "p2", "u1"); err != nil {
		t.Errorf("many-to-many must allow: %v", err)
	}
}

func TestReconcile_HealsMissingEdge(t *testing.T) {
	fs := newMemFS()
	m := NewManager(fs, "/ctx/relations", fullCatalog())
	// Both declaration halves exist, but the edge is only on the project side.
	_ = m.saveRelationsLocked("project.yaml", []Relation{{To: "person.yaml", Cardinality: ManyToMany, Edges: []Edge{{From: "p1", To: "u1"}}}})
	_ = m.saveRelationsLocked("person.yaml", []Relation{{To: "project.yaml", Cardinality: ManyToMany}})
	rep, err := m.Reconcile()
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if rep.EdgesHealed != 1 {
		t.Fatalf("want 1 edge healed, got %d (%+v)", rep.EdgesHealed, rep)
	}
	mir, _ := m.GetRelations("person.yaml")
	j := relationIndex(mir, "project.yaml")
	if j < 0 || len(mir[j].Edges) != 1 || mir[j].Edges[0] != (Edge{From: "u1", To: "p1"}) {
		t.Fatalf("missing reversed edge not healed: %+v", mir)
	}
}

func TestReconcile_RecreatedCounterpartCarriesReversedEdges(t *testing.T) {
	fs := newMemFS()
	m := NewManager(fs, "/ctx/relations", fullCatalog())
	_ = m.saveRelationsLocked("project.yaml", []Relation{{To: "person.yaml", Cardinality: OneToMany, Edges: []Edge{{From: "p1", To: "u1"}}}})
	if _, err := m.Reconcile(); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	mir, _ := m.GetRelations("person.yaml")
	j := relationIndex(mir, "project.yaml")
	if j < 0 || len(mir[j].Edges) != 1 || mir[j].Edges[0] != (Edge{From: "u1", To: "p1"}) {
		t.Fatalf("recreated counterpart should carry reversed edges: %+v", mir)
	}
}

func TestSetGet_RoundTrip(t *testing.T) {
	m := newMgr()
	in := []Relation{
		{To: "person.yaml", Cardinality: ManyToMany},
		{To: "team.yaml", Cardinality: OneToMany},
	}
	if err := m.SetRelations("project.yaml", in); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := m.GetRelations("project.yaml")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 2 || got[0].To != "person.yaml" || got[1].To != "team.yaml" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestGet_MissingReturnsNil(t *testing.T) {
	got, err := newMgr().GetRelations("nope.yaml")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != nil {
		t.Errorf("want nil for missing, got %+v", got)
	}
}

func TestEdges_RoundTrip(t *testing.T) {
	m := newMgr()
	in := []Relation{{To: "person.yaml", Cardinality: ManyToMany, Edges: []Edge{{From: "p1", To: "u1"}}}}
	if err := m.SetRelations("project.yaml", in); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, _ := m.GetRelations("project.yaml")
	if len(got) != 1 || len(got[0].Edges) != 1 || got[0].Edges[0].From != "p1" || got[0].Edges[0].To != "u1" {
		t.Fatalf("edges did not round-trip: %+v", got)
	}
}

func TestAddRemoveEdge(t *testing.T) {
	m := newMgr()
	_ = m.SetRelations("project.yaml", []Relation{{To: "person.yaml", Cardinality: ManyToMany}})

	if err := m.AddEdge("project.yaml", "person.yaml", Edge{From: "p1", To: "u1"}); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	got, _ := m.GetRelations("project.yaml")
	if len(got[0].Edges) != 1 {
		t.Fatalf("edge not added: %+v", got)
	}

	if err := m.AddEdge("project.yaml", "person.yaml", Edge{From: "p1", To: "u1"}); err == nil {
		t.Error("expected duplicate edge rejection")
	}
	if err := m.AddEdge("project.yaml", "nope.yaml", Edge{From: "p1", To: "u1"}); err == nil {
		t.Error("expected unknown-relation rejection")
	}
	if err := m.AddEdge("project.yaml", "person.yaml", Edge{From: "", To: "u1"}); err == nil {
		t.Error("expected empty-endpoint rejection")
	}

	if err := m.RemoveEdge("project.yaml", "person.yaml", Edge{From: "p1", To: "u1"}); err != nil {
		t.Fatalf("RemoveEdge: %v", err)
	}
	got, _ = m.GetRelations("project.yaml")
	if len(got[0].Edges) != 0 {
		t.Fatalf("edge not removed: %+v", got)
	}
	if err := m.RemoveEdge("project.yaml", "person.yaml", Edge{From: "p1", To: "u1"}); err == nil {
		t.Error("expected not-found on removing absent edge")
	}
}

func TestSet_RejectsMalformed(t *testing.T) {
	m := newMgr()
	cases := map[string]Relation{
		"empty target":    {To: "", Cardinality: OneToOne},
		"bad cardinality": {To: "person.yaml", Cardinality: "loose"},
	}
	for name, rel := range cases {
		t.Run(name, func(t *testing.T) {
			if err := m.SetRelations("project.yaml", []Relation{rel}); err == nil {
				t.Errorf("expected rejection for %s", name)
			}
		})
	}
}

func TestSet_RejectsDuplicateRelation(t *testing.T) {
	m := newMgr()
	dup := []Relation{
		{To: "person.yaml", Cardinality: OneToMany},
		{To: "person.yaml", Cardinality: ManyToMany},
	}
	if err := m.SetRelations("project.yaml", dup); err == nil {
		t.Error("expected rejection: duplicate relation to the same target")
	}
}

func TestSetRelations_RejectsNonCollectionEndpoints(t *testing.T) {
	m := newMgr()
	// target is not a collection
	if err := m.SetRelations("project.yaml", []Relation{{To: "note.yaml", Cardinality: OneToOne}}); err == nil {
		t.Error("expected rejection: target not a collection")
	}
	// source is not a collection
	if err := m.SetRelations("note.yaml", []Relation{{To: "person.yaml", Cardinality: OneToOne}}); err == nil {
		t.Error("expected rejection: source not a collection")
	}
}

func TestAddEdge_RejectsMissingRecord(t *testing.T) {
	m := newMgr()
	_ = m.SetRelations("project.yaml", []Relation{{To: "person.yaml", Cardinality: ManyToMany}})
	if err := m.AddEdge("project.yaml", "person.yaml", Edge{From: "p1", To: "ghost"}); err == nil {
		t.Error("expected rejection: target record does not exist")
	}
	if err := m.AddEdge("project.yaml", "person.yaml", Edge{From: "ghost", To: "u1"}); err == nil {
		t.Error("expected rejection: source record does not exist")
	}
}

// Volatility: a record (and even the target template) can vanish after an edge exists. Cleanup must
// still work, because RemoveEdge goes through the persistence floor, not the catalog-checked path.
func TestRemoveEdge_ToleratesDegradedTarget(t *testing.T) {
	fs := newMemFS()
	healthy := NewManager(fs, "/ctx/relations", fullCatalog())
	_ = healthy.SetRelations("project.yaml", []Relation{{To: "person.yaml", Cardinality: ManyToMany}})
	if err := healthy.AddEdge("project.yaml", "person.yaml", Edge{From: "p1", To: "u1"}); err != nil {
		t.Fatalf("setup AddEdge: %v", err)
	}

	// person.yaml is no longer a collection and its records are gone (deleted out from under us).
	degraded := NewManager(fs, "/ctx/relations", fakeCatalog{
		collections: map[string]bool{"project.yaml": true},
		records:     map[string]map[string]bool{},
	})
	if err := degraded.RemoveEdge("project.yaml", "person.yaml", Edge{From: "p1", To: "u1"}); err != nil {
		t.Fatalf("RemoveEdge must tolerate a degraded target: %v", err)
	}
	got, _ := degraded.GetRelations("project.yaml")
	if len(got[0].Edges) != 0 {
		t.Fatalf("stale edge not cleaned up: %+v", got)
	}
}
