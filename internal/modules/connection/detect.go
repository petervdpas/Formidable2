package connection

import (
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Resource detection reads the document and proposes bindings, so authoring a
// client starts from a draft instead of a blank form.
//
// The pairing is structural, not a guess: a GET on a path with no parameters is
// a collection, and a GET on that same path plus one key segment resolves one
// of its members. Everything drafted on top of that pair comes from the
// operation's response schema, and whatever the document cannot state is named
// in Guessed so the author knows exactly what to check.

// ResourceDraft is a proposed resource plus the attributes whose values were
// inferred rather than read off the document.
type ResourceDraft struct {
	Resource Resource `json:"resource"`
	Guessed  []string `json:"guessed,omitempty"`
}

// labelNames are the properties that usually carry a human label, in preference
// order. Nothing in OpenAPI marks one, so this is always a suggestion.
var labelNames = []string{"name", "title", "label", "displayname", "display_name", "description", "subject", "summary"}

// keyNames are the properties that usually carry an identity when nothing is
// called id.
var keyNames = []string{"key", "code", "uuid", "guid"}

// searchCandidates are the query options that usually carry a search, in
// preference order. A name only counts when the operation declares it or the
// dialect implies it.
var searchCandidates = []string{"$filter", "q", "search", "query", "filter", "keyword", "term"}

// selectCandidates are the query options that push a projection to the server.
var selectCandidates = []string{"$select", "select", "fields"}

// offsetPairs and pagePairs are limit/offset param names that travel together.
var offsetPairs = [][2]string{{"limit", "offset"}, {"$top", "$skip"}, {"limit", "skip"}, {"take", "skip"}}
var pagePairs = [][2]string{{"per_page", "page"}, {"pageSize", "page"}, {"page_size", "page"}, {"limit", "page"}}

// linkLimits are the page-size params a link-paged collection may still accept.
var linkLimits = []string{"$top", "limit", "top"}

// Detection is what a detection run found: the proposals, plus why the rest of
// the document yielded nothing. Without the counts an empty result cannot be
// told apart from "everything here is already bound", which are opposite
// answers to the same screen.
type Detection struct {
	Drafts []ResourceDraft `json:"drafts"`

	// Bound counts collection operations an existing resource already lists.
	Bound int `json:"bound"`

	// NoCollection counts operations that read as a single record rather than
	// a list: no array of objects in the response, and no entity operation
	// pairing with them.
	NoCollection int `json:"no_collection"`
}

// Detect proposes resources and reports what it passed over. DetectResources is
// the same run without the counts.
func Detect(cat *Catalog, c *Connection) Detection {
	out := Detection{Drafts: []ResourceDraft{}}
	if cat == nil {
		return out
	}
	out.Drafts, out.Bound, out.NoCollection = detect(cat, c)
	return out
}

// DetectResources proposes a resource for every list/get pair in the catalog.
//
// Operations a resource already binds as its list are skipped, so running
// detection again after hand-editing proposes only what is still missing, and
// existing keys are never reused.
func DetectResources(cat *Catalog, c *Connection) []ResourceDraft {
	if cat == nil {
		return nil
	}
	drafts, _, _ := detect(cat, c)
	return drafts
}

func detect(cat *Catalog, c *Connection) (drafts []ResourceDraft, bound, noCollection int) {
	d := dialectOf(c)

	isBound := map[string]bool{}
	taken := map[string]bool{}
	if c != nil {
		for _, r := range c.Resources {
			isBound[r.List.Operation] = true
			taken[r.Key] = true
		}
	}

	// Entity operations, indexed by the collection path they hang off. The
	// catalog is path-sorted, so first-wins is stable across parses.
	entity := map[string]Operation{}
	for _, op := range cat.Operations {
		if op.Method != "GET" {
			continue
		}
		if coll, ok := collectionPathOf(op.Path); ok {
			if _, dup := entity[coll]; !dup {
				entity[coll] = op
			}
		}
	}

	for _, op := range cat.Operations {
		// A path with a parameter of its own is a sub-collection: it lists
		// something under one parent, which a draft cannot supply.
		if op.Method != "GET" || strings.Contains(op.Path, "{") {
			continue
		}
		if isBound[op.ID] {
			bound++
			continue
		}
		get, hasGet := entity[op.Path]
		if !hasGet && (op.Result == nil || !op.Result.Collection) {
			noCollection++
			continue
		}
		drafts = append(drafts, draftResource(op, get, hasGet, d, taken))
	}
	return drafts, bound, noCollection
}

// collectionPathOf strips the key segment off an entity path and reports the
// collection it addresses. Both shapes in the wild are covered: /customers/{id}
// for plain REST and /Customers({key}) for OData.
func collectionPathOf(path string) (string, bool) {
	if strings.HasSuffix(path, ")") {
		if open := strings.LastIndex(path, "("); open > 0 {
			if isSolePlaceholder(path[open+1 : len(path)-1]) {
				return path[:open], true
			}
		}
	}
	cut := strings.LastIndex(path, "/")
	if cut <= 0 {
		return "", false
	}
	if !isSolePlaceholder(path[cut+1:]) {
		return "", false
	}
	return path[:cut], true
}

// isSolePlaceholder reports whether s is exactly one {parameter} and nothing
// else, so /files/{name}.json is not mistaken for an entity path.
func isSolePlaceholder(s string) bool {
	if len(s) < 3 || s[0] != '{' || s[len(s)-1] != '}' {
		return false
	}
	return !strings.ContainsAny(s[1:len(s)-1], "{}")
}

func draftResource(list, get Operation, hasGet bool, d dialectSpec, taken map[string]bool) ResourceDraft {
	name := trimPathExtension(lastSegment(list.Path))
	key := uniqueKey(slugKey(name), "-", taken)
	taken[key] = true

	r := Resource{
		Key:   key,
		Label: titleFirst(name),
		List:  OpRef{Operation: list.ID},
	}
	if hasGet {
		r.Get = OpRef{Operation: get.ID}
	}

	var guessed []string
	mark := func(attr string) { guessed = append(guessed, attr) }

	// The list describes the items; the entity operation is the fallback for a
	// document that types one but not the other.
	props := propertiesOf(list.Result)
	if len(props) == 0 && hasGet {
		props = propertiesOf(get.Result)
	}

	keyed, scalar := false, false
	if res := list.Result; res != nil {
		r.ItemsPath = res.ItemsPath
		if res.ItemsMode == ItemsMap {
			r.ItemsMode = ItemsMap
			keyed = true
		}
		scalar = res.Scalar
		if res.Ambiguous {
			mark("items_path")
		}
	}

	switch {
	case keyed || scalar:
		// The identity is not inside the record. A keyed member is addressed
		// by its key and a plain value is its own id, so there is no pointer
		// to draft and nothing for the author to check.
	default:
		idParam := ""
		if hasGet {
			if p := get.PathParams(); len(p) == 1 {
				idParam = p[0].Name
			}
		}
		idPath, derived := pickIDPath(props, idParam)
		r.IDPath = idPath
		if !derived {
			mark("id_path")
		}
	}

	if !scalar {
		// Nothing in OpenAPI says which property a human reads, so this is a
		// suggestion every time, even when it is obviously right.
		r.LabelPath = pickLabelPath(props, r.IDPath)
		mark("label_path")
		r.Fields = fieldMaps(props)
	}

	if p := pickParam(list, d, searchCandidates); p != "" {
		r.SearchParam = p
		mark("search_param")
		// An expression option takes a predicate, not the typed text. Without
		// a template the service would filter on the literal string.
		if p == "$filter" {
			if prop := pointerToProperty(r.LabelPath); prop != "" {
				r.SearchTemplate = "contains(" + prop + ",'{q}')"
				mark("search_template")
			}
		}
	}
	if p := pickParam(list, d, selectCandidates); p != "" {
		r.SelectParam = p
		mark("select_param")
	}
	if p := pickPagination(list, d); p.Style != "" {
		r.Pagination = p
		mark("pagination")
	}

	sort.Strings(guessed)
	return ResourceDraft{Resource: r, Guessed: guessed}
}

func propertiesOf(res *Result) []Property {
	if res == nil {
		return nil
	}
	return res.Properties
}

// pickIDPath finds the property carrying the item's identity. derived is true
// only when the document actually says so: a property named id, or one named
// after the entity path's parameter. Everything below that is a suggestion.
func pickIDPath(props []Property, idParam string) (path string, derived bool) {
	for _, p := range props {
		if strings.EqualFold(p.Name, "id") {
			return p.Pointer, true
		}
	}
	if idParam != "" {
		for _, p := range props {
			if strings.EqualFold(p.Name, idParam) {
				return p.Pointer, true
			}
		}
	}
	for _, p := range props {
		if looksLikeID(p.Name) {
			return p.Pointer, false
		}
	}
	for _, want := range keyNames {
		for _, p := range props {
			if strings.EqualFold(p.Name, want) {
				return p.Pointer, false
			}
		}
	}
	if idParam != "" {
		return "/" + escapeToken(idParam), false
	}
	return "/id", false
}

// looksLikeID accepts customer_id and customerId, and refuses valid.
func looksLikeID(name string) bool {
	lower := strings.ToLower(name)
	if lower == "id" || strings.HasSuffix(lower, "_id") {
		return true
	}
	return len(name) > 2 && (strings.HasSuffix(name, "Id") || strings.HasSuffix(name, "ID"))
}

func pickLabelPath(props []Property, idPath string) string {
	for _, want := range labelNames {
		for _, p := range props {
			if strings.EqualFold(p.Name, want) {
				return p.Pointer
			}
		}
	}
	for _, p := range props {
		if p.Type == "string" && p.Pointer != idPath {
			return p.Pointer
		}
	}
	return "/name"
}

// fieldMaps offers every scalar leaf as a projectable field. The menu is meant
// to be trimmed: a resource declares what a form may pick, and deleting a row
// is cheaper than hand-writing a pointer into someone else's payload.
func fieldMaps(props []Property) []FieldMap {
	if len(props) == 0 {
		return nil
	}
	taken := map[string]bool{}
	out := make([]FieldMap, 0, len(props))
	for _, p := range props {
		key := uniqueKey(slugKey(pointerToProperty(p.Pointer)), "_", taken)
		taken[key] = true
		out = append(out, FieldMap{
			Key:     key,
			Label:   pointerToProperty(p.Pointer),
			Pointer: p.Pointer,
			Type:    p.Type,
		})
	}
	return out
}

// pickParam returns the first candidate the operation can actually carry.
func pickParam(op Operation, d dialectSpec, candidates []string) string {
	for _, name := range candidates {
		if queryParamOK(op, d, name) {
			return name
		}
	}
	return ""
}

// pickPagination proposes how to walk the collection. A protocol that hands
// back a next-page URL wins outright; otherwise the operation's own query
// params have to name the pair, since OpenAPI never describes paging.
func pickPagination(op Operation, d dialectSpec) Pagination {
	if d.linkPath != "" {
		p := Pagination{Style: PageLink}
		for _, name := range linkLimits {
			if queryParamOK(op, d, name) {
				p.LimitParam = name
				break
			}
		}
		return p
	}
	for _, pair := range offsetPairs {
		if queryParamOK(op, d, pair[0]) && queryParamOK(op, d, pair[1]) {
			return Pagination{Style: PageOffset, LimitParam: pair[0], OffsetParam: pair[1]}
		}
	}
	for _, pair := range pagePairs {
		if queryParamOK(op, d, pair[0]) && queryParamOK(op, d, pair[1]) {
			return Pagination{Style: PagePage, LimitParam: pair[0], OffsetParam: pair[1]}
		}
	}
	return Pagination{}
}

func lastSegment(path string) string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "resource"
	}
	if cut := strings.LastIndex(trimmed, "/"); cut >= 0 {
		trimmed = trimmed[cut+1:]
	}
	return trimmed
}

