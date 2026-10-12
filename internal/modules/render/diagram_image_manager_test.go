package render

import (
	"context"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestManager_RenderingARecordStoresItsDiagramImage(t *testing.T) {
	sys := system.NewManager(t.TempDir(), nil)
	tplM := template.NewManager(sys, "templates", nil)
	stoM := storage.NewManager(sys, sfr.NewManager(sys, nil), tplM, "storage", nil)
	tpl := &template.Template{
		Name: "M", Filename: "m.yaml", MarkdownTemplate: `{{virtual-field "lineage" "image"}}`,
		Fields: []template.Field{
			{Key: "id", Type: "guid"},
			{Key: "mapping", Type: "table", Format: template.TablePatternDataLineage},
			{Key: "lineage", Type: "diagram", Label: "Lineage", Options: []any{
				map[string]any{"value": template.DiagramSource, "label": "mapping"},
			}},
		},
	}
	if err := tplM.SaveTemplate("m.yaml", tpl); err != nil {
		t.Fatal(err)
	}
	save := func(target string) {
		r := stoM.SaveForm(context.Background(), "m.yaml", "r1.meta.json", map[string]any{
			"mapping": []any{[]any{"A", "x", target, "y", "", ""}},
		})
		if !r.Success {
			t.Fatal(r.Error)
		}
	}
	save("First")
	name := template.DiagramImageName("m.yaml", "r1.meta.json", "lineage")
	m := NewManager(tplM, stoM, func(_, n string) string { return "/api/images/m/" + n }, nil, nil)

	md, err := m.RenderMarkdown("m.yaml", "r1.meta.json")
	if err != nil || md != "![Lineage](/api/images/m/"+name+")" {
		t.Fatalf("md = %q err %v", md, err)
	}
	raw, _, _ := stoM.OpenImageFile("m.yaml", name)
	if !strings.Contains(string(raw), ">First<") {
		t.Fatalf("image not written by the render: %.80q", raw)
	}

	// The image follows the record: re-render after a change redraws it.
	save("Second")
	if _, err := m.RenderMarkdown("m.yaml", "r1.meta.json"); err != nil {
		t.Fatal(err)
	}
	raw, _, _ = stoM.OpenImageFile("m.yaml", name)
	if !strings.Contains(string(raw), ">Second<") || strings.Contains(string(raw), ">First<") {
		t.Fatalf("image is stale: %.200s", raw)
	}

	// An emptied table renders nothing and removes the file.
	if r := stoM.SaveForm(context.Background(), "m.yaml", "r1.meta.json", map[string]any{"mapping": []any{}}); !r.Success {
		t.Fatal(r.Error)
	}
	if md, _ := m.RenderMarkdown("m.yaml", "r1.meta.json"); md != "" {
		t.Fatalf("md = %q, want empty", md)
	}
	if raw, _, _ := stoM.OpenImageFile("m.yaml", name); raw != nil {
		t.Fatal("stale image left behind for an empty diagram")
	}
}

func TestManager_StoreWithoutImageSupportRendersInline(t *testing.T) {
	tpl := diagramRenderTpl(`{{virtual-field "lineage" "image"}}`)
	store := &fakeFormStore{form: &storage.Form{Data: oneMapping}}
	md, err := NewManager(&fakeTemplateLoader{tpl: tpl}, store, nil, nil, nil).RenderMarkdown("m.yaml", "r1.meta.json")
	if err != nil || !strings.Contains(md, "<svg ") || strings.Contains(md, "![") {
		t.Fatalf("md = %.120q err %v", md, err)
	}
}

func TestManager_VersionedDiagramImagesOnlyWhenEnabled(t *testing.T) {
	sys := system.NewManager(t.TempDir(), nil)
	tplM := template.NewManager(sys, "templates", nil)
	stoM := storage.NewManager(sys, sfr.NewManager(sys, nil), tplM, "storage", nil)
	tpl := &template.Template{
		Name: "M", Filename: "m.yaml", MarkdownTemplate: `{{virtual-field "lineage" "image"}}`,
		Fields: []template.Field{
			{Key: "id", Type: "guid"},
			{Key: "mapping", Type: "table", Format: template.TablePatternDataLineage},
			{Key: "lineage", Type: "diagram", Options: []any{map[string]any{"value": template.DiagramSource, "label": "mapping"}}},
		},
	}
	if err := tplM.SaveTemplate("m.yaml", tpl); err != nil {
		t.Fatal(err)
	}
	save := func(target string) {
		if r := stoM.SaveForm(context.Background(), "m.yaml", "r1.meta.json", map[string]any{"mapping": []any{[]any{"A", "x", target, "y", "", ""}}}); !r.Success {
			t.Fatal(r.Error)
		}
	}
	url := func(_, n string) string { return "/api/images/m/" + n }
	plain := NewManager(tplM, stoM, url, nil, nil)
	studio := NewManager(tplM, stoM, url, nil, nil)
	studio.SetVersionedDiagramImages(true)

	save("First")
	if md, _ := plain.RenderMarkdown("m.yaml", "r1.meta.json"); strings.Contains(md, "?v=") {
		t.Fatalf("unversioned manager emitted a version: %q", md)
	}
	before, _ := studio.RenderMarkdown("m.yaml", "r1.meta.json")
	save("Second")
	after, _ := studio.RenderMarkdown("m.yaml", "r1.meta.json")
	if !strings.Contains(before, "?v=") || before == after {
		t.Fatalf("after a change the studio HTML must differ so the image reloads:\n before %q\n after  %q", before, after)
	}
}
