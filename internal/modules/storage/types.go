// Package storage owns Formidable's per-template form storage:
// `<context>/storage/<template-name>/<form>.meta.json` files plus the
// `images/` subfolder for image fields. Form sanitization is template-
// driven (defaults filled per field type, tags collected, etc).
//
// Mirrors `controls/formManager.js` and `schemas/meta.schema.js` semantics.
package storage

import "encoding/json"

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
// Facets is keyed by Template.Facets[i].Key; each entry's Set is
// required (mirrors the legacy `flagged` bool) and Selected may be
// empty (mirrors the legacy `flag_state` string - `set: true` without
// a chosen option renders as the facet's uncolored icon).
type FormMeta struct {
	ID       string                `json:"id"`
	Template string                `json:"template"`
	Created  AuditEntry            `json:"created"`
	Updated  AuditEntry            `json:"updated"`
	Facets   map[string]FacetState `json:"facets,omitempty"`
	Tags     []string              `json:"tags"`
}

// FacetState is the per-record state for one facet key. Set is the
// required "is this facet stamped on this form" bool; Selected is the
// optional option label chosen from the template's facet options.
type FacetState struct {
	Set      bool   `json:"set"`
	Selected string `json:"selected,omitempty"`
}

// UnmarshalJSON accepts both the new `facets` map shape and the legacy
// `flagged` + `flag_state` pair. When the legacy pair is present and
// the new shape is not, a single synthetic facet keyed "flag" is
// materialised so on-disk records keep loading unchanged.
func (m *FormMeta) UnmarshalJSON(data []byte) error {
	type metaAlias FormMeta
	aux := struct {
		*metaAlias
		LegacyFlagged   *bool   `json:"flagged,omitempty"`
		LegacyFlagState *string `json:"flag_state,omitempty"`
	}{metaAlias: (*metaAlias)(m)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if m.Facets == nil && (aux.LegacyFlagged != nil || aux.LegacyFlagState != nil) {
		flagged := aux.LegacyFlagged != nil && *aux.LegacyFlagged
		selected := ""
		if aux.LegacyFlagState != nil {
			selected = *aux.LegacyFlagState
		}
		if flagged || selected != "" {
			m.Facets = map[string]FacetState{
				"flag": {Set: flagged, Selected: selected},
			}
		}
	}
	return nil
}

// FormSummary is one row in ExtendedListForms output. Title falls back
// to filename when item_field is unset or its value is empty.
type FormSummary struct {
	Filename        string         `json:"filename"`
	Meta            FormMeta       `json:"meta"`
	Title           string         `json:"title"`
	ExpressionItems map[string]any `json:"expressionItems"`
}

// MigrateResult reports the outcome of MigrateTemplateMeta - a per-
// template bulk operation that rewrites legacy meta shape (flat
// author_name/email + string created/updated) into the AuditEntry
// pair. Migrated files keep their original authorship intact (no
// Updated.by restamp); already-new files are skipped without touching
// the file (mtime preserved). Per-file errors land in Errors so the
// caller can surface them without aborting the whole pass.
type MigrateResult struct {
	Total    int      `json:"total"`
	Migrated int      `json:"migrated"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors,omitempty"`
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
// field is non-empty - used by SaveForm to lock the creator across
// edits (opts.Created = prev.Meta.Created) and to stamp the current
// profile (opts.Updated = {At: now, Name: profile, Email: profile}).
//
// Facets, when non-nil, replaces whatever the raw meta supplied. A nil
// map lets the raw payload's facets (or legacy `flagged`/`flag_state`
// pair) survive untouched.
type SanitizeOptions struct {
	ID           string
	TemplateName string
	Created      AuditEntry
	Updated      AuditEntry
	Facets       map[string]FacetState
	Tags         []string
}

// AuthorProvider returns the current actor's name + email. Wired by
// the composition root from config.Manager so storage.SaveForm can
// stamp Updated.* (and Created.* on first save) with the active
// profile. Tests may swap a fixed-value provider.
type AuthorProvider func() (name, email string)
