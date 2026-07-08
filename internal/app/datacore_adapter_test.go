package app

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/datacore"
	"github.com/petervdpas/formidable2/internal/modules/relation"
	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

type fakeRelReader struct {
	rels map[string][]relation.Relation
}

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

	dt, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, nil, nil, "assets.yaml", false))
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

// TestDatacoreAdapter_LoopRowsGatedByFlag confirms a loop field is ingested as a
// row table only when loopRows is on (the graph_loop_rows config). Off, the loop
// is absent from the tensor; on, its rows are Followable like a table's.
func TestDatacoreAdapter_LoopRowsGatedByFlag(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	tplM := template.NewManager(sys, "templates", log)
	if err := tplM.EnsureTemplateDirectory(); err != nil {
		t.Fatalf("EnsureTemplateDirectory: %v", err)
	}
	sfrM := sfr.NewManager(sys, log)
	stoM := storage.NewManager(sys, sfrM, tplM, "storage", log)

	// A loop "resources" with two scalar child fields, between loopstart/loopstop.
	tpl := &template.Template{
		Name:     "adapters",
		Filename: "adapters.yaml",
		Fields: []template.Field{
			{Key: "title", Type: "text"},
			{Key: "resources", Type: "loopstart"},
			{Key: "name", Type: "text"},
			{Key: "kind", Type: "text"},
			{Key: "resources", Type: "loopstop"},
		},
	}
	if err := tplM.SaveTemplate("adapters.yaml", tpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}

	data := map[string]any{
		"title": "Progress",
		"resources": []any{
			map[string]any{"name": "plan", "kind": "app-service"},
			map[string]any{"name": "kv", "kind": "key-vault"},
		},
	}
	if r := stoM.SaveForm(context.Background(), "adapters.yaml", "x.meta.json", data); !r.Success {
		t.Fatalf("SaveForm: %s", r.Error)
	}

	// Off: the loop is not ingested, so Follow yields nothing.
	off, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, nil, nil, "adapters.yaml", false))
	if err != nil {
		t.Fatalf("datacore.Build (off): %v", err)
	}
	assertBuckets(t, "resources.name (off)", off.View().Follow("resources").Distribution("name"), map[string]int{})

	// On: the loop becomes a row table; its rows are Followable like a table's.
	on, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, nil, nil, "adapters.yaml", true))
	if err != nil {
		t.Fatalf("datacore.Build (on): %v", err)
	}
	assertBuckets(t, "resources.name (on)", on.View().Follow("resources").Distribution("name"),
		map[string]int{"plan": 1, "kv": 1})
	assertBuckets(t, "resources.kind (on)", on.View().Follow("resources").Distribution("kind"),
		map[string]int{"app-service": 1, "key-vault": 1})
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

	dt, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, nil, rel, "entities.yaml", false))
	if err != nil {
		t.Fatalf("datacore.Build: %v", err)
	}
	// Following the self-relation from every record lands on the parent (deduped).
	assertBuckets(t, "rel:entities.yaml -> name",
		dt.View().Follow("rel:entities.yaml").Distribution("name"),
		map[string]int{"Parent": 1})
}

