package app

import (
	"context"
	"sort"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/datadb"
	"github.com/petervdpas/formidable2/internal/modules/dataprovider"
	"github.com/petervdpas/formidable2/internal/modules/relation"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
	"github.com/petervdpas/formidable2/internal/modules/wiki"
)

// exportDataPacker implements wiki.DataPacker: it turns the collection templates
// among an export selection into the bundle's queryable data (a SQLite image of
// the records plus an OpenAPI spec built from the actual templates). Each record
// carries its field values, facets, tags, and outgoing relation edges; the
// flattened text feeds the full-text index. Non-collection templates
// (presentations, documents that are not collections) are skipped.
type exportDataPacker struct {
	dp  *dataprovider.Manager
	sto *storage.Manager
	rel *relation.Manager
	tpl *template.Manager
}

func (p exportDataPacker) BuildDataPack(ctx context.Context, filenames []string) (wiki.DataPack, error) {
	var records []datadb.Record
	var specs []datadb.TemplateSpec
	for _, fn := range filenames {
		if !p.dp.IsCollectionExposed(ctx, fn) {
			continue
		}
		specs = append(specs, p.templateSpec(fn))
		forms, err := p.dp.ListForms(ctx, fn, dataprovider.ListOpts{})
		if err != nil {
			return wiki.DataPack{}, err
		}
		edges := p.outgoingEdges(fn) // record guid -> target template -> [to-guid]
		for _, fs := range forms {
			form := p.sto.LoadForm(fn, fs.Filename)
			if form == nil || form.Meta.ID == "" {
				continue
			}
			guid := form.Meta.ID
			records = append(records, datadb.Record{
				Template: fn,
				GUID:     guid,
				Title:    fs.Title,
				Payload: map[string]any{
					"fields":    form.Data,
					"facets":    form.Meta.Facets,
					"tags":      form.Meta.Tags,
					"relations": edges[guid],
				},
				Text: flattenText(fs.Title, form),
			})
		}
	}
	if len(records) == 0 {
		return wiki.DataPack{}, nil
	}
	db, err := datadb.Build(records)
	if err != nil {
		return wiki.DataPack{}, err
	}
	return wiki.DataPack{
		DB:      db,
		OpenAPI: datadb.BuildOpenAPI(specs),
		Context: datadb.BuildContext(specs),
	}, nil
}

// templateSpec describes one collection template for the OpenAPI document: its
// display name and top-level fields, each with a JSON Schema type.
func (p exportDataPacker) templateSpec(filename string) datadb.TemplateSpec {
	spec := datadb.TemplateSpec{Filename: filename, Name: filename}
	t, err := p.tpl.LoadTemplate(filename)
	if err != nil || t == nil {
		return spec
	}
	if n := strings.TrimSpace(t.Name); n != "" {
		spec.Name = n
	}
	for _, f := range t.Fields {
		if f.Key == "" {
			continue
		}
		spec.Fields = append(spec.Fields, datadb.FieldSpec{
			Key:   f.Key,
			Label: f.Label,
			Type:  jsonType(f.Type),
		})
	}
	return spec
}

// jsonType maps a Formidable field type to a JSON Schema type for the spec.
func jsonType(fieldType string) string {
	switch fieldType {
	case "number", "range":
		return "number"
	case "boolean", "checkbox", "switch":
		return "boolean"
	case "table", "loop", "list":
		return "array"
	case "object", "group":
		return "object"
	default:
		return "string"
	}
}

// outgoingEdges indexes a template's relation edges by source record guid, then
// by target template, so each record's payload can carry its own links. The
// relation store mirrors edges onto both sides, so filtering by the From guid
// yields a record's outgoing links whichever side authored them.
func (p exportDataPacker) outgoingEdges(template string) map[string]map[string][]string {
	out := map[string]map[string][]string{}
	rels, err := p.rel.GetRelations(template)
	if err != nil {
		return out
	}
	for _, r := range rels {
		for _, e := range r.Edges {
			if e.From == "" || e.To == "" {
				continue
			}
			byTarget := out[e.From]
			if byTarget == nil {
				byTarget = map[string][]string{}
				out[e.From] = byTarget
			}
			byTarget[r.To] = append(byTarget[r.To], e.To)
		}
	}
	for _, byTarget := range out {
		for target, tos := range byTarget {
			byTarget[target] = uniqueSorted(tos)
		}
	}
	return out
}

// flattenText gathers a record's searchable text: its title, every string leaf
// in its field data, and its tags.
func flattenText(title string, form *storage.Form) string {
	var b strings.Builder
	b.WriteString(title)
	collectStrings(&b, form.Data)
	for _, tag := range form.Meta.Tags {
		b.WriteByte(' ')
		b.WriteString(tag)
	}
	return b.String()
}

func collectStrings(b *strings.Builder, v any) {
	switch t := v.(type) {
	case string:
		b.WriteByte(' ')
		b.WriteString(t)
	case map[string]any:
		for _, vv := range t {
			collectStrings(b, vv)
		}
	case []any:
		for _, vv := range t {
			collectStrings(b, vv)
		}
	}
}

func uniqueSorted(in []string) []string {
	if len(in) == 0 {
		return in
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
