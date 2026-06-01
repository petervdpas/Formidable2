package expression

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"testing"

	"github.com/cucumber/godog"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initExpressionScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			Output:   colorWriter(),
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fail()
	}
}

func colorWriter() io.Writer {
	if w, ok := any(os.Stdout).(io.Writer); ok {
		return w
	}
	return io.Discard
}

// exprWorld is per-scenario state. Holds the in-test Manager, the
// last Result returned, and the last error so multiple Then
// steps can check different facets of one result.
type exprWorld struct {
	mgr    *Manager
	tpl    fakeTpl
	sto    fakeSto
	result Result
	resErr error

	// formula scenarios
	fctx   map[string]any
	fvals  map[string]any
	value  any
	valErr error
	funcs  []FunctionDoc
}

func (w *exprWorld) build() {
	w.mgr = NewManager(w.tpl, w.sto)
}

func initExpressionScenario(ctx *godog.ScenarioContext) {
	w := &exprWorld{}

	ctx.Step(`^a template with sidebar expression "([^"]*)" and expression field "([^"]*)"$`,
		func(src, field string) error {
			w.tpl = fakeTpl{src: src, fields: []string{field}}
			w.sto = fakeSto{}
			return nil
		})

	ctx.Step(`^a template with no sidebar expression$`, func() error {
		w.tpl = fakeTpl{src: ""}
		w.sto = fakeSto{}
		return nil
	})

	ctx.Step(`^records:$`, func(table *godog.Table) error {
		// First row is header. Header columns: filename, title, <field-keys>...
		if len(table.Rows) < 1 {
			return fmt.Errorf("records table needs at least a header row")
		}
		header := table.Rows[0].Cells
		colNames := make([]string, len(header))
		for i, c := range header {
			colNames[i] = c.Value
		}
		recs := make([]Record, 0, len(table.Rows)-1)
		for _, row := range table.Rows[1:] {
			r := Record{Context: map[string]any{}}
			for i, cell := range row.Cells {
				switch colNames[i] {
				case "filename":
					r.Filename = cell.Value
				case "title":
					r.Title = cell.Value
				default:
					r.Context[colNames[i]] = cell.Value
				}
			}
			recs = append(recs, r)
		}
		w.sto.records = recs
		return nil
	})

	ctx.Step(`^I evaluate the sidebar for "([^"]*)"$`, func(filename string) error {
		w.build()
		w.result, w.resErr = w.mgr.EvaluateListOne("any", filename)
		return nil
	})

	ctx.Step(`^the result filename is "([^"]*)"$`, func(want string) error {
		if w.result.Filename != want {
			return fmt.Errorf("filename = %q, want %q", w.result.Filename, want)
		}
		return nil
	})

	ctx.Step(`^the result text is "([^"]*)"$`, func(want string) error {
		if w.result.Text != want {
			return fmt.Errorf("text = %q, want %q", w.result.Text, want)
		}
		return nil
	})

	ctx.Step(`^there is no result error$`, func() error {
		if w.resErr != nil {
			return fmt.Errorf("unexpected error: %v", w.resErr)
		}
		if w.result.Error != "" {
			return fmt.Errorf("expected empty Error, got %q", w.result.Error)
		}
		return nil
	})

	ctx.Step(`^the result error is non-empty$`, func() error {
		if w.result.Error == "" {
			return fmt.Errorf("expected Result.Error to be non-empty")
		}
		return nil
	})

	ctx.Step(`^the result classes contain "([^"]*)"$`, func(want string) error {
		if !slices.Contains(w.result.Classes, want) {
			return fmt.Errorf("classes = %v, want to contain %q", w.result.Classes, want)
		}
		return nil
	})

	ctx.Step(`^the call returned ErrNoExpression$`, func() error {
		if !errors.Is(w.resErr, ErrNoExpression) {
			return fmt.Errorf("err = %v, want ErrNoExpression", w.resErr)
		}
		return nil
	})

	// ── Formula steps ────────────────────────────────────────────────

	ctx.Step(`^a formula context:$`, func(table *godog.Table) error {
		w.fctx = map[string]any{}
		for _, row := range table.Rows[1:] { // skip header (key|value)
			if len(row.Cells) < 2 {
				continue
			}
			w.fctx[row.Cells[0].Value] = cellValue(row.Cells[1].Value)
		}
		return nil
	})

	ctx.Step(`^I evaluate these expressions:$`, func(table *godog.Table) error {
		w.build()
		for _, row := range table.Rows[1:] { // skip header (expression|result)
			src, want := row.Cells[0].Value, row.Cells[1].Value
			got, err := w.mgr.EvaluateValue(src, copyCtx(w.fctx))
			if err != nil {
				return fmt.Errorf("%s: did not evaluate: %v", src, err)
			}
			if g := fmt.Sprint(got); g != want {
				return fmt.Errorf("%s = %q, want %q", src, g, want)
			}
		}
		return nil
	})

	ctx.Step(`^I evaluate the formulas:$`, func(table *godog.Table) error {
		w.build()
		specs := make([]FormulaSpec, 0, len(table.Rows)-1)
		for _, row := range table.Rows[1:] { // skip header (key|type|expression)
			specs = append(specs, FormulaSpec{
				Key:        row.Cells[0].Value,
				Type:       row.Cells[1].Value,
				Expression: row.Cells[2].Value,
			})
		}
		w.fvals = w.mgr.EvaluateFormulas(specs, copyCtx(w.fctx))
		return nil
	})

	ctx.Step(`^formula "([^"]*)" is "([^"]*)"$`, func(key, want string) error {
		v, ok := w.fvals[key]
		if !ok {
			return fmt.Errorf("formula %q produced no value", key)
		}
		if g := fmt.Sprint(v); g != want {
			return fmt.Errorf("formula %q = %q, want %q", key, g, want)
		}
		return nil
	})

	ctx.Step(`^formula "([^"]*)" is absent$`, func(key string) error {
		if _, ok := w.fvals[key]; ok {
			return fmt.Errorf("formula %q should be absent, got %#v", key, w.fvals[key])
		}
		return nil
	})

	ctx.Step(`^I evaluate the value:$`, func(doc *godog.DocString) error {
		w.build()
		w.value, w.valErr = w.mgr.EvaluateValue(doc.Content, copyCtx(w.fctx))
		return nil
	})

	ctx.Step(`^the value is "([^"]*)"$`, func(want string) error {
		if w.valErr != nil {
			return fmt.Errorf("unexpected error: %v", w.valErr)
		}
		if g := fmt.Sprint(w.value); g != want {
			return fmt.Errorf("value = %q, want %q", g, want)
		}
		return nil
	})

	ctx.Step(`^evaluation fails$`, func() error {
		if w.valErr == nil {
			return fmt.Errorf("expected an evaluation error, got value %#v", w.value)
		}
		return nil
	})

	ctx.Step(`^I list the formula functions$`, func() error {
		w.funcs = Functions()
		return nil
	})

	ctx.Step(`^the catalog includes "([^"]*)"$`, func(name string) error {
		for _, f := range w.funcs {
			if f.Name == name {
				return nil
			}
		}
		return fmt.Errorf("catalog has no function named %q", name)
	})

	ctx.Step(`^the catalog has an entry in category "([^"]*)"$`, func(cat string) error {
		for _, f := range w.funcs {
			if f.Category == cat {
				return nil
			}
		}
		return fmt.Errorf("catalog has no entry in category %q", cat)
	})

	ctx.Step(`^every function has a category and a snippet$`, func() error {
		for _, f := range w.funcs {
			if f.Name == "" || f.Snippet == "" || f.Category == "" {
				return fmt.Errorf("incomplete function entry: %+v", f)
			}
		}
		return nil
	})

	ctx.After(func(c context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		w.tpl = fakeTpl{}
		w.sto = fakeSto{}
		w.mgr = nil
		w.result = Result{}
		w.resErr = nil
		w.fctx = nil
		w.fvals = nil
		w.value = nil
		w.valErr = nil
		w.funcs = nil
		return c, nil
	})
}

// cellValue parses a feature-table cell into a typed value so arithmetic and
// boolean logic behave as they would on real form data: true/false -> bool, a
// number -> float64, otherwise the literal string.
func cellValue(s string) any {
	switch s {
	case "true":
		return true
	case "false":
		return false
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return s
}

// copyCtx clones the context so EvaluateFormulas/EvaluateValue (which seed
// results back into ctx) don't leak between the table rows of one scenario.
func copyCtx(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
