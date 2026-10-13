package app

import (
	"context"
	"log/slog"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

const savedInvoiceYAML = `name: Invoice
enable_collection: true
item_field: invoice_number
fields:
  - key: id
    type: guid
  - key: invoice_number
    type: text
    label: Number
  - key: lines
    type: loopstart
    label: Lines
  - key: line_description
    type: text
    label: Description
    level_scope: 1
  - key: lines
    type: loopstop
    label: Lines
`

func formAdapterFixture(t *testing.T) (pluginFormAdapter, *[]string) {
	t.Helper()
	log := slog.Default()
	sys := system.NewManager(t.TempDir(), log)
	tplM := template.NewManager(sys, "templates", log)
	if err := tplM.CreateFromYAML("invoice.yaml", []byte(savedInvoiceYAML)); err != nil {
		t.Fatal(err)
	}
	stoM := storage.NewManager(sys, sfr.NewManager(sys, log), tplM, "storage", log)
	var told []string
	return pluginFormAdapter{sto: stoM, changed: func(tpl string) { told = append(told, tpl) }}, &told
}

func TestPluginFormAdapter_SaveTellsWhichTemplateChanged(t *testing.T) {
	a, told := formAdapterFixture(t)
	if err := a.SaveForm(context.Background(), "invoice.yaml", "a.meta.json", map[string]any{"invoice_number": "2026-001"}); err != nil {
		t.Fatalf("SaveForm: %v", err)
	}
	if len(*told) != 1 || (*told)[0] != "invoice.yaml" {
		t.Errorf("told = %v, want the saved template once", *told)
	}
}

func TestPluginFormAdapter_FailedSaveTellsNothing(t *testing.T) {
	a, told := formAdapterFixture(t)
	if err := a.SaveForm(context.Background(), "invoice.yaml", "../escape.meta.json", map[string]any{}); err == nil {
		t.Fatal("a datafile outside the folder was saved")
	}
	if len(*told) != 0 {
		t.Errorf("told = %v after a refused save", *told)
	}
}

func TestPluginFormAdapter_NoListenerIsFine(t *testing.T) {
	a, _ := formAdapterFixture(t)
	a.changed = nil
	if err := a.SaveForm(context.Background(), "invoice.yaml", "a.meta.json", map[string]any{"invoice_number": "x"}); err != nil {
		t.Fatalf("SaveForm: %v", err)
	}
}

// What the Themis fetch relies on: a script-chosen file name, a guid supplied
// in the data that becomes the record's own id, and a repeating block saved in
// the same call.
func TestPluginFormAdapter_RecordTakesTheGuidItIsGivenAndKeepsItsLines(t *testing.T) {
	a, _ := formAdapterFixture(t)
	guid := "77696695-1ace-419f-917e-53ae7f5a861e"
	data := map[string]any{
		"id": guid, "invoice_number": "2025-001",
		"lines": []any{map[string]any{"line_description": "Hosting"}, map[string]any{"line_description": "Domain"}},
	}
	if err := a.SaveForm(context.Background(), "invoice.yaml", guid+".meta.json", data); err != nil {
		t.Fatalf("SaveForm: %v", err)
	}
	form := a.sto.LoadForm("invoice.yaml", guid+".meta.json")
	if form == nil {
		t.Fatal("record not found under the file name the script chose")
	}
	if form.Meta.ID != guid || form.Data["id"] != guid {
		t.Errorf("meta.id = %q, data.id = %v; want the supplied guid in both", form.Meta.ID, form.Data["id"])
	}
	lines, _ := form.Data["lines"].([]any)
	if len(lines) != 2 {
		t.Fatalf("lines = %v", form.Data["lines"])
	}
	// Fetching the same invoice again lands on the same record and keeps its id.
	data["invoice_number"] = "2025-001 (again)"
	if err := a.SaveForm(context.Background(), "invoice.yaml", guid+".meta.json", data); err != nil {
		t.Fatal(err)
	}
	again := a.sto.LoadForm("invoice.yaml", guid+".meta.json")
	if again.Meta.ID != guid || again.Data["invoice_number"] != "2025-001 (again)" {
		t.Errorf("after a second save: id = %q, number = %v", again.Meta.ID, again.Data["invoice_number"])
	}
	files, err := a.sto.ListForms("invoice.yaml")
	if err != nil || len(files) != 1 {
		t.Errorf("files = %v, want one record", files)
	}
}
