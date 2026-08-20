// Package connection is Formidable's dynamic OpenAPI interpreter. A Connection
// is a named, app-level remote service: a base URL, an uploaded Swagger/OpenAPI
// document, and a set of Resources that bind spec operations to the two verbs a
// reference field needs (list candidates, fetch one by id).
//
// Nothing is code-generated. The spec is parsed into a Catalog of Operations at
// load time and invoked generically, so a new service is an upload, not a build.
//
// Connections live under AppRoot, outside the synced context folder, because
// they are machine and deployment scoped. Secrets never live in the definition:
// Auth names the shape, and the value is resolved from the OS keychain.
package connection

// Auth kinds.
const (
	AuthNone   = "none"
	AuthAPIKey = "apikey"
	AuthBearer = "bearer"
	AuthBasic  = "basic"
)

// Pagination styles. PageLink covers every service that hands back a complete
// next-page URL instead of a token: OData's @odata.nextLink, JSON:API's
// links.next, a Link header copied into the body.
const (
	PageNone   = "none"
	PageOffset = "offset"
	PagePage   = "page"
	PageCursor = "cursor"
	PageLink   = "link"
)

// Key styles: how a stored id is written into an entity URL. Protocols differ,
// and getting this wrong produces a 400 that looks like a mystery.
//
//	KeyRaw     /customers/ALFKI      plain REST
//	KeyQuoted  /Customers('ALFKI')   OData, always a string literal
//	KeyTyped   quoted when the spec declares the key parameter as a string,
//	           bare otherwise, which is the OData rule for mixed key types
const (
	KeyRaw    = "raw"
	KeyQuoted = "quoted"
	KeyTyped  = "typed"
)

// Items modes: how a list response's item container yields records.
//
//	ItemsArray  ItemsPath addresses a JSON array; each entry is a record
//	ItemsMap    ItemsPath addresses a JSON object; each key is a record's id
//	            and its value is the record
//
// The map shape is what APIs.guru, feature-flag services and plenty of
// registries publish. It cannot be read as an array: the identity lives in the
// key, which no JSON pointer into the value can reach.
const (
	ItemsArray = "array"
	ItemsMap   = "map"
)

// Param locations, mirroring the OpenAPI `in` values.
const (
	InPath   = "path"
	InQuery  = "query"
	InHeader = "header"
	InCookie = "cookie"
)

// Auth describes how to authenticate, never with what. The secret is fetched
// from the keychain under KeychainAccount(id); only its shape is persisted.
// OAuth2 client credentials (client id + secret + token URL) is the next kind
// to land here, and wants a real key vault behind it rather than one string.
type Auth struct {
	Kind string `json:"kind" yaml:"kind"`
	In   string `json:"in,omitempty" yaml:"in,omitempty"`
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	User string `json:"user,omitempty" yaml:"user,omitempty"`
}

// Pagination names the params that walk a list operation. OpenAPI does not
// describe paging, so the author binds it explicitly per resource.
type Pagination struct {
	Style       string `json:"style,omitempty" yaml:"style,omitempty"`
	LimitParam  string `json:"limit_param,omitempty" yaml:"limit_param,omitempty"`
	OffsetParam string `json:"offset_param,omitempty" yaml:"offset_param,omitempty"`
	CursorParam string `json:"cursor_param,omitempty" yaml:"cursor_param,omitempty"`
	CursorPath  string `json:"cursor_path,omitempty" yaml:"cursor_path,omitempty"`
	LinkPath    string `json:"link_path,omitempty" yaml:"link_path,omitempty"`
}

// OpRef binds one spec operation plus the params the author fixes at design
// time. Params keys are param names as they appear in the spec.
type OpRef struct {
	Operation string            `json:"operation" yaml:"operation"`
	Params    map[string]string `json:"params,omitempty" yaml:"params,omitempty"`
}

// FieldMap is one value a caller can pull out of a remote item, beyond the id
// and label every reference already carries. Key is the local name a form field
// binds to; Pointer is where the value sits inside one item.
//
// The connection author declares the menu here; a consuming field picks the
// subset it wants through ListRequest.Select. Curating the menu once beats
// every template hand-writing pointers into someone else's payload.
// Remote is the property name to name in a server-side projection, when the
// service supports one. It defaults to the pointer with its leading slash
// stripped, which is right whenever the payload key and the property name agree.
type FieldMap struct {
	Key     string `json:"key" yaml:"key"`
	Label   string `json:"label,omitempty" yaml:"label,omitempty"`
	Pointer string `json:"pointer" yaml:"pointer"`
	Type    string `json:"type,omitempty" yaml:"type,omitempty"`
	Remote  string `json:"remote,omitempty" yaml:"remote,omitempty"`
}