// TestDatacoreAdapter_CrossTemplateFollowReachesTable is the cross-template
// counterpart to the self chain: project relates to person, and following the
// relation reaches person records (loaded as satellites) AND descends into their
// table. The satellites must NOT inflate the primary root count.
func TestDatacoreAdapter_CrossTemplateFollowReachesTable(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	tplM := template.NewManager(sys, "templates", log)
	if err := tplM.EnsureTemplateDirectory(); err != nil {
		t.Fatalf("EnsureTemplateDirectory: %v", err)
	}
	sfrM := sfr.NewManager(sys, log)
	stoM := storage.NewManager(sys, sfrM, tplM, "storage", log)

	project := &template.Template{
		Name: "project", Filename: "project.yaml", EnableCollection: true,
		Fields: []template.Field{{Key: "id", Type: "guid"}, {Key: "name", Type: "text"}},
	}
	person := &template.Template{
		Name: "person", Filename: "person.yaml", EnableCollection: true,
		Fields: []template.Field{
			{Key: "id", Type: "guid"},
			{Key: "name", Type: "text"},
			{Key: "roles", Type: "table", Options: []any{
				map[string]any{"value": "role", "label": "Role"},
			}},
		},
	}
	if err := tplM.SaveTemplate("project.yaml", project); err != nil {
		t.Fatalf("SaveTemplate project: %v", err)
	}
	if err := tplM.SaveTemplate("person.yaml", person); err != nil {
		t.Fatalf("SaveTemplate person: %v", err)
	}

	guid := func(tpl, file string, data map[string]any) string {
		if r := stoM.SaveForm(context.Background(), tpl, file, data); !r.Success {
			t.Fatalf("SaveForm %s/%s: %s", tpl, file, r.Error)
		}
		f := stoM.LoadForm(tpl, file)
		if f == nil || f.Meta.ID == "" {
			t.Fatalf("no guid for %s/%s", tpl, file)
		}
		return f.Meta.ID
	}
	gP := guid("project.yaml", "p1.meta.json", map[string]any{"name": "Proj"})
	gA := guid("person.yaml", "alice.meta.json", map[string]any{"name": "Alice", "roles": []any{[]any{"dev"}, []any{"lead"}}})
	gB := guid("person.yaml", "bob.meta.json", map[string]any{"name": "Bob", "roles": []any{[]any{"ops"}}})

	rel := fakeRelReader{rels: map[string][]relation.Relation{
		"project.yaml": {{
			To:          "person.yaml",
			Cardinality: relation.OneToMany,
			Edges:       []relation.Edge{{From: gP, To: gA}, {From: gP, To: gB}},
		}},
	}}
	_ = gP

	dt, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, nil, rel, "project.yaml", false))
	if err != nil {
		t.Fatalf("datacore.Build: %v", err)
	}
	// Satellites do not inflate the primary root count.
	if got := dt.View().Count(); got != 1 {
		t.Fatalf("root count = %d, want 1 (person satellites must not count)", got)
	}
	// Nor do satellite field values leak into a primary field reduction: a
	// Distribution over "name" sees only the project root, not Alice/Bob.
	assertBuckets(t, "primary name isolation",
		dt.View().Distribution("name"), map[string]int{"Proj": 1})
	// Cross-template follow reaches the person records.
	assertBuckets(t, "rel:person -> name",
		dt.View().Follow("rel:person.yaml").Distribution("name"),
		map[string]int{"Alice": 1, "Bob": 1})
	// ...and descends into their table: project -> person -> person.roles.
	assertBuckets(t, "rel:person -> roles.role",
		dt.View().Follow("rel:person.yaml").Follow("roles").Distribution("role"),
		map[string]int{"dev": 1, "lead": 1, "ops": 1})
	// Summarize iterates roots, so it sees only the one project record (with its
	// two outgoing links), never the person satellites as their own roots.
	sum := dt.View().Summarize("rel:person.yaml", "")
	if len(sum) != 1 || sum[0].Rows != 2 {
		t.Fatalf("Summarize should yield one project root with 2 links, got %+v", sum)
	}
}

// twoCollectionEnv sets up project + person collection templates and returns the
// managers, for the cross-template unhappy-path tests.
func twoCollectionEnv(t *testing.T) (*template.Manager, *storage.Manager) {
	t.Helper()
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	tplM := template.NewManager(sys, "templates", log)
	if err := tplM.EnsureTemplateDirectory(); err != nil {
		t.Fatalf("EnsureTemplateDirectory: %v", err)
	}
	sfrM := sfr.NewManager(sys, log)
	stoM := storage.NewManager(sys, sfrM, tplM, "storage", log)
	for _, tpl := range []*template.Template{
		{Name: "project", Filename: "project.yaml", EnableCollection: true,
			Fields: []template.Field{{Key: "id", Type: "guid"}, {Key: "name", Type: "text"}}},
		{Name: "person", Filename: "person.yaml", EnableCollection: true,
			Fields: []template.Field{{Key: "id", Type: "guid"}, {Key: "name", Type: "text"}}},
	} {
		if err := tplM.SaveTemplate(tpl.Filename, tpl); err != nil {
			t.Fatalf("SaveTemplate %s: %v", tpl.Filename, err)
		}
	}
	return tplM, stoM
}

func saveGuid(t *testing.T, stoM *storage.Manager, tpl, file string, data map[string]any) string {
	t.Helper()
	if r := stoM.SaveForm(context.Background(), tpl, file, data); !r.Success {
		t.Fatalf("SaveForm %s/%s: %s", tpl, file, r.Error)
	}
	f := stoM.LoadForm(tpl, file)
	if f == nil || f.Meta.ID == "" {
		t.Fatalf("no guid for %s/%s", tpl, file)
	}
	return f.Meta.ID
}

