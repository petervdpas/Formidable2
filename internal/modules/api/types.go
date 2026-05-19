// Package api ports the original Formidable internalServer's
// /api/collections/* endpoints to Go. It's a peer surface to the
// wiki HTML server (internal/modules/wiki); both are mounted by the
// composition root onto the same loopback listener, but their read
// paths, content types, and response shapes are fully independent.
//
// All endpoints are read-only in the current slice. Writes (POST /
// PUT / PATCH / DELETE / batch) are tracked separately and would
// require a small write-side interface on top of the storage manager.
package api

// TemplateRow is one entry in the /api/collections directory listing.
// Mirrors the original Formidable shape so existing API consumers
// don't have to change anything.
type TemplateRow struct {
	ID   string `json:"id"`   // stem (e.g. "recepten")
	Name string `json:"name"` // display name from yaml `name`, falling back to stem
	Href string `json:"href"` // /api/collections/<id>
}

// CountResponse is the body of /api/collections/{template}/count.
type CountResponse struct {
	Template string `json:"template"`
	Total    int    `json:"total"`
}

// errorBody is the standard JSON error envelope. Kept tiny — the
// original Formidable used the same `{ "error": "<slug>" }` shape and
// existing clients (incl. the OpenAPI consumers) match against
// these slugs, not freeform messages.
type errorBody struct {
	Error string `json:"error"`
}

// itemResponse is the body of GET /api/collections/{tpl}/{id}. The
// shape matches the original Formidable internalServer's JSON exactly
// so existing API consumers don't need to change.
type itemResponse struct {
	Template string         `json:"template"`
	ID       string         `json:"id"`
	Filename string         `json:"filename"`
	Title    string         `json:"title"`
	Meta     map[string]any `json:"meta"`
	Data     map[string]any `json:"data"`
	Links    itemLinks      `json:"links"`
	Rev      itemRev        `json:"rev"`
}

type itemLinks struct {
	Self string `json:"self"`
	HTML string `json:"html"`
}

type itemRev struct {
	ETag string `json:"etag"`
}

// designResponse is the body of GET /api/collections/design/{tpl}.
// Mirrors the shape the original Formidable apiCollections.js returned
// — top-level template metadata plus the field list. Each field is a
// map[string]any built by JSON-roundtripping template.Field so type-
// specific properties (format, rows, run_mode, collection, map, ...)
// pass through automatically; only `options` is shaped explicitly
// into normalized {value, label} pairs because YAML may carry bare
// scalars there.
type designResponse struct {
	Name              string           `json:"name"`
	Filename          string           `json:"filename"`
	ItemField         string           `json:"item_field"`
	MarkdownTemplate  string           `json:"markdown_template"`
	SidebarExpression string           `json:"sidebar_expression"`
	EnableCollection  bool             `json:"enable_collection"`
	Fields            []map[string]any `json:"fields"`
	// Facets is the template's filter contract — same shape served by
	// the dedicated /facets endpoint. `omitempty` keeps the design
	// payload tidy for templates without facets (consumers shouldn't
	// have to handle an empty array when they wouldn't render anything).
	Facets []facetEntry `json:"facets,omitempty"`
}

// facetsResponse is the body of GET /api/collections/{tpl}/facets.
// Mirrors the filter contract API consumers need to pass
// ?facet.<key>=LABEL query params on the list / count endpoints.
type facetsResponse struct {
	Template string        `json:"template"`
	Facets   []facetEntry  `json:"facets"`
}

// facetEntry is one declared facet projected for the wire. Mirrors
// template.Facet but stays in this package so the api contract is
// independent of internal struct evolutions on the template side.
type facetEntry struct {
	Key     string              `json:"key"`
	Icon    string              `json:"icon"`
	Options []facetOptionEntry  `json:"options"`
}

type facetOptionEntry struct {
	Label string `json:"label"`
	Color string `json:"color"`
}

// designOption is the normalized shape of one entry in field.options.
// Both label and value are always strings — the original UI/API
// stringified them eagerly to dodge JSON-vs-YAML type mismatches.
type designOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
