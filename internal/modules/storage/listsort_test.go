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

// Flipping the flag on one template must not change another template's order:
// sortSummaries reads each template's own flag, so the two lists diverge.
func TestExtendedListForms_PerTemplateIsolation(t *testing.T) {
	m, _, tplM, _ := newTestStack(t)
	ctx := context.Background()

	sorted := &template.Template{
		Name: "Sorted", Filename: "sorted.yaml", ItemField: "title",
		SortByItemField: true,
		Fields:          []template.Field{{Key: "title", Type: "text"}},
	}
	plain := &template.Template{
		Name: "Plain", Filename: "plain.yaml", ItemField: "title",
		SortByItemField: false,
		Fields:          []template.Field{{Key: "title", Type: "text"}},
	}
	if err := tplM.SaveTemplate("sorted.yaml", sorted); err != nil {
		t.Fatal(err)
	}
	if err := tplM.SaveTemplate("plain.yaml", plain); err != nil {
		t.Fatal(err)
	}

	for _, tpl := range []string{"sorted.yaml", "plain.yaml"} {
		m.SaveForm(ctx, tpl, "a.meta.json", map[string]any{"title": "Zebra"})
		m.SaveForm(ctx, tpl, "z.meta.json", map[string]any{"title": "Apple"})
	}

	gotSorted, err := m.ExtendedListForms("sorted.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if s := titles(gotSorted); s[0] != "Apple" || s[1] != "Zebra" {
		t.Fatalf("sorted.yaml = %v; want title order [Apple Zebra]", s)
	}

	gotPlain, err := m.ExtendedListForms("plain.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if s := titles(gotPlain); s[0] != "Zebra" || s[1] != "Apple" {
		t.Fatalf("plain.yaml = %v; want filename order [Zebra Apple] (unaffected by the other template)", s)
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
