package app

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestDiagramImagePrune_RemovingTheFieldRemovesItsFiles(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sys := system.NewManager(t.TempDir(), log)
	tplM := template.NewManager(sys, "templates", log)
	stoM := storage.NewManager(sys, sfr.NewManager(sys, log), tplM, "storage", log)
	tplM.AddUpdateObserver(diagramImagePrune{sto: stoM})

	table := template.Field{Key: "mapping", Type: "table", Format: template.TablePatternDataLineage}
	diagramF := template.Field{Key: "lineage", Type: "diagram", Options: []any{
		map[string]any{"value": template.DiagramSource, "label": "mapping"},
	}}
	tpl := func(fields ...template.Field) *template.Template {
		return &template.Template{Name: "m", Filename: "m.yaml", Fields: append([]template.Field{{Key: "id", Type: "guid"}}, fields...)}
	}
	if err := tplM.SaveTemplate("m.yaml", tpl(table, diagramF)); err != nil {
		t.Fatal(err)
	}
	if r := stoM.SaveForm(context.Background(), "m.yaml", "r1.meta.json", map[string]any{}); !r.Success {
		t.Fatal(r.Error)
	}
	name := template.DiagramImageName("m.yaml", "r1.meta.json", "lineage")
	if err := stoM.WriteDiagramImage("m.yaml", name, "<svg/>"); err != nil {
		t.Fatal(err)
	}

	// An unrelated template save keeps the file.
	if err := tplM.SaveTemplate("m.yaml", tpl(table, diagramF, template.Field{Key: "note", Type: "text"})); err != nil {
		t.Fatal(err)
	}
	if raw, _, _ := stoM.OpenImageFile("m.yaml", name); raw == nil {
		t.Fatal("file of a still-present diagram field was pruned")
	}

	// Removing the diagram field removes its generated file.
	if err := tplM.SaveTemplate("m.yaml", tpl(table)); err != nil {
		t.Fatal(err)
	}
	if raw, _, _ := stoM.OpenImageFile("m.yaml", name); raw != nil {
		t.Fatal("orphaned diagram file left behind")
	}
}
