package integrity

// FixStrategy names a concrete repair action; the set is closed and unknown strategies fail at Fix-time.
type FixStrategy string

const (
	// FixStrip removes an orphaned data key (extra_field).
	FixStrip FixStrategy = "strip"

	// FixFillDefault writes the per-type default for a missing field.
	FixFillDefault FixStrategy = "fill_default"

	// FixCoerce converts a wrong-typed value to the declared type; failures are reported skipped.
	FixCoerce FixStrategy = "coerce"

	// FixClear wipes a populated-but-wrong value back to the per-type default.
	FixClear FixStrategy = "clear"

	// FixMintUUID generates a fresh meta.id (meta_missing on guid templates).
	FixMintUUID FixStrategy = "mint_uuid"

	// FixSyncGuid writes meta.id (canonical) into the guid data field.
	FixSyncGuid FixStrategy = "sync_guid"

	// FixRestamp overwrites a bad timestamp with now, or clears a stale facet label (same intent: make it valid).
	FixRestamp FixStrategy = "restamp"

	// FixSeedFacet writes the facet field's default onto a form whose disk is missing that facet (facet_unseeded).
	FixSeedFacet FixStrategy = "seed_facet"

	// FixSkip leaves the issue alone; the sentinel for kinds with no in-app repair (unreadable).
	FixSkip FixStrategy = "skip"
)

// FixPlanItem maps one issue kind to one strategy (at most one item per kind).
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
	Filename string   `json:"filename"`
	Applied  int      `json:"applied"`
	Skipped  int      `json:"skipped"`
	Saved    bool     `json:"saved"`
	Notes    []string `json:"notes,omitempty"`
}

// FixResult is the aggregate Fix response; ScannedAfter is a fresh analyze count so the UI can show "Y remain".
type FixResult struct {
	FormsTouched int          `json:"forms_touched"`
	FormsSaved   int          `json:"forms_saved"`
	Applied      int          `json:"applied"`
	Skipped      int          `json:"skipped"`
	ScannedAfter int          `json:"scanned_after"`
	Outcomes     []FixOutcome `json:"outcomes,omitempty"`
}
