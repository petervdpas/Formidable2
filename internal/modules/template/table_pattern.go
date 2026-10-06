package template

import (
	"maps"
	"strings"
)

// A table's Format names its pattern: a fixed, ordered column set whose keys and
// types are locked (labels stay editable), with free extra columns after them.
// Regular (stored as "") is today's free-form table.
const (
	TablePatternRegular     = "regular"
	TablePatternDataLineage = "data-lineage"
)

// TablePatternDescriptor is one entry of the pattern picker; Shape is nil for regular.
// Diagram names the projection a diagram field draws it with ("" = none).
type TablePatternDescriptor struct {
	ID       string             `json:"id"`
	LabelKey string             `json:"label_key"`
	Shape    *FixedOptionsShape `json:"shape,omitempty"`
	Diagram  string             `json:"diagram,omitempty"`
}

type patternColumn struct {
	key, captionKey, label, typ string
}

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
			{"source_entity", "workspace.templates.table_pattern.col.source_entity", "Source entity", "string"},
			{"source_attribute", "workspace.templates.table_pattern.col.source_attribute", "Source attribute", "string"},
			{"target_entity", "workspace.templates.table_pattern.col.target_entity", "Target entity", "string"},
			{"target_attribute", "workspace.templates.table_pattern.col.target_attribute", "Target attribute", "string"},
			{"example_value", "workspace.templates.table_pattern.col.example_value", "Example value", "string"},
			{"transaction_rule", "workspace.templates.table_pattern.col.transaction_rule", "Transaction rule", "reference"},
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
					Defaults: map[string]any{"value": c.key, "type": c.typ, "label": c.label},
				})
			}
		}
		out = append(out, d)
	}
	return out
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
		label := c.label
		if l := strings.TrimSpace(labels[c.key]); l != "" {
			label = l
		}
		col := map[string]any{"value": c.key, "label": label, "type": c.typ}
		if m, ok := existing[c.key]; ok {
			maps.Copy(col, m)
			col["type"] = c.typ
			if l, _ := m["label"].(string); l == "" {
				col["label"] = label
			}
		}
		out = append(out, col)
	}
	for _, k := range order {
		if !inPattern[k] {
			out = append(out, existing[k])
		}
	}
	return out
}
