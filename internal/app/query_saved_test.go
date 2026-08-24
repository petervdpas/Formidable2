package app

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/config"
	"github.com/petervdpas/formidable2/internal/modules/query"
	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// TestSavedQueryRoundTripRuns stitches the real filesystem, storage and query
// managers together: build a spec, run it, save it, reload it from disk, and
// run the reloaded spec. The rows must match, which is the whole promise of a
// saved query. It also pins the on-disk location, since that is what decides
// whether a saved query rides along with a git/gigot sync.
func TestSavedQueryRoundTripRuns(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	tplM := template.NewManager(sys, "templates", log)
	if err := tplM.EnsureTemplateDirectory(); err != nil {
		t.Fatalf("EnsureTemplateDirectory: %v", err)
	}
	sfrM := sfr.NewManager(sys, log)
	storagePath := filepath.Join(root, "storage")
	stoM := storage.NewManager(sys, sfrM, tplM, storagePath, log)

	tpl := &template.Template{
		Name:     "audit",
		Filename: "audit.yaml",
		Fields: []template.Field{
			{Key: "nba_id", Type: "text", Label: "NBA-ID"},
			{Key: "category", Type: "text", Label: "Category"},
		},
	}
	if err := tplM.SaveTemplate("audit.yaml", tpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	rows := []struct {
		file, id, cat string
	}{
		{"a.meta.json", "CH.02", "Impact assessment"},
		{"b.meta.json", "CH.03", "Noodaanpassingen"},
		{"c.meta.json", "HR.03", "Afhankelijkheid"},
	}
	for _, r := range rows {
		res := stoM.SaveForm(context.Background(), "audit.yaml", r.file, map[string]any{"nba_id": r.id, "category": r.cat})
		if !res.Success {
			t.Fatalf("SaveForm %s: %s", r.file, res.Error)
		}
	}

	qm := query.NewManager(newQueryLoaderAdapter(tplM, stoM))
	localRoot := filepath.Join(root, "queries")
	location := query.LocationLocal
	store := query.NewStore(sys, localRoot, storagePath, func() string { return location }, log)

	spec := query.Spec{
		Template: "audit.yaml",
		Columns: []query.Column{
			{Header: "NBA-ID", Source: query.Source{Kind: "field", Key: "nba_id"}},
			{Header: "Category", Source: query.Source{Kind: "field", Key: "category"}},
		},
		Filters: []query.Filter{{Source: query.Source{Kind: "field", Key: "nba_id"}, Op: "ne", Value: "HR.03"}},
		OrderBy: []query.Sort{{Column: 0}},
	}
	before, err := qm.Run(spec)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if before.Count != 2 {
		t.Fatalf("rows = %d, want 2", before.Count)
	}

	saved, err := store.Save(query.SavedQuery{Name: "In scope", Template: "audit.yaml", Spec: spec})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	onDisk := filepath.Join(localRoot, "audit", "in-scope.json")
	if _, err := os.Stat(onDisk); err != nil {
		t.Fatalf("saved query not at %s: %v", onDisk, err)
	}
	if saved.Location != query.LocationLocal {
		t.Fatalf("location = %q, want local", saved.Location)
	}

	list, err := store.List("audit.yaml")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != saved.ID || list[0].Name != "In scope" {
		t.Fatalf("List = %+v, want the one saved query", list)
	}

	reopened, err := store.Get("audit.yaml", saved.ID, saved.Location)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	after, err := qm.Run(reopened.Spec)
	if err != nil {
		t.Fatalf("Run reopened: %v", err)
	}
	if after.Count != before.Count || len(after.Rows) != len(before.Rows) {
		t.Fatalf("reopened result differs: %d rows vs %d", after.Count, before.Count)
	}
	for i := range before.Rows {
		for j := range before.Rows[i] {
			if after.Rows[i][j].Text != before.Rows[i][j].Text {
				t.Fatalf("row %d col %d = %q, want %q", i, j, after.Rows[i][j].Text, before.Rows[i][j].Text)
			}
		}
	}

	// The default location must leave the synced tree untouched: saving a
	// query is not a reason to produce a commit.
	if entries, err := os.ReadDir(filepath.Join(storagePath, "audit")); err == nil {
		for _, e := range entries {
			if e.IsDir() && e.Name() == "queries" {
				t.Fatal("a local save reached the storage tree")
			}
		}
	}

	// Switch the setting to shared: the same query now lands in the storage
	// tree, and a saved query must still never become a record, since the form
	// readers only see .meta.json files.
	location = query.LocationShared
	shared, err := store.Save(query.SavedQuery{Name: "Team scope", Template: "audit.yaml", Spec: spec})
	if err != nil {
		t.Fatalf("Save shared: %v", err)
	}
	if shared.Location != query.LocationShared {
		t.Fatalf("location = %q, want shared", shared.Location)
	}
	sharedOnDisk := filepath.Join(storagePath, "audit", "queries", "team-scope.json")
	if _, err := os.Stat(sharedOnDisk); err != nil {
		t.Fatalf("shared query not at %s: %v", sharedOnDisk, err)
	}
	forms, err := stoM.ListForms("audit.yaml")
	if err != nil {
		t.Fatalf("ListForms: %v", err)
	}
	if len(forms) != 3 {
		t.Fatalf("ListForms = %v, want the 3 records only", forms)
	}

	// Both roots stay readable whatever the setting says.
	both, err := store.List("audit.yaml")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(both) != 2 {
		t.Fatalf("List = %+v, want the local and the shared one", both)
	}

	if err := store.Delete("audit.yaml", saved.ID, saved.Location); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(onDisk); !os.IsNotExist(err) {
		t.Fatalf("saved query survived delete: %v", err)
	}
}

// TestConfigDefaultSavedQueryLocationIsKnown is the drift guard between the
// config default and the closed set the query module owns. The composition
// root is where the two meet, so the guard lives here.
func TestConfigDefaultSavedQueryLocationIsKnown(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	cfgM, err := config.NewManager(sys, nil)
	if err != nil {
		t.Fatalf("config.NewManager: %v", err)
	}
	cfg, err := cfgM.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	if cfg.SavedQueryLocation != query.LocationLocal {
		t.Fatalf("default saved_query_location = %q, want %q (a default that syncs would publish queries nobody asked to share)",
			cfg.SavedQueryLocation, query.LocationLocal)
	}
	if got := query.NormalizeLocation(cfg.SavedQueryLocation); got != cfg.SavedQueryLocation {
		t.Fatalf("default %q normalizes to %q; the config default is not in query.StoreLocations", cfg.SavedQueryLocation, got)
	}
}
