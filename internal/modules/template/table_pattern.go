package template

import (
	"maps"
	"strings"
)

// A table's Format names its pattern: a fixed, ordered column set whose keys and
// types are locked (labels stay editable), with free extra columns after them.
// Regular (stored as "") is today's free-form table.
const (
	TablePatternRegular         = "regular"
	TablePatternDataLineage     = "data-lineage"
	TablePatternEntityRelations = "entity-relations"
)

// TablePatternDescriptor is one entry of the pattern picker; Shape is nil for regular.
// Diagram names the projection a diagram field draws it with ("" = none).
type TablePatternDescriptor struct {
	ID       string             `json:"id"`
	LabelKey string             `json:"label_key"`
	Shape    *FixedOptionsShape `json:"shape,omitempty"`
	Diagram  string             `json:"diagram,omitempty"`
}

// patternColumn: a dropdown column also fixes its choice values (choices);
// their labels stay editable, like the column label.
type patternColumn struct {
	key, captionKey, label, typ string
	choices                     []patternChoice
}

type patternChoice struct{ value, label string }

type tablePattern struct {
	id, labelKey string
	diagram      string
	columns      []patternColumn
	// roles binds diagram column roles to pattern column keys.
	roles map[string]string
}

var tablePatterns = []tablePattern{
	{id: TablePatternRegular, labelKey: "workspace.templates.table_pattern.regular"},
	{
		id:       TablePatternDataLineage,
		labelKey: "workspace.templates.table_pattern.data_lineage",
		diagram:  DiagramProjectionLineage,
		columns: []patternColumn{
			{key: "source_entity", captionKey: "workspace.templates.table_pattern.col.source_entity", label: "Source entity", typ: "string"},
			{key: "source_attribute", captionKey: "workspace.templates.table_pattern.col.source_attribute", label: "Source attribute", typ: "string"},
			{key: "target_entity", captionKey: "workspace.templates.table_pattern.col.target_entity", label: "Target entity", typ: "string"},
			{key: "target_attribute", captionKey: "workspace.templates.table_pattern.col.target_attribute", label: "Target attribute", typ: "string"},
			{key: "example_value", captionKey: "workspace.templates.table_pattern.col.example_value", label: "Example value", typ: "string"},
			{key: "transaction_rule", captionKey: "workspace.templates.table_pattern.col.transaction_rule", label: "Transaction rule", typ: "reference"},
		},
		roles: map[string]string{
			DiagramFromEntity: "source_entity",
			DiagramFromAttr:   "source_attribute",
			DiagramToEntity:   "target_entity",
			DiagramToAttr:     "target_attribute",
			DiagramLabel:      "transaction_rule",
			DiagramExample:    "example_value",
		},
	},
	{
		// Relations between two entities on the same side of a mapping (joins on
		// the source side, structure on the target side). Drawn on its own, or
		// fed into a lineage diagram as a second table.
		id:       TablePatternEntityRelations,
		labelKey: "workspace.templates.table_pattern.entity_relations",
		diagram:  DiagramProjectionRelations,
		columns: []patternColumn{
			{key: "side", captionKey: "workspace.templates.table_pattern.col.side", label: "Side", typ: "dropdown",
				choices: []patternChoice{{RelationSideSource, "Source"}, {RelationSideTarget, "Target"}}},
			{key: "from_entity", captionKey: "workspace.templates.table_pattern.col.from_entity", label: "From entity", typ: "string"},
			{key: "from_attribute", captionKey: "workspace.templates.table_pattern.col.from_attribute", label: "From attribute", typ: "string"},
			{key: "to_entity", captionKey: "workspace.templates.table_pattern.col.to_entity", label: "To entity", typ: "string"},
			{key: "to_attribute", captionKey: "workspace.templates.table_pattern.col.to_attribute", label: "To attribute", typ: "string"},
			{key: "label", captionKey: "workspace.templates.table_pattern.col.relation_label", label: "Label", typ: "string"},
		},
		roles: map[string]string{
			DiagramSide:       "side",
			DiagramFromEntity: "from_entity",
			DiagramFromAttr:   "from_attribute",
			DiagramToEntity:   "to_entity",
			DiagramToAttr:     "to_attribute",
			DiagramLabel:      "label",
		},
	},
}

