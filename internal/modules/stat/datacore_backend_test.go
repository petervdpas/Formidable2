package stat

import (
	"strconv"

	"github.com/petervdpas/formidable2/internal/modules/datacore"
	"github.com/petervdpas/formidable2/internal/modules/index"
	"github.com/petervdpas/formidable2/internal/statengine"
)

// cellValue is the string the tensor should carry for one indexed value. A
// numeric fixture stores the number in Num with an empty Text (the index keeps
// num_value and text_value in separate columns); datacore is string-first and
// re-parses, so format Num when Text is absent.
func cellValue(v index.FormValueRow) string {
	if v.Text == "" && v.Num != nil {
		return strconv.FormatFloat(*v.Num, 'f', -1, 64)
	}
	return v.Text
}

// The stat module's end-to-end tests run the engine over a REAL backend, not a
// hand-fed fake. That backend is now datacore (the shipped engine), reached
// through statengine.DatacoreIndex. The fixtures stay written as index.FormRow
// (the historical shape these tests speak); recordsFromForms projects them into
// the datacore records the tensor ingests, so the existing test bodies and
// assertions are unchanged and now verify the engine actually in production.

// colNamer is the test ColumnNamer: field -> col index -> column value-key. The
// e2e fixtures use a single table column (code-repositories col 0 = the
// application name), matching the fakeColResolver the tests already wire.
type colNamer map[string]map[int]string

func (n colNamer) ColumnKey(_, fieldKey string, col int) (string, bool) {
	if m, ok := n[fieldKey]; ok {
		if k, ok := m[col]; ok {
			return k, true
		}
	}
	return "", false
}

// repoNamer mirrors the fakeColResolver every e2e test installs. Passing it
// everywhere is harmless for scalar-only fixtures (it is simply never consulted).
var repoNamer = colNamer{"code-repositories": {0: "application"}}

// recordsFromForms projects index.FormRow fixtures into datacore records.
// Scalar values (Col nil) become fields; each table-column value (Col set)
// becomes its own single-column table row keyed by the column's value-key
// (the e2e fixtures use single-column tables only); set facets map straight
// across. Blank/unset facets are dropped, exactly as the datacore loader does.
func recordsFromForms(forms []index.FormRow, namer colNamer) []datacore.Record {
	out := make([]datacore.Record, 0, len(forms))
	for _, f := range forms {
		rec := datacore.Record{ID: f.Filename}
		for _, v := range f.Values {
			if v.Col == nil {
				if rec.Fields == nil {
					rec.Fields = map[string]string{}
				}
				rec.Fields[v.FieldKey] = cellValue(v)
				continue
			}
			colKey, ok := namer.ColumnKey("", v.FieldKey, *v.Col)
			if !ok {
				continue
			}
			if rec.Tables == nil {
				rec.Tables = map[string][]map[string]string{}
			}
			rec.Tables[v.FieldKey] = append(rec.Tables[v.FieldKey], map[string]string{colKey: cellValue(v)})
		}
		for _, ff := range f.Facets {
			if ff.Set && ff.Selected != "" {
				if rec.Facets == nil {
					rec.Facets = map[string]string{}
				}
				rec.Facets[ff.Key] = ff.Selected
			}
		}
		out = append(out, rec)
	}
	return out
}

type recordLoader struct{ recs []datacore.Record }

func (l recordLoader) Records() ([]datacore.Record, error) { return l.recs, nil }

// datacoreBackend builds the datacore-backed stat.Index over the fixtures. It
// is the drop-in for the old realIndex: same fixtures, the tensor computing.
func datacoreBackend(forms []index.FormRow) *statengine.DatacoreIndex {
	recs := recordsFromForms(forms, repoNamer)
	dc := datacore.NewService(func(string) datacore.Loader { return recordLoader{recs} })
	return statengine.New(dc, repoNamer)
}
