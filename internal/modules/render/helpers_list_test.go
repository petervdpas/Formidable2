package render

import (
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestListHelper_OrderedModes(t *testing.T) {
	tpl := &template.Template{
		Name:     "tpl",
		Filename: "tpl.yaml",
		MarkdownTemplate: "U:\n{{list \"steps\"}}\n\n" +
			"B:\n{{list \"steps\" ordered=true}}\n\n" +
			"S:\n{{list \"steps\" mode=\"ordered\"}}",
		Fields: []template.Field{{Key: "steps", Type: "list"}},
	}
	got, err := RenderMarkdown(map[string]any{"steps": []any{"a", "b"}}, tpl, &Options{})
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if !strings.Contains(got, "- a\n- b") {
		t.Errorf("default list should be bulleted:\n%s", got)
	}
	// Both the boolean flag and the quoted string produce numbered markers.
	if n := strings.Count(got, "1. a"); n != 2 {
		t.Errorf("ordered=true and mode=\"ordered\" should both number (got %d '1. a'):\n%s", n, got)
	}
}
