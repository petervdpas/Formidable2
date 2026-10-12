package app

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestDiagramImageSync_TemplateChangeDrawsExistingRecordsAndPrunes(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sys := system.NewManager(t.TempDir(), log)
	tplM := template.NewManager(sys, "templates", log)
	stoM := storage.NewManager(sys, sfr.NewManager(sys, log), tplM, "storage", log)
	tplM.AddUpdateObserver(tableColumnMigrator{sto: stoM})
	tplM.AddUpdateObserver(diagramImageSync{sto: stoM})

	table := template.Field{Key: "mapping", Type: "table", Format: template.TablePatternDataLineage}
	diagramF := template.Field{Key: "lineage", Type: "diagram", Options: []any{
		map[string]any{"value": template.DiagramSource, "label": "mapping"},
	}}
	tpl := func(fields ...template.Field) *template.Template {
		return &template.Template{Name: "m", Filename: "m.yaml", Fields: append([]template.Field{{Key: "id", Type: "guid"}}, fields...)}
	}
	if err := tplM.SaveTemplate("m.yaml", tpl(table)); err != nil {
		t.Fatal(err)
	}
	r := stoM.SaveForm(context.Background(), "m.yaml", "r1.meta.json", map[string]any{
		"mapping": []any{[]any{"A", "x", "Target", "y", "", ""}},
	})
	if !r.Success {
		t.Fatal(r.Error)
	}
	name := template.DiagramImageName("m.yaml", "r1.meta.json", "lineage")
	if raw, _, _ := stoM.OpenImageFile("m.yaml", name); raw != nil {
		t.Fatal("no diagram field yet, no file expected")
	}

	// Adding the diagram field draws the record that already existed.
	if err := tplM.SaveTemplate("m.yaml", tpl(table, diagramF)); err != nil {
		t.Fatal(err)
	}
	raw, _, _ := stoM.OpenImageFile("m.yaml", name)
	if !strings.Contains(string(raw), ">Target<") {
		t.Fatalf("existing record not drawn on template change: %.80q", raw)
	}

	// Removing it cleans the generated file up again.
	if err := tplM.SaveTemplate("m.yaml", tpl(table)); err != nil {
		t.Fatal(err)
	}
	if raw, _, _ := stoM.OpenImageFile("m.yaml", name); raw != nil {
		t.Fatal("orphaned diagram file left behind")
	}
}
