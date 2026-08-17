package connection

import "strings"

// Dialects. A dialect is a preset, never a branch: it fills in defaults for the
// resource strategy fields and names the query options the protocol implies but
// a document may not enumerate. Every value it supplies is overridable per
// resource, and the invoker only ever reads the resolved strategy.
const (
	DialectREST  = "rest"
	DialectOData = "odata"
)

// dialectSpec is what a preset contributes.
type dialectSpec struct {
	// implied are query options the protocol defines for every collection, so
	// binding them is legal even when the document does not list them. Plain
	// REST implies nothing: there, an undeclared parameter really is a typo.
	implied []string

	keyStyle string
	linkPath string
}

var dialects = map[string]dialectSpec{
	DialectREST: {
		keyStyle: KeyRaw,
	},
	DialectOData: {
		// OData v4 system query options. A service may support a subset, but
		// none of them is ever a typo.
		implied: []string{
			"$top", "$skip", "$skiptoken", "$filter", "$search",
			"$select", "$expand", "$orderby", "$count", "$format",
		},
		keyStyle: KeyTyped,
		linkPath: "/@odata.nextLink",
	},
}

// KnownDialects lists the presets, for the editor to offer.
func KnownDialects() []string { return []string{DialectREST, DialectOData} }

// dialectOf resolves a connection's preset, falling back to plain REST.
func dialectOf(c *Connection) dialectSpec {
	if c == nil {
		return dialects[DialectREST]
	}
	if d, ok := dialects[c.Dialect]; ok {
		return d
	}
	return dialects[DialectREST]
}

// validDialect reports whether a stored dialect name is one we know. An empty
// name means the default rather than an error.
func validDialect(name string) bool {
	if name == "" {
		return true
	}
	_, ok := dialects[name]
	return ok
}

// impliesParam reports whether the protocol defines name for every collection.
func (d dialectSpec) impliesParam(name string) bool {
	for _, p := range d.implied {
		if p == name {
			return true
		}
	}
	return false
}

// strategy is a resource's execution plan with dialect defaults folded in. The
// invoker reads only this, so no execution path ever asks which dialect it is.
type strategy struct {
	keyStyle    string
	linkPath    string
	searchParam string
	searchTmpl  string
	selectParam string
	paging      Pagination
}

// strategyFor folds the dialect defaults under whatever the resource states.
func strategyFor(c *Connection, r Resource) strategy {
	d := dialectOf(c)

	s := strategy{
		keyStyle:    firstNonEmpty(r.KeyStyle, d.keyStyle, KeyRaw),
		searchParam: r.SearchParam,
		searchTmpl:  r.SearchTemplate,
		selectParam: r.SelectParam,
		paging:      r.Pagination,
	}
	s.linkPath = firstNonEmpty(r.Pagination.LinkPath, d.linkPath)
	return s
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// formatKey writes a stored id into an entity path segment. The percent
// escaping happens later, in buildURL; this decides only whether the protocol
// wants a bare value or a string literal.
//
// Doubling an embedded quote is the OData (and SQL-family) escape, so an id
// like O'Brien addresses correctly instead of truncating the literal.
func formatKey(style string, p Param, id string) string {
	switch style {
	case KeyQuoted:
		return "'" + strings.ReplaceAll(id, "'", "''") + "'"
	case KeyTyped:
		if p.Type == "" || p.Type == "string" {
			return "'" + strings.ReplaceAll(id, "'", "''") + "'"
		}
		return id
	default:
		return id
	}
}

// unformatKey reverses formatKey, so an id read back out of a URL or a path
// matches the id the payload reported. Round-tripping matters: a stored value
// has to compare equal to what the list returned.
func unformatKey(s string) string {
	if len(s) >= 2 && strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'") {
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'")
	}
	return s
}

// applySearchTemplate substitutes the user's text into an expression. The value
// has its single quotes doubled first, so text typed into a picker cannot close
// a string literal and rewrite the rest of the filter.
func applySearchTemplate(tmpl, q string) string {
	return strings.ReplaceAll(tmpl, searchPlaceholder, strings.ReplaceAll(q, "'", "''"))
}

// searchPlaceholder is the only placeholder a search template may use.
const searchPlaceholder = "{q}"

// remoteName is the property a server-side projection should ask for. An
// explicit Remote wins; otherwise the pointer's tokens are the property path,
// which is what OData's $select wants for a complex type (Address/City).
func remoteName(f FieldMap) string {
	if f.Remote != "" {
		return f.Remote
	}
	return pointerToProperty(f.Pointer)
}

// pointerToProperty turns a JSON pointer into a dotted-free property path,
// unescaping the RFC 6901 tokens on the way. An empty pointer means the whole
// record and has no name, so it yields "".
func pointerToProperty(ptr string) string {
	if ptr == "" || !strings.HasPrefix(ptr, "/") {
		return ""
	}
	var parts []string
	for tok := range strings.SplitSeq(ptr[1:], "/") {
		parts = append(parts, unescapeToken(tok))
	}
	return strings.Join(parts, "/")
}
