package app

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/datacore"
	"github.com/petervdpas/formidable2/internal/modules/relation"
	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

type fakeRelReader struct{ rels map[string][]relation.Relation }

func (f fakeRelReader) GetRelations(template string) ([]relation.Relation, error) {
	return f.rels[template], nil
}

// TestDatacoreAdapter_IngestsLiveTemplate stitches the real template + storage
// managers together, saves fixture forms, then builds a tensor through the
// datacore loader adapter and exercises every shape the adapter produces:
// a scalar field, a table column reached by Follow, a multi-valued (tags)
// field as a one-column table, and a facet crossed against the scalar.
func TestDatacoreAdapter_IngestsLiveTemplate(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	tplM := template.NewManager(sys, "templates", log)
	if err := tplM.EnsureTemplateDirectory(); err != nil {
		t.Fatalf("EnsureTemplateDirectory: %v", err)
	}
	sfrM := sfr.NewManager(sys, log)
	stoM := storage.NewManager(sys, sfrM, tplM, "storage", log)

	tpl := &template.Template{
		Name:     "assets",
		Filename: "assets.yaml",
		Fields: []template.Field{
			{Key: "title", Type: "text"},
			{Key: "status", Type: "dropdown", Options: []any{
				map[string]any{"value": "active", "label": "Active"},
				map[string]any{"value": "retired", "label": "Retired"},
			}},
			{Key: "labels", Type: "tags"},
			{Key: "items", Type: "table", Options: []any{
				map[string]any{"value": "name", "label": "Name", "type": "text"},
				map[string]any{"value": "cost", "label": "Cost", "type": "number"},
			}},
		},
		Facets: []template.Facet{
			{Key: "tier", Icon: "fa-flag", Options: []template.FacetOption{
				{Label: "GOLD", Color: "amber"},
				{Label: "SILVER", Color: "blue"},
			}},
		},
	}
	if err := tplM.SaveTemplate("assets.yaml", tpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}

	saves := []struct {
		filename string
		data     map[string]any
	}{
		{"a.meta.json", map[string]any{
			"title": "A", "status": "active",
			"labels": []any{"x", "y"},
			"items":  []any{[]any{"disk", float64(100)}, []any{"ram", float64(50)}},
			"meta":   map[string]any{"facets": map[string]any{"tier": map[string]any{"set": true, "selected": "GOLD"}}},
		}},
		{"b.meta.json", map[string]any{
			"title": "B", "status": "active",
			"labels": []any{"x"},
			"items":  []any{[]any{"disk", float64(100)}},
			"meta":   map[string]any{"facets": map[string]any{"tier": map[string]any{"set": true, "selected": "SILVER"}}},
		}},
		{"c.meta.json", map[string]any{
			"title": "C", "status": "retired",
			"labels": []any{"z"},
			"items":  []any{[]any{"gpu", float64(300)}},
			"meta":   map[string]any{"facets": map[string]any{"tier": map[string]any{"set": true, "selected": "GOLD"}}},
		}},
	}
	for _, s := range saves {
		if r := stoM.SaveForm(context.Background(), "assets.yaml", s.filename, s.data); !r.Success {
			t.Fatalf("SaveForm %s: %s", s.filename, r.Error)
		}
	}

	dt, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, nil, nil, "assets.yaml"))
	if err != nil {
		t.Fatalf("datacore.Build: %v", err)
	}

	if got := dt.View().Count(); got != 3 {
		t.Fatalf("record count = %d, want 3", got)
	}

	// Scalar field distribution.
	assertBuckets(t, "status", dt.View().Distribution("status"), map[string]int{"active": 2, "retired": 1})

	// Table column reached by Follow: disk appears in a and b, gpu in c, ram in a.
	assertBuckets(t, "items.name", dt.View().Follow("items").Distribution("name"),
		map[string]int{"disk": 2, "ram": 1, "gpu": 1})

	// Multi-valued tags as a one-column table.
	assertBuckets(t, "labels.value", dt.View().Follow("labels").Distribution("value"),
		map[string]int{"x": 2, "y": 1, "z": 1})

	// Facet ingested as a context field, crossed against status.
	ct := dt.View().Cross("facet:tier", "status")
	if got := ct.Count("GOLD", "active"); got != 1 {
		t.Fatalf("GOLD/active = %d, want 1", got)
	}
	if got := ct.Count("GOLD", "retired"); got != 1 {
		t.Fatalf("GOLD/retired = %d, want 1", got)
	}
	if got := ct.Count("SILVER", "active"); got != 1 {
		t.Fatalf("SILVER/active = %d, want 1", got)
	}
}

