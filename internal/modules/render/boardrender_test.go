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
	got := boardGantt(sampleBoard(), kinds, true)
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

func TestBoardGantt_FramesFullWindow(t *testing.T) {
	// Events sit mid-July but the board window runs into next January; the gantt
	// must span the full window, not just the events. A "Project" section with a
	// from→to bar anchors the axis (mermaid has no chart-level min/max).
	b := sampleBoard()
	b.From = "2026-07-15"
	b.To = "2027-01-29"
	got := boardGantt(b, map[string]string{"taak": "Taak", "vakantie": "Vakantie"}, true)
	if !strings.Contains(got, "    section Project\n        Test :2026-07-15, 2027-01-29") {
		t.Errorf("full-window frame missing:\n%s", got)
	}
	// The frame precedes the resource sections.
	if strings.Index(got, "section Project") > strings.Index(got, "section Peter") {
		t.Errorf("frame section must come first:\n%s", got)
	}
}

func TestBoardGantt_NoFrameWithoutWindow(t *testing.T) {
	// sampleBoard has no From/To → auto-fit, no frame bar.
	if got := boardGantt(sampleBoard(), nil, true); strings.Contains(got, "section Project") {
		t.Errorf("no window should mean no frame:\n%s", got)
	}
}

func TestBoardGantt_ScopeOptOut(t *testing.T) {
	// A windowed board with showScope=false ({{board scope=false}}) drops the
	// frame bar and lets the chart auto-fit to the events.
	b := sampleBoard()
	b.From = "2026-07-15"
	b.To = "2027-01-29"
	if got := boardGantt(b, nil, false); strings.Contains(got, "section Project") {
		t.Errorf("scope=false should omit the frame:\n%s", got)
	}
}

func TestBoardGantt_Milestone(t *testing.T) {
	b := Board{
		Name:      "P",
		Resources: []template.ResourceDescriptor{{Value: "dev", Label: "Dev"}},
		Bars:      []BoardBar{{Resource: "dev", Kind: "milestone", Start: "2026-07-20", Milestone: true}},
	}
	got := boardGantt(b, map[string]string{"milestone": "Release"}, true)
	if !strings.Contains(got, "        Release :milestone, 2026-07-20, 0d") {
		t.Errorf("milestone task missing:\n%s", got)
	}
}

