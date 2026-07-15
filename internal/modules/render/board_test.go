package render

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestBoardTicks_WeeklyRange(t *testing.T) {
	ticks := boardTicks("2026-06-29", "2026-08-24", "week")
	if len(ticks) != 9 {
		t.Fatalf("want 9 weekly ticks, got %d", len(ticks))
	}
	if ticks[0].Start != "2026-06-29" || ticks[0].End != "2026-07-06" {
		t.Errorf("tick0 = %+v", ticks[0])
	}
	if ticks[0].Label != "wk 27" {
		t.Errorf("tick0 label = %q, want wk 27", ticks[0].Label)
	}
}

func TestBoardTicks_BadRange(t *testing.T) {
	for _, c := range []struct{ from, to string }{
		{"", "2026-08-24"}, {"2026-06-29", ""}, {"nope", "2026-08-24"},
		{"2026-08-24", "2026-06-29"}, // inverted
	} {
		if got := boardTicks(c.from, c.to, "week"); got != nil {
			t.Errorf("boardTicks(%q,%q) = %+v, want nil", c.from, c.to, got)
		}
	}
}

func TestBoardTicks_Granularities(t *testing.T) {
	if got := len(boardTicks("2026-07-01", "2026-07-05", "day")); got != 5 {
		t.Errorf("daily ticks = %d, want 5", got)
	}
	if got := len(boardTicks("2026-01-01", "2026-03-31", "month")); got != 3 {
		t.Errorf("monthly ticks = %d, want 3", got)
	}
	if got := len(boardTicks("2026-06-29", "2026-08-24", "2-week")); got != 5 {
		t.Errorf("2-week ticks = %d, want 5", got)
	}
}

func TestPlaceBar_SpanClampAndMilestone(t *testing.T) {
	ticks := boardTicks("2026-06-29", "2026-08-24", "week") // 9 ticks (0..8)

	// A task spanning two weeks lands on its start/end ticks.
	bar, ok := placeBar(template.EventDoc{Start: "2026-07-06", End: "2026-07-20", Kind: "task"}, ticks)
	if !ok || bar.StartTick != 1 || bar.EndTick != 3 || bar.Milestone {
		t.Errorf("task bar = %+v ok=%v", bar, ok)
	}

	// A milestone is a zero-span point.
	ms, ok := placeBar(template.EventDoc{Start: "2026-07-13", Kind: "milestone"}, ticks)
	if !ok || !ms.Milestone || ms.StartTick != ms.EndTick || ms.StartTick != 2 {
		t.Errorf("milestone bar = %+v ok=%v", ms, ok)
	}

	// An event starting before the axis clamps its start to tick 0.
	clamped, ok := placeBar(template.EventDoc{Start: "2026-05-01", End: "2026-07-06", Kind: "task"}, ticks)
	if !ok || clamped.StartTick != 0 {
		t.Errorf("clamped bar start = %+v ok=%v", clamped, ok)
	}

	// An event ending after the axis clamps its end to the last tick.
	tail, ok := placeBar(template.EventDoc{Start: "2026-08-17", End: "2026-12-01", Kind: "task"}, ticks)
	if !ok || tail.EndTick != len(ticks)-1 {
		t.Errorf("tail bar end = %+v ok=%v", tail, ok)
	}
}

func TestBuildBoard_AxisAndBars(t *testing.T) {
	tpl := &template.Template{ProjectMode: true, Fields: []template.Field{
		{Key: "project", Type: "project", Options: []any{
			map[string]any{"value": "from", "label": "2026-06-29"},
			map[string]any{"value": "to", "label": "2026-08-24"},
			map[string]any{"value": "timeblock", "label": "week"},
			map[string]any{"value": "dev", "label": "Development"},
			map[string]any{"value": "qa", "label": "QA"},
		}},
		{Key: "events", Type: "loopstart"},
		{Key: "event", Type: "event"},
		{Key: "events", Type: "loopstop"},
	}}
	store := &keyedFormStore{forms: map[string]*storage.Form{
		"test.meta.json": {Data: map[string]any{
			"project": map[string]any{"name": "Test"},
			"events": []any{
				map[string]any{"event": map[string]any{
					"start": "2026-07-06", "end": "2026-07-20", "kind": "task", "resource": "dev",
				}},
			},
		}},
	}}
	m := NewManager(&fakeTemplateLoader{tpl: tpl}, store, nil, nil, nil)

	board, err := m.BuildBoard("tpl", "test.meta.json")
	if err != nil {
		t.Fatalf("BuildBoard: %v", err)
	}
	if board.Name != "Test" {
		t.Errorf("name = %q, want Test", board.Name)
	}
	if len(board.Ticks) != 9 {
		t.Errorf("ticks = %d, want 9", len(board.Ticks))
	}
	if len(board.Resources) != 2 || board.Resources[0].Value != "dev" || board.Resources[0].Label != "Development" {
		t.Errorf("resources = %+v, want [dev/Development, qa/QA]", board.Resources)
	}
	if len(board.Bars) != 1 {
		t.Fatalf("bars = %d, want 1", len(board.Bars))
	}
	b := board.Bars[0]
	if b.Resource != "dev" || b.Kind != "task" || b.StartTick != 1 || b.EndTick != 3 {
		t.Errorf("bar = %+v", b)
	}
}

func TestBuildBoard_NoProjectField(t *testing.T) {
	tpl := &template.Template{Fields: []template.Field{{Key: "id", Type: "guid"}}}
	m := NewManager(&fakeTemplateLoader{tpl: tpl}, &fakeFormStore{}, nil, nil, nil)
	if _, err := m.BuildBoard("tpl", "x"); err == nil {
		t.Errorf("expected an error when the template has no project field")
	}
}