// TestDatacoreAdapter_SelfRelationLinksAreFollowable saves three collection
// records, declares a self-relation whose edges point each child at the parent,
// and confirms the adapter turns those edges into a Followable "rel:<to>" link:
// following it from every record lands on the parent. This is the seam that lets
// datacore traverse the relation graph (the inheritance/hierarchy case).
func TestDatacoreAdapter_SelfRelationLinksAreFollowable(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	tplM := template.NewManager(sys, "templates", log)
	if err := tplM.EnsureTemplateDirectory(); err != nil {
		t.Fatalf("EnsureTemplateDirectory: %v", err)
	}
	sfrM := sfr.NewManager(sys, log)
	stoM := storage.NewManager(sys, sfrM, tplM, "storage", log)

	tpl := &template.Template{
		Name:             "entities",
		Filename:         "entities.yaml",
		EnableCollection: true,
		Fields: []template.Field{
			{Key: "id", Type: "guid"},
			{Key: "name", Type: "text"},
		},
	}
	if err := tplM.SaveTemplate("entities.yaml", tpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}

	files := []string{"parent.meta.json", "childa.meta.json", "childb.meta.json"}
	labels := []string{"Parent", "ChildA", "ChildB"}
	guid := map[string]string{}
	for i, fn := range files {
		if r := stoM.SaveForm(context.Background(), "entities.yaml", fn, map[string]any{"name": labels[i]}); !r.Success {
			t.Fatalf("SaveForm %s: %s", fn, r.Error)
		}
		f := stoM.LoadForm("entities.yaml", fn)
		if f == nil || f.Meta.ID == "" {
			t.Fatalf("record %s has no guid", fn)
		}
		guid[fn] = f.Meta.ID
	}

	// Self-relation: each child points to the parent (the base-entity / reports_to shape).
	rel := fakeRelReader{rels: map[string][]relation.Relation{
		"entities.yaml": {{
			To:          "entities.yaml",
			Cardinality: relation.ManyToOne,
			Edges: []relation.Edge{
				{From: guid["childa.meta.json"], To: guid["parent.meta.json"]},
				{From: guid["childb.meta.json"], To: guid["parent.meta.json"]},
			},
		}},
	}}

	dt, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, nil, rel, "entities.yaml"))
	if err != nil {
		t.Fatalf("datacore.Build: %v", err)
	}
	// Following the self-relation from every record lands on the parent (deduped).
	assertBuckets(t, "rel:entities.yaml -> name",
		dt.View().Follow("rel:entities.yaml").Distribution("name"),
		map[string]int{"Parent": 1})
}

// TestDatacoreAdapter_FollowRelationThenTable proves the composed traversal
// table -> record -> self-relation -> record -> table is reachable in one
// single-template tensor: from the children, follow the self-relation to the
// parent, then descend into the parent's table. Both hops are ordinary Follow
// calls, so they chain.
func TestDatacoreAdapter_FollowRelationThenTable(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	tplM := template.NewManager(sys, "templates", log)
	if err := tplM.EnsureTemplateDirectory(); err != nil {
		t.Fatalf("EnsureTemplateDirectory: %v", err)
	}
	sfrM := sfr.NewManager(sys, log)
	stoM := storage.NewManager(sys, sfrM, tplM, "storage", log)

	tpl := &template.Template{
		Name:             "entities",
		Filename:         "entities.yaml",
		EnableCollection: true,
		Fields: []template.Field{
			{Key: "id", Type: "guid"},
			{Key: "name", Type: "text"},
			{Key: "attrs", Type: "table", Options: []any{
				map[string]any{"value": "attr", "label": "Attr"},
				map[string]any{"value": "kind", "label": "Kind"},
			}},
		},
	}
	if err := tplM.SaveTemplate("entities.yaml", tpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}

	saves := []struct {
		filename string
		data     map[string]any
	}{
		{"parent.meta.json", map[string]any{"name": "Parent",
			"attrs": []any{[]any{"a1", "string"}, []any{"a2", "int"}}}},
		{"childa.meta.json", map[string]any{"name": "ChildA",
			"attrs": []any{[]any{"c1", "bool"}}}},
	}
	guid := map[string]string{}
	for _, s := range saves {
		if r := stoM.SaveForm(context.Background(), "entities.yaml", s.filename, s.data); !r.Success {
			t.Fatalf("SaveForm %s: %s", s.filename, r.Error)
		}
		f := stoM.LoadForm("entities.yaml", s.filename)
		if f == nil || f.Meta.ID == "" {
			t.Fatalf("record %s has no guid", s.filename)
		}
		guid[s.filename] = f.Meta.ID
	}

	rel := fakeRelReader{rels: map[string][]relation.Relation{
		"entities.yaml": {{
			To:          "entities.yaml",
			Cardinality: relation.ManyToOne,
			Edges:       []relation.Edge{{From: guid["childa.meta.json"], To: guid["parent.meta.json"]}},
		}},
	}}

	dt, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, nil, rel, "entities.yaml"))
	if err != nil {
		t.Fatalf("datacore.Build: %v", err)
	}
	// Follow the relation to the parent, then descend into the parent's attrs table.
	assertBuckets(t, "rel -> attrs.attr",
		dt.View().Follow("rel:entities.yaml").Follow("attrs").Distribution("attr"),
		map[string]int{"a1": 1, "a2": 1})
}

func assertBuckets(t *testing.T, label string, got []datacore.Bucket, want map[string]int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d buckets, want %d (%v)", label, len(got), len(want), got)
	}
	for _, b := range got {
		if want[b.Value] != b.Count {
			t.Fatalf("%s: %q = %d, want %d", label, b.Value, b.Count, want[b.Value])
		}
	}
}
