package connection

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
	oasyaml "github.com/oasdiff/yaml"
)

// Spec formats ParseSpec accepts. Swagger 2.0 is converted to OpenAPI 3 on the
// way in, so everything downstream sees one shape.
const (
	FormatOpenAPI3 = "openapi3"
	FormatSwagger2 = "swagger2"
)

// ErrUnknownSpecFormat is returned when a document carries neither a
// `swagger: "2.0"` nor an `openapi: 3.x` version marker.
var ErrUnknownSpecFormat = errors.New("connection: not an OpenAPI or Swagger document")

// methodOrder is the canonical method ordering within one path. Catalogs are
// sorted by path then by this order so a re-parse of the same bytes yields the
// same slice; spec maps are unordered and would otherwise shuffle every load.
var methodOrder = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "TRACE"}

// ParseSpec turns an uploaded Swagger/OpenAPI document into a Catalog. Input
// may be JSON or YAML. External $refs are refused: a spec must be
// self-contained, so parsing never reaches the filesystem or the network.
func ParseSpec(data []byte) (*Catalog, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, errors.New("connection: empty spec")
	}

	jsonData, err := oasyaml.YAMLToJSON(data)
	if err != nil {
		return nil, fmt.Errorf("connection: spec is neither valid JSON nor YAML: %w", err)
	}

	format, err := detectFormat(jsonData)
	if err != nil {
		return nil, err
	}

	doc, err := loadV3(jsonData, format)
	if err != nil {
		return nil, err
	}
	return buildCatalog(doc, format)
}

// detectFormat reads the version marker without committing to a full parse, so
// a wrong-shaped document fails with a clear reason instead of a schema error.
func detectFormat(jsonData []byte) (string, error) {
	var probe struct {
		Swagger string `json:"swagger"`
		OpenAPI string `json:"openapi"`
	}
	if err := json.Unmarshal(jsonData, &probe); err != nil {
		return "", fmt.Errorf("connection: malformed spec: %w", err)
	}
	switch {
	case strings.HasPrefix(probe.Swagger, "2."):
		return FormatSwagger2, nil
	case strings.HasPrefix(probe.OpenAPI, "3."):
		return FormatOpenAPI3, nil
	default:
		return "", ErrUnknownSpecFormat
	}
}

// loadV3 returns an OpenAPI 3 document, converting from 2.0 when needed.
func loadV3(jsonData []byte, format string) (*openapi3.T, error) {
	if format == FormatSwagger2 {
		var doc2 openapi2.T
		if err := json.Unmarshal(jsonData, &doc2); err != nil {
			return nil, fmt.Errorf("connection: malformed Swagger 2.0 document: %w", err)
		}
		doc3, err := openapi2conv.ToV3(&doc2)
		if err != nil {
			return nil, fmt.Errorf("connection: cannot convert Swagger 2.0 to OpenAPI 3: %w", err)
		}
		if len(doc3.Servers) == 0 {
			doc3.Servers = serversFromV2(&doc2)
		}
		return doc3, nil
	}

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	doc, err := loader.LoadFromData(jsonData)
	if err != nil {
		return nil, fmt.Errorf("connection: cannot parse OpenAPI document: %w", err)
	}
	return doc, nil
}

// serversFromV2 rebuilds server URLs from the 2.0 host/basePath/schemes trio
// for the case the converter leaves Servers empty.
func serversFromV2(doc2 *openapi2.T) openapi3.Servers {
	if doc2.Host == "" {
		return nil
	}
	schemes := doc2.Schemes
	if len(schemes) == 0 {
		schemes = []string{"https"}
	}
	var out openapi3.Servers
	for _, scheme := range schemes {
		out = append(out, &openapi3.Server{URL: scheme + "://" + doc2.Host + doc2.BasePath})
	}
	return out
}

// buildCatalog flattens the document into the interpreter's operation list.
func buildCatalog(doc *openapi3.T, format string) (*Catalog, error) {
	cat := &Catalog{SpecFormat: format}
	if doc.Info != nil {
		cat.Title = doc.Info.Title
		cat.Version = doc.Info.Version
	}
	for _, s := range doc.Servers {
		if s != nil && s.URL != "" {
			cat.Servers = append(cat.Servers, s.URL)
		}
	}

	if doc.Paths == nil {
		return cat, nil
	}
	paths := doc.Paths.Map()
	keys := make([]string, 0, len(paths))
	for p := range paths {
		keys = append(keys, p)
	}
	sort.Strings(keys)

	seen := map[string]string{}
	for _, path := range keys {
		item := paths[path]
		if item == nil {
			continue
		}
		for _, method := range methodOrder {
			op := item.GetOperation(method)
			if op == nil {
				continue
			}
			entry := Operation{
				Method:  method,
				Path:    path,
				Summary: op.Summary,
				Params:  mergeParams(item.Parameters, op.Parameters),
				Result:  describeResult(op),
			}
			entry.ID = op.OperationID
			if entry.ID == "" {
				entry.ID = SyntheticOperationID(method, path)
				entry.Synthetic = true
			}
			if prev, dup := seen[entry.ID]; dup {
				return nil, fmt.Errorf("connection: duplicate operationId %q on %s and %s %s",
					entry.ID, prev, method, path)
			}
			seen[entry.ID] = method + " " + path
			cat.Operations = append(cat.Operations, entry)
		}
	}
	return cat, nil
}

// SyntheticOperationID is the stable identity of an operation whose spec omits
// an operationId. OpenAPI already requires method+path to be unique, so this
// needs no counter and survives re-parsing and reordering.
func SyntheticOperationID(method, path string) string {
	return strings.ToLower(method) + ":" + path
}

// mergeParams flattens path-level params followed by operation-level ones. An
// operation param overrides a path param with the same name and location, which
// is the rule the OpenAPI spec states.
func mergeParams(pathLevel, opLevel openapi3.Parameters) []Param {
	var out []Param
	index := map[string]int{}

	add := func(refs openapi3.Parameters) {
		for _, ref := range refs {
			if ref == nil || ref.Value == nil {
				continue
			}
			p := Param{
				Name:     ref.Value.Name,
				In:       ref.Value.In,
				Required: ref.Value.Required,
				Type:     schemaType(ref.Value.Schema),
			}
			// A path param is required by definition, whatever the doc says.
			if p.In == InPath {
				p.Required = true
			}
			key := p.In + "\x00" + p.Name
			if at, ok := index[key]; ok {
				out[at] = p
				continue
			}
			index[key] = len(out)
			out = append(out, p)
		}
	}

	add(pathLevel)
	add(opLevel)
	return out
}

// schemaType reduces a schema to its primary JSON type. OpenAPI 3.1 allows a
// type array; the first non-null entry is the one a form field binds to.
func schemaType(ref *openapi3.SchemaRef) string {
	if ref == nil || ref.Value == nil || ref.Value.Type == nil {
		return ""
	}
	for _, t := range *ref.Value.Type {
		if t != "" && t != "null" {
			return t
		}
	}
	return ""
}

// SpecExtension picks the on-disk extension for a spec upload, so the stored
// copy stays byte-identical to what the author supplied and still opens in an
// editor with the right highlighting. An OpenAPI JSON document is an object, so
// a leading "{" is a complete test.
func SpecExtension(data []byte) string {
	if strings.HasPrefix(strings.TrimSpace(string(data)), "{") {
		return ".json"
	}
	return ".yaml"
}