// Regression for the satellite-leak bug: a primary template with ZERO records
// that declares a cross relation must reduce to nothing, the pulled-in
// satellites must not become the working set when rootList is empty.
func TestDatacoreAdapter_ZeroPrimaryWithSatellitesIsEmpty(t *testing.T) {
	tplM, stoM := twoCollectionEnv(t)
	gA := saveGuid(t, stoM, "person.yaml", "alice.meta.json", map[string]any{"name": "Alice"})
	gB := saveGuid(t, stoM, "person.yaml", "bob.meta.json", map[string]any{"name": "Bob"})
	// project has no records; the edges' source is a since-gone project record.
	rel := fakeRelReader{rels: map[string][]relation.Relation{
		"project.yaml": {{To: "person.yaml", Cardinality: relation.OneToMany,
			Edges: []relation.Edge{{From: "ghost-project", To: gA}, {From: "ghost-project", To: gB}}}},
	}}
	dt, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, nil, rel, "project.yaml", false))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := dt.View().Count(); got != 0 {
		t.Fatalf("zero-primary count = %d, want 0 (satellites must not leak as roots)", got)
	}
	if got := dt.View().Distribution("name"); len(got) != 0 {
		t.Fatalf("zero-primary distribution = %v, want empty", got)
	}
}

// The composite key must keep the SAME filename in two templates distinct even
// when one is a satellite of the other.
func TestDatacoreAdapter_SatelliteFilenameCollisionStaysDistinct(t *testing.T) {
	tplM, stoM := twoCollectionEnv(t)
	gP := saveGuid(t, stoM, "project.yaml", "x.meta.json", map[string]any{"name": "Proj"})
	gPerson := saveGuid(t, stoM, "person.yaml", "x.meta.json", map[string]any{"name": "Alice"})
	rel := fakeRelReader{rels: map[string][]relation.Relation{
		"project.yaml": {{To: "person.yaml", Cardinality: relation.OneToMany,
			Edges: []relation.Edge{{From: gP, To: gPerson}}}},
	}}
	dt, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, nil, rel, "project.yaml", false))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := dt.View().Count(); got != 1 {
		t.Fatalf("count = %d, want 1 (only the project x.meta.json is a root)", got)
	}
	assertBuckets(t, "primary x", dt.View().Distribution("name"), map[string]int{"Proj": 1})
	assertBuckets(t, "follow to person x",
		dt.View().Follow("rel:person.yaml").Distribution("name"), map[string]int{"Alice": 1})
}

// Two primary records linking the SAME target ingest the satellite once; Follow
// dedups to a single target.
func TestDatacoreAdapter_SatelliteDedup(t *testing.T) {
	tplM, stoM := twoCollectionEnv(t)
	gP1 := saveGuid(t, stoM, "project.yaml", "p1.meta.json", map[string]any{"name": "P1"})
	gP2 := saveGuid(t, stoM, "project.yaml", "p2.meta.json", map[string]any{"name": "P2"})
	gA := saveGuid(t, stoM, "person.yaml", "alice.meta.json", map[string]any{"name": "Alice"})
	rel := fakeRelReader{rels: map[string][]relation.Relation{
		"project.yaml": {{To: "person.yaml", Cardinality: relation.ManyToMany,
			Edges: []relation.Edge{{From: gP1, To: gA}, {From: gP2, To: gA}}}},
	}}
	dt, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, nil, rel, "project.yaml", false))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	assertBuckets(t, "deduped follow",
		dt.View().Follow("rel:person.yaml").Distribution("name"), map[string]int{"Alice": 1})
}

