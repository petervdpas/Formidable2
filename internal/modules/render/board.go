package render

import (
	"fmt"
	"strconv"
	"time"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// A plan board is one record: the project field carries the shared time axis
// (from/to dates + a time-block granularity) in its options, and the record's
// "events" loop holds the bars. BuildBoard turns that into a structured layout -
// a sequence of axis ticks plus each event mapped to the tick range it spans -
// so the frontend draws the grid without re-deriving any date math.

const boardDateLayout = "2006-01-02"

// BoardTick is one column of the board's time axis: a half-open date range
// [Start, End) and a display label (ISO week for week-based blocks, the date for
// days, YYYY-MM for months).
type BoardTick struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Label string `json:"label"`
}

// BoardBar is one event laid onto the axis. StartTick/EndTick are inclusive tick
// indices (clamped to the axis); Milestone marks a zero-span point (a task with
// no end, or kind "milestone").
type BoardBar struct {
	Resource  string `json:"resource"`
	Kind      string `json:"kind"`
	Start     string `json:"start"`
	End       string `json:"end"`
	StartTick int    `json:"start_tick"`
	EndTick   int    `json:"end_tick"`
	Milestone bool   `json:"milestone"`
}

// Board is the structured layout of a single plan-board record.
type Board struct {
	Name      string      `json:"name"`
	From      string      `json:"from"`
	To        string      `json:"to"`
	TimeBlock string      `json:"time_block"`
	Ticks     []BoardTick `json:"ticks"`
	Bars      []BoardBar  `json:"bars"`
}

// BuildBoard reads one record's project axis and events loop and returns the
// board layout. A missing/unparseable axis yields a board with no ticks (the
// viewer prompts the author to set the range); events outside the axis are
// clamped, and any event that can't be placed at all is dropped.
func (m *Manager) BuildBoard(templateName, datafile string) (Board, error) {
	tpl, err := m.templates.LoadTemplate(templateName)
	if err != nil {
		return Board{}, fmt.Errorf("render: load template %q: %w", templateName, err)
	}
	var project *template.Field
	for i := range tpl.Fields {
		if tpl.Fields[i].Type == "project" {
			project = &tpl.Fields[i]
			break
		}
	}
	if project == nil {
		return Board{}, fmt.Errorf("render: template %q has no project field", templateName)
	}

	from, to := template.ProjectDateRange(*project)
	tb := template.ProjectTimeBlock(*project)
	board := Board{From: from, To: to, TimeBlock: tb}

	loaded := m.storage.LoadForm(templateName, datafile)
	if loaded != nil {
		if doc, derr := template.ParseProjectDoc(loaded.Data[project.Key]); derr == nil {
			board.Name = doc.Name
		}
	}

	board.Ticks = boardTicks(from, to, tb)
	if len(board.Ticks) == 0 {
		return board, nil
	}
	if loaded != nil {
		board.Bars = boardBars(loaded.Data["events"], board.Ticks)
	}
	return board, nil
}

// boardTicks divides [from, to] into contiguous half-open columns of the given
// time-block width. Returns nil when either endpoint is missing or unparseable,
// or the range is inverted.
func boardTicks(from, to, tb string) []BoardTick {
	start, err1 := time.Parse(boardDateLayout, from)
	end, err2 := time.Parse(boardDateLayout, to)
	if from == "" || to == "" || err1 != nil || err2 != nil || end.Before(start) {
		return nil
	}
	var ticks []BoardTick
	for cur := start; !cur.After(end); {
		next := stepTick(cur, tb)
		if !next.After(cur) { // guard against a zero-width step
			next = cur.AddDate(0, 0, 1)
		}
		ticks = append(ticks, BoardTick{
			Start: cur.Format(boardDateLayout),
			End:   next.Format(boardDateLayout),
			Label: tickLabel(cur, tb),
		})
		cur = next
	}
	return ticks
}

// stepTick advances one time-block width from cur.
func stepTick(cur time.Time, tb string) time.Time {
	switch tb {
	case template.TimeBlockDay:
		return cur.AddDate(0, 0, 1)
	case template.TimeBlock2Week:
		return cur.AddDate(0, 0, 14)
	case template.TimeBlock3Week:
		return cur.AddDate(0, 0, 21)
	case template.TimeBlockMonth:
		return cur.AddDate(0, 1, 0)
	default: // week (and any unknown, defaulted upstream)
		return cur.AddDate(0, 0, 7)
	}
}

// tickLabel renders a column header: the ISO week for week-based blocks, the
// date for days, YYYY-MM for months.
func tickLabel(cur time.Time, tb string) string {
	switch tb {
	case template.TimeBlockDay:
		return cur.Format(boardDateLayout)
	case template.TimeBlockMonth:
		return cur.Format("2006-01")
	default:
		_, wk := cur.ISOWeek()
		return "wk " + strconv.Itoa(wk)
	}
}

// boardBars maps each event in the loop value onto tick indices. The loop value
// is []any of {event: {...}} maps (one per iteration).
func boardBars(v any, ticks []BoardTick) []BoardBar {
	rows, ok := v.([]any)
	if !ok {
		return nil
	}
	var bars []BoardBar
	for _, row := range rows {
		m, ok := row.(map[string]any)
		if !ok {
			continue
		}
		doc, err := template.ParseEventDoc(m["event"])
		if err != nil || doc.Start == "" {
			continue
		}
		bar, ok := placeBar(doc, ticks)
		if !ok {
			continue
		}
		bars = append(bars, bar)
	}
	return bars
}

// placeBar clamps an event to the axis and returns its inclusive tick span, or
// ok=false when it falls entirely outside the axis. A milestone (kind
// "milestone", or no distinct end) is a zero-span point at its start tick.
func placeBar(doc template.EventDoc, ticks []BoardTick) (BoardBar, bool) {
	milestone := doc.Kind == template.EventKindMilestone || doc.End == "" || doc.End == doc.Start
	end := doc.End
	if milestone {
		end = doc.Start
	}
	startTick, ok1 := clampTick(doc.Start, ticks)
	endTick, ok2 := clampTick(end, ticks)
	if !ok1 || !ok2 {
		return BoardBar{}, false
	}
	if endTick < startTick {
		startTick, endTick = endTick, startTick
	}
	if milestone {
		endTick = startTick
	}
	return BoardBar{
		Resource:  doc.Resource,
		Kind:      doc.Kind,
		Start:     doc.Start,
		End:       doc.End,
		StartTick: startTick,
		EndTick:   endTick,
		Milestone: milestone,
	}, true
}

// clampTick returns the tick index containing date, clamping a date before the
// axis to 0 and after the axis to the last tick. ok=false only when the date is
// unparseable.
func clampTick(date string, ticks []BoardTick) (int, bool) {
	d, err := time.Parse(boardDateLayout, date)
	if err != nil {
		return 0, false
	}
	first, _ := time.Parse(boardDateLayout, ticks[0].Start)
	last, _ := time.Parse(boardDateLayout, ticks[len(ticks)-1].End)
	if d.Before(first) {
		return 0, true
	}
	if !d.Before(last) {
		return len(ticks) - 1, true
	}
	for i, t := range ticks {
		ts, _ := time.Parse(boardDateLayout, t.Start)
		te, _ := time.Parse(boardDateLayout, t.End)
		if !d.Before(ts) && d.Before(te) {
			return i, true
		}
	}
	return len(ticks) - 1, true
}
