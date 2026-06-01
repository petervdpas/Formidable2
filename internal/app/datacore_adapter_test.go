package app

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/datacore"
	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

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

	dt, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, nil, "assets.yaml"))
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
