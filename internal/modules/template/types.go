// Package template owns Formidable's schema-driven YAML templates at templates/<name>.yaml.
// Validation enforces loop pairing/nesting (max depth 2), a single tags field, and api-field shape rules.
package template

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// Template is the on-disk shape of a template YAML file.
// AuthorName/AuthorEmail sit at the YAML root (no meta block) and are auto-filled by SaveTemplate
// so PullWithStash can name who last touched the template without walking git log.
type Template struct {
	Name              string      `yaml:"name" json:"name"`
	Filename          string      `yaml:"filename" json:"filename"`
	AuthorName        string      `yaml:"author_name,omitempty" json:"author_name,omitempty"`
	AuthorEmail       string      `yaml:"author_email,omitempty" json:"author_email,omitempty"`
	ItemField         string      `yaml:"item_field,omitempty" json:"item_field"`
	// SortByItemField orders the storage record list by the item-field value
	// (its display title) instead of the default filename order.
	SortByItemField   bool        `yaml:"sort_by_item_field,omitempty" json:"sort_by_item_field"`
	GraphPrefixField  string      `yaml:"graph_prefix_field,omitempty" json:"graph_prefix_field"`
	// GraphColor tints this template's record nodes in the datacore graph (a CSS
	// color string). Empty leaves them at the default kind-based color.
	GraphColor        string      `yaml:"graph_color,omitempty" json:"graph_color"`
	MarkdownTemplate  string      `yaml:"markdown_template,omitempty" json:"markdown_template"`
	SidebarExpression string      `yaml:"sidebar_expression,omitempty" json:"sidebar_expression"`
	EnableCollection  bool        `yaml:"enable_collection,omitempty" json:"enable_collection"`
	// Presentation turns the collection's record list into an ordered slide deck:
	// the studio list sorts by the sequence field and gains drag-to-reorder.
	// Requires a sequence field (which in turn requires collection mode).
	Presentation      bool        `yaml:"presentation,omitempty" json:"presentation"`
	PDF               *PDFConfig  `yaml:"pdf,omitempty" json:"pdf,omitempty"`
	Facets            []Facet     `yaml:"facets,omitempty" json:"facets"`
	Statistics        []Statistic `yaml:"statistics,omitempty" json:"statistics"`
	Scalings          []Scaling   `yaml:"scalings,omitempty" json:"scalings"`
	Formulas          []Formula   `yaml:"formulas,omitempty" json:"formulas"`
	Fields            []Field     `yaml:"fields" json:"fields"`
	NeedsResave       bool        `yaml:"-" json:"needs_resave"`
}

// Formula is one author-defined computed field: a named per-record expression
// (expression-engine syntax, F["key"]) evaluated in the datacore loader, so it
// becomes an ordinary datacore field usable as a statistics dimension/measure.
// Type is the result coercion: "number" | "text" | "date" | "bool".
type Formula struct {
	Key        string `yaml:"key" json:"key"`
	Label      string `yaml:"label,omitempty" json:"label,omitempty"`
	Type       string `yaml:"type,omitempty" json:"type"`
	Expression string `yaml:"expression" json:"expression"`
}

// Statistic is one author-defined statistical object: exactly one of DSL / Composite / Scaling is set.
// See internal/modules/stat, design/statistics-dsl.md, design/statistics-composite.md.
type Statistic struct {
	Name      string         `yaml:"name" json:"name"`
	Label     string         `yaml:"label,omitempty" json:"label,omitempty"`
	DSL       string         `yaml:"dsl,omitempty" json:"dsl"`
	Composite *StatComposite `yaml:"composite,omitempty" json:"composite,omitempty"`
	Scaling   *StatScaling   `yaml:"scaling,omitempty" json:"scaling,omitempty"`
}

// StatComposite is the stored composite: a parent name plus per-branch child names. The engine
// checks that each child filters the parent's branch dimension to its branch value.
type StatComposite struct {
	Parent string              `yaml:"parent" json:"parent"`
	Edges  []StatCompositeEdge `yaml:"edges,omitempty" json:"edges"`
}

// StatCompositeEdge maps one parent branch value to the child object that drills it.
type StatCompositeEdge struct {
	Branch string `yaml:"branch" json:"branch"`
	Child  string `yaml:"child" json:"child"`
}

// StatScaling is the stored scaling: a per-form categorical source plus an option->factor map and a default.
// Source must be a facet or scalar dropdown/radio field (per-form), never a table column.
type StatScaling struct {
	Source  StatSource        `yaml:"source" json:"source"`
	Weights []StatWeightEntry `yaml:"weights,omitempty" json:"weights"`
	Default float64           `yaml:"default" json:"default"`
}

