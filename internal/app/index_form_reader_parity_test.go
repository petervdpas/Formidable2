package app

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/index"
	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// TestIndexFormReader_ParityWithDiskPath stitches the real composition
// - template + storage + index + event handler + the new
// indexFormReader - together and proves the index-sourced
// ExtendedListForms returns the same FormSummary slice as the disk
// path it replaces. Covers all of step 2 at the integration level:
// facets, audit identity (created/updated name+email), tags,
// expression-items round-trip through the SQLite blob.
func TestIndexFormReader_ParityWithDiskPath(t *testing.T) {
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

	// Template with the full feature surface: item_field, tags field,
	// an expression-flagged field, and two facets so the round-trip
	// covers everything FormSummary carries.
	tpl := &template.Template{
		Name:      "recipes",
		Filename:  "recipes.yaml",
		ItemField: "title",
		Fields: []template.Field{
			{Key: "title", Type: "text"},
			{Key: "labels", Type: "tags"},
			{Key: "category", Type: "text", ExpressionItem: true},
		},
		Facets: []template.Facet{
			{Key: "stage", Icon: "fa-flag", Options: []template.FacetOption{
				{Label: "DRAFT", Color: "amber"},
				{Label: "PUBLISHED", Color: "green"},
			}},
			{Key: "priority", Icon: "fa-fire", Options: []template.FacetOption{
				{Label: "LOW", Color: "blue"},
				{Label: "HIGH", Color: "red"},
			}},
		},
	}
	if err := tplM.SaveTemplate("recipes.yaml", tpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}

	// Two fixture records covering both facet variants (set=true with
	// selected, set=false) and disjoint tag sets.
	saves := []struct {
		filename string
		data     map[string]any
	}{
		{"alpha.meta.json", map[string]any{
			"title":    "Alpha",
			"labels":   []any{"red", "blue"},
			"category": "savoury",
			"meta": map[string]any{
				"facets": map[string]any{
					"stage": map[string]any{"set": true, "selected": "DRAFT"},
				},
			},
		}},
		{"beta.meta.json", map[string]any{
			"title":    "Beta",
			"labels":   []any{"green"},
			"category": "sweet",
			"meta": map[string]any{
				"facets": map[string]any{
					"stage":    map[string]any{"set": true, "selected": "PUBLISHED"},
					"priority": map[string]any{"set": false},
				},
			},
		}},
	}
	for _, s := range saves {
		r := stoM.SaveForm(context.Background(), "recipes.yaml", s.filename, s.data)
		if !r.Success {
			t.Fatalf("SaveForm %s: %s", s.filename, r.Error)
		}
	}

	// Snapshot the disk path BEFORE installing the reader - that's the
	// only way to call the disk fallback without poking at unexported
	// helpers. SaveForm already triggered the index event handler, so
	// the index is populated and ready for the second snapshot.
	disk, err := stoM.ExtendedListForms("recipes.yaml")
	if err != nil {
		t.Fatalf("disk ExtendedListForms: %v", err)
	}

	stoM.SetReader(newIndexFormReader(idxM))
	idxSummaries, err := stoM.ExtendedListForms("recipes.yaml")
	if err != nil {
		t.Fatalf("indexed ExtendedListForms: %v", err)
	}

	if len(disk) != len(idxSummaries) {
		t.Fatalf("len mismatch: disk=%d idx=%d", len(disk), len(idxSummaries))
	}

	byFile := func(in []storage.FormSummary) map[string]storage.FormSummary {
		out := make(map[string]storage.FormSummary, len(in))
		for _, s := range in {
			sort.Strings(s.Meta.Tags)
			out[s.Filename] = s
		}
		return out
	}
	d := byFile(disk)
	x := byFile(idxSummaries)

	for filename, dSum := range d {
		xSum, ok := x[filename]
		if !ok {
			t.Errorf("index missing record %q", filename)
			continue
		}
		// FormMeta sub-fields, walked individually so failures point at
		// the exact mismatch instead of dumping the whole struct.
		if dSum.Meta.ID != xSum.Meta.ID {
			t.Errorf("%s ID: disk=%q idx=%q", filename, dSum.Meta.ID, xSum.Meta.ID)
		}
		if dSum.Meta.Template != xSum.Meta.Template {
			t.Errorf("%s Template: disk=%q idx=%q", filename, dSum.Meta.Template, xSum.Meta.Template)
		}
		if dSum.Meta.Created != xSum.Meta.Created {
			t.Errorf("%s Created: disk=%+v idx=%+v", filename, dSum.Meta.Created, xSum.Meta.Created)
		}
		if dSum.Meta.Updated != xSum.Meta.Updated {
			t.Errorf("%s Updated: disk=%+v idx=%+v", filename, dSum.Meta.Updated, xSum.Meta.Updated)
		}
		if !reflect.DeepEqual(dSum.Meta.Tags, xSum.Meta.Tags) {
			t.Errorf("%s Tags: disk=%v idx=%v", filename, dSum.Meta.Tags, xSum.Meta.Tags)
		}
		if !reflect.DeepEqual(dSum.Meta.Facets, xSum.Meta.Facets) {
			t.Errorf("%s Facets: disk=%+v idx=%+v", filename, dSum.Meta.Facets, xSum.Meta.Facets)
		}
		if dSum.Title != xSum.Title {
			t.Errorf("%s Title: disk=%q idx=%q", filename, dSum.Title, xSum.Title)
		}
		if !reflect.DeepEqual(dSum.ExpressionItems, xSum.ExpressionItems) {
			t.Errorf("%s ExpressionItems: disk=%+v idx=%+v",
				filename, dSum.ExpressionItems, xSum.ExpressionItems)
		}
	}
}
