package relation

import (
	"os"
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
			"project.yaml": {"p1": true, "p2": true},
			"person.yaml":  {"u1": true, "u2": true},
		},
	}
}

func newMgr() *Manager { return NewManager(newMemFS(), "/ctx/relations", fullCatalog()) }

func TestSetGet_RoundTrip(t *testing.T) {
	m := newMgr()
	in := []Relation{
		{Name: "author", To: "person.yaml", Cardinality: ManyToMany},
		{Name: "owner", To: "team.yaml", Cardinality: OneToMany},
	}
	if err := m.SetRelations("project.yaml", in); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := m.GetRelations("project.yaml")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 2 || got[0].Name != "author" || got[1].To != "team.yaml" {
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
	in := []Relation{{Name: "author", To: "person.yaml", Cardinality: ManyToMany, Edges: []Edge{{From: "p1", To: "u1"}}}}
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
	_ = m.SetRelations("project.yaml", []Relation{{Name: "author", To: "person.yaml", Cardinality: ManyToMany}})

	if err := m.AddEdge("project.yaml", "author", Edge{From: "p1", To: "u1"}); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	got, _ := m.GetRelations("project.yaml")
	if len(got[0].Edges) != 1 {
		t.Fatalf("edge not added: %+v", got)
	}

	if err := m.AddEdge("project.yaml", "author", Edge{From: "p1", To: "u1"}); err == nil {
		t.Error("expected duplicate edge rejection")
	}
	if err := m.AddEdge("project.yaml", "nope", Edge{From: "p1", To: "u1"}); err == nil {
		t.Error("expected unknown-relation rejection")
	}
	if err := m.AddEdge("project.yaml", "author", Edge{From: "", To: "u1"}); err == nil {
		t.Error("expected empty-endpoint rejection")
	}

	if err := m.RemoveEdge("project.yaml", "author", Edge{From: "p1", To: "u1"}); err != nil {
		t.Fatalf("RemoveEdge: %v", err)
	}
	got, _ = m.GetRelations("project.yaml")
	if len(got[0].Edges) != 0 {
		t.Fatalf("edge not removed: %+v", got)
	}
	if err := m.RemoveEdge("project.yaml", "author", Edge{From: "p1", To: "u1"}); err == nil {
		t.Error("expected not-found on removing absent edge")
	}
}

func TestSet_RejectsMalformed(t *testing.T) {
	m := newMgr()
	cases := map[string]Relation{
		"empty name":      {Name: "", To: "person.yaml", Cardinality: OneToOne},
		"empty target":    {Name: "x", To: "", Cardinality: OneToOne},
		"bad cardinality": {Name: "x", To: "person.yaml", Cardinality: "loose"},
	}
	for name, rel := range cases {
		t.Run(name, func(t *testing.T) {
			if err := m.SetRelations("project.yaml", []Relation{rel}); err == nil {
				t.Errorf("expected rejection for %s", name)
			}
		})
	}
}

func TestSetRelations_RejectsNonCollectionEndpoints(t *testing.T) {
	m := newMgr()
	// target is not a collection
	if err := m.SetRelations("project.yaml", []Relation{{Name: "x", To: "note.yaml", Cardinality: OneToOne}}); err == nil {
		t.Error("expected rejection: target not a collection")
	}
	// source is not a collection
	if err := m.SetRelations("note.yaml", []Relation{{Name: "x", To: "person.yaml", Cardinality: OneToOne}}); err == nil {
		t.Error("expected rejection: source not a collection")
	}
}

func TestAddEdge_RejectsMissingRecord(t *testing.T) {
	m := newMgr()
	_ = m.SetRelations("project.yaml", []Relation{{Name: "author", To: "person.yaml", Cardinality: ManyToMany}})
	if err := m.AddEdge("project.yaml", "author", Edge{From: "p1", To: "ghost"}); err == nil {
		t.Error("expected rejection: target record does not exist")
	}
	if err := m.AddEdge("project.yaml", "author", Edge{From: "ghost", To: "u1"}); err == nil {
		t.Error("expected rejection: source record does not exist")
	}
}

// Volatility: a record (and even the target template) can vanish after an edge exists. Cleanup must
// still work, because RemoveEdge goes through the persistence floor, not the catalog-checked path.
func TestRemoveEdge_ToleratesDegradedTarget(t *testing.T) {
	fs := newMemFS()
	healthy := NewManager(fs, "/ctx/relations", fullCatalog())
	_ = healthy.SetRelations("project.yaml", []Relation{{Name: "author", To: "person.yaml", Cardinality: ManyToMany}})
	if err := healthy.AddEdge("project.yaml", "author", Edge{From: "p1", To: "u1"}); err != nil {
		t.Fatalf("setup AddEdge: %v", err)
	}

	// person.yaml is no longer a collection and its records are gone (deleted out from under us).
	degraded := NewManager(fs, "/ctx/relations", fakeCatalog{
		collections: map[string]bool{"project.yaml": true},
		records:     map[string]map[string]bool{},
	})
	if err := degraded.RemoveEdge("project.yaml", "author", Edge{From: "p1", To: "u1"}); err != nil {
		t.Fatalf("RemoveEdge must tolerate a degraded target: %v", err)
	}
	got, _ := degraded.GetRelations("project.yaml")
	if len(got[0].Edges) != 0 {
		t.Fatalf("stale edge not cleaned up: %+v", got)
	}
}
