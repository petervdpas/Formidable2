// Package dataprovider is the read-only facade the wiki HTTP server and
// REST API use to answer page requests, composing the SQLite index with
// the render module and exposing HTTP-friendly types. Writes never go
// through here; every method takes a context.Context so per-template
// access control can be layered on later.
package dataprovider

// TemplateSummary is the public projection of an index template row.
// Filename is the YAML name ("basic.yaml"); Stem is the slug ("basic").
type TemplateSummary struct {
	Stem                string `json:"stem"`
	Filename            string `json:"filename"`
	Name                string `json:"name"`
	ItemField           string `json:"itemField,omitempty"`
	GuidField           string `json:"guidField,omitempty"`
	TagsField           string `json:"tagsField,omitempty"`
	HasMarkdownTemplate bool   `json:"hasMarkdownTemplate"`
	EnableCollection    bool   `json:"enableCollection"`
	// Presentation marks a slide-deck template: a collection whose records are
	// slides. Data surfaces (api/query/datacore/stat) exclude it via
	// IsCollectionExposed; it stays editable everywhere else.
	Presentation bool `json:"presentation"`
}

// FormSummary is the per-form data the wiki list pages need: identity,
// title, audit fields, tags. No body content; that is RenderForm's job.
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

// RenderedPage is the render pipeline output plus the frontmatter title
// lifted out, so HTTP handlers don't re-parse the markdown.
type RenderedPage struct {
	Template string `json:"template"`
	Filename string `json:"filename"`
	Title    string `json:"title"`
	Markdown string `json:"markdown"`
	HTML     string `json:"html"`
}

// ListOpts is the read-side equivalent of index.QueryOpts, kept separate
// so the public API isn't tied to the index's internal options shape.
type ListOpts struct {
	Limit   int
	Offset  int
	OrderBy string   // see index.orderBySQL for accepted values
	Tags    []string // AND semantics
}

// CollectionItem is one row in a collection listing: identity, title,
// tags, and ready-to-use links to the JSON resource and HTML page.
type CollectionItem struct {
	Template string   `json:"template"`         // stem (e.g. "recepten")
	ID       string   `json:"id"`               // GUID
	Filename string   `json:"filename"`
	Title    string   `json:"title"`
	Tags     []string `json:"tags,omitempty"`
	HrefSelf string   `json:"hrefSelf"`         // /api/collections/<stem>/<guid>
	HrefHTML string   `json:"hrefHtml"`         // /template/<stem>/form/<filename>
}

// CollectionPage is the /api/collections/<template> response: enabled
// flag (false when the template doesn't opt in), total, and the items.
type CollectionPage struct {
	Enabled  bool             `json:"collectionEnabled"`
	Template string           `json:"template,omitempty"`  // stem
	Total    int              `json:"total"`
	Limit    int              `json:"limit"`
	Offset   int              `json:"offset"`
	Items    []CollectionItem `json:"items"`
}

// CollectionListOpts shapes a collection listing. Q is a case-insensitive
// substring filter on title+tags; Tags and Facets add AND filtering.
// A Facets entry matches only when set==true and selected==value.
type CollectionListOpts struct {
	Limit  int
	Offset int
	Q      string
	Tags   []string
	Facets map[string]string
	Filter *CollectionFieldFilter
}

// CollectionFieldFilter is one data-field predicate applied via the value index
// (form_values): Op is eq/ne/gt/ge/lt/le. For a date field a compare value must
// be the epoch seconds the index stores. Facet-field filtering goes through
// Facets, not here; the api handler routes facet vs data using the target
// template, so ListCollection stays type-agnostic.
type CollectionFieldFilter struct {
	FieldKey string
	Op       string
	Value    string
}