func TestBoardGantt_NoBars(t *testing.T) {
	if got := boardGantt(Board{Name: "Empty"}, nil, true); got != "" {
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
	got := boardGantt(b, nil, true)
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

func metaProjectField() *template.Field {
	return &template.Field{Key: "project", Type: "project", Options: []any{
		map[string]any{"value": "from", "label": "2026-07-15"},
		map[string]any{"value": "to", "label": "2026-08-26"}, // 6 weeks / 42 days
		map[string]any{"value": "timeblock", "label": "week"},
		map[string]any{"value": "peter", "label": "Peter"},
		map[string]any{"value": "jack", "label": "Jack"},
	}}
}

func TestBoardMeta_Properties(t *testing.T) {
	project := metaProjectField()
	ctx := map[string]any{
		"project": map[string]any{"name": "Projectos"},
		"events": []any{
			map[string]any{"event": map[string]any{"start": "2026-07-16", "end": "2026-07-28"}},
			map[string]any{"event": map[string]any{"start": "2026-07-21", "end": "2026-07-25"}},
		},
	}
	cases := []struct {
		prop, unit string
		want       any
	}{
		{"name", "", "Projectos"},
		{"from", "", "2026-07-15"},
		{"to", "", "2026-08-26"},
		{"timeblock", "", "week"},
		{"duration", "", 42},
		{"duration", "days", 42},
		{"duration", "weeks", 6},
		{"ticks", "", 7}, // 42 days / 7 spans 6 full weeks + 1 boundary tick
		{"events", "", 2},
		{"resources", "", 2},
		{"bogus", "", ""},
	}
	for _, c := range cases {
		if got := boardMetaValue(c.prop, c.unit, project, ctx); got != c.want {
			t.Errorf("boardMeta(%q,%q) = %v (%T), want %v", c.prop, c.unit, got, got, c.want)
		}
	}
}

func TestBoardMeta_TimeBlockHonorsRecordOverride(t *testing.T) {
	project := metaProjectField() // template default = week
	// No override → template default.
	if got := boardMetaValue("timeblock", "", project, map[string]any{
		"project": map[string]any{"name": "P"},
	}); got != "week" {
		t.Errorf("default timeblock = %v, want week", got)
	}
	// Record override wins.
	if got := boardMetaValue("timeblock", "", project, map[string]any{
		"project": map[string]any{"name": "P", "timeBlock": "month"},
	}); got != "month" {
		t.Errorf("override timeblock = %v, want month", got)
	}
}

func TestBoardMeta_DurationMonths(t *testing.T) {
	project := &template.Field{Key: "project", Type: "project", Options: []any{
		map[string]any{"value": "from", "label": "2026-07-15"},
		map[string]any{"value": "to", "label": "2027-01-29"},
		map[string]any{"value": "timeblock", "label": "week"},
	}}
	if got := boardMetaValue("duration", "months", project, map[string]any{}); got != 6 {
		t.Errorf("duration months = %v, want 6", got)
	}
}

func TestBoardMeta_UnsetAxis(t *testing.T) {
	project := &template.Field{Key: "project", Type: "project"} // no from/to options
	if got := boardMetaValue("duration", "", project, map[string]any{}); got != "" {
		t.Errorf("duration with no axis = %v, want empty", got)
	}
	if got := boardMetaValue("events", "", project, map[string]any{}); got != 0 {
		t.Errorf("events with no data = %v, want 0", got)
	}
}

func TestBoardMetaHelper_ThroughRender(t *testing.T) {
	tpl := &template.Template{
		ProjectMode:      true,
		MarkdownTemplate: `{{boardMeta "name"}} runs {{boardMeta "duration" "weeks"}} weeks with {{boardMeta "events"}} events`,
		Fields:           []template.Field{*metaProjectField()},
	}
	values := map[string]any{
		"project": map[string]any{"name": "Projectos"},
		"events":  []any{map[string]any{"event": map[string]any{"start": "2026-07-16", "end": "2026-07-28"}}},
	}
	out, err := RenderMarkdown(values, tpl, nil)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if out != "Projectos runs 6 weeks with 1 events" {
		t.Errorf("render = %q", out)
	}
}

func TestEventsInWindow_Overlap(t *testing.T) {
	events := []any{
		map[string]any{"event": map[string]any{"start": "2026-07-07", "end": "2026-07-10"}}, // fully in
		map[string]any{"event": map[string]any{"start": "2026-07-27", "end": "2026-08-10"}}, // straddles
		map[string]any{"event": map[string]any{"start": "2026-08-20", "end": "2026-08-25"}}, // after
		map[string]any{"event": map[string]any{"start": "", "end": ""}},                     // no date
	}
	got := eventsInWindow(events, "2026-07-06", "2026-08-03")
	if len(got) != 2 { // the in-window one and the straddler
		t.Fatalf("want 2 events in [07-06,08-03), got %d: %+v", len(got), got)
	}
	// Empty window returns everything (minus nothing; the filter only runs with a window).
	if all := eventsInWindow(events, "", ""); len(all) != 4 {
		t.Errorf("empty window should pass all rows, got %d", len(all))
	}
}

func TestClipBarsToWindow_ClipsSpanning(t *testing.T) {
	bars := []BoardBar{
		{Resource: "a", Start: "2026-07-27", End: "2026-08-10"}, // straddles [.,08-03)
		{Resource: "b", Start: "2026-09-01", End: "2026-09-05"}, // outside
	}
	got := clipBarsToWindow(bars, "2026-07-06", "2026-08-03")
	if len(got) != 1 {
		t.Fatalf("want 1 clipped bar, got %d: %+v", len(got), got)
	}
	if got[0].End != "2026-08-03" {
		t.Errorf("spanning bar not clipped to window end: %+v", got[0])
	}
	if got[0].Start != "2026-07-27" {
		t.Errorf("start inside window should be untouched: %+v", got[0])
	}
}

func TestBoardSliceWindows_TilesAndSkipsEmpty(t *testing.T) {
	ticks := boardTicks("2026-07-06", "2026-08-30", "week") // 8 weekly ticks
	events := []any{
		map[string]any{"event": map[string]any{"start": "2026-07-07", "end": "2026-07-10"}},
		map[string]any{"event": map[string]any{"start": "2026-08-20", "end": "2026-08-25"}},
	}
	board := Board{From: "2026-07-06", To: "2026-08-30", Ticks: ticks, Bars: boardBars(events, ticks, nil)}
	// trim + size=4 → two 4-week slices, both populated (events span ticks 0..7).
	got := boardSliceWindows(board, events, 4, 0, true)
	if len(got) != 2 {
		t.Fatalf("want 2 slices, got %d: %+v", len(got), got)
	}
	if got[0].from != "2026-07-06" || got[1].to != "2026-08-30" {
		t.Errorf("slice windows off: %+v", got)
	}

	// trim drops a middle empty slice: one event in the first week only.
	sparse := []any{
		map[string]any{"event": map[string]any{"start": "2026-07-07", "end": "2026-07-09"}},
	}
	sparseBoard := Board{From: "2026-07-06", To: "2026-08-30", Ticks: ticks, Bars: boardBars(sparse, ticks, nil)}
	if s := boardSliceWindows(sparseBoard, sparse, 2, 0, true); len(s) != 1 {
		t.Errorf("trim should skip empty slices, got %d: %+v", len(s), s)
	}
}

func TestBoardSliceWindows_FullWindowByDefault(t *testing.T) {
	// Default (no trim): the WHOLE project window is tiled, so the full project
	// prints across the pages even where a stretch has no events. A 6-month board
	// with July-only events and count=2 yields two ~3-month halves, from the real
	// project start to its real end.
	ticks := boardTicks("2026-07-15", "2027-01-29", "week")
	events := []any{
		map[string]any{"event": map[string]any{"start": "2026-07-16", "end": "2026-07-27"}},
	}
	board := Board{From: "2026-07-15", To: "2027-01-29", Ticks: ticks, Bars: boardBars(events, ticks, nil)}
	got := boardSliceWindows(board, events, 0, 2, false) // count=2, full window
	if len(got) != 2 {
		t.Fatalf("full-window count=2 must yield 2 pages, got %d: %+v", len(got), got)
	}
	if got[0].from != "2026-07-15" {
		t.Errorf("first page must start at the project start: %+v", got[0])
	}
	if got[1].to != "2027-01-29" {
		t.Errorf("last page must reach the project end: %+v", got[1])
	}
	// The second half has no events but is still a page (part of the plan).
	if len(eventsInWindow(events, got[1].from, got[1].to)) != 0 {
		t.Errorf("second half should be the empty stretch: %+v", got[1])
	}
}

func TestBoardSliceWindows_CountTilesEventSpanNotWholeWindow(t *testing.T) {
	// Regression: the project window runs half a year but the events cluster in a
	// mid-July fortnight. count=2 must split the EVENT span into two pages, not the
	// whole window (which would leave both events in one half → a single page).
	board := Board{
		From: "2026-07-15", To: "2027-01-29",
		Ticks: boardTicks("2026-07-15", "2027-01-29", "week"),
	}
	events := []any{
		map[string]any{"event": map[string]any{"start": "2026-07-16", "end": "2026-07-27"}},
		map[string]any{"event": map[string]any{"start": "2026-07-21", "end": "2026-07-24"}},
	}
	// Place bars so their tick indices are known (boardSliceWindows reads them).
	board.Bars = boardBars(events, board.Ticks, nil)
	got := boardSliceWindows(board, events, 0, 2, true) // count=2, trim to events
	if len(got) != 2 {
		t.Fatalf("trim count=2 over a clustered event span must yield 2 pages, got %d: %+v", len(got), got)
	}
	// Both pages sit in July, not out in the empty autumn tail.
	if got[0].from != "2026-07-15" || got[1].to >= "2026-08-15" {
		t.Errorf("trim slices should hug the event span, got %+v", got)
	}
}

func TestBoardGantt_EmptyPageStillFramed(t *testing.T) {
	// A full-window slice with no events must still render its framed axis (so an
	// empty stretch of the plan prints as a page), unlike a windowless empty board.
	framed := boardGantt(Board{Name: "Test", From: "2026-10-22", To: "2027-01-29"}, nil, true)
	if !strings.Contains(framed, "section Project") || !strings.Contains(framed, "Test :2026-10-22, 2027-01-29") {
		t.Errorf("empty windowed page should still emit the frame:\n%s", framed)
	}
	if boardGantt(Board{Name: "Test", From: "2026-10-22", To: "2027-01-29"}, nil, false) != "" {
		t.Error("empty page with scope=false should emit nothing")
	}
}

func TestBoardSlicesHelper_PerSliceGanttAndTable(t *testing.T) {
	tpl := &template.Template{
		ProjectMode: true,
		MarkdownTemplate: "{{#boardSlices size=4}}SLICE {{index}}/{{total}}\n" +
			"{{{gantt}}}\n\n{{{table}}}\n" +
			"{{#unless isLast}}<div class=\"page-break\"></div>{{/unless}}\n{{/boardSlices}}",
		Fields: []template.Field{
			{Key: "project", Type: "project", Options: []any{
				map[string]any{"value": "from", "label": "2026-07-06"},
				map[string]any{"value": "to", "label": "2026-08-30"},
				map[string]any{"value": "timeblock", "label": "week"},
				map[string]any{"value": "peter", "label": "Peter"},
			}},
			{Key: "events", Type: "loopstart"},
			{Key: "event", Type: "event", Options: []any{map[string]any{"value": "taak", "label": "Taak"}}},
			{Key: "omschrijving", Type: "text", Label: "Omschrijving"},
			{Key: "events", Type: "loopstop"},
		},
	}
	values := map[string]any{
		"project": map[string]any{"name": "Big"},
		"events": []any{
			map[string]any{"event": map[string]any{"resource": "peter", "kind": "taak", "start": "2026-07-07", "end": "2026-07-10", "omschrijving": "Vroeg"}},
			map[string]any{"event": map[string]any{"resource": "peter", "kind": "taak", "start": "2026-07-27", "end": "2026-08-10", "omschrijving": "Straddle"}},
			map[string]any{"event": map[string]any{"resource": "peter", "kind": "taak", "start": "2026-08-20", "end": "2026-08-25", "omschrijving": "Laat"}},
		},
	}
	out, err := RenderMarkdown(values, tpl, nil)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if !strings.Contains(out, "SLICE 1/2") || !strings.Contains(out, "SLICE 2/2") {
		t.Errorf("index/total not exposed to the block:\n%s", out)
	}
	if strings.Count(out, "```mermaid") != 2 {
		t.Errorf("want one gantt per slice (2), got %d:\n%s", strings.Count(out, "```mermaid"), out)
	}
	if strings.Count(out, `<div class="page-break">`) != 1 {
		t.Errorf("want exactly one page break between two slices:\n%s", out)
	}
	// The straddling event shows in BOTH slices' tables (inSlice = overlaps).
	if strings.Count(out, "Straddle") != 2 {
		t.Errorf("straddling event should appear in both slice tables, got %d:\n%s", strings.Count(out, "Straddle"), out)
	}
	// Slice-local events show once each.
	if strings.Count(out, "Vroeg") != 1 || strings.Count(out, "Laat") != 1 {
		t.Errorf("slice-local events should appear once each:\n%s", out)
	}
}

func TestBoardSlicesHelper_TrimDropsProjectScopeBar(t *testing.T) {
	// A big project window with July-only events. Full-window default keeps the
	// "Project" scope bar per page; trim=true hugs the events and must NOT draw a
	// scope bar (it would claim the 6-month project is only a week long).
	fields := []template.Field{
		{Key: "project", Type: "project", Options: []any{
			map[string]any{"value": "from", "label": "2026-07-15"},
			map[string]any{"value": "to", "label": "2027-01-29"},
			map[string]any{"value": "timeblock", "label": "week"},
			map[string]any{"value": "peter", "label": "Peter"},
		}},
		{Key: "events", Type: "loopstart"},
		{Key: "event", Type: "event", Options: []any{map[string]any{"value": "taak", "label": "Taak"}}},
		{Key: "events", Type: "loopstop"},
	}
	values := map[string]any{
		"project": map[string]any{"name": "Test"},
		"events": []any{
			map[string]any{"event": map[string]any{"resource": "peter", "kind": "taak", "start": "2026-07-16", "end": "2026-07-24"}},
		},
	}

	full, err := RenderMarkdown(values, &template.Template{ProjectMode: true, Fields: fields,
		MarkdownTemplate: "{{#boardSlices count=2}}{{{gantt}}}{{/boardSlices}}"}, nil)
	if err != nil {
		t.Fatalf("full: %v", err)
	}
	if !strings.Contains(full, "section Project") {
		t.Errorf("full-window slices should keep the project scope bar:\n%s", full)
	}

	trimmed, err := RenderMarkdown(values, &template.Template{ProjectMode: true, Fields: fields,
		MarkdownTemplate: "{{#boardSlices count=2 trim=true}}{{{gantt}}}{{/boardSlices}}"}, nil)
	if err != nil {
		t.Fatalf("trim: %v", err)
	}
	if strings.Contains(trimmed, "section Project") {
		t.Errorf("trim slices must not draw the misleading project scope bar:\n%s", trimmed)
	}
	if !strings.Contains(trimmed, "```mermaid") || !strings.Contains(trimmed, "section Peter") {
		t.Errorf("trim slices should still show the event bars:\n%s", trimmed)
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
	// Default shows the full-window frame; {{board scope=false}} omits it.
	if !strings.Contains(out, "section Project") {
		t.Errorf("default {{board}} should frame the window:\n%s", out)
	}
	tpl.MarkdownTemplate = "{{board scope=false}}"
	noScope, err := RenderMarkdown(values, tpl, nil)
	if err != nil {
		t.Fatalf("RenderMarkdown(scope=false): %v", err)
	}
	if strings.Contains(noScope, "section Project") {
		t.Errorf("{{board scope=false}} should omit the frame:\n%s", noScope)
	}
	if !strings.Contains(noScope, "| Resource | Kind | Start | End |") {
		t.Errorf("table still expected with scope=false:\n%s", noScope)
	}
}
