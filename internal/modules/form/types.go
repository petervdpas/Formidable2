// Package form orchestrates the form view. It sits above the template +
// storage modules and exposes the single object Vue needs to render and
// round-trip one (template, datafile) pair. Type-specific render rules
// live in Vue; only the glue (default injection, loop pairing) is in Go.
package form

import (
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// FormView is the Vue-facing payload from BuildView and SaveValues.
// Values is keyed by field.key; loop keys hold []map[string]any, one
// entry per item with inner field values keyed inside.
type FormView struct {
	Template   *template.Template `json:"template"`
	Values     map[string]any     `json:"values"`
	Meta       storage.FormMeta   `json:"meta"`
	LoopGroups []LoopGroup        `json:"loop_groups"`
	Datafile   string             `json:"datafile"`
	Saved      bool               `json:"saved"`
	// NeedsSave marks a loaded record whose on-disk data was not canonical and
	// got healed in the view (empty loop iterations pruned). The frontend surfaces
	// it as a dirty form so the user can persist the cleanup with one save.
	NeedsSave bool `json:"needs_save"`
}

// LoopGroup is one precomputed loopstart/loopstop pair. SummaryFieldKey
// is the field key whose value shows in the loop-item header when
// collapsed (mirrors loopstart.summary_field).
type LoopGroup struct {
	Key              string `json:"key"`
	StartIndex       int    `json:"start_index"`
	StopIndex        int    `json:"stop_index"`
	Depth            int    `json:"depth"`
	SummaryFieldKey  string `json:"summary_field_key"`
	DefaultCollapsed bool   `json:"default_collapsed"`
}

// SavePayload is what Vue sends to SaveValues. Datafile may be empty for
// never-persisted forms; the UI gathers a filename from the user first.
type SavePayload struct {
	Datafile string           `json:"datafile"`
	Values   map[string]any   `json:"values"`
	Meta     storage.FormMeta `json:"meta"`
}
