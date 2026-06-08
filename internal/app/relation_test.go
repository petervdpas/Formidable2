package app

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/relation"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// memFS is a flat in-memory filesystem satisfying relation's fs port.
type memFS struct{ files map[string]string }

func newMemFS() *memFS { return &memFS{files: map[string]string{}} }

func (m *memFS) FileExists(path string) bool  { _, ok := m.files[path]; return ok }
func (m *memFS) EnsureDirectory(string) error { return nil }
func (m *memFS) LoadFile(path string) (string, error) {
	return m.files[path], nil
}
func (m *memFS) SaveFile(path, content string) error { m.files[path] = content; return nil }
func (m *memFS) ListFiles(dir string) ([]string, error) {
	var out []string
	for p := range m.files {
		if filepath.Dir(p) == dir {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

// allTrueCatalog accepts every collection and record so AddEdge/SetRelations
// validation passes; the syncer logic is what's under test.
type allTrueCatalog struct{}

func (allTrueCatalog) IsCollection(string) bool         { return true }
func (allTrueCatalog) RecordExists(string, string) bool { return true }

// hostEdges returns the To-ids of edges from hostGuid in host's relation to target.
func hostEdges(t *testing.T, rel *relation.Manager, host, target, hostGuid string) []string {
	t.Helper()
	rels, err := rel.GetRelations(host)
	if err != nil {
		t.Fatalf("GetRelations: %v", err)
	}
	var out []string
	for _, r := range rels {
		if r.To != target {
			continue
		}
		for _, e := range r.Edges {
			if e.From == hostGuid {
				out = append(out, e.To)
			}
		}
	}
	sort.Strings(out)
	return out
}

func apiField(key, collection string) template.Field {
	return template.Field{Key: key, Type: "api", Collection: collection}
}

func newSyncWorld(t *testing.T, card relation.Cardinality) (referenceEdgeSyncer, *relation.Manager) {
	t.Helper()
	rel := relation.NewManager(newMemFS(), "relations", allTrueCatalog{})
	if err := rel.SetRelations("book.yaml", []relation.Relation{
		{To: "person.yaml", Cardinality: card},
	}); err != nil {
		t.Fatalf("SetRelations: %v", err)
	}
	return referenceEdgeSyncer{rel: rel}, rel
}

func TestSyncReferenceEdges_AddsEdgeForPick(t *testing.T) {
	s, rel := newSyncWorld(t, relation.OneToMany)
	fields := []template.Field{apiField("author", "person.yaml")}
	data := map[string]any{"author": "p-1"}

	if err := s.SyncReferenceEdges("book.yaml", "b-1", fields, data); err != nil {
		t.Fatalf("sync: %v", err)
	}
	got := hostEdges(t, rel, "book.yaml", "person.yaml", "b-1")
	if len(got) != 1 || got[0] != "p-1" {
		t.Errorf("edges = %v, want [p-1]", got)
	}
}

func TestSyncReferenceEdges_ToManyListAddsAll(t *testing.T) {
	s, rel := newSyncWorld(t, relation.ManyToMany)
	fields := []template.Field{apiField("people", "person.yaml")}
	data := map[string]any{"people": []any{"p-1", "p-2"}}

	if err := s.SyncReferenceEdges("book.yaml", "b-1", fields, data); err != nil {
		t.Fatalf("sync: %v", err)
	}
	got := hostEdges(t, rel, "book.yaml", "person.yaml", "b-1")
	if len(got) != 2 || got[0] != "p-1" || got[1] != "p-2" {
		t.Errorf("edges = %v, want [p-1 p-2]", got)
	}
}

func TestSyncReferenceEdges_RemovesOrphanedEdge(t *testing.T) {
	s, rel := newSyncWorld(t, relation.ManyToMany)
	fields := []template.Field{apiField("people", "person.yaml")}

	// Link two, then save with only one referenced: the other is drained.
	if err := s.SyncReferenceEdges("book.yaml", "b-1", fields, map[string]any{"people": []any{"p-1", "p-2"}}); err != nil {
		t.Fatalf("sync1: %v", err)
	}
	if err := s.SyncReferenceEdges("book.yaml", "b-1", fields, map[string]any{"people": []any{"p-1"}}); err != nil {
		t.Fatalf("sync2: %v", err)
	}
	got := hostEdges(t, rel, "book.yaml", "person.yaml", "b-1")
	if len(got) != 1 || got[0] != "p-1" {
		t.Errorf("edges = %v, want [p-1] after drain", got)
	}
}

func TestSyncReferenceEdges_TwoFieldsShareOneEdgeList(t *testing.T) {
	// Two api-fields point at the same target with different ids; their union
	// lands in the single role-agnostic edge list (deduped, no collision).
	s, rel := newSyncWorld(t, relation.ManyToMany)
	fields := []template.Field{
		apiField("author", "person.yaml"),
		apiField("editor", "person.yaml"),
	}
	data := map[string]any{"author": "p-1", "editor": "p-2"}

	if err := s.SyncReferenceEdges("book.yaml", "b-1", fields, data); err != nil {
		t.Fatalf("sync: %v", err)
	}
	got := hostEdges(t, rel, "book.yaml", "person.yaml", "b-1")
	if len(got) != 2 || got[0] != "p-1" || got[1] != "p-2" {
		t.Errorf("edges = %v, want [p-1 p-2]", got)
	}
}

func TestSyncReferenceEdges_SharedTargetKeepsEdgeWhileOneFieldStillRefs(t *testing.T) {
	// Author and editor both link p-1; clearing author must NOT remove the edge
	// because editor still references it (union ref-count via reconciliation).
	s, rel := newSyncWorld(t, relation.ManyToMany)
	fields := []template.Field{
		apiField("author", "person.yaml"),
		apiField("editor", "person.yaml"),
	}
	if err := s.SyncReferenceEdges("book.yaml", "b-1", fields,
		map[string]any{"author": "p-1", "editor": "p-1"}); err != nil {
		t.Fatalf("sync1: %v", err)
	}
	if err := s.SyncReferenceEdges("book.yaml", "b-1", fields,
		map[string]any{"author": "", "editor": "p-1"}); err != nil {
		t.Fatalf("sync2: %v", err)
	}
	got := hostEdges(t, rel, "book.yaml", "person.yaml", "b-1")
	if len(got) != 1 || got[0] != "p-1" {
		t.Errorf("edges = %v, want [p-1] (editor still refs it)", got)
	}
}

func TestSyncReferenceEdges_ClearedFieldDrainsEdges(t *testing.T) {
	s, rel := newSyncWorld(t, relation.OneToMany)
	fields := []template.Field{apiField("author", "person.yaml")}

	if err := s.SyncReferenceEdges("book.yaml", "b-1", fields, map[string]any{"author": "p-1"}); err != nil {
		t.Fatalf("sync1: %v", err)
	}
	if err := s.SyncReferenceEdges("book.yaml", "b-1", fields, map[string]any{"author": ""}); err != nil {
		t.Fatalf("sync2: %v", err)
	}
	if got := hostEdges(t, rel, "book.yaml", "person.yaml", "b-1"); len(got) != 0 {
		t.Errorf("edges = %v, want empty after clear", got)
	}
}

func TestSyncReferenceEdges_UndeclaredTargetIsTolerated(t *testing.T) {
	// No relation to ghost.yaml: AddEdge fails internally, sync swallows it.
	s, rel := newSyncWorld(t, relation.OneToMany)
	fields := []template.Field{apiField("ref", "ghost.yaml")}

	if err := s.SyncReferenceEdges("book.yaml", "b-1", fields, map[string]any{"ref": "x-1"}); err != nil {
		t.Fatalf("sync should tolerate undeclared target: %v", err)
	}
	if got := hostEdges(t, rel, "book.yaml", "ghost.yaml", "b-1"); len(got) != 0 {
		t.Errorf("edges = %v, want none for undeclared relation", got)
	}
}

func TestSyncReferenceEdges_CardinalityBreachIsTolerated(t *testing.T) {
	// ManyToOne limits one target per source: linking two ids breaches on the
	// second AddEdge, which sync swallows best-effort. Exactly one edge lands and
	// sync still returns nil (which id wins is map-order, so don't assert it).
	s, rel := newSyncWorld(t, relation.ManyToOne)
	fields := []template.Field{apiField("people", "person.yaml")}
	data := map[string]any{"people": []any{"p-1", "p-2"}}

	if err := s.SyncReferenceEdges("book.yaml", "b-1", fields, data); err != nil {
		t.Fatalf("sync should tolerate a cardinality breach: %v", err)
	}
	if got := hostEdges(t, rel, "book.yaml", "person.yaml", "b-1"); len(got) != 1 {
		t.Errorf("edges = %v, want exactly one (cardinality caps at one)", got)
	}
}

func TestApiRelations_GetRelationsErrorPropagates(t *testing.T) {
	// The apiRelations adapter maps relation defs to api defs; a read error from
	// the underlying manager surfaces as nil result + error, not a partial map.
	mfs := newMemFS()
	mfs.files[filepath.Join("relations", "book.yaml")] = "relations: [bad : yaml : here"
	rel := relation.NewManager(mfs, "relations", allTrueCatalog{})
	a := apiRelations{rel: rel}

	out, err := a.GetRelations("book.yaml")
	if err == nil {
		t.Fatal("expected a parse error to propagate, got nil")
	}
	if out != nil {
		t.Errorf("expected nil result on error, got %v", out)
	}
}

func TestApiRelations_MapsDefsAndEdges(t *testing.T) {
	// Happy mapping: relation defs and their edges translate field-for-field into
	// the api shape.
	rel := relation.NewManager(newMemFS(), "relations", allTrueCatalog{})
	if err := rel.SetRelations("book.yaml", []relation.Relation{
		{To: "person.yaml", Cardinality: relation.ManyToMany},
	}); err != nil {
		t.Fatalf("SetRelations: %v", err)
	}
	if err := rel.AddEdge("book.yaml", "person.yaml", relation.Edge{From: "b-1", To: "p-1"}); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	a := apiRelations{rel: rel}
	out, err := a.GetRelations("book.yaml")
	if err != nil {
		t.Fatalf("GetRelations: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d defs, want 1", len(out))
	}
	if out[0].To != "person.yaml" || out[0].Cardinality != string(relation.ManyToMany) {
		t.Errorf("def = %+v, want To=person.yaml Cardinality=many-to-many", out[0])
	}
	if len(out[0].Edges) != 1 || out[0].Edges[0].From != "b-1" || out[0].Edges[0].To != "p-1" {
		t.Errorf("edges = %+v, want [{b-1 p-1}]", out[0].Edges)
	}
}

func TestSyncReferenceEdges_GetRelationsErrorPropagates(t *testing.T) {
	// A corrupt relations file makes GetRelations fail; SyncReferenceEdges
	// returns that error rather than swallowing it (the one fatal path).
	mfs := newMemFS()
	mfs.files[filepath.Join("relations", "book.yaml")] = "relations: [this is : not : valid"
	rel := relation.NewManager(mfs, "relations", allTrueCatalog{})
	s := referenceEdgeSyncer{rel: rel}

	fields := []template.Field{apiField("author", "person.yaml")}
	err := s.SyncReferenceEdges("book.yaml", "b-1", fields, map[string]any{"author": "p-1"})
	if err == nil {
		t.Fatal("expected GetRelations parse error to propagate, got nil")
	}
}
