// Package dataprovider is the read-only facade the wiki HTTP server
// (and the future REST API) uses to answer page requests. It composes
// the SQLite-backed index (fast metadata lookups) with the render
// module (markdown + HTML), and exposes HTTP-friendly types so the
// transport layer can JSON-encode results without further mapping.
//
// Writes never go through dataprovider — the in-app event hooks and
// the post-sync RescanAll path own those. Keeping this side read-only
// makes it easy to layer per-template access control later: every
// method already takes a context.Context, so a future principal goes
// in a context value and gets checked here before any data flows out.
package dataprovider

// TemplateSummary is the public projection of an index template row.
// `Filename` is the YAML name (e.g. "basic.yaml"); `Stem` is the
// extension-less form (e.g. "basic") which matches the URL slug used
// by the original Formidable wiki ("/template/basic").
type TemplateSummary struct {
	Stem                string `json:"stem"`
	Filename            string `json:"filename"`
	Name                string `json:"name"`
	ItemField           string `json:"itemField,omitempty"`
	GuidField           string `json:"guidField,omitempty"`
	TagsField           string `json:"tagsField,omitempty"`
	HasMarkdownTemplate bool   `json:"hasMarkdownTemplate"`
	EnableCollection    bool   `json:"enableCollection"`
}

// FormSummary mirrors the per-form data the wiki sidebar/list pages
// need: identity (filename + optional GUID), display title, audit
// fields, and the inverted tags. Body content is intentionally NOT
// here — fetching that is RenderForm's job.
type FormSummary struct {
	Template        string   `json:"template"`        // "basic.yaml"
	Filename        string   `json:"filename"`        // "test.meta.json"
	ID              string   `json:"id,omitempty"`    // GUID from the template's guid_field
	Title           string   `json:"title"`
	FmTitle         string   `json:"fmTitle,omitempty"`
	Author          string   `json:"author,omitempty"`
	Created         string   `json:"created,omitempty"`
	Updated         string   `json:"updated,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	ExpressionItems string   `json:"expressionItems,omitempty"`
}

// RenderedPage carries the full output of the render pipeline plus
// the title lifted out of the markdown's frontmatter (so HTTP
// handlers don't have to re-parse it).
type RenderedPage struct {
	Template string `json:"template"`
	Filename string `json:"filename"`
	Title    string `json:"title"`
	Markdown string `json:"markdown"`
	HTML     string `json:"html"`
}

// ListOpts is the read-side equivalent of index.QueryOpts. We keep a
// separate type so dataprovider's public API isn't tied to the
// index's internal options shape and so HTTP handlers map query
// strings directly into this.
type ListOpts struct {
	Limit   int
	Offset  int
	OrderBy string   // see index.orderBySQL for accepted values
	Tags    []string // AND semantics
}

// CollectionItem is one row in a collection listing. Mirrors the old
// internal-server `listCollection` output: identity (guid as id),
// filename, display title, tags, and ready-to-use HTTP links to the
// JSON resource and the rendered HTML page.
type CollectionItem struct {
	Template string   `json:"template"`         // stem (e.g. "recepten")
	ID       string   `json:"id"`               // GUID
	Filename string   `json:"filename"`
	Title    string   `json:"title"`
	Tags     []string `json:"tags,omitempty"`
	HrefSelf string   `json:"hrefSelf"`         // /api/collections/<stem>/<guid>
	HrefHTML string   `json:"hrefHtml"`         // /template/<stem>/form/<filename>
}

// CollectionPage is what the /api/collections/<template> endpoint
// returns: enabled flag (false when the template doesn't opt in),
// total before pagination, and the page's items.
type CollectionPage struct {
	Enabled  bool             `json:"collectionEnabled"`
	Template string           `json:"template,omitempty"`  // stem
	Total    int              `json:"total"`
	Limit    int              `json:"limit"`
	Offset   int              `json:"offset"`
	Items    []CollectionItem `json:"items"`
}

// CollectionListOpts shapes a collection listing. `Q` is a
// case-insensitive substring filter applied to title + tags
// (matching the wiki's old behaviour). Tags add AND filtering.
// Include selects how much per-item data the response carries —
// summary (default) keeps it small; full would include full data
// (deferred to a later v).
type CollectionListOpts struct {
	Limit  int
	Offset int
	Q      string
	Tags   []string
}
