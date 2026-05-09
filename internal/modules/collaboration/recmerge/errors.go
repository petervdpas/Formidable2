// Package recmerge implements structured 3-way merge for Formidable
// record files (storage/<template>/*.meta.json). It is a leaf package
// inside Formidable2: it imports nothing else from this project and
// has no side effects.
//
// Source of truth: github.com/petervdpas/GiGot/internal/formidable
// (public repo). This is a vendored copy — Formidable2 does NOT take a
// build-time dependency on GiGot; the file is duplicated by hand and
// kept byte-for-byte aligned with gigot's. When the merge rule is
// revised, ship the same change to BOTH repositories in the same
// session — drift between the two copies is a correctness bug
// (Formidable as the client and gigot as the server must produce
// byte-identical merged output for the same triple of inputs).
//
// The merge rule is uniform across all field types — every data field
// is treated as an atomic value and last-writer-wins (by meta.updated)
// resolves any both-sides-changed disagreement. See gigot's
// docs/design/structured-sync-api.md §10.2–§10.3 for the full spec.
package recmerge

import "errors"

var ErrMalformedRecord = errors.New("recmerge: malformed record")

// FieldConflict is one entry in a RecordConflict. Scope is always
// "meta" today — data fields never produce conflicts under the uniform
// rule. The field is kept for forward-compat with future strictness
// modes.
type FieldConflict struct {
	Scope  string `json:"scope"`
	Key    string `json:"key"`
	Reason string `json:"reason,omitempty"`
}

// RecordConflict is the conflict body shape inherited from gigot's
// §10.6. Today it only appears for immutable-meta violations on
// created / id / template.
type RecordConflict struct {
	Path           string          `json:"path"`
	CurrentVersion string          `json:"current_version,omitempty"`
	FieldConflicts []FieldConflict `json:"field_conflicts"`
}
