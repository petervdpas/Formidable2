package app

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/dataprovider"
	"github.com/petervdpas/formidable2/internal/modules/form"
	"github.com/petervdpas/formidable2/internal/modules/index"
	"github.com/petervdpas/formidable2/internal/modules/relation"
	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// fakeFormCfg satisfies form's (unexported) configReader with zero defaults.
type fakeFormCfg struct{}

func (fakeFormCfg) FormDefaults() form.ConfigDefaults { return form.ConfigDefaults{} }

// TestImportRelationEdges_RealStack exercises the relations-import pass through
// the real composition (template + storage + SQLite index + dataprovider + form
// manager + relation manager), exactly as the importer dialog drives it: records
// already saved with their source GUIDs, then ImportRelationEdges writes the api
// value onto the source record and the save-time syncer mirrors the edges into
// the relation file. The earlier coverage was all stub-backed; this is the
// integration gap that let the "relations import does nothing" bug hide.
func TestImportRelationEdges_RealStack(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	tplM := template.NewManager(sys, "templates", log)
	if err := tplM.EnsureTemplateDirectory(); err != nil {
		t.Fatalf("EnsureTemplateDirectory: %v", err)
	}
	sfrM := sfr.NewManager(sys, log)
	stoM := storage.NewManager(sys, sfrM, tplM, "storage", log)

	idxM, err := index.NewManager(filepath.Join(root, "index", "test.db"))
	if err != nil {
		t.Fatalf("index.NewManager: %v", err)
	}
	t.Cleanup(func() { idxM.Close() })
	loaderAdapter := newIndexLoaderAdapter(tplM, stoM)
	ehM := index.NewEventHandler(idxM, loaderAdapter, loaderAdapter)
	ehM.SetRoot(root)
	tplM.SetIndexer(ehM)
	stoM.SetIndexer(ehM)

	// Source collection with an api field -> target collection; target collection.
	for _, tpl := range []*template.Template{
		{Name: "functie", Filename: "functie.yaml", ItemField: "naam", EnableCollection: true,
			Fields: []template.Field{
				{Key: "id", Type: "guid"},
				{Key: "naam", Type: "text"},
				{Key: "processen", Type: "api", Collection: "proces.yaml",
					Map: []template.APIMap{{Key: "proces"}}},
			}},
		{Name: "proces", Filename: "proces.yaml", ItemField: "proces", EnableCollection: true,
			Fields: []template.Field{{Key: "id", Type: "guid"}, {Key: "proces", Type: "text"}}},
	} {
		if err := tplM.SaveTemplate(tpl.Filename, tpl); err != nil {
			t.Fatalf("SaveTemplate %s: %v", tpl.Filename, err)
		}
	}

	// Save records WITH explicit ids, mirroring the records-import pass that maps
	// the source ID column onto the guid field.
	save := func(tpl, file string, data map[string]any) {
		if r := stoM.SaveForm(context.Background(), tpl, file, data); !r.Success {
			t.Fatalf("SaveForm %s/%s: %s", tpl, file, r.Error)
		}
		f := stoM.LoadForm(tpl, file)
		if f == nil || f.Meta.ID != data["id"] {
			t.Fatalf("record %s/%s did not keep id %v (got %v)", tpl, file, data["id"], f)
		}
	}
	save("proces.yaml", "p1.meta.json", map[string]any{"id": "P-1", "proces": "Proc1"})
	save("proces.yaml", "p2.meta.json", map[string]any{"id": "P-2", "proces": "Proc2"})
	save("functie.yaml", "f1.meta.json", map[string]any{"id": "F-1", "naam": "Func1"})

	dpM := dataprovider.NewManager(idxM, nil, stoM)
	relM := relation.NewManager(sys, "relations", relationCatalog{dp: dpM})
	if err := relM.SetRelations("functie.yaml", []relation.Relation{
		{To: "proces.yaml", Cardinality: relation.ManyToMany}}); err != nil {
		t.Fatalf("SetRelations: %v", err)
	}

	// Wire the form manager exactly as app.go does: edge syncer + guid->datafile
	// resolver over the real dataprovider.
	formM := form.NewManager(tplM, stoM, fakeFormCfg{}, log)
	formM.SetReferenceEdgeSyncer(referenceEdgeSyncer{rel: relM})
	formM.SetRecordResolver(func(tpl, guid string) (string, bool) {
		item, ok, err := dpM.ResolveCollectionByID(context.Background(), tpl, guid)
		if err != nil || !ok || item == nil {
			return "", false
		}
		return item.Filename, true
	})

	// The relations import: link F-1 to P-1 and P-2.
	res, err := formM.ImportRelationEdges("functie.yaml", "processen", []form.EdgePair{
		{From: "F-1", To: "P-1"},
		{From: "F-1", To: "P-2"},
	})
	if err != nil {
		t.Fatalf("ImportRelationEdges: %v", err)
	}
	if res.Linked != 2 || res.Records != 1 || res.MissingFrom != 0 || res.MissingTo != 0 {
		t.Fatalf("result = %+v, want Linked=2 Records=1 no misses", res)
	}

	// 1) The api value must be persisted on the source record.
	f := stoM.LoadForm("functie.yaml", "f1.meta.json")
	ids := toStringSet(f.Data["processen"])
	if !ids["P-1"] || !ids["P-2"] || len(ids) != 2 {
		t.Fatalf("processen api value = %#v, want [P-1 P-2]", f.Data["processen"])
	}

	// 2) The edges must be mirrored into the relation graph by the syncer.
	rels, err := relM.GetRelations("functie.yaml")
	if err != nil {
		t.Fatalf("GetRelations: %v", err)
	}
	edges := map[string]bool{}
	for _, r := range rels {
		if r.To == "proces.yaml" {
			for _, e := range r.Edges {
				if e.From == "F-1" {
					edges[e.To] = true
				}
			}
		}
	}
	if !edges["P-1"] || !edges["P-2"] {
		t.Fatalf("relation edges = %v, want F-1 -> P-1,P-2", edges)
	}
}

func toStringSet(v any) map[string]bool {
	out := map[string]bool{}
	switch t := v.(type) {
	case []string:
		for _, s := range t {
			out[s] = true
		}
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok {
				out[s] = true
			}
		}
	}
	return out
}
