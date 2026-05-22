package template

import "strings"

// FieldUnit is the runtime tree shape the template editor renders.
// It folds a matched loopstart/loopstop pair (and everything between
// them) into a single indivisible unit so the UI cannot reorder a row
// across the loop boundary and create an orphan marker.
//
// The flat []Field shape on disk is still the source of truth; this
// type is a view over it. BuildFieldTree produces the view,
// FlattenFieldTree returns to the flat form. Orphan loopstart /
// loopstop rows (no matching partner) are emitted as plain field
// units so backend validation can still flag them - silently
// dropping data would be worse than rendering a broken pair.
//
// The struct is one shape with a Kind discriminator + nullable
// payload fields so the Wails-generated TypeScript stays simple.
type FieldUnit struct {
	Kind  string      `json:"kind"`
	Field *Field      `json:"field,omitempty"`
	Start *Field      `json:"start,omitempty"`
	Stop  *Field      `json:"stop,omitempty"`
	Items []FieldUnit `json:"items,omitempty"`
}

const (
	UnitKindField = "field"
	UnitKindLoop  = "loop"
)

func loopType(f Field) string {
	return strings.ToLower(f.Type)
}

// BuildFieldTree walks the flat fields once and pairs each loopstart
// with the nearest matching (by key) loopstop. Mirrors the pairing
// stack in validate.go's loopPairingErrors so the tree shape and the
// validator agree on what counts as a well-formed pair.
func BuildFieldTree(fields []Field) []FieldUnit {
	state := &treeBuilder{src: fields}
	return state.consumeUntil("", false)
}

type treeBuilder struct {
	src []Field
	pos int
}

// consumeUntil collects units until it sees a loopstop whose key
// matches the open `stopKey` (when inLoop is true), or until EOF.
// Returns the items collected at this depth; the caller checks
// builder.pos to know whether a matching stop was found.
func (b *treeBuilder) consumeUntil(stopKey string, inLoop bool) []FieldUnit {
	out := []FieldUnit{}
	for b.pos < len(b.src) {
		f := b.src[b.pos]
		t := loopType(f)
		switch t {
		case "loopstart":
			start := f
			b.pos++
			savedPos := b.pos
			inner := b.consumeUntil(start.Key, true)
			if b.pos < len(b.src) && loopType(b.src[b.pos]) == "loopstop" && b.src[b.pos].Key == start.Key {
				stop := b.src[b.pos]
				b.pos++
				start := start
				stop2 := stop
				out = append(out, FieldUnit{
					Kind:  UnitKindLoop,
					Start: &start,
					Stop:  &stop2,
					Items: inner,
				})
			} else {
				// No matching stop within this scope - back up and
				// emit the start as a plain row, then re-walk what we
				// consumed at the same level.
				b.pos = savedPos
				start := start
				out = append(out, FieldUnit{Kind: UnitKindField, Field: &start})
			}
		case "loopstop":
			if inLoop && f.Key == stopKey {
				// Don't advance - the caller handles consuming the
				// matched stop and producing the loop unit.
				return out
			}
			// Orphan stop - emit as plain row.
			ff := f
			out = append(out, FieldUnit{Kind: UnitKindField, Field: &ff})
			b.pos++
		default:
			ff := f
			out = append(out, FieldUnit{Kind: UnitKindField, Field: &ff})
			b.pos++
		}
	}
	return out
}

// FlattenFieldTree is the inverse of BuildFieldTree. By construction
// it guarantees that every loop's start sits immediately before its
// items and its stop immediately after - the bracket invariant that
// makes drag-into-loop corruption impossible at this layer.
func FlattenFieldTree(units []FieldUnit) []Field {
	out := []Field{}
	var walk func(us []FieldUnit)
	walk = func(us []FieldUnit) {
		for _, u := range us {
			switch u.Kind {
			case UnitKindLoop:
				if u.Start != nil {
					out = append(out, *u.Start)
				}
				walk(u.Items)
				if u.Stop != nil {
					out = append(out, *u.Stop)
				}
			default:
				if u.Field != nil {
					out = append(out, *u.Field)
				}
			}
		}
	}
	walk(units)
	return out
}
