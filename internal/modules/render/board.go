package render

import (
	"fmt"
	"sort"
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
	Index     int    `json:"index"` // source events-loop entry index (for click-to-edit)
	Resource  string `json:"resource"`
	Kind      string `json:"kind"`
	Color     string `json:"color"` // the kind's colour (from the event field options)
	Start     string `json:"start"`
	End       string `json:"end"`
	StartTick int    `json:"start_tick"`
	EndTick   int    `json:"end_tick"`
	Milestone bool   `json:"milestone"`
}

// Board is the structured layout of a single plan-board record: the X axis
// (Ticks), the Y axis (Resources), and the events placed on both (Bars). Each
// bar's Resource names the row it belongs to.
type Board struct {
	Name      string                        `json:"name"`
	From      string                        `json:"from"`
	To        string                        `json:"to"`
	TimeBlock string                        `json:"time_block"`
	Ticks     []BoardTick                   `json:"ticks"`
	Resources []template.ResourceDescriptor `json:"resources"`
	Bars      []BoardBar                    `json:"bars"`
}

// BuildBoard reads one record's project axis and events loop and returns the
// board layout. A missing/unparseable axis yields a board with no ticks (the
// viewer prompts the author to set the range); events outside the axis are
// clamped, and any event that can't be placed at all is dropped.
func (m *Manager) BuildBoard(templateName, datafile string) (Board, error) {
	project, event, err := m.boardFields(templateName)
	if err != nil {
		return Board{}, err
	}
	var name string
	var events any
	var order []string
	var timeBlock string
	if loaded := m.storage.LoadForm(templateName, datafile); loaded != nil {
		if doc, derr := template.ParseProjectDoc(loaded.Data[project.Key]); derr == nil {
			name = doc.Name
			order = doc.ResourceOrder
			timeBlock = doc.TimeBlock
		}
		events = loaded.Data["events"]
	}
	return buildBoard(project, event, name, events, order, timeBlock), nil
}

// BuildBoardLive lays the given in-progress events onto the template's project
// axis, without reading the saved record. The form editor calls this so the
// board updates as the user edits the events loop. name titles the board; events
// is the loop value ([{event:{...}}, ...]); resourceOrder is this record's Y-axis
// order; timeBlock is this record's granularity override (empty = the field's
// authored default). Both persist on the project value.
func (m *Manager) BuildBoardLive(templateName, name string, events any, resourceOrder []string, timeBlock string) (Board, error) {
	project, event, err := m.boardFields(templateName)
	if err != nil {
		return Board{}, err
	}
	return buildBoard(project, event, name, events, resourceOrder, timeBlock), nil
}

// boardFields loads a template and returns its project field (required) and
// event field (may be nil).
func (m *Manager) boardFields(templateName string) (project, event *template.Field, err error) {
	tpl, err := m.templates.LoadTemplate(templateName)
	if err != nil {
		return nil, nil, fmt.Errorf("render: load template %q: %w", templateName, err)
	}
	for i := range tpl.Fields {
		switch tpl.Fields[i].Type {
		case "project":
			project = &tpl.Fields[i]
		case "event":
			event = &tpl.Fields[i]
		}
	}
	if project == nil {
		return nil, nil, fmt.Errorf("render: template %q has no project field", templateName)
	}
	return project, event, nil
}

// orderResources sorts the resources to match `order` (by value); resources not
// named keep their relative order after the named ones. Empty order = no change.
func orderResources(rs []template.ResourceDescriptor, order []string) []template.ResourceDescriptor {
	if len(order) == 0 || len(rs) == 0 {
		return rs
	}
	rank := make(map[string]int, len(order))
	for i, v := range order {
		rank[v] = i
	}
	out := make([]template.ResourceDescriptor, len(rs))
	copy(out, rs)
	sort.SliceStable(out, func(a, b int) bool {
		ra, oka := rank[out[a].Value]
		rb, okb := rank[out[b].Value]
		if oka && okb {
			return ra < rb
		}
		return oka && !okb
	})
	return out
}

// resolveTimeBlock picks the record's granularity override when it names a known
// time block, else the field's authored default.
func resolveTimeBlock(override string, project *template.Field) string {
	if template.IsTimeBlock(override) {
		return override
	}
	return template.ProjectTimeBlock(*project)
}

// eventKindColors maps each author-defined kind value to its colour, read from
// the event field options ({value, color} rows).
func eventKindColors(event *template.Field) map[string]string {
	out := map[string]string{}
	if event == nil {
		return out
	}
	for _, opt := range event.Options {
		m, ok := opt.(map[string]any)
		if !ok {
			continue
		}
		val, _ := m["value"].(string)
		color, _ := m["color"].(string)
		if val != "" {
			out[val] = color
		}
	}
	return out
}

// buildBoard is the shared layout core: axis + resources from the project field
// (resources ordered by the record's resourceOrder), bars from an events value,
// each bar coloured by its kind. timeBlock is the record's granularity override;
// an empty or unrecognised value falls back to the field's authored default.
func buildBoard(project, event *template.Field, name string, events any, resourceOrder []string, timeBlock string) Board {
	from, to := template.ProjectDateRange(*project)
	board := Board{
		Name:      name,
		From:      from,
		To:        to,
		TimeBlock: resolveTimeBlock(timeBlock, project),
		Resources: orderResources(template.ProjectResources(*project), resourceOrder),
	}
	board.Ticks = boardTicks(from, to, board.TimeBlock)
	if len(board.Ticks) == 0 {
		return board
	}
	board.Bars = boardBars(events, board.Ticks, eventKindColors(event))
	return board
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
func boardBars(v any, ticks []BoardTick, kindColors map[string]string) []BoardBar {
	rows, ok := v.([]any)
	if !ok {
		return nil
	}
	var bars []BoardBar
	for i, row := range rows {
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
		bar.Index = i
		bar.Color = kindColors[doc.Kind]
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
