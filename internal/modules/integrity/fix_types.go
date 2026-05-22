package integrity

// FixStrategy names a concrete repair action. The set is closed -
// every kind maps to one or more strategies, and unknown strategies
// fail Fix-time rather than at the per-issue level.
type FixStrategy string

const (
	// FixStrip - remove the offending key from the data map. Used for
	// extra_field. Lossless: the data was orphaned from the template.
	FixStrip FixStrategy = "strip"

	// FixFillDefault - write the per-type default for a missing field
	// (matches storage.Sanitize's defaultForType). Used for missing_field.
	FixFillDefault FixStrategy = "fill_default"

	// FixCoerce - attempt to convert a wrong-typed value into the
	// declared type. Used for type_mismatch and bad_date_format. Items
	// where coercion fails are reported as "skipped" in the result and
	// the form is left untouched.
	FixCoerce FixStrategy = "coerce"

	// FixClear - wipe the value back to the per-type default. Same
	// effect as FixFillDefault but applied to a populated-but-wrong
	// value rather than an absent one. Used for type_mismatch /
	// bad_date_format when the user prefers "clear and re-enter" over
	// "attempt to coerce".
	FixClear FixStrategy = "clear"

	// FixMintUUID - generate a fresh UUID for meta.id. Used for
	// meta_missing on guid templates.
	FixMintUUID FixStrategy = "mint_uuid"

	// FixRestamp - overwrite a bad timestamp with time.Now().UTC().
	// Used for meta_bad_format on meta.created and meta.updated. For
	// meta.flag_state the same strategy clears the stale label
	// (different concrete change, same intent: "make it valid").
	FixRestamp FixStrategy = "restamp"

	// FixSkip - leave the issue alone. Used as the sentinel for kinds
	// where no in-app repair exists (unreadable: needs the user to
	// edit the file). Selecting Skip from the UI means "don't change
	// anything for issues of this kind".
	FixSkip FixStrategy = "skip"
)

// FixPlanItem says how to repair every issue of a given kind in this
// run. There is at most one item per kind - the UI summarises by kind
// so the user picks one strategy for all forms in that bucket.
type FixPlanItem struct {
	Kind     IssueKind   `json:"kind"`
	Strategy FixStrategy `json:"strategy"`
}

// FixPlan is the bundle the frontend submits to Fix.
type FixPlan struct {
	Items []FixPlanItem `json:"items"`
}

// FixOutcome is the per-form summary of what changed.
type FixOutcome struct {
	Filename string `json:"filename"`
	// Applied is the count of issues actually repaired in this form.
	Applied int `json:"applied"`
	// Skipped is the count of issues left alone because Skip was
	// chosen for their kind, the strategy couldn't apply (e.g. coerce
	// failed), or no plan item targeted them.
	Skipped int `json:"skipped"`
	// Saved indicates whether the form file was rewritten. False when
	// Applied was 0 - no work means no write.
	Saved bool `json:"saved"`
	// Notes carries human-readable per-form annotations: failed
	// coercions, "form skipped because unreadable", etc.
	Notes []string `json:"notes,omitempty"`
}

// FixResult is the aggregate response to a Fix call. ScannedAfter is
// the issue count from a fresh analyze pass run after writes, so the
// frontend can show "X repaired, Y still remain" without a second
// round-trip.
type FixResult struct {
	FormsTouched int          `json:"forms_touched"`
	FormsSaved   int          `json:"forms_saved"`
	Applied      int          `json:"applied"`
	Skipped      int          `json:"skipped"`
	ScannedAfter int          `json:"scanned_after"`
	Outcomes     []FixOutcome `json:"outcomes,omitempty"`
}