// Scaling is a reusable per-form weighting, named at the template level: a
// per-form categorical source (a facet, or a dropdown/radio field) plus an
// option->factor map and a default. It is exposed to the expression engine as
// S["name"] (the per-record factor) and to the Statistical Engine through the
// DSL scale "<name>" clause. Source must be per-form, never a table column.
type Scaling struct {
	Name    string            `yaml:"name" json:"name"`
	Label   string            `yaml:"label,omitempty" json:"label,omitempty"`
	Source  StatSource        `yaml:"source" json:"source"`
	Weights []StatWeightEntry `yaml:"weights,omitempty" json:"weights"`
	Default float64           `yaml:"default" json:"default"`
}

// StatSource is a serialised source reference (mirrors stat.SourceRef).
type StatSource struct {
	Kind   string `yaml:"kind" json:"kind"` // "field" | "facet"
	Key    string `yaml:"key" json:"key"`
	Column string `yaml:"column,omitempty" json:"column,omitempty"`
}

// StatWeightEntry maps one option value to its multiplier.
type StatWeightEntry struct {
	Label  string  `yaml:"label" json:"label"`
	Factor float64 `yaml:"factor" json:"factor"`
}

// UnmarshalYAML accepts both `facets:` and legacy `flag_definitions:`; legacy entries become one
// synthetic facet keyed "flag" so existing on-disk templates keep rendering without rewrites.
func (t *Template) UnmarshalYAML(node *yaml.Node) error {
	type tplAlias Template
	aux := struct {
		*tplAlias      `yaml:",inline"`
		LegacyFlagDefs []FacetOption `yaml:"flag_definitions"`
	}{tplAlias: (*tplAlias)(t)}
	if err := node.Decode(&aux); err != nil {
		return err
	}
	if len(t.Facets) == 0 && len(aux.LegacyFlagDefs) > 0 {
		t.Facets = []Facet{{
			Key:     "flag",
			Icon:    "fa-flag",
			Options: aux.LegacyFlagDefs,
		}}
	}
	return nil
}

// PDFConfig is the per-template PDF export defaults, feeding the manifest layer in pdf.Merge
// (precedence: document frontmatter > form meta > template manifest > global config).
// Style accepts the same values as picoloom's WithStyle (theme name, .css path, or raw CSS).
type PDFConfig struct {
	Style string          `yaml:"style,omitempty" json:"style,omitempty"`
	Cover *PDFCoverConfig `yaml:"cover,omitempty" json:"cover,omitempty"`
}

