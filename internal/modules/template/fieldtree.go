package template

import "strings"

// FieldUnit is the editor tree view over the flat []Field source of truth: it folds a matched
// loopstart/loopstop pair into one indivisible unit so a row can't be reordered across the loop boundary.
// Orphan markers are emitted as plain field units so validation can still flag them. Kind discriminates.
type FieldUnit struct {
	Kind  string `json:"kind"`
	Field *Field `json:"field,omitempty"`
	Start *Field `json:"start,omitempty"`
	Stop  *Field `json:"stop,omitempty"`
	// No omitempty: an empty loop must round-trip as "items": [] so the drag-into-loop binding mutates a persistent array.
	Items []FieldUnit `json:"items"`
}

const (
	UnitKindField = "field"
	UnitKindLoop  = "loop"
)

func loopType(f Field) string {
	return strings.ToLower(f.Type)
}

// BuildFieldTree pairs each loopstart with the nearest matching loopstop, mirroring loopPairingErrors
// so the tree shape and the validator agree on what's a well-formed pair.
func BuildFieldTree(fields []Field) []FieldUnit {
	state := &treeBuilder{src: fields}
	return state.consumeUntil("", false)
}

type treeBuilder struct {
	src []Field
	pos int
}

// consumeUntil collects units until a matching loopstop (when inLoop) or EOF; caller checks b.pos for the stop.
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
				// No matching stop in scope: back up and emit the start as a plain row, then re-walk.
				b.pos = savedPos
				start := start
				out = append(out, FieldUnit{Kind: UnitKindField, Field: &start})
			}
		case "loopstop":
			if inLoop && f.Key == stopKey {
				// Don't advance: the caller consumes the matched stop and produces the loop unit.
				return out
			}
			// Orphan stop: emit as plain row.
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

// SummaryFieldOption is a candidate for a loopstart's summary_field
// binding: one direct child field of the loop, by key + display label.
type SummaryFieldOption struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// SummaryFieldCandidates returns the loop's direct child fields as summary_field options; loop markers
// and deeper-nested fields are excluded (nested-loop fields live in their own per-iteration record).
func SummaryFieldCandidates(fields []Field, loopKey string) []SummaryFieldOption {
	tree := BuildFieldTree(fields)
	var find func(us []FieldUnit) []FieldUnit
	find = func(us []FieldUnit) []FieldUnit {
		for _, u := range us {
			if u.Kind != UnitKindLoop {
				continue
			}
			if u.Start != nil && u.Start.Key == loopKey {
				return u.Items
			}
			if items := find(u.Items); items != nil {
				return items
			}
		}
		return nil
	}
	out := []SummaryFieldOption{}
	for _, u := range find(tree) {
		if u.Kind != UnitKindField || u.Field == nil {
			continue
		}
		label := u.Field.Label
		if label == "" {
			label = u.Field.Key
		}
		out = append(out, SummaryFieldOption{Key: u.Field.Key, Label: label})
	}
	return out
}

// FlattenFieldTree is the inverse of BuildFieldTree, guaranteeing each loop's start/stop bracket its items.
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
