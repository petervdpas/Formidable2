package render

import "github.com/petervdpas/formidable2/internal/modules/template"

// The render context is a map[string]any built by RenderMarkdown:
// data values plus reserved keys (_fields, _template, _loopGroups, and
// per-iteration _loopKey / _loopIndex). Helpers reach this state through
// the accessors below.

func contextMap(ctx any) map[string]any {
	if m, ok := ctx.(map[string]any); ok {
		return m
	}
	return nil
}

func contextFields(ctx any) []template.Field {
	m := contextMap(ctx)
	if m == nil {
		return nil
	}
	switch f := m["_fields"].(type) {
	case []template.Field:
		return f
	case []any:
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

func findField(ctx any, key string) *template.Field {
	fields := contextFields(ctx)
	for i := range fields {
		if fields[i].Key == key {
			return &fields[i]
		}
	}
	return nil
}

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

// buildNestedLoopGroups snapshots the fields between each loopstart and
// loopstop, keyed by the loop's field key.
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
