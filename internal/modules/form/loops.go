package form

import "github.com/petervdpas/formidable2/internal/modules/template"

// ComputeLoopGroups pairs loopstart/loopstop fields into render groups.
// Pairing is best-effort: structural problems are rejected by
// template.Validate elsewhere, so here we skip what we can't pair rather
// than crash the form view. Per-loop Collapsible override wins over
// defaultCollapsed. Returned groups are in start-index order.
func ComputeLoopGroups(fields []template.Field, defaultCollapsed bool) []LoopGroup {
	out := make([]LoopGroup, 0)
	if len(fields) == 0 {
		return out
	}

	type frame struct {
		field template.Field
		index int
	}
	var stack []frame

	for i := range fields {
		f := fields[i]
		switch f.Type {
		case "loopstart":
			stack = append(stack, frame{field: f, index: i})
		case "loopstop":
			if len(stack) == 0 {
				continue
			}
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if top.field.Key != f.Key {
				continue
			}
			depth := len(stack) + 1

			collapsed := defaultCollapsed
			if top.field.Collapsible != nil && *top.field.Collapsible {
				collapsed = true
			}

			out = append(out, LoopGroup{
				Key:              top.field.Key,
				StartIndex:       top.index,
				StopIndex:        i,
				Depth:            depth,
				SummaryFieldKey:  top.field.SummaryField,
				DefaultCollapsed: collapsed,
			})
		}
	}

	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].StartIndex < out[i].StartIndex {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
