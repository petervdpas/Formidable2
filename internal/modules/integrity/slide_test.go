package integrity

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// analyzeSlide runs the doctor over a one-field slide template whose record
// holds the given slide-document value.
func analyzeSlide(t *testing.T, doc any) Report {
	t.Helper()
	tpl := &template.Template{
		Name:     "Deck",
		Filename: "deck.yaml",
		Fields:   []template.Field{{Key: "slide", Type: "slide"}},
	}
	form := &storage.Form{
		Meta: storage.FormMeta{
			ID:      "fixture-id",
			Created: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
			Updated: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
		},
		Data: map[string]any{"slide": doc},
	}
	m := newM(t, tpl, map[string]*storage.Form{"a.meta.json": form})
	r, err := m.AnalyzeTemplate("deck.yaml")
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	return r
}

func block(kind string, content any) map[string]any {
	return map[string]any{
		"id": "b", "kind": kind, "content": content,
		"x": float64(0), "y": float64(0), "w": float64(100), "h": float64(80),
	}
}

func TestSlide_GoodDocumentHasNoIssues(t *testing.T) {
	doc := map[string]any{"blocks": []any{
		block("text", "## Title"),
		block("mermaid", "graph TD; A-->B"),
		block("table", []any{[]any{"a", "b"}, []any{"c", "d"}}),
	}}
	if r := analyzeSlide(t, doc); r.IssueCount != 0 {
		t.Errorf("good slide should have no issues, got %d: %+v", r.IssueCount, r.Forms)
	}
}

func TestSlide_UnknownKindFlagged(t *testing.T) {
	doc := map[string]any{"blocks": []any{block("formula", "x")}}
	r := analyzeSlide(t, doc)
	findIssue(t, r, "a.meta.json", IssueTypeMismatch, "slide.blocks[0]")
}

func TestSlide_BadGeometryFlagged(t *testing.T) {
	bad := block("text", "hi")
	bad["w"] = float64(0) // zero width is degenerate
	doc := map[string]any{"blocks": []any{bad}}
	r := analyzeSlide(t, doc)
	findIssue(t, r, "a.meta.json", IssueTypeMismatch, "slide.blocks[0]")
}

func TestSlide_BlockContentRecursesToKind(t *testing.T) {
	// A table block whose content is a string (not a 2D array) is wrong, and the
	// recursion through the table value-check must catch it.
	doc := map[string]any{"blocks": []any{block("table", "not-a-table")}}
	r := analyzeSlide(t, doc)
	findIssue(t, r, "a.meta.json", IssueTypeMismatch, "slide.blocks[0]")
}