// pathExtensions are the suffixes a REST path puts on a collection segment to
// name a representation. They belong to the URL, not to the collection, so
// /providers.json is the providers resource.
var pathExtensions = []string{".json", ".xml", ".yaml", ".yml"}

func trimPathExtension(seg string) string {
	for _, ext := range pathExtensions {
		if len(seg) > len(ext) && strings.EqualFold(seg[len(seg)-len(ext):], ext) {
			return seg[:len(seg)-len(ext)]
		}
	}
	return seg
}

func titleFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// slugKey renders a spec name as a resource or field key: camel case becomes
// snake case, and anything outside the key alphabet becomes an underscore.
func slugKey(name string) string {
	var b strings.Builder
	prev := rune(0)
	for i, r := range name {
		if i > 0 && unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev)) {
			b.WriteRune('_')
		}
		b.WriteRune(unicode.ToLower(r))
		prev = r
	}

	out := make([]rune, 0, b.Len())
	for _, r := range b.String() {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	slug := strings.Trim(collapse(string(out)), "._-")
	if slug == "" || !slugRe.MatchString(slug) {
		return "field"
	}
	return slug
}

func collapse(s string) string {
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}
	return s
}

// uniqueKey suffixes base until it is free, so two properties that slug to the
// same name stay addressable.
func uniqueKey(base, sep string, taken map[string]bool) string {
	if !taken[base] {
		return base
	}
	for n := 2; ; n++ {
		candidate := base + sep + strconv.Itoa(n)
		if !taken[candidate] {
			return candidate
		}
	}
}
