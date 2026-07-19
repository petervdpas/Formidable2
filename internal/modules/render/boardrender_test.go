package render

import (
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

func sampleBoard() Board {
	return Board{
		Name: "Test",
		Resources: []template.ResourceDescriptor{
			{Value: "peter", Label: "Peter"},
			{Value: "jack", Label: "Jack"},
		},
		Bars: []BoardBar{
			{Resource: "peter", Kind: "taak", Start: "2026-07-16", End: "2026-07-28"},
			{Resource: "jack", Kind: "vakantie", Start: "2026-07-21", End: "2026-07-25"},
		},
	}
}

func TestBoardGantt_SectionsAndTasks(t *testing.T) {
	kinds := map[string]string{"taak": "Taak", "vakantie": "Vakantie"}
	got := boardGantt(sampleBoard(), kinds)
	want := strings.Join([]string{
		"```mermaid",
		"gantt",
		"    title Test",
		"    dateFormat YYYY-MM-DD",
		"    axisFormat %b %d",
		"    section Peter",
		"        Taak :2026-07-16, 2026-07-28",
		"    section Jack",
		"        Vakantie :2026-07-21, 2026-07-25",
		"```",
	}, "\n")
	if got != want {
		t.Errorf("gantt mismatch:\n got:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestBoardGantt_Milestone(t *testing.T) {
	b := Board{
		Name:      "P",
		Resources: []template.ResourceDescriptor{{Value: "dev", Label: "Dev"}},
		Bars:      []BoardBar{{Resource: "dev", Kind: "milestone", Start: "2026-07-20", Milestone: true}},
	}
	got := boardGantt(b, map[string]string{"milestone": "Release"})
	if !strings.Contains(got, "        Release :milestone, 2026-07-20, 0d") {
		t.Errorf("milestone task missing:\n%s", got)
	}
}

func TestBoardGantt_NoBars(t *testing.T) {
	if got := boardGantt(Board{Name: "Empty"}, nil); got != "" {
		t.Errorf("empty board should emit no gantt, got:\n%s", got)
	}
}

func TestBoardGantt_UndeclaredResourceStillCharted(t *testing.T) {
	b := Board{
		Name:      "P",
		Resources: []template.ResourceDescriptor{{Value: "dev", Label: "Dev"}},
		Bars: []BoardBar{
			{Resource: "dev", Kind: "k", Start: "2026-07-01", End: "2026-07-02"},
			{Resource: "ghost", Kind: "k", Start: "2026-07-03", End: "2026-07-04"},
		},
	}
	got := boardGantt(b, nil)
	if !strings.Contains(got, "    section Dev") || !strings.Contains(got, "    section ghost") {
		t.Errorf("undeclared resource dropped from chart:\n%s", got)
	}
}

func TestEventsTable_AxesPlusAuthorFields(t *testing.T) {
	events := []any{
		map[string]any{"event": map[string]any{
			"resource": "peter", "kind": "taak", "start": "2026-07-16", "end": "2026-07-28",
			"omschrijving": "Bouw de tuin",
		}},
		map[string]any{"event": map[string]any{
			"resource": "jack", "kind": "vakantie", "start": "2026-07-21", "end": "2026-07-25",
		}},
	}
	kinds := map[string]string{"taak": "Taak", "vakantie": "Vakantie"}
	resources := map[string]string{"peter": "Peter", "jack": "Jack"}
	author := []template.Field{{Key: "omschrijving", Type: "text", Label: "Omschrijving"}}
	got := eventsTable(events, kinds, resources, author, nil)
	want := strings.Join([]string{
		"| Resource | Kind | Start | End | Omschrijving |",
		"| --- | --- | --- | --- | --- |",
		"| Peter | Taak | 2026-07-16 | 2026-07-28 | Bouw de tuin |",
		"| Jack | Vakantie | 2026-07-21 | 2026-07-25 |  |",
	}, "\n")
	if got != want {
		t.Errorf("table mismatch:\n got:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestEventsTable_EscapesPipesAndNewlines(t *testing.T) {
	events := []any{map[string]any{"event": map[string]any{
		"resource": "x", "kind": "k", "start": "2026-01-01", "end": "2026-01-02",
		"note": "a|b\nc",
	}}}
	author := []template.Field{{Key: "note", Type: "text", Label: "Note"}}
	got := eventsTable(events, nil, nil, author, nil)
	if !strings.Contains(got, `a\|b c`) {
		t.Errorf("pipe/newline not escaped:\n%s", got)
	}
}

func TestEventsTable_NoEvents(t *testing.T) {
	if got := eventsTable([]any{}, nil, nil, nil, nil); got != "" {
		t.Errorf("empty events should emit no table, got:\n%s", got)
	}
}

func TestBoardHelper_RendersGanttAndTable(t *testing.T) {
	tpl := &template.Template{
		ProjectMode:      true,
		MarkdownTemplate: "{{board}}",
		Fields: []template.Field{
			{Key: "project", Type: "project", Options: []any{
				map[string]any{"value": "from", "label": "2026-07-01"},
				map[string]any{"value": "to", "label": "2026-07-31"},
				map[string]any{"value": "timeblock", "label": "week"},
				map[string]any{"value": "peter", "label": "Peter"},
				map[string]any{"value": "jack", "label": "Jack"},
			}},
			{Key: "events", Type: "loopstart"},
			{Key: "event", Type: "event", Options: []any{
				map[string]any{"value": "taak", "label": "Taak"},
				map[string]any{"value": "vakantie", "label": "Vakantie"},
			}},
			{Key: "omschrijving", Type: "text", Label: "Omschrijving"},
			{Key: "events", Type: "loopstop"},
		},
	}
	values := map[string]any{
		"project": map[string]any{"name": "Test"},
		"events": []any{
			map[string]any{"event": map[string]any{
				"resource": "peter", "kind": "taak", "start": "2026-07-06", "end": "2026-07-20",
				"omschrijving": "Klus",
			}},
		},
	}
	out, err := RenderMarkdown(values, tpl, nil)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if !strings.Contains(out, "```mermaid") || !strings.Contains(out, "gantt") {
		t.Errorf("gantt block missing:\n%s", out)
	}
	if !strings.Contains(out, "| Resource | Kind | Start | End | Omschrijving |") {
		t.Errorf("events table missing:\n%s", out)
	}
	if !strings.Contains(out, "Klus") {
		t.Errorf("author field value missing from table:\n%s", out)
	}
}
