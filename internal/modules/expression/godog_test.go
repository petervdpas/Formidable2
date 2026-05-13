package expression

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
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
// last SidebarItem returned, and the last error so multiple Then
// steps can check different facets of one result.
type exprWorld struct {
	mgr     *Manager
	tpl     fakeTpl
	sto     fakeSto
	result  SidebarItem
	resErr  error
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
		w.result, w.resErr = w.mgr.EvaluateSidebarOne("any", filename)
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
			return fmt.Errorf("expected SidebarItem.Error to be non-empty")
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

	ctx.After(func(c context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		w.tpl = fakeTpl{}
		w.sto = fakeSto{}
		w.mgr = nil
		w.result = SidebarItem{}
		w.resErr = nil
		return c, nil
	})
}
