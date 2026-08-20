package connection

import (
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// The catalog keeps the shape of what an operation returns, not just what it
// takes. Resource detection reads it: an array in the response is what makes a
// path a collection, and the item's scalar leaves are the menu a field map is
// picked from. Nothing here is executed; it is the document's own description.

// maxShapeDepth bounds how deep into a nested item scalar leaves are collected.
// One level of nesting covers the usual address/owner case. Deeper than that a
// pointer stops being something a form field would sensibly bind to, and a
// self-referential schema would walk forever.
const maxShapeDepth = 2

// collectionNames are the wrapper properties that carry the items array, in
// preference order, for responses that declare more than one array. Picking by
// name is a guess, which is why Result records that it happened.
var collectionNames = []string{"value", "items", "data", "results", "records", "rows", "content", "list"}

// Property is one scalar leaf of an item schema. Pointer is RFC 6901, relative
// to a single item, so it can be used as a FieldMap pointer unchanged.
type Property struct {
	Name    string `json:"name"`
	Pointer string `json:"pointer"`
	Type    string `json:"type,omitempty"`
}

// Result is an operation's success response, distilled.
//
// Collection says the body yields items; ItemsPath points at the container and
// is empty when the body is itself one. ItemsMode says whether that container
// is an array or a map keyed by id, and Scalar whether its members are plain
// values rather than records. Ambiguous marks an ItemsPath that was chosen by
// name because the schema declared several arrays.
type Result struct {
	Collection bool       `json:"collection"`
	ItemsPath  string     `json:"items_path,omitempty"`
	ItemsMode  string     `json:"items_mode,omitempty"`
	Scalar     bool       `json:"scalar,omitempty"`
	Ambiguous  bool       `json:"ambiguous,omitempty"`
	Properties []Property `json:"properties,omitempty"`
}

// Property returns the leaf under pointer.
func (r *Result) Property(ptr string) (Property, bool) {
	if r == nil {
		return Property{}, false
	}
	for _, p := range r.Properties {
		if p.Pointer == ptr {
			return p, true
		}
	}
	return Property{}, false
}

// describeResult reads an operation's success response. A response with no
// JSON body yields nil, which is not an error: plenty of documents describe
// their operations without ever describing what comes back.
func describeResult(op *openapi3.Operation) *Result {
	if op == nil {
		return nil
	}
	schema := successBody(op.Responses)
	if schema == nil {
		return nil
	}
	return shapeOf(schema)
}

// successBody picks the JSON schema of the operation's success response. The
// default response is skipped: it is conventionally the error shape.
func successBody(responses *openapi3.Responses) *openapi3.SchemaRef {
	if responses == nil {
		return nil
	}
	resp := firstSuccess(responses)
	if resp == nil || resp.Value == nil {
		return nil
	}
	return jsonSchema(resp.Value.Content)
}

func firstSuccess(responses *openapi3.Responses) *openapi3.ResponseRef {
	for _, code := range []string{"200", "201", "2XX"} {
		if r := responses.Value(code); r != nil {
			return r
		}
	}
	m := responses.Map()
	codes := make([]string, 0, len(m))
	for code := range m {
		if strings.HasPrefix(code, "2") {
			codes = append(codes, code)
		}
	}
	sort.Strings(codes)
	if len(codes) == 0 {
		return nil
	}
	return m[codes[0]]
}

// jsonSchema picks the JSON media type out of a content map. An exact
// application/json wins; otherwise any +json structured syntax suffix does,
// which is what JSON:API and HAL publish under.
func jsonSchema(content openapi3.Content) *openapi3.SchemaRef {
	if mt := content["application/json"]; mt != nil {
		return mt.Schema
	}
	types := make([]string, 0, len(content))
	for name := range content {
		types = append(types, name)
	}
	sort.Strings(types)
	for _, name := range types {
		base := strings.TrimSpace(strings.Split(name, ";")[0])
		if strings.HasSuffix(base, "+json") || base == "text/json" {
			return content[name].Schema
		}
	}
	return nil
}

// shapeOf turns a response schema into a Result.
//
// Three container shapes yield items: an array, a map whose keys are ids and
// whose values are the records, and either of those wrapped in a single
// property. Members may be records or plain values; a scalar member is its own
// id, which is how a list of names or a flag map is referenced.
func shapeOf(ref *openapi3.SchemaRef) *Result {
	s := schemaOf(ref)
	if s == nil {
		return nil
	}
	if isArraySchema(s) {
		return collectionResult("", s.Items, ItemsArray, false)
	}
	if isKeyedSchema(s) {
		return collectionResult("", s.AdditionalProperties.Schema, ItemsMap, false)
	}
	if !isObjectSchema(s) {
		return nil
	}
	if name, items, mode, sole := containerProperty(s); items != nil {
		return collectionResult("/"+escapeToken(name), items, mode, !sole)
	}
	return &Result{Properties: leaves(ref)}
}

// collectionResult describes a container whose members are ref.
func collectionResult(itemsPath string, member *openapi3.SchemaRef, mode string, ambiguous bool) *Result {
	res := &Result{
		Collection: true,
		ItemsPath:  itemsPath,
		ItemsMode:  mode,
		Ambiguous:  ambiguous,
	}
	if isObjectSchema(schemaOf(member)) {
		res.Properties = leaves(member)
		return res
	}
	res.Scalar = true
	return res
}

// containerProperty finds the property wrapping the items. A property holding
// records wins outright, whether it is an array or a keyed map; with several,
// a known wrapper name decides and sole comes back false so the caller can flag
// the choice.
//
// A container of plain values only counts when it is the object's sole
// property, since a scalar array is otherwise a value of the record: a
// customer's tags must not turn the customer into a collection.
func containerProperty(s *openapi3.Schema) (name string, items *openapi3.SchemaRef, mode string, sole bool) {
	props := mergedProperties(s, map[*openapi3.Schema]bool{})

	var found []string
	for key, ref := range props {
		if member, _ := containerOf(schemaOf(ref)); isObjectSchema(schemaOf(member)) {
			found = append(found, key)
		}
	}
	sort.Strings(found)

	pick := func(key string) (string, *openapi3.SchemaRef, string, bool) {
		member, mode := containerOf(schemaOf(props[key]))
		return key, member, mode, len(found) == 1
	}

	switch len(found) {
	case 0:
		if len(props) == 1 {
			for key := range props {
				if member, mode := containerOf(schemaOf(props[key])); member != nil {
					return key, member, mode, true
				}
			}
		}
		return "", nil, "", false
	case 1:
		return pick(found[0])
	}
	for _, known := range collectionNames {
		for _, key := range found {
			if strings.EqualFold(key, known) {
				return pick(key)
			}
		}
	}
	return pick(found[0])
}

// containerOf reports the member schema of an array or keyed map, and which of
// the two it is. Anything else yields nil.
func containerOf(s *openapi3.Schema) (*openapi3.SchemaRef, string) {
	switch {
	case isArraySchema(s):
		return s.Items, ItemsArray
	case isKeyedSchema(s):
		return s.AdditionalProperties.Schema, ItemsMap
	}
	return nil, ""
}

// isKeyedSchema reports whether the schema is a JSON object used as a keyed
// collection: free-form keys, one declared value schema, and no fixed
// properties of its own to read it as a record instead.
func isKeyedSchema(s *openapi3.Schema) bool {
	if s == nil || s.AdditionalProperties.Schema == nil {
		return false
	}
	if s.Type != nil && !s.Type.Is("object") && len(*s.Type) > 0 {
		return false
	}
	return len(s.Properties) == 0 && len(s.AllOf) == 0
}

// leaves collects the scalar properties of one item, sorted by pointer so a
// re-parse of the same document yields the same slice.
func leaves(ref *openapi3.SchemaRef) []Property {
	var out []Property
	collectLeaves(ref, "", 1, &out)
	sort.Slice(out, func(i, j int) bool { return out[i].Pointer < out[j].Pointer })
	return out
}

func collectLeaves(ref *openapi3.SchemaRef, prefix string, depth int, out *[]Property) {
	s := schemaOf(ref)
	if s == nil {
		return
	}
	props := mergedProperties(s, map[*openapi3.Schema]bool{})
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		child := props[name]
		ptr := prefix + "/" + escapeToken(name)
		if t := schemaType(child); isScalarType(t) {
			*out = append(*out, Property{Name: name, Pointer: ptr, Type: t})
			continue
		}
		cs := schemaOf(child)
		if cs == nil || !isObjectSchema(cs) || depth >= maxShapeDepth {
			continue
		}
		collectLeaves(child, ptr, depth+1, out)
	}
}