// PDFCoverConfig mirrors pdf.CoverFM; field tags match document-frontmatter casing for one vocabulary across layers.
type PDFCoverConfig struct {
	Enabled      *bool  `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Template     string `yaml:"template,omitempty" json:"template,omitempty"`
	TemplatePath string `yaml:"template_path,omitempty" json:"template_path,omitempty"`
	Title        string `yaml:"title,omitempty" json:"title,omitempty"`
	Subtitle     string `yaml:"subtitle,omitempty" json:"subtitle,omitempty"`
	Logo         string `yaml:"logo,omitempty" json:"logo,omitempty"`
	Author       string `yaml:"author,omitempty" json:"author,omitempty"`
	AuthorTitle  string `yaml:"authorTitle,omitempty" json:"authorTitle,omitempty"`
	Organization string `yaml:"organization,omitempty" json:"organization,omitempty"`
	Date         string `yaml:"date,omitempty" json:"date,omitempty"`
	Version      string `yaml:"version,omitempty" json:"version,omitempty"`
	ClientName   string `yaml:"clientName,omitempty" json:"clientName,omitempty"`
	ProjectName  string `yaml:"projectName,omitempty" json:"projectName,omitempty"`
	DocumentType string `yaml:"documentType,omitempty" json:"documentType,omitempty"`
	DocumentID   string `yaml:"documentID,omitempty" json:"documentID,omitempty"`
	Description  string `yaml:"description,omitempty" json:"description,omitempty"`
	Department   string `yaml:"department,omitempty" json:"department,omitempty"`
}

// Facet is one named meta-classification dimension: a stable Key (the FormMeta.Facets map key),
// an Icon, and mutually-exclusive Options. Templates may declare up to 16 facets, each up to 16 options.
type Facet struct {
	Key     string        `yaml:"key" json:"key"`
	Icon    string        `yaml:"icon" json:"icon"`
	Options []FacetOption `yaml:"options" json:"options"`
}

// FacetOption is one selectable label within a facet; Color names a token from the shared 16-token palette.
type FacetOption struct {
	Label string `yaml:"label" json:"label"`
	Color string `yaml:"color" json:"color"`
}

// Field describes one input in a template; type-specific properties sit alongside the common ones.
type Field struct {
	Key         string `yaml:"key" json:"key"`
	Type        string `yaml:"type" json:"type"`
	Label       string `yaml:"label,omitempty" json:"label"`
	Description string `yaml:"description,omitempty" json:"description"`
	// I18n is the optional base key for plugin field translation (resolves <plugin-ns>.<I18n>.<sub>); templates don't need it.
	I18n           string `yaml:"i18n,omitempty" json:"i18n,omitempty"`
	SummaryField   string `yaml:"summary_field,omitempty" json:"summary_field,omitempty"`
	ExpressionItem bool   `yaml:"expression_item,omitempty" json:"expression_item"`
	LevelScope     int    `yaml:"level_scope" json:"level_scope"`
	TwoColumn      bool   `yaml:"two_column,omitempty" json:"two_column"`
	Collapsible    *bool  `yaml:"collapsible,omitempty" json:"collapsible,omitempty"`
	Readonly       bool   `yaml:"readonly,omitempty" json:"readonly"`
	// UseInStatistics opts a field into the statistics index (default false keeps form_values lean).
	// For table fields it gates the field; StatisticsColumns then enumerates which columns get indexed.
	UseInStatistics   bool     `yaml:"use_in_statistics,omitempty" json:"use_in_statistics"`
	StatisticsColumns []string `yaml:"statistics_columns,omitempty" json:"statistics_columns,omitempty"`
	Default           any      `yaml:"default,omitempty" json:"default"`
	Options           []any    `yaml:"options,omitempty" json:"options"`
	PrimaryKey        bool     `yaml:"primary_key,omitempty" json:"primary_key,omitempty"`

	// textarea-specific
	Format string `yaml:"format,omitempty" json:"format,omitempty"`

	// api-specific. Map's column types are resolved live from the source template, never stored, to avoid stale-cache drift.
	Collection string     `yaml:"collection,omitempty" json:"collection,omitempty"`
	Map        []APIMap   `yaml:"map,omitempty" json:"map,omitempty"`
	Filter     *APIFilter `yaml:"filter,omitempty" json:"filter,omitempty"`

	// facet-specific. FacetKey binds a virtual field to a declared facet; value lives in meta.facets[FacetKey], not data.
	FacetKey string `yaml:"facet_key,omitempty" json:"facet_key,omitempty"`

	// formula-specific (virtual). The field carries no data slot of its own: it
	// writes the FormulaKey formula's output into the TargetKey data field's slot.
	// Trigger is "load" (on form open), "save" (on persist), or "live" (manual,
	// via the field's Compute button). The computed value persists like a typed entry.
	FormulaKey string `yaml:"formula_key,omitempty" json:"formula_key,omitempty"`
	TargetKey  string `yaml:"target_key,omitempty" json:"target_key,omitempty"`
	Trigger    string `yaml:"trigger,omitempty" json:"trigger,omitempty"`

	// Extra preserves unknown fields verbatim (e.g. plugin metadata).
	Extra map[string]any `yaml:",inline" json:"-"`
}

// APIMap is one column projected from the source template into the host form's api-field row.
// Type is intentionally absent: it is derived live from the source template to avoid drift.
type APIMap struct {
	Key   string `yaml:"key" json:"key"`
	Label string `yaml:"label,omitempty" json:"label,omitempty"`
}

// APIFilter is one optional predicate that narrows the api field's target list to
// records where FieldKey Op Value holds. Op is one of eq/ne/gt/ge/lt/le. The
// target field must be a facet or a use_in_statistics-indexed field so the value
// is queryable; the editor offers only eligible fields.
type APIFilter struct {
	FieldKey string `yaml:"field_key" json:"field_key"`
	Op       string `yaml:"op" json:"op"`
	Value    string `yaml:"value,omitempty" json:"value"`
}

// ValidationError is one issue found by Validate.
type ValidationError struct {
	Type    string         `json:"type"`
	Message string         `json:"message,omitempty"`
	Key     string         `json:"key,omitempty"`
	Keys    []string       `json:"keys,omitempty"`
	Field   *Field         `json:"field,omitempty"`
	Index   int            `json:"index,omitempty"`
	Detail  map[string]any `json:"detail,omitempty"`
}

// ValidationFailedError wraps a slice of ValidationError so callers can errors.As to the structured set.
type ValidationFailedError struct {
	Errors []ValidationError
}

func (e *ValidationFailedError) Error() string {
	if e == nil || len(e.Errors) == 0 {
		return "template: validation failed"
	}
	parts := make([]string, 0, len(e.Errors))
	for _, ve := range e.Errors {
		if ve.Message != "" {
			parts = append(parts, ve.Message)
		} else {
			parts = append(parts, ve.Type)
		}
	}
	return "template: validation failed: " + joinSemicolon(parts)
}

func joinSemicolon(parts []string) string {
	return strings.Join(parts, "; ")
}

// Descriptor is the {name, yaml, storageLocation} bundle returned by GetDescriptor.
type Descriptor struct {
	Name            string    `json:"name"`
	YAML            *Template `json:"yaml"`
	StorageLocation string    `json:"storageLocation"`
}

// ItemField is one row in the "possible item fields" picker.
type ItemField struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// LoadManyResult is one slot in LoadMany's response; Template is nil on failure (Error carries why).
type LoadManyResult struct {
	Filename string    `json:"filename"`
	Template *Template `json:"template,omitempty"`
	Error    string    `json:"error,omitempty"`
}

// Known field types live in field_registry.go's fieldDescriptors.
// Use IsKnownFieldType(t) to check membership.
