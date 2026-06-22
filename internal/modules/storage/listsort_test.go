package storage

import (
	"context"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// titles returns the summary titles in list order, for order assertions.
func titles(rows []FormSummary) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Title
	}
	return out
}

func saveSortTemplate(t *testing.T, tplM *template.Manager, sortByItem bool) {
	t.Helper()
	tpl := &template.Template{
		Name:            "Sortable",
		Filename:        "s.yaml",
		ItemField:       "title",
		SortByItemField: sortByItem,
		Fields:          []template.Field{{Key: "title", Type: "text"}},
	}
	if err := tplM.SaveTemplate("s.yaml", tpl); err != nil {
		t.Fatalf("save template: %v", err)
	}
}

func TestExtendedListForms_SortByItemField(t *testing.T) {
	m, _, tplM, _ := newTestStack(t)
	saveSortTemplate(t, tplM, true)

	// Filenames sort a < z, but the titles run the other way, so a
	// title-sorted list must reorder them.
	ctx := context.Background()
	m.SaveForm(ctx, "s.yaml", "a.meta.json", map[string]any{"title": "Zebra"})
	m.SaveForm(ctx, "s.yaml", "z.meta.json", map[string]any{"title": "Apple"})

	out, err := m.ExtendedListForms("s.yaml")
	if err != nil {
		t.Fatal(err)
	}
	got := titles(out)
	if len(got) != 2 || got[0] != "Apple" || got[1] != "Zebra" {
		t.Fatalf("title order = %v; want [Apple Zebra]", got)
	}
}

func TestExtendedListForms_DefaultFilenameOrder(t *testing.T) {
	m, _, tplM, _ := newTestStack(t)
	saveSortTemplate(t, tplM, false) // toggle off = filename order

	ctx := context.Background()
	m.SaveForm(ctx, "s.yaml", "a.meta.json", map[string]any{"title": "Zebra"})
	m.SaveForm(ctx, "s.yaml", "z.meta.json", map[string]any{"title": "Apple"})

	out, err := m.ExtendedListForms("s.yaml")
	if err != nil {
		t.Fatal(err)
	}
	got := titles(out)
	// a.meta.json (Zebra) before z.meta.json (Apple): filename order, not title.
	if len(got) != 2 || got[0] != "Zebra" || got[1] != "Apple" {
		t.Fatalf("filename order = %v; want [Zebra Apple]", got)
	}
}