// The graph must render a cross-template satellite as a record (kind "root"),
// not a table row, and never leak the raw composite id as its label.
func TestDatacoreAdapter_GraphSatelliteIsRecordNotRow(t *testing.T) {
	tplM, stoM := twoCollectionEnv(t) // person has no ItemField, exercises the label fallback
	gP := saveGuid(t, stoM, "project.yaml", "p1.meta.json", map[string]any{"name": "Proj"})
	gA := saveGuid(t, stoM, "person.yaml", "alice.meta.json", map[string]any{"name": "Alice"})
	rel := fakeRelReader{rels: map[string][]relation.Relation{
		"project.yaml": {{To: "person.yaml", Cardinality: relation.OneToMany,
			Edges: []relation.Edge{{From: gP, To: gA}}}},
	}}
	dt, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, nil, rel, "project.yaml", false))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	satID := datacore.NewID("person.yaml", "alice.meta.json")
	var sat *datacore.GraphNode
	g := dt.GraphFrom(datacore.NewID("project.yaml", "p1.meta.json"), 2)
	for i := range g.Nodes {
		if g.Nodes[i].ID == satID {
			sat = &g.Nodes[i]
		}
	}
	if sat == nil {
		t.Fatalf("satellite node missing from graph: %+v", g.Nodes)
	}
	if sat.Kind != "root" {
		t.Fatalf("satellite kind = %q, want root (it is a record, not a table row)", sat.Kind)
	}
	if strings.Contains(sat.Label, "\x1f") {
		t.Fatalf("satellite label leaks the composite id: %q", sat.Label)
	}
}

// A cross-template edge whose target record was deleted must be tolerated: the
// build succeeds and the follow returns only the live targets (volatility).
func TestDatacoreAdapter_CrossTemplate_DanglingEdgeTolerated(t *testing.T) {
	tplM, stoM := twoCollectionEnv(t)
	gP := saveGuid(t, stoM, "project.yaml", "p1.meta.json", map[string]any{"name": "Proj"})
	gA := saveGuid(t, stoM, "person.yaml", "alice.meta.json", map[string]any{"name": "Alice"})

	rel := fakeRelReader{rels: map[string][]relation.Relation{
		"project.yaml": {{To: "person.yaml", Cardinality: relation.OneToMany,
			Edges: []relation.Edge{{From: gP, To: gA}, {From: gP, To: "ghost-guid"}}}},
	}}
	dt, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, nil, rel, "project.yaml", false))
	if err != nil {
		t.Fatalf("Build must tolerate a dangling edge: %v", err)
	}
	assertBuckets(t, "live target only",
		dt.View().Follow("rel:person.yaml").Distribution("name"), map[string]int{"Alice": 1})
}

// A relation pointing at a template that no longer exists must not break the
// primary tensor build.
func TestDatacoreAdapter_CrossTemplate_MissingTargetTemplateTolerated(t *testing.T) {
	tplM, stoM := twoCollectionEnv(t)
	gP := saveGuid(t, stoM, "project.yaml", "p1.meta.json", map[string]any{"name": "Proj"})

	rel := fakeRelReader{rels: map[string][]relation.Relation{
		"project.yaml": {{To: "gone.yaml", Cardinality: relation.OneToMany,
			Edges: []relation.Edge{{From: gP, To: "g-x"}}}},
	}}
	dt, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, nil, rel, "project.yaml", false))
	if err != nil {
		t.Fatalf("Build must tolerate a missing target template: %v", err)
	}
	if got := dt.View().Count(); got != 1 {
		t.Fatalf("root count = %d, want 1", got)
	}
	if got := dt.View().Follow("rel:gone.yaml").Count(); got != 0 {
		t.Fatalf("follow into a gone template = %d, want 0", got)
	}
}

// One template carrying both a self relation and a cross relation: each follow
// key resolves independently, and the count stays the primary roots only.
func TestDatacoreAdapter_SelfAndCrossMix(t *testing.T) {
	tplM, stoM := twoCollectionEnv(t)
	gP1 := saveGuid(t, stoM, "project.yaml", "p1.meta.json", map[string]any{"name": "P1"})
	gP2 := saveGuid(t, stoM, "project.yaml", "p2.meta.json", map[string]any{"name": "P2"})
	gA := saveGuid(t, stoM, "person.yaml", "alice.meta.json", map[string]any{"name": "Alice"})

	rel := fakeRelReader{rels: map[string][]relation.Relation{
		"project.yaml": {
			{To: "project.yaml", Cardinality: relation.ManyToOne, Edges: []relation.Edge{{From: gP1, To: gP2}}},
			{To: "person.yaml", Cardinality: relation.OneToMany, Edges: []relation.Edge{{From: gP1, To: gA}}},
		},
	}}
	dt, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, nil, rel, "project.yaml", false))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := dt.View().Count(); got != 2 {
		t.Fatalf("root count = %d, want 2 (the two project records)", got)
	}
	assertBuckets(t, "self follow",
		dt.View().Follow("rel:project.yaml").Distribution("name"), map[string]int{"P2": 1})
	assertBuckets(t, "cross follow",
		dt.View().Follow("rel:person.yaml").Distribution("name"), map[string]int{"Alice": 1})
}

