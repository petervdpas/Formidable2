// Package integrity audits a template's stored forms against its current field declarations,
// detecting data drift, date-format errors, meta-block issues, and unreadable forms. AnalyzeTemplate
// reports; FixTemplate repairs. Both share the Issue vocabulary defined below.
package integrity

import "time"

// IssueKind enumerates structural problems in a stored form; the string values are stable wire identifiers.
type IssueKind string

const (
	// IssueMissingField: template declares a key with no data entry (form predates the field).
	IssueMissingField IssueKind = "missing_field"

	// IssueExtraField: data has a key the template no longer declares (renamed/deleted field).
	IssueExtraField IssueKind = "extra_field"

	// IssueTypeMismatch: the value isn't assignable to the field's declared type.
	IssueTypeMismatch IssueKind = "type_mismatch"

	// IssueBadDateFormat: a non-ISO date. For table date columns only conforming values get this, with the
	// resolved ISO in Suggest so the fixer converts deterministically (no re-guessing).
	IssueBadDateFormat IssueKind = "bad_date_format"

	// IssueDateAnomaly: a table date cell that doesn't fit the column's inferred format. No safe auto-conversion:
	// surfaced for manual fix (Clear/Skip, not Coerce).
	IssueDateAnomaly IssueKind = "date_anomaly"

	// IssueMetaMissing: a required meta key is empty.
	IssueMetaMissing IssueKind = "meta_missing"

	// IssueMetaBadFormat: meta.created/updated isn't a parseable RFC3339 timestamp.
	IssueMetaBadFormat IssueKind = "meta_bad_format"

	// IssueGuidUnsynced: the data guid field disagrees with meta.id. Suggest carries meta.id so the fix is verbatim.
	IssueGuidUnsynced IssueKind = "guid_unsynced"

	// IssueUnreadable: the form file couldn't be loaded or parsed; emitted as the single issue.
	IssueUnreadable IssueKind = "unreadable"
)

// Issue is one problem in one form; Path is a data location ("key" / "loopKey[idx].field") or a meta key.
type Issue struct {
	Kind   IssueKind `json:"kind"`
	Path   string    `json:"path,omitempty"`
	Detail string    `json:"detail,omitempty"`
	// Value is the offending value as a literal string, for the user to see (e.g. hand-fixed date anomalies).
	Value string `json:"value,omitempty"`
	// Suggest is a resolved value the fixer writes verbatim (e.g. the conformant ISO for an inferred date column).
	Suggest string `json:"suggest,omitempty"`
}

// FormReport groups every issue in one form; Filename is the .meta.json basename.
type FormReport struct {
	Filename string  `json:"filename"`
	Issues   []Issue `json:"issues"`
}

// Report is the result of AnalyzeTemplate; only forms with issues appear in Forms.
type Report struct {
	Template   string       `json:"template"`
	FormCount  int          `json:"form_count"`
	IssueCount int          `json:"issue_count"`
	ScannedAt  time.Time    `json:"scanned_at"`
	Forms      []FormReport `json:"forms,omitempty"`
}
