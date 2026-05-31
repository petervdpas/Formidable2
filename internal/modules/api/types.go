// Package api serves the /api/collections/* JSON endpoints, a peer surface to the wiki HTML server
// on the same loopback listener with fully independent read paths, content types, and shapes.
package api

// TemplateRow is one entry in the /api/collections directory listing.
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

// errorBody is the standard JSON error envelope; clients match against the slug, not freeform text.
type errorBody struct {
	Error string `json:"error"`
}

// itemResponse is the body of GET /api/collections/{tpl}/{id}.
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

// designResponse is the body of GET /api/collections/design/{tpl}: template metadata plus fields.
type designResponse struct {
	Name              string           `json:"name"`
	Filename          string           `json:"filename"`
	ItemField         string           `json:"item_field"`
	MarkdownTemplate  string           `json:"markdown_template"`
	SidebarExpression string           `json:"sidebar_expression"`
	EnableCollection  bool             `json:"enable_collection"`
	Fields            []map[string]any `json:"fields"`
	// omitempty so facet-less templates don't ship an empty array.
	Facets []facetEntry `json:"facets,omitempty"`
}

// facetsResponse is the body of GET /api/collections/{tpl}/facets.
type facetsResponse struct {
	Template string       `json:"template"`
	Facets   []facetEntry `json:"facets"`
}

// facetEntry projects one declared facet for the wire, decoupled from template.Facet evolution.
type facetEntry struct {
	Key     string             `json:"key"`
	Icon    string             `json:"icon"`
	Options []facetOptionEntry `json:"options"`
}

type facetOptionEntry struct {
	Label string `json:"label"`
	Color string `json:"color"`
}

// designOption is one normalized field.options entry; both fields are stringified to dodge JSON-vs-YAML type mismatches.
type designOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