// TestDatacoreAdapter_IdentityCarriesTemplate pins the composite-key contract at
// the loader boundary: the loader must address records by template+filename, not
// the bare filename. Asserted directly because the value-based Follow tests would
// pass either way.
func TestDatacoreAdapter_IdentityCarriesTemplate(t *testing.T) {
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
		Name: "entities", Filename: "entities.yaml", EnableCollection: true,
		Fields: []template.Field{{Key: "id", Type: "guid"}, {Key: "name", Type: "text"}},
	}
	if err := tplM.SaveTemplate("entities.yaml", tpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	if r := stoM.SaveForm(context.Background(), "entities.yaml", "rec.meta.json", map[string]any{"name": "Rec"}); !r.Success {
		t.Fatalf("SaveForm: %s", r.Error)
	}

	dt, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, nil, nil, "entities.yaml", false))
	if err != nil {
		t.Fatalf("datacore.Build: %v", err)
	}
	if g := dt.GraphFrom(datacore.NewID("entities.yaml", "rec.meta.json"), 0); len(g.Nodes) != 1 {
		t.Fatalf("composite identity not found in tensor: %+v", g.Nodes)
	}
	if g := dt.GraphFrom("rec.meta.json", 0); len(g.Nodes) != 0 {
		t.Fatalf("bare filename resolved; identity must carry the template")
	}
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

	dt, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, nil, rel, "entities.yaml", false))
	if err != nil {
		t.Fatalf("datacore.Build: %v", err)
	}
	// Follow the relation to the parent, then descend into the parent's attrs table.
	assertBuckets(t, "rel -> attrs.attr",
		dt.View().Follow("rel:entities.yaml").Follow("attrs").Distribution("attr"),
		map[string]int{"a1": 1, "a2": 1})
}

// A template's GraphPrefixField is a free-text origin short-code prepended to
// its graph node labels, so records from different collections that share an
// item-field value (e.g. an audit-control code) read distinctly by origin:
// "TK: CH.02" vs "PRC: CH.02". Without it they collapse onto the shared code
// (the bug this fixes).
func TestDatacoreAdapter_GraphPrefixLabelsRelatedNodes(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	tplM := template.NewManager(sys, "templates", log)
	if err := tplM.EnsureTemplateDirectory(); err != nil {
		t.Fatalf("EnsureTemplateDirectory: %v", err)
	}
	sfrM := sfr.NewManager(sys, log)
	stoM := storage.NewManager(sys, sfrM, tplM, "storage", log)

	// A control collection linked to three satellite collections, each with its
	// own origin short-code; every record carries the same item-field code.
	type tdef struct{ file, code string }
	focusT := tdef{"control.yaml", "CTRL"}
	satT := []tdef{{"toets.yaml", "TK"}, {"proces.yaml", "PRC"}, {"richtlijn.yaml", "RL"}}
	saveTpls := func(withPrefix bool) {
		for _, d := range append([]tdef{focusT}, satT...) {
			prefix := ""
			if withPrefix {
				prefix = d.code
			}
			tpl := &template.Template{
				Name: d.file, Filename: d.file, EnableCollection: true,
				ItemField: "code", GraphPrefixField: prefix,
				Fields: []template.Field{{Key: "id", Type: "guid"}, {Key: "code", Type: "text"}},
			}
			if err := tplM.SaveTemplate(d.file, tpl); err != nil {
				t.Fatalf("SaveTemplate %s: %v", d.file, err)
			}
		}
	}
	saveTpls(true)

	gC := saveGuid(t, stoM, focusT.file, "c.meta.json", map[string]any{"code": "CH.02"})
	satIDs := map[string]bool{}
	rels := make([]relation.Relation, 0, len(satT))
	for _, d := range satT {
		g := saveGuid(t, stoM, d.file, "r.meta.json", map[string]any{"code": "CH.02"})
		rels = append(rels, relation.Relation{To: d.file, Cardinality: relation.OneToMany,
			Edges: []relation.Edge{{From: gC, To: g}}})
		satIDs[datacore.NewID(d.file, "r.meta.json")] = true
	}
	rel := fakeRelReader{rels: map[string][]relation.Relation{focusT.file: rels}}

	focus := datacore.NewID(focusT.file, "c.meta.json")
	graphLabels := func() (string, map[string]bool) {
		dt, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, nil, rel, focusT.file, false))
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		g := dt.GraphFrom(focus, 2)
		focusLabel := ""
		sats := map[string]bool{}
		for i := range g.Nodes {
			n := g.Nodes[i]
			if n.ID == focus {
				focusLabel = n.Label
			} else if satIDs[n.ID] {
				sats[n.Label] = true
			}
		}
		return focusLabel, sats
	}

	// With origin short-codes, focus and every satellite read distinctly by
	// origin, the shared code still on the tail.
	focusLabel, sats := graphLabels()
	if focusLabel != "CTRL: CH.02" {
		t.Fatalf("focus label = %q, want %q", focusLabel, "CTRL: CH.02")
	}
	for _, want := range []string{"TK: CH.02", "PRC: CH.02", "RL: CH.02"} {
		if !sats[want] {
			t.Fatalf("satellite label %q missing; got %v", want, sats)
		}
	}
	if len(sats) != 3 {
		t.Fatalf("satellite labels = %v, want 3 distinct origins", sats)
	}

	// Without prefixes, the item field (shared code) labels them all - the
	// collapse this feature fixes.
	saveTpls(false)
	if _, collapsed := graphLabels(); len(collapsed) != 1 || !collapsed["CH.02"] {
		t.Fatalf("without prefix, satellite labels = %v, want all %q", collapsed, "CH.02")
	}
}