// mergedProperties flattens a schema's own properties with its allOf members'.
// The schema's own declaration wins, and seen cuts a self-referential allOf.
func mergedProperties(s *openapi3.Schema, seen map[*openapi3.Schema]bool) openapi3.Schemas {
	if s == nil || seen[s] {
		return nil
	}
	seen[s] = true

	out := openapi3.Schemas{}
	for name, ref := range s.Properties {
		out[name] = ref
	}
	for _, member := range s.AllOf {
		ms := schemaOf(member)
		if ms == nil {
			continue
		}
		for name, ref := range mergedProperties(ms, seen) {
			if _, taken := out[name]; !taken {
				out[name] = ref
			}
		}
	}
	return out
}

func schemaOf(ref *openapi3.SchemaRef) *openapi3.Schema {
	if ref == nil {
		return nil
	}
	return ref.Value
}

// isObjectSchema accepts a declared object and an undeclared one that carries
// properties anyway, which plenty of documents do.
func isObjectSchema(s *openapi3.Schema) bool {
	if s == nil {
		return false
	}
	if s.Type != nil && s.Type.Is("object") {
		return true
	}
	if s.Type != nil && !s.Type.Is("object") && len(*s.Type) > 0 {
		return false
	}
	return len(s.Properties) > 0 || len(s.AllOf) > 0
}

func isArraySchema(s *openapi3.Schema) bool {
	return s != nil && s.Type != nil && s.Type.Is("array") && s.Items != nil
}

func isScalarType(t string) bool {
	switch t {
	case "string", "number", "integer", "boolean":
		return true
	}
	return false
}
