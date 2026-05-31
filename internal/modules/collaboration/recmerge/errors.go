// Package recmerge implements structured 3-way merge for Formidable record files
// (storage/<template>/*.meta.json). Leaf package: imports nothing else here, no side effects.
//
// Source of truth: github.com/petervdpas/GiGot/internal/formidable. This is a hand-vendored copy
// (no build-time dependency on GiGot). Client and server must produce byte-identical merged output
// for the same input triple, so any merge-rule change must ship to BOTH repos in the same session;
// drift is a correctness bug. Full spec: gigot's docs/design/structured-sync-api.md §10.2-§10.3.
package recmerge

import "errors"

var ErrMalformedRecord = errors.New("recmerge: malformed record")

// FieldConflict is one entry in a RecordConflict; Scope is always "meta" today (data fields never conflict under the uniform rule).
type FieldConflict struct {
	Scope  string `json:"scope"`
	Key    string `json:"key"`
	Reason string `json:"reason,omitempty"`
}

// RecordConflict is the conflict body from gigot §10.6; today only immutable-meta violations on created/id/template.
type RecordConflict struct {
	Path           string          `json:"path"`
	CurrentVersion string          `json:"current_version,omitempty"`
	FieldConflicts []FieldConflict `json:"field_conflicts"`
}
