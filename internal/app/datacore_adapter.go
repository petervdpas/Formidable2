package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/datacore"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// datacoreLoaderAdapter bridges template + storage into datacore.Loader.
// Reads structured form data directly (not the index), so any field is
// reachable and table rows keep their identity. Lives in the composition
// root: datacore owns no opinion about storage, storage none about the tensor.
type datacoreLoaderAdapter struct {
	tpl          *template.Manager
	sto          *storage.Manager
	templateFile string
}

func newDatacoreLoaderAdapter(tpl *template.Manager, sto *storage.Manager, templateFile string) *datacoreLoaderAdapter {
	return &datacoreLoaderAdapter{tpl: tpl, sto: sto, templateFile: templateFile}
}

// datacoreSkipTypes are field types that carry no statable value of their own
// (presentation, actions, or cross-template lookups handled elsewhere).
var datacoreSkipTypes = map[string]bool{
	"image": true, "api": true, "button": true, "facet": true, "heading": true,
}

func (a *datacoreLoaderAdapter) Records() ([]datacore.Record, error) {
	files, err := a.sto.ListForms(a.templateFile)
	if err != nil {
		return nil, err
	}
	return a.load(files)
}

// LoadSubset materializes only the named forms (datacore.SubsetLoader) so the
// planner seam can ingest just the index-narrowed records instead of every form.
func (a *datacoreLoaderAdapter) LoadSubset(ids []string) ([]datacore.Record, error) {
	return a.load(ids)
}

// load shapes the named forms into Records, loading the template once and
// skipping any form that fails to read (same tolerance as query and index).
func (a *datacoreLoaderAdapter) load(files []string) ([]datacore.Record, error) {
	tpl, err := a.tpl.LoadTemplate(a.templateFile)
	if err != nil {
		return nil, err
	}
	out := make([]datacore.Record, 0, len(files))
	for _, file := range files {
		f := a.sto.LoadForm(a.templateFile, file)
		if f == nil {
			continue
		}
		out = append(out, datacoreRecord(tpl, file, f))
	}
	return out, nil
}

// datacoreRecord shapes one live form into a Record. Scalars become fields;
// tables and multi-valued fields (list/tags/multioption) become row-identity
// tables (a multi-valued field is a one-column table whose column is "value");
// set facets become context-keyed values. Identity is the filename so the
// studio (which works in filenames) can anchor the graph on the selected item.
func datacoreRecord(tpl *template.Template, file string, f *storage.Form) datacore.Record {
	rec := datacore.Record{ID: file}
	if tpl.ItemField != "" {
		if v, ok := f.Data[tpl.ItemField]; ok {
			rec.Label = dcText(v)
		}
	}

	for _, fld := range tpl.Fields {
		if datacoreSkipTypes[fld.Type] {
			continue
		}
		v, present := f.Data[fld.Key]
		if !present {
			continue
		}
		switch {
		case fld.Type == "table":
			rows, labels := dcTableRows(fld, v)
			addTable(&rec, fld.Key, rows, labels)
		case isMultiValued(fld.Type):
			rows, labels := dcMultiRows(v)
			addTable(&rec, fld.Key, rows, labels)
		default:
			if s := dcText(v); s != "" {
				if rec.Fields == nil {
					rec.Fields = map[string]string{}
				}
				rec.Fields[fld.Key] = s
			}
		}
	}

	// Set-but-unselected facet carries no value: blank is absence, uniformly
	// for fields and facets (substrate ruling). Datacore does not manufacture
	// the index's "(unset)" bucket; intended divergence settled for the stat
	// migration (design/datacore-stat-migration.md), pinned by
	// TestStatAdapter_FacetUnsetBucketDiverges.
	for k, st := range f.Meta.Facets {
		if st.Set && st.Selected != "" {
			if rec.Facets == nil {
				rec.Facets = map[string]string{}
			}
			rec.Facets[k] = st.Selected
		}
	}
	return rec
}

func addTable(rec *datacore.Record, field string, rows []map[string]string, labels []string) {
	if len(rows) == 0 {
		return
	}
	if rec.Tables == nil {
		rec.Tables = map[string][]map[string]string{}
	}
	rec.Tables[field] = rows
	if rec.TableLabels == nil {
		rec.TableLabels = map[string][]string{}
	}
	rec.TableLabels[field] = labels
}

func isMultiValued(t string) bool {
	return t == "list" || t == "tags" || t == "multioption"
}

// dcTableRows maps each table row's positional cells onto their column keys
// (the option `value` of each column), dropping blank cells. Second return is
// a per-row label (first non-empty column value) naming the row node.
func dcTableRows(fld template.Field, v any) ([]map[string]string, []string) {
	cols := make([]string, len(fld.Options))
	for i, opt := range fld.Options {
		if mp, ok := opt.(map[string]any); ok {
			cols[i], _ = mp["value"].(string)
		}
	}
	var rows []map[string]string
	var labels []string
	for _, e := range dcSlice(v) {
		cells := dcSlice(e)
		row := map[string]string{}
		label := ""
		for i, colKey := range cols {
			if colKey == "" || i >= len(cells) {
				continue
			}
			if s := dcText(cells[i]); s != "" {
				row[colKey] = s
				if label == "" {
					label = s
				}
			}
		}
		if len(row) > 0 {
			rows = append(rows, row)
			labels = append(labels, label)
		}
	}
	return rows, labels
}

func dcMultiRows(v any) ([]map[string]string, []string) {
	var rows []map[string]string
	var labels []string
	for _, e := range dcSlice(v) {
		if s := dcText(e); s != "" {
			rows = append(rows, map[string]string{"value": s})
			labels = append(labels, s)
		}
	}
	return rows, labels
}

func dcSlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

func dcText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", x))
	}
}
