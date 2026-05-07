package render

import "github.com/petervdpas/formidable2/internal/modules/template"

// ─────────────────────────────────────────────────────────────────────
// Context plumbing for the Handlebars helpers.
//
// The render context is a map[string]any built by RenderMarkdown:
// data values + three reserved keys —
//
//   _fields      — []template.Field (for findField / fieldRaw / etc.)
//   _template    — *template.Template (for the loop helper to attach
//                  to per-iteration sub-contexts)
//   _loopGroups  — map[string][]template.Field (per-loop body roster,
//                  used by the loop helper to set inner _fields)
//
// Per-iteration sub-contexts also carry _loopKey + _loopIndex so the
// loopItemBefore / loopItemAfter / loopKey / loopIndex helpers can
// read them without re-deriving.
//
// All helpers (in helpers.go, helpers_field.go, apifield_helpers.go)
// reach into this state through the small accessor surface below.
// ─────────────────────────────────────────────────────────────────────

// contextMap normalizes raymond's Ctx() to map[string]any when possible.
func contextMap(ctx any) map[string]any {
	if m, ok := ctx.(map[string]any); ok {
		return m
	}
	return nil
}

// contextFields pulls the field list from the current context's
// `_fields` slot. Returns an empty slice if missing.
func contextFields(ctx any) []template.Field {
	m := contextMap(ctx)
	if m == nil {
		return nil
	}
	switch f := m["_fields"].(type) {
	case []template.Field:
		return f
	case []any:
		// Preserves the same shape after JSON round-trip.
		out := make([]template.Field, 0, len(f))
		for _, raw := range f {
			if mm, ok := raw.(map[string]any); ok {
				out = append(out, fieldFromMap(mm))
			}
		}
		return out
	}
	return nil
}

// loopIndexFromCtx coerces _loopIndex to int. Per-iteration context
// is built by the loop helper so the value is always set; this is
// defensive for hand-rolled callers.
func loopIndexFromCtx(ctx map[string]any) int {
	switch v := ctx["_loopIndex"].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

// findField returns a pointer to the field with key in the current
// context's `_fields` (nil if missing).
func findField(ctx any, key string) *template.Field {
	fields := contextFields(ctx)
	for i := range fields {
		if fields[i].Key == key {
			return &fields[i]
		}
	}
	return nil
}

// fieldFromMap rebuilds a template.Field from a map[string]any (used
// when context fields arrived via JSON unmarshal rather than directly
// from the typed slice).
func fieldFromMap(m map[string]any) template.Field {
	f := template.Field{
		Key:         stringify(m["key"]),
		Type:        stringify(m["type"]),
		Label:       stringify(m["label"]),
		Description: stringify(m["description"]),
	}
	if opts, ok := m["options"].([]any); ok {
		f.Options = opts
	}
	return f
}

// loopGroupFields fetches the field slice associated with the named
// loop from `_loopGroups`. Empty when missing.
func loopGroupFields(ctx map[string]any, key string) []template.Field {
	switch g := ctx["_loopGroups"].(type) {
	case map[string][]template.Field:
		return g[key]
	case map[string]any:
		if v, ok := g[key]; ok {
			if fs, ok := v.([]template.Field); ok {
				return fs
			}
		}
	}
	return nil
}

// buildNestedLoopGroups mirrors the original JS helper of the same
// name: walks fields, pushes a stack on `loopstart`, pops on
// `loopstop`, and snapshots the in-between fields per loop key.
func buildNestedLoopGroups(fields []template.Field) map[string][]template.Field {
	out := map[string][]template.Field{}
	type frame struct {
		key    string
		fields []template.Field
	}
	var stack []frame
	for _, f := range fields {
		switch f.Type {
		case "loopstart":
			stack = append(stack, frame{key: f.Key, fields: nil})
		case "loopstop":
			if len(stack) == 0 {
				continue
			}
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			out[top.key] = top.fields
		default:
			if len(stack) > 0 {
				stack[len(stack)-1].fields = append(stack[len(stack)-1].fields, f)
			}
		}
	}
	return out
}
