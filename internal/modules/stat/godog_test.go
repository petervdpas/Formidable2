package stat

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"github.com/petervdpas/formidable2/internal/modules/index"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initStatScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			Output:   statColorWriter(),
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fail()
	}
}

func statColorWriter() io.Writer {
	if w, ok := any(os.Stdout).(io.Writer); ok {
		return w
	}
	return io.Discard
}

// statWorld is per-scenario state: the accumulated forms and named objects, the
// built engine, and the last evaluation outcome so several Then steps can check
// one result.
type statWorld struct {
	forms   []index.FormRow
	objects []StatObject
	backend Index

	grid    *Grid
	comp    *CompositeGrid
	evalErr error
}

// build stands up the datacore engine over the accumulated forms once the first
// evaluation step runs. The fixtures stay index.FormRow; recordsFromForms feeds
// them to the tensor, so these scenarios verify the shipped engine.
func (w *statWorld) build() error {
	w.backend = datacoreBackend(w.forms)
	return nil
}

func (w *statWorld) service() *Service {
	m := NewManager(w.backend)
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"code-repositories.application": 0}})
	return NewService(m, fakeSource{list: w.objects})
}

func (w *statWorld) cleanup() {}

// numCell is a scalar numeric field value (Col nil); scoreField below uses it.
func numCell(key string, n float64) index.FormValueRow {
	v := n
	return index.FormValueRow{FieldKey: key, ValueType: "number", Num: &v}
}

// labelOnAxis0 returns the cell value for the given axis-0 category and measure
// index, plus whether the category is present in the grid.
func valueOnAxis0(g *Grid, label string, measure int) (float64, bool) {
	for i, l := range g.Axes[0].Labels {
		if l != label {
			continue
		}
		for _, c := range g.Cells {
			if len(c.Coords) > 0 && c.Coords[0] == i {
				if measure < len(c.Values) {
					return c.Values[measure], true
				}
			}
		}
		return 0, true // category exists but no cell (zero) -> treat as 0
	}
	return 0, false
}

func pctOnAxis0(g *Grid, label string, measure int) (float64, bool) {
	for i, l := range g.Axes[0].Labels {
		if l != label {
			continue
		}
		for _, c := range g.Cells {
			if len(c.Coords) > 0 && c.Coords[0] == i && measure < len(c.Pct) {
				return c.Pct[measure], true
			}
		}
		return 0, true
	}
	return 0, false
}

