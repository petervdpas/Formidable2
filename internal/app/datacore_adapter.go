package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/datacore"
	"github.com/petervdpas/formidable2/internal/modules/expression"
	"github.com/petervdpas/formidable2/internal/modules/relation"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// relationReader is the narrow seam onto declared relations + their edges, so
// the tensor can expose them as datacore Links (followable). Satisfied by
// *relation.Manager; kept to one method so the adapter borrows nothing else.
type relationReader interface {
	GetRelations(template string) ([]relation.Relation, error)
}

// datacoreLoaderAdapter bridges template + storage into datacore.Loader.
// Reads structured form data directly (not the index), so any field is
// reachable and table rows keep their identity. Lives in the composition
// root: datacore owns no opinion about storage, storage none about the tensor.
type datacoreLoaderAdapter struct {
	tpl          *template.Manager
	sto          *storage.Manager
	ev           *expression.Manager
	rel          relationReader
	templateFile string
	// loopRows, when set, ingests loop fields as row tables so loop rows
	// surface as graph nodes. Off by default (config graph_loop_rows): a loop
	// adds rows×columns cells per record, which the studio gates behind a
	// toggle since it grows the tensor on large templates.
	loopRows bool
}

func newDatacoreLoaderAdapter(tpl *template.Manager, sto *storage.Manager, ev *expression.Manager, rel relationReader, templateFile string, loopRows bool) *datacoreLoaderAdapter {
	return &datacoreLoaderAdapter{tpl: tpl, sto: sto, ev: ev, rel: rel, templateFile: templateFile, loopRows: loopRows}
}

