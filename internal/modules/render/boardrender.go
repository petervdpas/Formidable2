package render

import (
	"strings"
	"time"

	"github.com/aymerick/raymond"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// A plan-board record renders to two complementary Markdown blocks. The mermaid
// Gantt is the visual timeline: resource lanes, bars, milestones. It can only
// carry the event's own axes (resource, dates, kind); any field the author adds
// to the events loop (a description, a note, a link) has nowhere to sit on the
// chart. So the GFM table is the detail view: one row per event, one column per
// field, including every author-added one. boardMarkdown emits both from the
// same events data, Gantt first, table second.

// boardGantt renders a Board as a ```mermaid gantt fenced block. Sections are
// the board's resources in record order; each event is one task under its
// resource. kindLabels maps a kind value to the display label used as the task
// name (falls back to the raw value). Returns "" when the board has no bars, so
// a record with no events emits no stray empty chart.
func boardGantt(b Board, kindLabels map[string]string) string {
	if len(b.Bars) == 0 {
		return ""
	}
	lines := []string{"gantt"}
	if title := ganttSanitize(b.Name); title != "" {
		lines = append(lines, "    title "+title)
	}
	lines = append(lines, "    dateFormat YYYY-MM-DD", "    axisFormat %b %d")

	emitSection := func(header string, bars []BoardBar) {
		if len(bars) == 0 {
			return
		}
		lines = append(lines, "    section "+ganttSanitize(header))
		for _, bar := range bars {
			lines = append(lines, "        "+ganttTask(bar, kindLabels))
		}
	}

	seen := map[string]bool{}
	for _, r := range b.Resources {
		emitSection(r.Label, barsForResource(b.Bars, r.Value))
		seen[r.Value] = true
	}
	// Bars whose resource isn't a declared row still belong on the chart: one
	// section per distinct value, in first-seen order.
	for _, bar := range b.Bars {
		if seen[bar.Resource] {
			continue
		}
		seen[bar.Resource] = true
		emitSection(bar.Resource, barsForResource(b.Bars, bar.Resource))
	}
	return "```mermaid\n" + strings.Join(lines, "\n") + "\n```"
}

// ganttTask renders one bar as a mermaid gantt task line. A milestone is a
// zero-span marker; a normal task carries its start and end dates.
func ganttTask(bar BoardBar, kindLabels map[string]string) string {
	name := ganttSanitize(labelOr(kindLabels, bar.Kind))
	if bar.Milestone {
		return name + " :milestone, " + bar.Start + ", 0d"
	}
	end := bar.End
	if end == "" {
		end = bar.Start
	}
	return name + " :" + bar.Start + ", " + end
}

// barsForResource returns the bars belonging to one resource, order preserved.
func barsForResource(bars []BoardBar, resource string) []BoardBar {
	var out []BoardBar
	for _, b := range bars {
		if b.Resource == resource {
			out = append(out, b)
		}
	}
	return out
}

// ganttSanitize strips the mermaid gantt delimiters (":" opens the metadata,
// "," separates it) and newlines from a label so it can't break the syntax.
func ganttSanitize(s string) string {
	r := strings.NewReplacer(":", " ", ",", " ", ";", " ", "\n", " ", "\r", " ")
	return strings.TrimSpace(r.Replace(s))
}

// eventsTable renders the events loop as a GFM table: the four event axes
// (resource, kind, start, end) followed by one column per author-added field.
// authorFields are the events-loop inner fields with the event field itself
// removed; their values are read from inside the event object (Option A folds
// them there), with an iteration-level fallback. Returns "" when empty.
func eventsTable(events any, kindLabels, resourceLabels map[string]string, authorFields []template.Field, opts *Options) string {
	rows, ok := events.([]any)
	if !ok || len(rows) == 0 {
		return ""
	}
	headers := []string{"Resource", "Kind", "Start", "End"}
	for _, f := range authorFields {
		h := f.Label
		if h == "" {
			h = f.Key
		}
		headers = append(headers, cellEscape(h))
	}

	var b strings.Builder
	b.WriteString("| " + strings.Join(headers, " | ") + " |\n")
	sep := make([]string, len(headers))
	for i := range sep {
		sep[i] = "---"
	}
	b.WriteString("| " + strings.Join(sep, " | ") + " |")

	for _, row := range rows {
		m, ok := row.(map[string]any)
		if !ok {
			continue
		}
		ev, _ := m["event"].(map[string]any)
		doc, _ := template.ParseEventDoc(m["event"])
		cells := []string{
			cellEscape(labelOr(resourceLabels, doc.Resource)),
			cellEscape(labelOr(kindLabels, doc.Kind)),
			doc.Start,
			doc.End,
		}
		for _, f := range authorFields {
			var v any
			if ev != nil {
				v = ev[f.Key]
			}
			if v == nil {
				v = m[f.Key] // sibling fallback if not folded
			}
			f := f
			cells = append(cells, cellEscape(emitFieldValue(v, &f, opts)))
		}
		b.WriteString("\n| " + strings.Join(cells, " | ") + " |")
	}
	return b.String()
}

// labelOr resolves value to its display label, falling back to the value.
func labelOr(labels map[string]string, value string) string {
	if lab := labels[value]; lab != "" {
		return lab
	}
	return value
}

// cellEscape makes a string safe inside one GFM table cell: pipes escaped,
// newlines collapsed to a space (a table cell is single-line).
func cellEscape(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.TrimSpace(s)
}

// kindLabelMap maps each author-defined kind value to its display label.
func kindLabelMap(event *template.Field) map[string]string {
	out := map[string]string{}
	if event == nil {
		return out
	}
	for _, opt := range event.Options {
		val, lab := optionPair(opt)
		if val != "" {
			out[val] = lab
		}
	}
	return out
}

// resourceLabelMap maps each resource value to its display label.
func resourceLabelMap(project *template.Field) map[string]string {
	out := map[string]string{}
	if project == nil {
		return out
	}
	for _, r := range template.ProjectResources(*project) {
		out[r.Value] = r.Label
	}
	return out
}

// loopAuthorFields returns the events-loop inner fields the author added, i.e.
// everything but the event field and the loop markers.
func loopAuthorFields(inner []template.Field) []template.Field {
	var out []template.Field
	for _, f := range inner {
		switch f.Type {
		case "event", "loopstart", "loopstop":
			continue
		}
		out = append(out, f)
	}
	return out
}

// boardMetaValue resolves one {{boardMeta}} property off a plan-board record.
// project is the template's project field (axis window + resources); ctx holds
// the record values (project name, events loop). Counts return an int; strings
// return "". An unknown prop returns "".
func boardMetaValue(prop, unit string, project *template.Field, ctx map[string]any) any {
	doc, _ := template.ParseProjectDoc(ctx["project"])
	var from, to, tb string
	if project != nil {
		from, to = template.ProjectDateRange(*project)
		tb = resolveTimeBlock(doc.TimeBlock, project)
	}
	switch prop {
	case "name":
		return doc.Name
	case "from":
		return from
	case "to":
		return to
	case "timeblock":
		return tb
	case "duration":
		return projectDuration(from, to, unit)
	case "ticks":
		return len(boardTicks(from, to, tb))
	case "events":
		return loopLen(ctx["events"])
	case "resources":
		if project == nil {
			return 0
		}
		return len(template.ProjectResources(*project))
	default:
		return ""
	}
}

// projectDuration returns the axis window length. Default unit is calendar days
// between from and to; "weeks" floors days/7; "months" counts whole months.
// A missing or inverted window returns "".
func projectDuration(from, to, unit string) any {
	start, err1 := time.Parse(boardDateLayout, from)
	end, err2 := time.Parse(boardDateLayout, to)
	if from == "" || to == "" || err1 != nil || err2 != nil || end.Before(start) {
		return ""
	}
	days := int(end.Sub(start).Hours() / 24)
	switch unit {
	case "week", "weeks":
		return days / 7
	case "month", "months":
		return monthsBetween(start, end)
	default:
		return days
	}
}

// monthsBetween counts whole months from start to end (a partial trailing month
// doesn't count). Never negative.
func monthsBetween(start, end time.Time) int {
	months := (end.Year()-start.Year())*12 + int(end.Month()) - int(start.Month())
	if end.Day() < start.Day() {
		months--
	}
	if months < 0 {
		return 0
	}
	return months
}

// loopLen is the entry count of a loop value ([]any), 0 for anything else.
func loopLen(v any) int {
	if arr, ok := v.([]any); ok {
		return len(arr)
	}
	return 0
}

// registerBoardMetaHelper binds {{boardMeta "prop" [unit]}}: read one scalar off
// the plan-board record (name, from, to, timeblock, duration, ticks, events,
// resources). The read-out companion to {{board}}'s full render.
func registerBoardMetaHelper(tpl *raymond.Template) {
	tpl.RegisterHelper("boardMeta", func(options *raymond.Options) any {
		params := options.Params()
		var prop, unit string
		if len(params) > 0 {
			prop, _ = params[0].(string)
		}
		if len(params) > 1 {
			unit, _ = params[1].(string)
		}
		prop = strings.ToLower(strings.TrimSpace(prop))
		unit = strings.ToLower(strings.TrimSpace(unit))

		ctx := contextMap(options.Ctx())
		if ctx == nil {
			return ""
		}
		var project *template.Field
		for _, f := range contextFields(options.Ctx()) {
			if f.Type == "project" {
				ff := f
				project = &ff
				break
			}
		}
		return boardMetaValue(prop, unit, project, ctx)
	})
}

// registerBoardHelper binds {{board}}: the plan-board render. It reads the
// record's project axis + events loop from the context and emits the mermaid
// Gantt followed by the events table. Options-only so a bare {{board}} works.
func registerBoardHelper(tpl *raymond.Template, opts *Options) {
	tpl.RegisterHelper("board", func(options *raymond.Options) raymond.SafeString {
		ctx := contextMap(options.Ctx())
		if ctx == nil {
			return ""
		}
		fields := contextFields(options.Ctx())
		var project, event *template.Field
		for i := range fields {
			switch fields[i].Type {
			case "project":
				project = &fields[i]
			case "event":
				event = &fields[i]
			}
		}
		if project == nil {
			return ""
		}

		events := ctx["events"]
		doc, _ := template.ParseProjectDoc(ctx["project"])
		board := buildBoard(project, event, doc.Name, events, doc.ResourceOrder, doc.TimeBlock)

		kinds := kindLabelMap(event)
		gantt := boardGantt(board, kinds)
		table := eventsTable(events, kinds, resourceLabelMap(project), loopAuthorFields(loopGroupFields(ctx, "events")), opts)

		var parts []string
		if gantt != "" {
			parts = append(parts, gantt)
		}
		if table != "" {
			parts = append(parts, table)
		}
		return raymond.SafeString(strings.Join(parts, "\n\n"))
	})
}
