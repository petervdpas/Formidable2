// Package template owns Formidable's templates: schema-driven YAML files
// at <context>/templates/<name>.yaml that declare a form's fields.
//
// Mirrors `controls/templateManager.js` semantics. Loop
// pairing/nesting validation (max depth 2), single tags-field rule,
// api-field shape rules, collection-mode requires a guid field.
package template

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// Template is the on-disk shape of a template YAML file.
//
// AuthorName / AuthorEmail mirror the per-record author identity that
// storage/<tpl>/<n>.meta.json carries in its meta envelope. They sit at
// the YAML root (templates have no separate meta block) and are
// auto-filled from config.author_name / config.author_email by
// SaveTemplate when the caller leaves them empty. Purpose: PullWithStash
// can name "who last touched this template" without walking git log,
// matching how it identifies record overrides.
type Template struct {
	Name              string     `yaml:"name" json:"name"`
	Filename          string     `yaml:"filename" json:"filename"`
	AuthorName        string     `yaml:"author_name,omitempty" json:"author_name,omitempty"`
	AuthorEmail       string     `yaml:"author_email,omitempty" json:"author_email,omitempty"`
	ItemField         string     `yaml:"item_field,omitempty" json:"item_field"`
	MarkdownTemplate  string     `yaml:"markdown_template,omitempty" json:"markdown_template"`
	SidebarExpression string     `yaml:"sidebar_expression,omitempty" json:"sidebar_expression"`
	EnableCollection  bool       `yaml:"enable_collection,omitempty" json:"enable_collection"`
	PDF               *PDFConfig `yaml:"pdf,omitempty" json:"pdf,omitempty"`
	Facets            []Facet    `yaml:"facets,omitempty" json:"facets"`
	Fields            []Field    `yaml:"fields" json:"fields"`
	NeedsResave       bool       `yaml:"-" json:"needs_resave"`
}

