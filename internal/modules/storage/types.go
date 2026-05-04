// Package storage owns Formidable's per-template form storage:
// `<context>/storage/<template-name>/<form>.meta.json` files plus the
// `images/` subfolder for image fields. Form sanitization is template-
// driven (defaults filled per field type, tags collected, etc).
//
// Mirrors `controls/formManager.js` and `schemas/meta.schema.js` semantics.
package storage

// Form is the on-disk shape of a `.meta.json` file.
type Form struct {
	Meta FormMeta       `json:"meta"`
	Data map[string]any `json:"data"`
}

// FormMeta carries identity + audit fields. Tags are deduped+sorted.
type FormMeta struct {
	ID          string   `json:"id"`
	AuthorName  string   `json:"author_name"`
	AuthorEmail string   `json:"author_email"`
	Template    string   `json:"template"`
	Created     string   `json:"created"`
	Updated     string   `json:"updated"`
	Flagged     bool     `json:"flagged"`
	Tags        []string `json:"tags"`
}

// FormSummary is one row in ExtendedListForms output. Title falls back
// to filename when item_field is unset or its value is empty.
type FormSummary struct {
	Filename        string         `json:"filename"`
	Meta            FormMeta       `json:"meta"`
	Title           string         `json:"title"`
	ExpressionItems map[string]any `json:"expressionItems"`
}

// SaveResult mirrors the JS shape used across SFR-backed modules.
type SaveResult struct {
	Success bool   `json:"success"`
	Path    string `json:"path,omitempty"`
	Error   string `json:"error,omitempty"`
}

// SanitizeOptions adjusts how Sanitize normalises the meta block. All
// fields are optional and default to "fill from raw or generate". This
// mirrors the option bag accepted by `schemas/meta.schema.js.sanitize`.
type SanitizeOptions struct {
	ID           string
	TemplateName string
	AuthorName   string
	AuthorEmail  string
	Created      string
	Updated      string
	Flagged      *bool
	Tags         []string
}