func findTablePattern(id string) *tablePattern {
	for i := range tablePatterns {
		if tablePatterns[i].id == id {
			return &tablePatterns[i]
		}
	}
	return nil
}

// TablePatterns lists the table patterns, regular first. Fresh values per call.
func TablePatterns() []TablePatternDescriptor {
	out := make([]TablePatternDescriptor, 0, len(tablePatterns))
	for _, p := range tablePatterns {
		d := TablePatternDescriptor{ID: p.id, LabelKey: p.labelKey, Diagram: p.diagram}
		if len(p.columns) > 0 {
			d.Shape = &FixedOptionsShape{LockedColumns: []string{"value", "type"}, AllowExtraRows: true}
			for _, c := range p.columns {
				d.Shape.Rows = append(d.Shape.Rows, FixedOptionRow{
					LabelKey: c.captionKey,
					Defaults: patternColumnOption(c, nil, nil),
				})
			}
		}
		out = append(out, d)
	}
	return out
}

// patternColumnOption builds a pattern column's option map: key and type are the
// pattern's; the label is the existing one, else labels[key], else the default.
// A dropdown's choices keep the pattern's values in order, each labelled by the
// existing choice label, else labels["key.value"], else the default.
func patternColumnOption(c patternColumn, existing map[string]any, labels map[string]string) map[string]any {
	pick := func(key, def string) string {
		if l := strings.TrimSpace(labels[key]); l != "" {
			return l
		}
		return def
	}
	col := map[string]any{}
	maps.Copy(col, existing)
	col["value"] = c.key
	col["type"] = c.typ
	if l, _ := col["label"].(string); strings.TrimSpace(l) == "" {
		col["label"] = pick(c.key, c.label)
	}
	if len(c.choices) > 0 {
		have := map[string]string{}
		raw, _ := col["choices"].(string)
		for _, piece := range strings.Split(raw, "|") {
			v, l, found := strings.Cut(strings.TrimSpace(piece), ":")
			if found && strings.TrimSpace(l) != "" {
				have[strings.TrimSpace(v)] = strings.TrimSpace(l)
			}
		}
		parts := make([]string, len(c.choices))
		for i, ch := range c.choices {
			label := have[ch.value]
			if label == "" {
				label = pick(c.key+"."+ch.value, ch.label)
			}
			parts[i] = ch.value + ":" + label
		}
		col["choices"] = strings.Join(parts, "|")
	}
	return col
}

// canonicalTablePattern maps a stored Format to a pattern id; "" means regular.
func canonicalTablePattern(format string) string {
	id := strings.ToLower(strings.TrimSpace(format))
	if p := findTablePattern(id); p != nil && len(p.columns) > 0 {
		return id
	}
	return ""
}

// ApplyTablePattern returns options with the pattern's columns first, in
// pattern order (an existing column keeps its label, its type is forced),
// then every other column in its original order. Nothing is dropped except
// malformed entries and duplicate keys. Regular or unknown: options unchanged.
func ApplyTablePattern(options []any, pattern string) []any {
	return ApplyTablePatternLabels(options, pattern, nil)
}

// ApplyTablePatternLabels is ApplyTablePattern with labels (column key ->
// label, e.g. the translated captions) for columns it adds; blank entries fall
// back to the built-in default. Existing labels are never replaced.
func ApplyTablePatternLabels(options []any, pattern string, labels map[string]string) []any {
	p := findTablePattern(canonicalTablePattern(pattern))
	if p == nil {
		return options
	}
	existing := map[string]map[string]any{}
	var order []string
	for _, o := range options {
		m, ok := o.(map[string]any)
		if !ok {
			continue
		}
		k, _ := m["value"].(string)
		if k == "" {
			continue
		}
		if _, dup := existing[k]; dup {
			continue
		}
		existing[k] = m
		order = append(order, k)
	}
	out := make([]any, 0, len(order)+len(p.columns))
	inPattern := map[string]bool{}
	for _, c := range p.columns {
		inPattern[c.key] = true
		out = append(out, patternColumnOption(c, existing[c.key], labels))
	}
	for _, k := range order {
		if !inPattern[k] {
			out = append(out, existing[k])
		}
	}
	return out
}
