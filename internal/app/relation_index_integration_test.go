package app

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/api"
	"github.com/petervdpas/formidable2/internal/modules/datacore"
	"github.com/petervdpas/formidable2/internal/modules/dataprovider"
	"github.com/petervdpas/formidable2/internal/modules/index"
	"github.com/petervdpas/formidable2/internal/modules/relation"
	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// TestRelationFollow_RealIndexEndToEnd wires the real composition (template +
// storage + SQLite index + dataprovider + relation manager) instead of stubs,
// so the guid->record resolution that relation-follow and the datacore loader
// depend on is exercised against the actual index: at AddEdge time
// (relationCatalog.RecordExists), at follow time (ResolveCollectionByID), and in
// the datacore loader's guid->identity map built from real LoadForm. Covers the
// integration gap the unit/godog tests (all stub-backed) leave open.
func TestRelationFollow_RealIndexEndToEnd(t *testing.T) {
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

	for _, tpl := range []*template.Template{
		{Name: "project", Filename: "project.yaml", ItemField: "name", EnableCollection: true,
			Fields: []template.Field{{Key: "id", Type: "guid"}, {Key: "name", Type: "text"}}},
		{Name: "person", Filename: "person.yaml", ItemField: "name", EnableCollection: true,
			Fields: []template.Field{{Key: "id", Type: "guid"}, {Key: "name", Type: "text"}}},
	} {
		if err := tplM.SaveTemplate(tpl.Filename, tpl); err != nil {
			t.Fatalf("SaveTemplate %s: %v", tpl.Filename, err)
		}
	}

	save := func(tpl, file string, data map[string]any) string {
		if r := stoM.SaveForm(context.Background(), tpl, file, data); !r.Success {
			t.Fatalf("SaveForm %s/%s: %s", tpl, file, r.Error)
		}
		f := stoM.LoadForm(tpl, file)
		if f == nil || f.Meta.ID == "" {
			t.Fatalf("no guid for %s/%s", tpl, file)
		}
		return f.Meta.ID
	}
	gP := save("project.yaml", "p1.meta.json", map[string]any{"name": "Proj"})
	gA := save("person.yaml", "alice.meta.json", map[string]any{"name": "Alice"})
	gB := save("person.yaml", "bob.meta.json", map[string]any{"name": "Bob"})

	dpM := dataprovider.NewManager(idxM, nil, stoM) // follow path builds hrefs without the renderer
	relM := relation.NewManager(sys, "relations", relationCatalog{dp: dpM})

	if err := relM.SetRelations("project.yaml", []relation.Relation{{To: "person.yaml", Cardinality: relation.OneToMany}}); err != nil {
		t.Fatalf("SetRelations: %v", err)
	}
	// AddEdge validates both endpoints exist via the REAL index (RecordExists ->
	// ResolveCollectionByID). A bad guid here would be rejected.
	for _, to := range []string{gA, gB} {
		if err := relM.AddEdge("project.yaml", "person.yaml", relation.Edge{From: gP, To: to}); err != nil {
			t.Fatalf("AddEdge %s->%s: %v", gP, to, err)
		}
	}

	// Follow over the real index through the api handler.
	h := api.NewHandler(dpM, stoM, stoM, tplM, nil, nil, apiRelations{rel: relM})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/collections/project/"+gP+"/relations/person", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("follow status = %d (%s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"total":2`) {
		t.Fatalf("want total 2 resolved through the real index: %s", body)
	}
	if !strings.Contains(body, "Alice") || !strings.Contains(body, "Bob") {
		t.Fatalf("follow did not resolve both linked records via the index: %s", body)
	}

	// Datacore cross-template Follow over the REAL loader (guid->identity map
	// built from real LoadForm + index-resolved relations).
	dt, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, nil, relM, "project.yaml", false))
	if err != nil {
		t.Fatalf("datacore.Build: %v", err)
	}
	if got := dt.View().Count(); got != 1 {
		t.Fatalf("datacore primary count = %d, want 1", got)
	}
	names := map[string]int{}
	for _, b := range dt.View().Follow("rel:person.yaml").Distribution("name") {
		names[b.Value] = b.Count
	}
	if names["Alice"] != 1 || names["Bob"] != 1 {
		t.Fatalf("datacore cross-template follow over real loader = %v, want Alice+Bob", names)
	}
}
