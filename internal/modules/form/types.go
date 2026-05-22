// Package form is Formidable's form-view orchestrator. It sits above
// the template + storage modules and exposes the single object Vue
// needs to render and round-trip one (template, datafile) pair.
//
// Mirrors the original `modules/formRenderer.js` + `modules/formActions.js`
// pipeline, with all of the type-specific render rules pushed down into
// Vue components and only the glue (default injection, loop pairing,
// eventual API live-fetch) left in Go.
package form

import (
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// FormView is the Vue-facing payload returned by BuildView and SaveValues.
// Vue iterates Template.Fields in order and consults LoopGroups to know
// when a slice of fields belongs to a loopstart/loopstop block. Values
// is keyed by field.key; loop keys hold []map[string]any (one entry
// per item), with inner field values keyed inside.
type FormView struct {
	Template   *template.Template `json:"template"`
	Values     map[string]any     `json:"values"`
	Meta       storage.FormMeta   `json:"meta"`
	LoopGroups []LoopGroup        `json:"loop_groups"`
	Datafile   string             `json:"datafile"`
	Saved      bool               `json:"saved"`
}

// LoopGroup is one precomputed loopstart/loopstop pair. Vue uses
// StartIndex/StopIndex to slice into Template.Fields and DefaultCollapsed
// for the initial UI state. SummaryFieldKey is the field key whose
// value is shown in the loop-item header when the item is collapsed
// (mirrors loopstart.summary_field).
type LoopGroup struct {
	Key              string `json:"key"`
	StartIndex       int    `json:"start_index"`
	StopIndex        int    `json:"stop_index"`
	Depth            int    `json:"depth"`
	SummaryFieldKey  string `json:"summary_field_key"`
	DefaultCollapsed bool   `json:"default_collapsed"`
}

// SavePayload is what Vue sends to SaveValues. Datafile may be empty
// for never-persisted forms - the caller (UI) is expected to gather
// a filename from the user (mirrors the original New-entry dialog).
type SavePayload struct {
	Datafile string           `json:"datafile"`
	Values   map[string]any   `json:"values"`
	Meta     storage.FormMeta `json:"meta"`
}
