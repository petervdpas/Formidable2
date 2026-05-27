package integrity

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"testing"

	"github.com/cucumber/godog"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initIntegrityScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}

type integrityWorld struct {
	tpl       *template.Template
	forms     map[string]*storage.Form
	report    Report
	fixResult FixResult
	lastErr   error
}

func initIntegrityScenario(ctx *godog.ScenarioContext) {
	w := &integrityWorld{}

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		w.tpl = nil
		w.forms = map[string]*storage.Form{}
		w.report = Report{}
		w.fixResult = FixResult{}
		w.lastErr = nil
		return ctx, nil
	})

	ctx.Step(`^a template "([^"]*)" with fields:$`, func(name string, tbl *godog.Table) error {
		tpl := &template.Template{Name: name, Filename: name}
		// First row is the header; skip it.
		for _, row := range tbl.Rows[1:] {
			if len(row.Cells) < 2 {
				return fmt.Errorf("field row needs key,type; got %v", row.Cells)
			}
			tpl.Fields = append(tpl.Fields, template.Field{
				Key:  row.Cells[0].Value,
				Type: row.Cells[1].Value,
			})
		}
		w.tpl = tpl
		return nil
	})

	ctx.Step(`^a form "([^"]*)" with data:$`, func(fn string, tbl *godog.Table) error {
		f := &storage.Form{
			Meta: storage.FormMeta{
				Created: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
				Updated: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
			},
			Data: map[string]any{},
		}
		for _, row := range tbl.Rows[1:] {
			if len(row.Cells) < 2 {
				return fmt.Errorf("data row needs key,value; got %v", row.Cells)
			}
			k := row.Cells[0].Value
			raw := row.Cells[1].Value
			f.Data[k] = coerceValue(w.tpl, k, raw)
		}
		w.forms[fn] = f
		return nil
	})

	ctx.Step(`^a form "([^"]*)" with meta id "([^"]*)" and data:$`, func(fn, id string, tbl *godog.Table) error {
		f := &storage.Form{
			Meta: storage.FormMeta{
				ID:      id,
				Created: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
				Updated: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
			},
			Data: map[string]any{},
		}
		for _, row := range tbl.Rows[1:] {
			if len(row.Cells) < 2 {
				return fmt.Errorf("data row needs key,value; got %v", row.Cells)
			}
			k := row.Cells[0].Value
			f.Data[k] = coerceValue(w.tpl, k, row.Cells[1].Value)
		}
		w.forms[fn] = f
		return nil
	})

	ctx.Step(`^an unreadable form "([^"]*)"$`, func(fn string) error {
		w.forms[fn] = nil
		return nil
	})

	ctx.Step(`^I repair "([^"]*)" with strategy "([^"]*)" on "([^"]*)"$`, func(kind, strat, tplName string) error {
		st := &stubTemplates{ts: map[string]*template.Template{}}
		if w.tpl != nil {
			st.ts[w.tpl.Filename] = w.tpl
		}
		so := &stubStorage{forms: map[string]map[string]*storage.Form{}}
		if w.tpl != nil {
			so.forms[w.tpl.Filename] = w.forms
		}
		m := NewManager(st, so)
		m.SetWriter(newStubWriter(so))
		w.fixResult, w.lastErr = m.FixTemplate(tplName, FixPlan{
			Items: []FixPlanItem{{Kind: IssueKind(kind), Strategy: FixStrategy(strat)}},
		})
		if w.tpl != nil {
			w.forms = so.forms[w.tpl.Filename]
		}
		return nil
	})

	ctx.Step(`^I analyze "([^"]*)"$`, func(tplName string) error {
		st := &stubTemplates{ts: map[string]*template.Template{}}
		if w.tpl != nil {
			st.ts[w.tpl.Filename] = w.tpl
		}
		so := &stubStorage{forms: map[string]map[string]*storage.Form{}}
		if w.tpl != nil {
			so.forms[w.tpl.Filename] = w.forms
		}
		m := NewManager(st, so)
		w.report, w.lastErr = m.AnalyzeTemplate(tplName)
		return nil
	})

	ctx.Step(`^the report has (\d+) forms? scanned$`, func(n int) error {
		if w.report.FormCount != n {
			return fmt.Errorf("FormCount = %d, want %d", w.report.FormCount, n)
		}
		return nil
	})

	ctx.Step(`^the report has (\d+) issues?$`, func(n int) error {
		if w.report.IssueCount != n {
			return fmt.Errorf("IssueCount = %d, want %d (forms: %+v)",
				w.report.IssueCount, n, w.report.Forms)
		}
		return nil
	})

	hasIssue := func(kind, path, fn string) error {
		for _, fr := range w.report.Forms {
			if fr.Filename != fn {
				continue
			}
			for _, iss := range fr.Issues {
				if string(iss.Kind) == kind && (path == "" || iss.Path == path) {
					return nil
				}
			}
		}
		// Build a stable summary for the failure message.
		var seen []string
		for _, fr := range w.report.Forms {
			for _, iss := range fr.Issues {
				seen = append(seen, fmt.Sprintf("%s@%s on %s", iss.Kind, iss.Path, fr.Filename))
			}
		}
		sort.Strings(seen)
		return fmt.Errorf("no %q issue at %q on %s; saw: %v", kind, path, fn, seen)
	}

	ctx.Step(`^the report has an? "([^"]*)" issue at "([^"]*)" on "([^"]*)"$`, hasIssue)

	ctx.Step(`^the report has an? "([^"]*)" issue on "([^"]*)"$`, func(kind, fn string) error {
		return hasIssue(kind, "", fn)
	})

	ctx.Step(`^the form "([^"]*)" has exactly (\d+) issues?$`, func(fn string, n int) error {
		for _, fr := range w.report.Forms {
			if fr.Filename == fn {
				if len(fr.Issues) != n {
					return fmt.Errorf("form %q has %d issues, want %d", fn, len(fr.Issues), n)
				}
				return nil
			}
		}
		return fmt.Errorf("form %q not in report", fn)
	})

	ctx.Step(`^the form "([^"]*)" data "([^"]*)" equals the meta id$`, func(fn, field string) error {
		f := w.forms[fn]
		if f == nil {
			return fmt.Errorf("no form %q", fn)
		}
		got, _ := f.Data[field].(string)
		if got == "" {
			return fmt.Errorf("data[%q] is empty, expected meta.id", field)
		}
		if got != f.Meta.ID {
			return fmt.Errorf("data[%q]=%q != meta.id %q", field, got, f.Meta.ID)
		}
		return nil
	})

	ctx.Step(`^the form "([^"]*)" meta id equals "([^"]*)"$`, func(fn, want string) error {
		f := w.forms[fn]
		if f == nil {
			return fmt.Errorf("no form %q", fn)
		}
		if f.Meta.ID != want {
			return fmt.Errorf("meta.id = %q, want %q", f.Meta.ID, want)
		}
		return nil
	})

	ctx.Step(`^the repair applied (\d+) fix(?:es)?$`, func(n int) error {
		if w.fixResult.Applied != n {
			return fmt.Errorf("applied = %d, want %d", w.fixResult.Applied, n)
		}
		return nil
	})

	ctx.Step(`^the repair leaves (\d+) issues?$`, func(n int) error {
		if w.fixResult.ScannedAfter != n {
			return fmt.Errorf("scanned_after = %d, want %d", w.fixResult.ScannedAfter, n)
		}
		return nil
	})

	ctx.Step(`^an integrity error occurred$`, func() error {
		if w.lastErr == nil {
			return errors.New("expected an error, got nil")
		}
		return nil
	})
}

// coerceValue converts a feature-file string into a typed value
// suitable for the matching template field. Without this, every value
// would arrive as a string and tests would spuriously trip
// IssueTypeMismatch on boolean/number cells. Falls back to leaving the
// raw string in place when the field is text-shaped (or unknown).
func coerceValue(tpl *template.Template, key, raw string) any {
	if tpl == nil {
		return raw
	}
	for _, f := range tpl.Fields {
		if f.Key != key {
			continue
		}
		switch f.Type {
		case "boolean":
			if b, err := strconv.ParseBool(raw); err == nil {
				return b
			}
			return raw // leave as-is so type_mismatch fires deterministically
		case "number", "range":
			if n, err := strconv.ParseFloat(raw, 64); err == nil {
				return n
			}
			return raw
		}
		return raw
	}
	return raw
}
