package index

// TemplateRow is one row in the index's `templates` table. The
// scan/reconcile pipeline produces these from on-disk YAML; the wiki
// HTTP server reads them back through dataprovider.
type TemplateRow struct {
	Filename            string // "basic.yaml"
	Name                string
	ItemField           string
	GuidField           string
	TagsField           string
	HasMarkdownTemplate bool
	EnableCollection    bool
	Mtime               int64
	Size                int64
}

// FormRow is one row in the index's `forms` table plus the inverted
// tags it owns. Tags are kept on the row (not a separate slice
// elsewhere) so the reconciler can sync them in lock-step with the
// upsert.
type FormRow struct {
	Template        string // "basic.yaml"
	Filename        string // "test.meta.json"
	ID              string // GUID from the template's guid_field, may be empty
	Title           string
	FmTitle         string
	Author          string
	Created         string
	Updated         string
	ExpressionItems string // JSON blob; opaque to the index
	Tags            []string
	Mtime           int64
	Size            int64
}

// ImageRow is one row in the index's `images` table. We don't track
// dimensions or size — just presence — because the wiki only needs to
// know "does this image exist for this template?" when it rewrites
// formidable:// links and image src attributes.
type ImageRow struct {
	Template string
	Filename string
	Mtime    int64
	Size     int64
}

// FormRef and ImageRef are compound keys for delete operations. They
// exist as named structs (rather than passing two strings) so the
// ReconcileBatch shape stays self-explanatory at call sites.
type FormRef struct {
	Template string
	Filename string
}

type ImageRef struct {
	Template string
	Filename string
}

// ReconcileBatch is a single transactional unit of work for the index.
// All upserts/deletes are applied inside one SQLite transaction; on
// error nothing is persisted and meta.rev is not bumped. An empty
// batch is a no-op (does not bump rev) so the reconciler can be
// called speculatively.
type ReconcileBatch struct {
	UpsertTemplates []TemplateRow
	DeleteTemplates []string

	UpsertForms []FormRow
	DeleteForms []FormRef

	UpsertImages []ImageRow
	DeleteImages []ImageRef
}

// QueryOpts shapes the result of ListForms (and its tag-filter cousin).
// Zero value = sensible defaults: no limit, no offset, sort by updated
// DESC, no tag filter.
type QueryOpts struct {
	// Limit caps the number of rows returned. 0 = no limit.
	Limit int
	// Offset skips the first N rows after sorting. 0 = no skip.
	Offset int
	// OrderBy is one of: "updated_desc" (default), "updated_asc",
	// "title_asc", "title_desc", "filename_asc", "filename_desc".
	// Empty string = "updated_desc". Unknown values fall back to the
	// default rather than failing — the wiki shouldn't 500 because of
	// a typo in a query string.
	OrderBy string
	// Tags filters to forms that own EVERY listed tag (AND semantics).
	// Empty/nil = no filter. Tags are matched case-sensitively against
	// the indexed values; normalize at the writer side, not the reader.
	Tags []string
}
