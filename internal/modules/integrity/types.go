// Package integrity audits a template's stored forms against the
// template's current field declarations. It detects four classes of
// drift: data-level issues (missing/extra/type-mismatched fields), date
// format errors, meta-block shape issues, and unreadable forms.
//
// Phase 1 is analyze-only - AnalyzeTemplate produces a Report that the
// frontend surfaces in the "Cleanup Storage" dialog. Phase 2 (repair)
// will add a per-issue Fix method that mutates the form file. Both
// phases share the Issue vocabulary defined below.
package integrity

import "time"

// IssueKind enumerates structural problems that can be detected in a
// stored form. The string values are stable wire identifiers used by
// the frontend to group/filter issues.
type IssueKind string

const (
	// IssueMissingField - the template declares Key K but the form's
	// data has no entry for K. Sanitize would have filled a default,
	// so this only surfaces on forms last saved before K was added.
	IssueMissingField IssueKind = "missing_field"

	// IssueExtraField - the form's data has Key K but the current
	// template has no field with that key. Usually a stale entry from
	// a field that was renamed or deleted.
	IssueExtraField IssueKind = "extra_field"

	// IssueTypeMismatch - the value present for Key K is not assignable
	// to the field's declared type (e.g. a string in a boolean field).
	// Detail describes the actual vs expected types.
	IssueTypeMismatch IssueKind = "type_mismatch"

	// IssueBadDateFormat - value is a string but doesn't parse as
	// "YYYY-MM-DD". Distinct from IssueTypeMismatch so the UI can
	// offer a date-specific quick-fix. For table date columns the
	// analyzer only emits this for values that match the column's
	// inferred dominant format; the resolved ISO value rides along in
	// Suggest so the fixer converts deterministically (no re-guessing).
	IssueBadDateFormat IssueKind = "bad_date_format"

	// IssueDateAnomaly - a date value inside a table date column that
	// doesn't fit the column's inferred dominant format (different
	// separator, contradicts the day/month order, unparseable, or the
	// column had no decisive evidence so the format is undecidable).
	// There's no safe automatic conversion: the doctor surfaces it for
	// the user to fix by hand. UI offers Clear / Skip, not Coerce.
	IssueDateAnomaly IssueKind = "date_anomaly"

	// IssueMetaMissing - a required meta key is empty.
	IssueMetaMissing IssueKind = "meta_missing"

	// IssueMetaBadFormat - meta.created / meta.updated isn't a
	// parseable RFC3339 timestamp.
	IssueMetaBadFormat IssueKind = "meta_bad_format"

	// IssueGuidUnsynced - the form declares a guid field whose data value
	// doesn't match meta.id (typically blank, since meta.id holds the
	// canonical guid and the data field was never mirrored). Surfaces so
	// downstream consumers that read the data block (CSV export, the API)
	// see the id. Suggest carries meta.id so the fix writes it verbatim.
	IssueGuidUnsynced IssueKind = "guid_unsynced"

	// IssueUnreadable - the form file couldn't be loaded or parsed.
	// Stops further analysis of that form; emitted as the single issue.
	IssueUnreadable IssueKind = "unreadable"
)

// Issue is one problem in one form. Path identifies the location inside
// data (top-level "key" or nested "loopKey[idx].field"); for meta-block
// issues Path is the meta key ("meta.id", "meta.created", …).
type Issue struct {
	Kind   IssueKind `json:"kind"`
	Path   string    `json:"path,omitempty"`
	Detail string    `json:"detail,omitempty"`
	// Value is the offending value as a literal string, surfaced in the
	// report so the user can see exactly what needs fixing (especially
	// for date anomalies they must resolve by hand). Empty when there's
	// no single meaningful value (e.g. a missing field).
	Value string `json:"value,omitempty"`
	// Suggest is an optional resolved value the fixer should write
	// instead of re-deriving one. Set for table date cells whose column
	// format was inferred: it carries the conformant ISO date so Coerce
	// is deterministic. Empty for issues the fixer resolves on its own.
	Suggest string `json:"suggest,omitempty"`
}

// FormReport groups every issue found in one form. Filename is the
// .meta.json basename (e.g. "x.meta.json") - the same identifier the
// storage module uses.
type FormReport struct {
	Filename string  `json:"filename"`
	Issues   []Issue `json:"issues"`
}

// Report is the result of AnalyzeTemplate. Only forms with at least one
// issue appear in Forms; IssueCount is the total across all of them.
type Report struct {
	Template   string       `json:"template"`
	FormCount  int          `json:"form_count"`
	IssueCount int          `json:"issue_count"`
	ScannedAt  time.Time    `json:"scanned_at"`
	Forms      []FormReport `json:"forms,omitempty"`
}