func initStatScenario(ctx *godog.ScenarioContext) {
	w := &statWorld{}

	// ── Data setup ─────────────────────────────────────────────────────

	ctx.Step(`^the ODS records:$`, func(table *godog.Table) error {
		if len(table.Rows) < 1 {
			return fmt.Errorf("records table needs a header row")
		}
		cols := map[string]int{}
		for i, c := range table.Rows[0].Cells {
			cols[c.Value] = i
		}
		for _, row := range table.Rows[1:] {
			cell := func(name string) string {
				if i, ok := cols[name]; ok && i < len(row.Cells) {
					return strings.TrimSpace(row.Cells[i].Value)
				}
				return ""
			}
			apps := []string{}
			for a := range strings.SplitSeq(cell("apps"), ",") {
				if a = strings.TrimSpace(a); a != "" {
					apps = append(apps, a)
				}
			}
			r := odsForm(cell("filename"), apps...)
			facets := []index.FormFacet{}
			if v := cell("flag"); v != "" {
				facets = append(facets, index.FormFacet{Key: "flag", Set: true, Selected: v})
			}
			if v := cell("fcdm"); v != "" {
				facets = append(facets, index.FormFacet{Key: "fcdm", Set: true, Selected: v})
			}
			r.Facets = facets
			if v := cell("score"); v != "" {
				n, err := strconv.ParseFloat(v, 64)
				if err != nil {
					return fmt.Errorf("score %q: %w", v, err)
				}
				r.Values = append(r.Values, numCell("score", n))
			}
			w.forms = append(w.forms, r)
		}
		return nil
	})

	ctx.Step(`^a scaling "([^"]*)" over facet "([^"]*)" with default factor "([^"]*)":$`,
		func(name, facet, def string, table *godog.Table) error {
			d, err := strconv.ParseFloat(def, 64)
			if err != nil {
				return fmt.Errorf("default factor %q: %w", def, err)
			}
			cols := map[string]int{}
			for i, c := range table.Rows[0].Cells {
				cols[c.Value] = i
			}
			weights := []WeightEntry{}
			for _, row := range table.Rows[1:] {
				label := strings.TrimSpace(row.Cells[cols["option"]].Value)
				f, err := strconv.ParseFloat(strings.TrimSpace(row.Cells[cols["factor"]].Value), 64)
				if err != nil {
					return fmt.Errorf("factor for %q: %w", label, err)
				}
				weights = append(weights, WeightEntry{Label: label, Factor: f})
			}
			w.objects = append(w.objects, StatObject{
				Name:    name,
				Scaling: &Scaling{Source: SourceRef{Kind: SourceFacet, Key: facet}, Weights: weights, Default: d},
			})
			return nil
		})

	ctx.Step(`^a statistic "([^"]*)":$`, func(name string, doc *godog.DocString) error {
		w.objects = append(w.objects, StatObject{Name: name, DSL: strings.TrimSpace(doc.Content)})
		return nil
	})

	ctx.Step(`^a composite "([^"]*)" drills "([^"]*)" branch "([^"]*)" into "([^"]*)"$`,
		func(name, parent, branch, child string) error {
			w.objects = append(w.objects, StatObject{
				Name:      name,
				Composite: &CompositeSpec{Parent: parent, Edges: []CompositeEdgeSpec{{Branch: branch, Child: child}}},
			})
			return nil
		})

	// ── Evaluation ─────────────────────────────────────────────────────

	ctx.Step(`^I evaluate the statistic "([^"]*)"$`, func(name string) error {
		if err := w.build(); err != nil {
			return err
		}
		w.grid, w.evalErr = w.service().EvaluateObject("ods.yaml", name)
		return nil
	})

	ctx.Step(`^I evaluate the DSL:$`, func(doc *godog.DocString) error {
		if err := w.build(); err != nil {
			return err
		}
		w.grid, w.evalErr = w.service().EvaluateDSL("ods.yaml", strings.TrimSpace(doc.Content))
		return nil
	})

	ctx.Step(`^I evaluate the composite "([^"]*)"$`, func(name string) error {
		if err := w.build(); err != nil {
			return err
		}
		w.comp, w.evalErr = w.service().EvaluateComposite("ods.yaml", name)
		return nil
	})

	// ── Outcome ────────────────────────────────────────────────────────

	ctx.Step(`^evaluation succeeds$`, func() error {
		if w.evalErr != nil {
			return fmt.Errorf("unexpected evaluation error: %v", w.evalErr)
		}
		return nil
	})

	ctx.Step(`^evaluation fails$`, func() error {
		if w.evalErr == nil {
			return fmt.Errorf("expected an evaluation error, got none")
		}
		return nil
	})

	ctx.Step(`^the grid has (\d+) categories$`, func(n int) error {
		if w.grid == nil {
			return fmt.Errorf("no grid evaluated")
		}
		if got := len(w.grid.Axes[0].Labels); got != n {
			return fmt.Errorf("category count = %d, want %d", got, n)
		}
		return nil
	})

	ctx.Step(`^category "([^"]*)" is present$`, func(label string) error {
		if _, ok := valueOnAxis0(w.grid, label, 0); !ok {
			return fmt.Errorf("category %q is absent, expected present", label)
		}
		return nil
	})

	ctx.Step(`^category "([^"]*)" is absent$`, func(label string) error {
		if _, ok := valueOnAxis0(w.grid, label, 0); ok {
			return fmt.Errorf("category %q is present, expected absent", label)
		}
		return nil
	})

	ctx.Step(`^category "([^"]*)", measure (\d+) is "([^"]*)"$`, func(label string, measure int, want string) error {
		return checkValue(w.grid, label, measure, want, valueOnAxis0)
	})

	ctx.Step(`^category "([^"]*)", measure (\d+) is "([^"]*)" percent$`, func(label string, measure int, want string) error {
		return checkValue(w.grid, label, measure, want, pctOnAxis0)
	})

	ctx.Step(`^application "([^"]*)" weighs "([^"]*)"$`, func(app, want string) error {
		return checkValue(w.grid, app, 0, want, valueOnAxis0)
	})

	ctx.Step(`^parent branch "([^"]*)" weighs "([^"]*)"$`, func(branch, want string) error {
		if w.comp == nil {
			return fmt.Errorf("no composite evaluated")
		}
		return checkValue(w.comp.Parent, branch, 0, want, valueOnAxis0)
	})

	ctx.Step(`^in branch "([^"]*)" application "([^"]*)" weighs "([^"]*)"$`, func(branch, app, want string) error {
		for _, b := range w.comp.Branches {
			if b.Branch == branch {
				if b.Child == nil {
					return fmt.Errorf("branch %q is a leaf, expected a drilled child", branch)
				}
				return checkValue(b.Child, app, 0, want, valueOnAxis0)
			}
		}
		return fmt.Errorf("branch %q not found", branch)
	})

	ctx.Step(`^branch "([^"]*)" is a solid leaf$`, func(branch string) error {
		for _, b := range w.comp.Branches {
			if b.Branch == branch {
				if b.Child != nil {
					return fmt.Errorf("branch %q drills, expected a solid leaf", branch)
				}
				return nil
			}
		}
		return fmt.Errorf("branch %q not found", branch)
	})

	// ── DSL serialization and validation (no index) ────────────────────

	ctx.Step(`^these DSL strings round-trip:$`, func(table *godog.Table) error {
		for _, row := range table.Rows {
			src := strings.TrimSpace(row.Cells[0].Value)
			cfg, err := Parse(src)
			if err != nil {
				return fmt.Errorf("parse %q: %w", src, err)
			}
			got, err := Compile(cfg)
			if err != nil {
				return fmt.Errorf("compile %q: %w", src, err)
			}
			if got != src {
				return fmt.Errorf("round-trip mismatch:\n  in:  %s\n  out: %s", src, got)
			}
		}
		return nil
	})

	ctx.Step(`^these DSL strings fail to parse:$`, func(table *godog.Table) error {
		for _, row := range table.Rows {
			src := strings.TrimSpace(row.Cells[0].Value)
			if _, err := Parse(src); err == nil {
				return fmt.Errorf("expected parse error for %q, got none", src)
			}
		}
		return nil
	})

	ctx.Step(`^these statistics fail to evaluate:$`, func(table *godog.Table) error {
		if err := w.build(); err != nil {
			return err
		}
		svc := w.service()
		for _, row := range table.Rows {
			src := strings.TrimSpace(row.Cells[0].Value)
			if _, err := svc.EvaluateDSL("ods.yaml", src); err == nil {
				return fmt.Errorf("expected evaluation error for %q, got none", src)
			}
		}
		return nil
	})

	ctx.After(func(c context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		w.cleanup()
		*w = statWorld{}
		return c, nil
	})
}

// checkValue compares a looked-up grid figure (value or percent) against want.
func checkValue(g *Grid, label string, measure int, want string, lookup func(*Grid, string, int) (float64, bool)) error {
	if g == nil {
		return fmt.Errorf("no grid evaluated")
	}
	wantF, err := strconv.ParseFloat(want, 64)
	if err != nil {
		return fmt.Errorf("want %q: %w", want, err)
	}
	got, ok := lookup(g, label, measure)
	if !ok {
		return fmt.Errorf("category %q not found in grid", label)
	}
	if !nearly(got, wantF) {
		return fmt.Errorf("category %q measure %d = %v, want %v", label, measure, got, wantF)
	}
	return nil
}

// nearly tolerates float rounding for percentages and reduces.
func nearly(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}
