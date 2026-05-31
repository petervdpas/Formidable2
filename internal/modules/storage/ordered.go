package storage

import (
	"bytes"
	"encoding/json"
	"sort"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// orderedObject marshals keys in recorded order (not Go's alphabetical map default) so the on-disk
// data block mirrors the template's field declaration order.
type orderedObject struct {
	keys []string
	vals map[string]any
}

func (o orderedObject) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		b.Write(kb)
		b.WriteByte(':')
		vb, err := json.Marshal(o.vals[k])
		if err != nil {
			return nil, err
		}
		b.Write(vb)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// orderedForm is the write-time envelope: meta in struct order, data in template field order.
type orderedForm struct {
	Meta FormMeta      `json:"meta"`
	Data orderedObject `json:"data"`
}

// orderData reshapes a data map into an orderedObject keyed in template field order, recursing into loops.
// Keys not declared by the template are appended sorted so nothing is dropped on save.
func orderData(data map[string]any, fields []template.Field) orderedObject {
	o := orderedObject{vals: make(map[string]any, len(data))}
	used := make(map[string]bool, len(data))
	add := func(k string, v any) {
		if used[k] {
			return
		}
		used[k] = true
		o.keys = append(o.keys, k)
		o.vals[k] = v
	}

	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if f.Type == "loopstart" {
			end := loopEnd(fields, i+1, f.Key)
			inner := fields[i+1 : end]
			if raw, ok := data[f.Key]; ok {
				add(f.Key, orderLoopItems(raw, inner))
			}
			i = end
			continue
		}
		if f.Type == "loopstop" || f.Type == "looper" {
			continue
		}
		if v, ok := data[f.Key]; ok {
			add(f.Key, v)
		}
	}

	extras := make([]string, 0)
	for k := range data {
		if !used[k] {
			extras = append(extras, k)
		}
	}
	sort.Strings(extras)
	for _, k := range extras {
		add(k, data[k])
	}
	return o
}

// orderLoopItems orders each loop entry's inner field map; non-map/non-array values pass through unchanged.
func orderLoopItems(raw any, inner []template.Field) any {
	arr, ok := raw.([]any)
	if !ok {
		return raw
	}
	out := make([]any, len(arr))
	for i, item := range arr {
		if m, ok := item.(map[string]any); ok {
			out[i] = orderData(m, inner)
		} else {
			out[i] = item
		}
	}
	return out
}

// loopEnd returns the matching loopstop index, honouring nested pairs; falls back to the last field when unpaired.
func loopEnd(fields []template.Field, start int, loopKey string) int {
	depth := 0
	for i := start; i < len(fields); i++ {
		switch fields[i].Type {
		case "loopstart":
			depth++
		case "loopstop":
			if depth == 0 && fields[i].Key == loopKey {
				return i
			}
			depth--
		}
	}
	return len(fields) - 1
}