func TestDatacoreAdapter_GraphColorTintsTemplateNodes(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	tplM := template.NewManager(sys, "templates", log)
	if err := tplM.EnsureTemplateDirectory(); err != nil {
		t.Fatalf("EnsureTemplateDirectory: %v", err)
	}
	sfrM := sfr.NewManager(sys, log)
	stoM := storage.NewManager(sys, sfrM, tplM, "storage", log)

	// A colored focus collection linked to one colored satellite and one with no
	// color, so the per-template color rides through to the node and an unset
	// color leaves the node default.
	focusFile, satFile, plainFile := "control.yaml", "toets.yaml", "proces.yaml"
	colors := map[string]string{focusFile: "#4a90e2", satFile: "#e84e4e", plainFile: ""}
	for file, color := range colors {
		tpl := &template.Template{
			Name: file, Filename: file, EnableCollection: true,
			ItemField: "code", GraphColor: color,
			Fields: []template.Field{{Key: "id", Type: "guid"}, {Key: "code", Type: "text"}},
		}
		if err := tplM.SaveTemplate(file, tpl); err != nil {
			t.Fatalf("SaveTemplate %s: %v", file, err)
		}
	}

	gC := saveGuid(t, stoM, focusFile, "c.meta.json", map[string]any{"code": "CH.02"})
	gS := saveGuid(t, stoM, satFile, "s.meta.json", map[string]any{"code": "TK.01"})
	gP := saveGuid(t, stoM, plainFile, "p.meta.json", map[string]any{"code": "PR.01"})
	rels := []relation.Relation{
		{To: satFile, Cardinality: relation.OneToMany, Edges: []relation.Edge{{From: gC, To: gS}}},
		{To: plainFile, Cardinality: relation.OneToMany, Edges: []relation.Edge{{From: gC, To: gP}}},
	}
	rel := fakeRelReader{rels: map[string][]relation.Relation{focusFile: rels}}

	focus := datacore.NewID(focusFile, "c.meta.json")
	dt, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, nil, rel, focusFile, false))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	g := dt.GraphFrom(focus, 2)

	color := map[string]string{}
	for i := range g.Nodes {
		color[g.Nodes[i].ID] = g.Nodes[i].Color
	}
	if got := color[focus]; got != "#4a90e2" {
		t.Fatalf("focus node color = %q, want %q", got, "#4a90e2")
	}
	if got := color[datacore.NewID(satFile, "s.meta.json")]; got != "#e84e4e" {
		t.Fatalf("satellite node color = %q, want %q", got, "#e84e4e")
	}
	if got := color[datacore.NewID(plainFile, "p.meta.json")]; got != "" {
		t.Fatalf("uncolored template node color = %q, want empty", got)
	}
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
