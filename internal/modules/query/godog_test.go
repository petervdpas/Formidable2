package query

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initQueryScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			Output:   queryColorWriter(),
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fail()
	}
}

func queryColorWriter() io.Writer {
	if w, ok := any(os.Stdout).(io.Writer); ok {
		return w
	}
	return io.Discard
}

// queryWorld is per-scenario state: the built matrix, the spec assembled by
// the When steps, and the last run outcome.
type queryWorld struct {
	m           *Matrix
	spec        Spec
	colKeys     []string          // header of each selected column, parallel to spec.Columns
	srcByHeader map[string]Source // header label -> its table-column source
	res         Result
	err         error
}

func splitList(s string) []string {
	var out []string
	for p := range strings.SplitSeq(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func initQueryScenario(ctx *godog.ScenarioContext) {
	w := &queryWorld{}

	// The harness models every column as a column of one implicit table
	// "t", so each row carries its per-form origin (row index + count).
	// That is faithful to how prepare flattens a real table: provenance is
	// present, so sum/avg dedupe correctly and count vs count_distinct
	// genuinely differ. A header label resolves to its table-column source.
	resolve := func(header string) (Source, bool) {
		s, ok := w.srcByHeader[header]
		return s, ok
	}

	ctx.Step(`^a data matrix:$`, func(table *godog.Table) error {
		if len(table.Rows) < 1 {
			return fmt.Errorf("matrix needs a header row")
		}
		headers := table.Rows[0].Cells
		if len(headers) < 1 || headers[0].Value != "#form" {
			return fmt.Errorf("first column must be #form")
		}
		labels := headers[1:]
		srcByHeader := map[string]Source{}
		var mcols []MatrixCol
		for i, h := range labels {
			ci := i
			src := Source{Kind: "field", Key: "t", Col: &ci}
			srcByHeader[h.Value] = src
			mcols = append(mcols, MatrixCol{ID: sourceID(src)})
		}
		m := &Matrix{Cols: mcols}
		// Two passes: gather rows per form for the origin Count, then emit
		// with sequential per-form row indices.
		type pending struct {
			form  string
			cells []string
		}
		var ps []pending
		perForm := map[string]int{}
		for _, r := range table.Rows[1:] {
			cells := make([]string, len(labels))
			for i := range labels {
				if i+1 < len(r.Cells) {
					cells[i] = strings.TrimSpace(r.Cells[i+1].Value)
				}
			}
			form := strings.TrimSpace(r.Cells[0].Value)
			ps = append(ps, pending{form, cells})
			perForm[form]++
		}
		seq := map[string]int{}
		for _, p := range ps {
			row := seq[p.form]
			seq[p.form]++
			m.Rows = append(m.Rows, MatrixRow{
				Form:    p.form,
				Origins: []Origin{{Field: "t", Row: row, Count: perForm[p.form]}},
				Cells:   p.cells,
			})
		}
		*w = queryWorld{m: m, srcByHeader: srcByHeader}
		return nil
	})

	ctx.Step(`^the numeric columns "([^"]*)"$`, func(list string) error {
		want := map[string]bool{}
		for _, k := range splitList(list) {
			if s, ok := resolve(k); ok {
				want[sourceID(s)] = true
			}
		}
		for i := range w.m.Cols {
			if want[w.m.Cols[i].ID] {
				w.m.Cols[i].Hint = "number"
			}
		}
		return nil
	})

	ctx.Step(`^I select "([^"]*)"$`, func(list string) error {
		for _, k := range splitList(list) {
			s, ok := resolve(k)
			if !ok {
				// An unknown header is a deliberately bad source: project a
				// source the matrix does not carry so Execute reports it.
				s = Source{Kind: "field", Key: k}
			}
			w.spec.Columns = append(w.spec.Columns, Column{Header: k, Source: s})
			w.colKeys = append(w.colKeys, k)
		}
		return nil
	})

	ctx.Step(`^I filter "([^"]*)" "([^"]*)" "([^"]*)"$`, func(key, op, val string) error {
		s, ok := resolve(key)
		if !ok {
			s = Source{Kind: "field", Key: key}
		}
		w.spec.Filters = append(w.spec.Filters, Filter{Source: s, Op: op, Value: val})
		return nil
	})

	ctx.Step(`^I want distinct rows$`, func() error {
		w.spec.Distinct = true
		return nil
	})

	ctx.Step(`^I group by "([^"]*)"$`, func(list string) error {
		for _, k := range splitList(list) {
			idx := indexOf(w.colKeys, k)
			if idx < 0 {
				return fmt.Errorf("group key %q is not a selected column", k)
			}
			w.spec.GroupBy = append(w.spec.GroupBy, idx)
		}
		return nil
	})

	ctx.Step(`^I measure (count|count_distinct) as "([^"]*)"$`, func(fn, header string) error {
		w.spec.Measures = append(w.spec.Measures, Measure{Func: fn, Header: header})
		return nil
	})

	ctx.Step(`^I measure (sum|avg|min|max) of "([^"]*)" as "([^"]*)"$`, func(fn, key, header string) error {
		s, ok := resolve(key)
		if !ok {
			s = Source{Kind: "field", Key: key}
		}
		w.spec.Measures = append(w.spec.Measures, Measure{Func: fn, Source: s, Header: header})
		return nil
	})

	ctx.Step(`^I order by "([^"]*)" (ascending|descending)( numeric)?$`, func(key, dir, numeric string) error {
		idx := indexOf(w.colKeys, key)
		if idx < 0 {
			return fmt.Errorf("order key %q is not a selected column", key)
		}
		w.spec.OrderBy = append(w.spec.OrderBy, Sort{Column: idx, Desc: dir == "descending", Numeric: strings.TrimSpace(numeric) == "numeric"})
		return nil
	})

	ctx.Step(`^I limit (\d+)$`, func(n int) error {
		w.spec.Limit = n
		return nil
	})

	ctx.Step(`^I run the query$`, func() error {
		w.res, w.err = w.m.Execute(w.spec)
		return nil
	})

	ctx.Step(`^the query succeeds$`, func() error {
		if w.err != nil {
			return fmt.Errorf("unexpected error: %v", w.err)
		}
		return nil
	})

	ctx.Step(`^the query fails$`, func() error {
		if w.err == nil {
			return fmt.Errorf("expected an error, got none")
		}
		return nil
	})

	ctx.Step(`^the result has (\d+) rows$`, func(n int) error {
		if w.res.Count != n {
			return fmt.Errorf("row count = %d, want %d", w.res.Count, n)
		}
		return nil
	})

	ctx.Step(`^the result columns are "([^"]*)"$`, func(list string) error {
		want := splitList(list)
		if len(w.res.Columns) != len(want) {
			return fmt.Errorf("columns = %v, want %v", w.res.Columns, want)
		}
		for i := range want {
			if w.res.Columns[i] != want[i] {
				return fmt.Errorf("column %d = %q, want %q", i, w.res.Columns[i], want[i])
			}
		}
		return nil
	})

	ctx.Step(`^row (\d+) is "([^"]*)"$`, func(n int, list string) error {
		if n < 0 || n >= len(w.res.Rows) {
			return fmt.Errorf("row %d out of range (have %d)", n, len(w.res.Rows))
		}
		want := splitList(list)
		row := w.res.Rows[n]
		if len(row) != len(want) {
			return fmt.Errorf("row %d width = %d, want %d", n, len(row), len(want))
		}
		for i := range want {
			if row[i].Text != want[i] {
				return fmt.Errorf("row %d cell %d = %q, want %q", n, i, row[i].Text, want[i])
			}
		}
		return nil
	})

	ctx.Step(`^the query reports (\d+) anomal(?:y|ies)$`, func(n int) error {
		if len(w.res.Anomalies) != n {
			return fmt.Errorf("anomalies = %d, want %d", len(w.res.Anomalies), n)
		}
		return nil
	})

	ctx.After(func(c context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		*w = queryWorld{}
		return c, nil
	})
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}