// UnmarshalYAML accepts both the new `facets:` shape and the legacy
// `flag_definitions:` shape. Legacy entries become one synthetic facet
// keyed "flag" with icon "fa-flag" and the list as its options, so
// existing on-disk templates keep rendering without rewrites.
func (t *Template) UnmarshalYAML(node *yaml.Node) error {
	type tplAlias Template
	aux := struct {
		*tplAlias        `yaml:",inline"`
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

// PDFConfig is the per-template PDF export defaults: a theme/style
// selector plus a cover-page block. Both are optional and feed the
// `manifest` layer in pdf.Merge (precedence: document frontmatter >
// form meta > template manifest > global config).
//
// Style accepts the same values as picoloom's WithStyle option:
// a built-in theme name ("default", "technical", …), a filesystem
// path to a custom .css, or raw CSS content.
type PDFConfig struct {
	Style string         `yaml:"style,omitempty" json:"style,omitempty"`
	Cover *PDFCoverConfig `yaml:"cover,omitempty" json:"cover,omitempty"`
}

// PDFCoverConfig mirrors pdf.CoverFM's shape so the template manifest
// can carry default cover values that document frontmatter can
// override. Field tags match the document-frontmatter casing so
// authors get one consistent vocabulary across both layers.
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

// Facet is one named dimension of meta classification on a template:
// a stable Key (used as the FormMeta.Facets map key), an Icon (a
// FontAwesome key rendered next to the chosen pill), and a list of
// mutually-exclusive Options to pick from on each record.
//
// Each facet on a record carries a required `set` bool plus an
// optional `selected` label — see storage.FacetState. Filter chips
// in the storage view auto-derive from a template's facets but only
// render when at least one record actually has `set: true` for the
// facet's key.
//
// Templates may declare up to 16 facets, each with up to 16 options.
type Facet struct {
	Key     string        `yaml:"key" json:"key"`
	Icon    string        `yaml:"icon" json:"icon"`
	Options []FacetOption `yaml:"options" json:"options"`
}

// FacetOption is one selectable label within a facet. Label is the
// user-visible identifier (also used as the value stored in
// FormMeta.Facets[key].Selected); Color names a token from the
// shared 16-token palette. Colors may repeat across labels.
type FacetOption struct {
	Label string `yaml:"label" json:"label"`
	Color string `yaml:"color" json:"color"`
}

// Field describes one input in a template. Type-specific properties
// (run_mode, options, collection, etc.) sit alongside the common ones —
// downstream consumers ignore irrelevant fields.
type Field struct {
	// Common
	Key            string  `yaml:"key" json:"key"`
	Type           string  `yaml:"type" json:"type"`
	Label          string  `yaml:"label,omitempty" json:"label"`
	Description    string  `yaml:"description,omitempty" json:"description"`
	// I18n is the optional base key for plugin field translation.
	// When set, the renderer resolves `<plugin-namespace>.<I18n>.<sub>`
	// for sub-keys `label`, `description`, `placeholder` via the
	// active locale, falling back to the literal Label/Description
	// on miss. Templates don't need this (user-authored labels are
	// the literal source); it's foremost a plugin-form.json signal.
	I18n         string `yaml:"i18n,omitempty" json:"i18n,omitempty"`
	SummaryField   string  `yaml:"summary_field,omitempty" json:"summary_field,omitempty"`
	ExpressionItem bool    `yaml:"expression_item,omitempty" json:"expression_item"`
	LevelScope     int     `yaml:"level_scope" json:"level_scope"`
	TwoColumn      bool    `yaml:"two_column,omitempty" json:"two_column"`
	Collapsible    *bool   `yaml:"collapsible,omitempty" json:"collapsible,omitempty"`
	Readonly       bool    `yaml:"readonly,omitempty" json:"readonly"`
	Default        any     `yaml:"default,omitempty" json:"default"`
	Options        []any   `yaml:"options,omitempty" json:"options"`
	PrimaryKey     bool    `yaml:"primary_key,omitempty" json:"primary_key,omitempty"`

	// textarea-specific
	Format string `yaml:"format,omitempty" json:"format,omitempty"`

	// api-specific. Collection is the source template (filename or
	// name). Map is the column list — each entry projects one
	// level-0 source field into the host form's row at fetch time.
	// Type is not stored; it is resolved live from the source
	// template (`source.Fields[Map[i].Key].Type`) so a source-side
	// rename or type change can't drift a stale cache.
	Collection string   `yaml:"collection,omitempty" json:"collection,omitempty"`
	Map        []APIMap `yaml:"map,omitempty" json:"map,omitempty"`

	// Extra fields preserved verbatim (e.g. plugin-specific metadata).
	Extra map[string]any `yaml:",inline" json:"-"`
}

// APIMap is one column projected from the source template into the
// host form's api-field row at fetch time.
//
//   - Key: source-template field key (must reference a level-0 field).
//     The same key is used as the column name in the host form's
//     stored row. Required.
//   - Label: optional display header for that column. When empty, the
//     editor / wiki falls back to the source field's Label.
//
// Type is intentionally absent — it is derived live from the source
// template (`source.Fields[Key].Type`). Storing it here would invite
// drift if the source template's field type changes.
type APIMap struct {
	Key   string `yaml:"key" json:"key"`
	Label string `yaml:"label,omitempty" json:"label,omitempty"`
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

// ValidationFailedError wraps a slice of ValidationError. SaveTemplate
// returns this when validation finds issues so programmatic callers can
// errors.As to the structured set; the Wails layer just relays Error()
// to the frontend, which has its own pre-validation gate.
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

// Descriptor is the {name, yaml, storageLocation} bundle returned by
// GetDescriptor. Mirrors templateManager.getTemplateDescriptor.
type Descriptor struct {
	Name            string    `json:"name"`
	YAML            *Template `json:"yaml"`
	StorageLocation string    `json:"storageLocation"`
}

// ItemField is one row in the "possible item fields" picker (top-level
// non-loop text fields, used to choose a collection's primary identifier).
type ItemField struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// LoadManyResult is one slot in LoadMany's response. Template is nil
// when the file was missing or unparseable — Error carries the
// per-row failure message. Filename is always stamped so callers can
// pair the result back to its input slot even when Template is nil.
type LoadManyResult struct {
	Filename string    `json:"filename"`
	Template *Template `json:"template,omitempty"`
	Error    string    `json:"error,omitempty"`
}

// Known field types live in field_registry.go's fieldDescriptors.
// Use IsKnownFieldType(t) to check membership.
