// Package storage owns per-template form storage: storage/<template>/<form>.meta.json files plus
// the images/ subfolder. Sanitization is template-driven (per-type defaults, tags collected, etc).
package storage

import "encoding/json"

// Form is the on-disk shape of a `.meta.json` file.
type Form struct {
	Meta FormMeta       `json:"meta"`
	Data map[string]any `json:"data"`
}

// AuditEntry records who and when, for FormMeta.Created (locked at first save) and Updated (re-stamped each save).
// On read, legacy flat author/timestamp fields are migrated in; on write only the nested shape is emitted.
type AuditEntry struct {
	At    string `json:"at"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// FormMeta carries identity + audit fields; Tags are deduped+sorted, Facets is keyed by Template.Facets[i].Key.
type FormMeta struct {
	ID       string                `json:"id"`
	Template string                `json:"template"`
	Created  AuditEntry            `json:"created"`
	Updated  AuditEntry            `json:"updated"`
	Facets   map[string]FacetState `json:"facets,omitempty"`
	Tags     []string              `json:"tags"`
}

// FacetState is the per-record state for one facet: Set (stamped?) plus an optional Selected option label.
type FacetState struct {
	Set      bool   `json:"set"`
	Selected string `json:"selected,omitempty"`
}

// UnmarshalJSON accepts the new `facets` map and the legacy `flagged`+`flag_state` pair, materialising
// a synthetic "flag" facet from the legacy pair so on-disk records keep loading unchanged.
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

// FormSummary is one ExtendedListForms row; Title falls back to filename when item_field is unset/empty.
type FormSummary struct {
	Filename        string         `json:"filename"`
	Meta            FormMeta       `json:"meta"`
	Title           string         `json:"title"`
	ExpressionItems map[string]any `json:"expressionItems"`
}

// MigrateResult reports the outcome of MigrateTemplateMeta; migrated files keep their original authorship,
// already-new files are skipped untouched, and per-file errors land in Errors without aborting the pass.
type MigrateResult struct {
	Total    int      `json:"total"`
	Migrated int      `json:"migrated"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors,omitempty"`
}

// SaveResult is the success/path/error shape used across SFR-backed modules.
type SaveResult struct {
	Success bool   `json:"success"`
	Path    string `json:"path,omitempty"`
	Error   string `json:"error,omitempty"`
}

// SanitizeOptions adjusts how Sanitize normalises the meta block; all fields are optional.
// Created/Updated override raw meta when their At is set; non-nil Facets replaces raw meta's facets.
type SanitizeOptions struct {
	ID           string
	TemplateName string
	Created      AuditEntry
	Updated      AuditEntry
	Facets       map[string]FacetState
	Tags         []string
}

// AuthorProvider returns the current actor's name + email for stamping Updated (and Created on first save).
type AuthorProvider func() (name, email string)