// datacoreSkipTypes are field types that carry no statable value of their own
// (presentation, actions, or cross-template lookups handled elsewhere).
var datacoreSkipTypes = map[string]bool{
	"image": true, "api": true, "button": true, "facet": true, "heading": true, "formula": true,
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

// load builds the tensor's records: the primary template's forms as roots, plus
// the records its cross-template relations point at, loaded as satellites so
// Follow can reach them (and their tables) without inflating the primary root
// set. One global guid->identity map resolves both self and cross-template edges.
func (a *datacoreLoaderAdapter) load(files []string) ([]datacore.Record, error) {
	primary, primaryGuids, err := a.loadForms(a.templateFile, files, false)
	if err != nil {
		return nil, err
	}
	guidToID := make(map[string]string, len(primaryGuids))
	for i, g := range primaryGuids {
		if g != "" {
			guidToID[g] = primary[i].ID
		}
	}

	// Cross-template relation targets become satellites. Self-relation targets
	// are already primary roots, so crossTargets skips them.
	var satellites []datacore.Record
	for relTpl, want := range a.crossTargets() {
		relFiles, err := a.sto.ListForms(relTpl)
		if err != nil {
			continue // a degraded / missing target template is tolerated
		}
		recs, guids, err := a.loadForms(relTpl, relFiles, true)
		if err != nil {
			continue // degraded target template (e.g. deleted): skip its satellites, keep the primary
		}
		for i, g := range guids {
			if g == "" || !want[g] {
				continue
			}
			guidToID[g] = recs[i].ID
			satellites = append(satellites, recs[i])
		}
	}

	// Attach links on the primary records (the link sources) against the global
	// map, then append satellites. attachLinks mutates primary in place before
	// the append copies it, so the returned records carry their links.
	a.attachLinks(primary, primaryGuids, guidToID)
	return append(primary, satellites...), nil
}

// loadForms shapes a template's forms into Records (composite identity, optional
// satellite marking), loading the template once and skipping unreadable forms.
// The returned guids are parallel to the records.
func (a *datacoreLoaderAdapter) loadForms(templateFile string, files []string, satellite bool) ([]datacore.Record, []string, error) {
	tpl, err := a.tpl.LoadTemplate(templateFile)
	if err != nil {
		return nil, nil, err
	}
	recs := make([]datacore.Record, 0, len(files))
	guids := make([]string, 0, len(files))
	for _, file := range files {
		f := a.sto.LoadForm(templateFile, file)
		if f == nil {
			continue
		}
		rec := datacoreRecord(tpl, file, f, a.loopRows)
		rec.ID = datacore.NewID(templateFile, file)
		rec.Satellite = satellite
		if rec.Label == "" {
			rec.Label = file // never let the raw composite id surface as a graph label
		}
		// A graph-prefix string prepends a per-template qualifier to the label,
		// so records that share an item field (e.g. an audit-control code) read
		// distinctly across collections in the graph: "Toetsingskader: CH.02".
		if tpl.GraphPrefixField != "" {
			rec.Label = tpl.GraphPrefixField + ": " + rec.Label
		}
		// A graph color tints every node of this template, so a record reads by
		// its template's color in the graph (sourced once per template, applied
		// per record).
		rec.Color = tpl.GraphColor
		applyFormulas(a.ev, tpl, f, &rec)
		recs = append(recs, rec)
		guids = append(guids, f.Meta.ID)
	}
	return recs, guids, nil
}

// crossTargets is the set of edge-target guids per related (non-self) template,
// the records to pull in as satellites so cross-template Follow resolves.
func (a *datacoreLoaderAdapter) crossTargets() map[string]map[string]bool {
	out := map[string]map[string]bool{}
	if a.rel == nil {
		return out
	}
	defs, err := a.rel.GetRelations(a.templateFile)
	if err != nil {
		return out
	}
	for _, d := range defs {
		if d.To == a.templateFile {
			continue // self: targets are already primary roots
		}
		for _, e := range d.Edges {
			if out[d.To] == nil {
				out[d.To] = map[string]bool{}
			}
			out[d.To][e.To] = true
		}
	}
	return out
}

// attachLinks turns declared relation edges into datacore Links (followable as
// "rel:<to>"). Edges reference records by guid; the tensor addresses records by
// composite identity, so a target resolves only if its record is in the tensor:
// self-relation targets (primary roots) and cross-template targets loaded as
// satellites. An edge whose target is absent (deleted, or a degraded template)
// is skipped, harmlessly.
func (a *datacoreLoaderAdapter) attachLinks(recs []datacore.Record, guids []string, guidToFile map[string]string) {
	if a.rel == nil {
		return
	}
	rels, err := a.rel.GetRelations(a.templateFile)
	if err != nil || len(rels) == 0 {
		return
	}
	perRel := map[string]map[string][]string{} // "rel:<to>" -> fromGuid -> []targetFile
	for _, r := range rels {
		byFrom := map[string][]string{}
		for _, e := range r.Edges {
			if tf, ok := guidToFile[e.To]; ok {
				byFrom[e.From] = append(byFrom[e.From], tf)
			}
		}
		if len(byFrom) > 0 {
			perRel["rel:"+r.To] = byFrom
		}
	}
	if len(perRel) == 0 {
		return
	}
	for i := range recs {
		for key, byFrom := range perRel {
			if targets := byFrom[guids[i]]; len(targets) > 0 {
				if recs[i].Links == nil {
					recs[i].Links = map[string][]string{}
				}
				recs[i].Links[key] = targets
			}
		}
	}
}

// datacoreRecord shapes one live form into a Record. Scalars become fields;
// tables and multi-valued fields (list/tags/multioption) become row-identity
// tables (a multi-valued field is a one-column table whose column is "value");
// set facets become context-keyed values. Identity is the filename so the
// studio (which works in filenames) can anchor the graph on the selected item.
func datacoreRecord(tpl *template.Template, file string, f *storage.Form, loopRows bool) datacore.Record {
	rec := datacore.Record{ID: file}
	if tpl.ItemField != "" {
		if v, ok := f.Data[tpl.ItemField]; ok {
			rec.Label = dcText(v)
		}
	}

	for i := 0; i < len(tpl.Fields); i++ {
		fld := tpl.Fields[i]
		if fld.Type == "loopstart" {
			// A loop's child fields belong to the loop (their data lives per-row
			// under the loopstart key), not the record. Consume the whole block;
			// ingest it as a row table only when the studio toggle enables it.
			loopKey := fld.Key
			depth := 1
			inner := []template.Field{}
			i++
			for i < len(tpl.Fields) && depth > 0 {
				ff := tpl.Fields[i]
				switch ff.Type {
				case "loopstart":
					depth++
				case "loopstop":
					depth--
				}
				if depth > 0 {
					inner = append(inner, ff)
				}
				i++
			}
			i-- // the for-loop's i++ steps past the matching loopstop
			if loopRows {
				rows, labels := dcLoopRows(inner, f.Data[loopKey])
				addTable(&rec, loopKey, rows, labels)
			}
			continue
		}
		if fld.Type == "loopstop" || datacoreSkipTypes[fld.Type] {
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
// a per-row label: all non-empty cells joined in column order, so a row reads
// fully (e.g. "string | bk") in the graph tooltip rather than just its first cell.
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
		var parts []string // one entry per declared column, positional ("" for blanks)
		nonEmpty := 0
		for i, colKey := range cols {
			if colKey == "" {
				continue // undefined column: not part of the row's data or label
			}
			s := ""
			if i < len(cells) {
				s = dcText(cells[i])
			}
			if s != "" {
				row[colKey] = s // data map keeps only non-empty cells
				nonEmpty++
			}
			parts = append(parts, s) // label stays positional so columns align
		}
		if nonEmpty > 0 {
			rows = append(rows, row)
			labels = append(labels, strings.Join(parts, " | "))
		}
	}
	return rows, labels
}

// dcLoopRows flattens a loop's row array (data under the loopstart key) into
// row-identity table rows: each row contributes its scalar child cells keyed by
// the child field key, dropping blanks. Nested loops/tables/multi-valued fields
// inside a row are not expanded (the flat ingestion the studio toggle enables);
// a row with no scalar cells is dropped. The label joins the row's cells in
// declared order, matching dcTableRows.
func dcLoopRows(inner []template.Field, v any) ([]map[string]string, []string) {
	cols := make([]string, 0, len(inner))
	for _, fld := range inner {
		if fld.Key == "" || fld.Type == "loopstart" || fld.Type == "loopstop" {
			continue
		}
		if fld.Type == "table" || isMultiValued(fld.Type) || datacoreSkipTypes[fld.Type] {
			continue
		}
		cols = append(cols, fld.Key)
	}
	var rows []map[string]string
	var labels []string
	for _, e := range dcSlice(v) {
		rm, ok := e.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]string{}
		var parts []string
		for _, key := range cols {
			s := dcText(rm[key])
			if s == "" {
				continue
			}
			row[key] = s
			parts = append(parts, s)
		}
		if len(row) > 0 {
			rows = append(rows, row)
			labels = append(labels, strings.Join(parts, " | "))
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
