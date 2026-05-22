package form

import "github.com/petervdpas/formidable2/internal/modules/template"

// ComputeLoopGroups pairs loopstart/loopstop fields and computes the
// derived bits Vue needs to render: depth (1 = top-level, 2 = nested),
// the summary field key (from loopstart.summary_field), and the initial
// collapsed state (per-loop override `loopstart.Collapsible == &true`
// wins over the config-wide defaultCollapsed).
//
// Pairing is best-effort: structural problems (unmatched start/stop,
// key mismatch) are rejected by template.Validate elsewhere; here we
// just skip what we can't pair so a malformed template doesn't crash
// the form view.
//
// Returned groups are in start-index order (outer before inner).
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
				// stranded loopstop - skip
				continue
			}
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if top.field.Key != f.Key {
				// key mismatch - drop both halves
				continue
			}
			// Depth = how many open frames remained AFTER popping ours.
			// 0 remaining → top-level → depth 1; 1 remaining → depth 2.
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

	// Sort by StartIndex so outer loops appear before inner.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].StartIndex < out[i].StartIndex {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
