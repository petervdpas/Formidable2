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

// AuditEntry records who did something and when. Used for both
// FormMeta.Created and FormMeta.Updated. Symmetric to git's
// author/committer split: Created is set once and preserved across
// every subsequent save; Updated is re-stamped on every save with the
// current profile's identity. On read, legacy flat `author_name` +
// `author_email` + flat `created`/`updated` strings are migrated into
// the AuditEntry pair; on write only the nested shape is emitted.
type AuditEntry struct {
	At    string `json:"at"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// FormMeta carries identity + audit fields. Tags are deduped+sorted.
// FlagState references a Template.FlagDefinitions label (e.g. "FLASH")
// when set; empty means no state is chosen. Independent of Flagged —
// legacy `flagged: true` forms keep their bool with FlagState empty,
// and the UI renders them as a generic uncolored flag.
type FormMeta struct {
	ID        string     `json:"id"`
	Template  string     `json:"template"`
	Created   AuditEntry `json:"created"`
	Updated   AuditEntry `json:"updated"`
	Flagged   bool       `json:"flagged"`
	FlagState string     `json:"flag_state"`
	Tags      []string   `json:"tags"`
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
// fields are optional and default to "fill from raw or generate".
//
// Created and Updated override anything in raw meta when their `At`
// field is non-empty — used by SaveForm to lock the creator across
// edits (opts.Created = prev.Meta.Created) and to stamp the current
// profile (opts.Updated = {At: now, Name: profile, Email: profile}).
type SanitizeOptions struct {
	ID           string
	TemplateName string
	Created      AuditEntry
	Updated      AuditEntry
	Flagged      *bool
	FlagState    string
	Tags         []string
}

// AuthorProvider returns the current actor's name + email. Wired by
// the composition root from config.Manager so storage.SaveForm can
// stamp Updated.* (and Created.* on first save) with the active
// profile. Tests may swap a fixed-value provider.
type AuthorProvider func() (name, email string)