// Resource is one referenceable collection inside a connection. List feeds the
// picker; Get resolves a stored id back to a record. ItemsPath, IDPath and
// LabelPath are JSON pointers (RFC 6901); an empty ItemsPath means the list
// response is itself the item container.
type Resource struct {
	Key         string     `json:"key" yaml:"key"`
	Label       string     `json:"label,omitempty" yaml:"label,omitempty"`
	List        OpRef      `json:"list" yaml:"list"`
	Get         OpRef      `json:"get,omitzero" yaml:"get,omitempty"`
	ItemsPath   string     `json:"items_path,omitempty" yaml:"items_path,omitempty"`
	IDPath      string     `json:"id_path" yaml:"id_path"`
	LabelPath   string     `json:"label_path" yaml:"label_path"`
	SearchParam string     `json:"search_param,omitempty" yaml:"search_param,omitempty"`
	Pagination  Pagination `json:"pagination,omitzero" yaml:"pagination,omitempty"`
	Fields      []FieldMap `json:"fields,omitempty" yaml:"fields,omitempty"`

	// ItemsMode says how the item container yields records. Empty means
	// ItemsArray. Under ItemsMap the key is the id, so IDPath has nothing to
	// address and must stay empty.
	ItemsMode string `json:"items_mode,omitempty" yaml:"items_mode,omitempty"`

	// KeyStyle is how a stored id is written into the get operation's path.
	// Empty takes the dialect default, which is KeyRaw for plain REST.
	KeyStyle string `json:"key_style,omitempty" yaml:"key_style,omitempty"`

	// SearchTemplate turns the user's text into an expression rather than
	// passing it through as a bare value, for services that filter through an
	// expression language: `contains(Name,'{q}')` bound to SearchParam
	// `$filter`. `{q}` is the only placeholder, and the substituted text has
	// its single quotes doubled so a search box cannot rewrite the filter.
	SearchTemplate string `json:"search_template,omitempty" yaml:"search_template,omitempty"`

	// SelectParam pushes the projection to the server, so asking for two
	// fields fetches two columns instead of the whole entity. Empty means the
	// full record comes back and the pointers do the work locally.
	SelectParam string `json:"select_param,omitempty" yaml:"select_param,omitempty"`
}

// Field returns the declared field map under key.
func (r Resource) Field(key string) (FieldMap, bool) {
	for _, f := range r.Fields {
		if f.Key == key {
			return f, true
		}
	}
	return FieldMap{}, false
}

// Resource returns the resource under key.
func (c *Connection) Resource(key string) (Resource, bool) {
	if c == nil {
		return Resource{}, false
	}
	for _, r := range c.Resources {
		if r.Key == key {
			return r, true
		}
	}
	return Resource{}, false
}

// Connection is the persisted definition, one YAML file per connection.
// SpecFile is relative to the connections directory. SpecURL, when set, is
// where the spec was fetched from and what a drift check re-reads.
//
// BaseURL and SpecURL are independent on purpose: the document that describes a
// service is routinely published somewhere other than the service itself, and
// the same spec is reused across acceptance and production hosts. An empty
// BaseURL falls back to the spec's first server.
type Connection struct {
	ID        string            `json:"id" yaml:"id"`
	Name      string            `json:"name" yaml:"name"`
	BaseURL   string            `json:"base_url,omitempty" yaml:"base_url,omitempty"`
	SpecFile  string            `json:"spec_file" yaml:"spec_file"`
	SpecURL   string            `json:"spec_url,omitempty" yaml:"spec_url,omitempty"`
	Dialect   string            `json:"dialect,omitempty" yaml:"dialect,omitempty"`
	Auth      Auth              `json:"auth" yaml:"auth"`
	Headers   map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	Resources []Resource        `json:"resources,omitempty" yaml:"resources,omitempty"`
}

// Param is one operation input distilled from the spec.
type Param struct {
	Name     string `json:"name"`
	In       string `json:"in"`
	Required bool   `json:"required"`
	Type     string `json:"type,omitempty"`
}

// Operation is one callable endpoint. ID is the spec's operationId when it has
// one, else the synthesized "method:path" (e.g. "get:/customers/{id}"), which
// is unique because OpenAPI already requires method+path to be unique.
type Operation struct {
	ID        string  `json:"id"`
	Method    string  `json:"method"`
	Path      string  `json:"path"`
	Summary   string  `json:"summary,omitempty"`
	Params    []Param `json:"params,omitempty"`
	Synthetic bool    `json:"synthetic,omitempty"`

	// Result is the shape of the success response, when the document describes
	// one. Resource detection reads it; execution never does.
	Result *Result `json:"result,omitempty"`
}

// Catalog is a parsed spec: the interpreter's whole view of a remote service.
type Catalog struct {
	Title      string      `json:"title"`
	Version    string      `json:"version"`
	SpecFormat string      `json:"spec_format"`
	Servers    []string    `json:"servers,omitempty"`
	Operations []Operation `json:"operations"`
}

// Op returns the operation with the given ID.
func (c *Catalog) Op(id string) (Operation, bool) {
	if c == nil {
		return Operation{}, false
	}
	for _, op := range c.Operations {
		if op.ID == id {
			return op, true
		}
	}
	return Operation{}, false
}

// Param returns the named param of an operation.
func (o Operation) Param(name string) (Param, bool) {
	for _, p := range o.Params {
		if p.Name == name {
			return p, true
		}
	}
	return Param{}, false
}

// PathParams returns the operation's path params in declaration order.
func (o Operation) PathParams() []Param {
	var out []Param
	for _, p := range o.Params {
		if p.In == InPath {
			out = append(out, p)
		}
	}
	return out
}

// ValidationError is one issue found by Validate. Type is the stable key the
// frontend translates; Message is a developer-facing fallback.
type ValidationError struct {
	Type     string         `json:"type"`
	Message  string         `json:"message,omitempty"`
	Resource string         `json:"resource,omitempty"`
	Detail   map[string]any `json:"detail,omitempty"`
}
