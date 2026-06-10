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

func TestTableColumnRemap(t *testing.T) {
	cases := []struct {
		name          string
		before, after []string
		wantPerm      []int
		wantOK        bool
	}{
		{"reorder", []string{"a", "b", "c"}, []string{"a", "c", "b"}, []int{0, 2, 1}, true},
		{"drop middle", []string{"a", "b", "c"}, []string{"a", "c"}, []int{0, 2}, true},
		{"insert middle", []string{"a", "b"}, []string{"a", "x", "b"}, []int{0, -1, 1}, true},
		{"unchanged", []string{"a", "b"}, []string{"a", "b"}, nil, false},
		{"rename reads as drop+add", []string{"a", "b"}, []string{"a", "c"}, []int{0, -1}, true},
		{"empty key before", []string{"a", ""}, []string{"a", "b"}, nil, false},
		{"duplicate key after", []string{"a", "b"}, []string{"a", "a"}, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			perm, ok := tableColumnRemap(c.before, c.after)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if !c.wantOK {
				return
			}
			if len(perm) != len(c.wantPerm) {
				t.Fatalf("perm = %v, want %v", perm, c.wantPerm)
			}
			for i := range perm {
				if perm[i] != c.wantPerm[i] {
					t.Fatalf("perm = %v, want %v", perm, c.wantPerm)
				}
			}
		})
	}
}

// End-to-end: saving a template with reordered table columns realigns the
// already-stored record data through the registered update observer.
func TestTableColumnMigrator_RealStack(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	tplM := template.NewManager(sys, "templates", log)
	if err := tplM.EnsureTemplateDirectory(); err != nil {
		t.Fatalf("EnsureTemplateDirectory: %v", err)
	}
	sfrM := sfr.NewManager(sys, log)
	stoM := storage.NewManager(sys, sfrM, tplM, "storage", log)
	tplM.AddUpdateObserver(tableColumnMigrator{sto: stoM})

	cols := func(keys ...string) []any {
		out := make([]any, 0, len(keys))
		for _, k := range keys {
			out = append(out, map[string]any{"value": k, "type": "string", "label": k})
		}
		return out
	}
	tpl := func(keys ...string) *template.Template {
		return &template.Template{
			Name: "t", Filename: "t.yaml",
			Fields: []template.Field{{Key: "tbl", Type: "table", Options: cols(keys...)}},
		}
	}

	if err := tplM.SaveTemplate("t.yaml", tpl("datum", "akkoord", "functie")); err != nil {
		t.Fatalf("seed template: %v", err)
	}
	r := stoM.SaveForm(context.Background(), "t.yaml", "r1.json", map[string]any{
		"tbl": []any{[]any{"01-11", "Rinzema", "Manager"}},
	})
	if !r.Success {
		t.Fatalf("seed record: %+v", r)
	}

	// Swap akkoord/functie: saving the template must realign the stored row.
	if err := tplM.SaveTemplate("t.yaml", tpl("datum", "functie", "akkoord")); err != nil {
		t.Fatalf("resave template: %v", err)
	}

	row := stoM.LoadForm("t.yaml", "r1.json").Data["tbl"].([]any)[0].([]any)
	if row[0] != "01-11" || row[1] != "Manager" || row[2] != "Rinzema" {
		t.Errorf("row = %v, want [01-11 Manager Rinzema] (data followed the column reorder)", row)
	}
}
