package csv

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

// fakeTplSource / fakeFormsSource back the export godog scenarios with
// in-memory templates and forms, so the export pipeline is exercised
// without storage or YAML on disk.
type fakeTplSource struct{ w *csvWorld }

func (f fakeTplSource) Fields(tpl string) ([]FieldSpec, error) {
	fs, ok := f.w.tplFields[tpl]
	if !ok {
		return nil, fmt.Errorf("no template %q", tpl)
	}
	return fs, nil
}

type fakeFormsSource struct{ w *csvWorld }

func (f fakeFormsSource) ListForms(tpl string) ([]string, error) {
	n := len(f.w.forms[tpl])
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("f%d.meta.json", i)
	}
	return out, nil
}

func (f fakeFormsSource) LoadFormData(tpl, datafile string) map[string]any {
	var i int
	if _, err := fmt.Sscanf(datafile, "f%d.meta.json", &i); err != nil {
		return nil
	}
	forms := f.w.forms[tpl]
	if i < 0 || i >= len(forms) {
		return nil
	}
	return forms[i]
}

// parseOptions turns "val:Label,val2:Label2" into the option-map shape the
// schema helpers expect. A bare "val" stands for both value and label.
func parseOptions(spec string) []any {
	if strings.TrimSpace(spec) == "" {
		return nil
	}
	out := []any{}
	for piece := range strings.SplitSeq(spec, ",") {
		piece = strings.TrimSpace(piece)
		if piece == "" {
			continue
		}
		v, l, found := strings.Cut(piece, ":")
		if !found {
			l = v
		}
		out = append(out, map[string]any{"value": v, "label": l})
	}
	return out
}

// parseCell parses a form value: JSON when it looks like an array/object,
// otherwise the literal string.
func parseCell(raw string) any {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "[") || strings.HasPrefix(s, "{") {
		var v any
		if err := json.Unmarshal([]byte(s), &v); err == nil {
			return v
		}
	}
	return raw
}

func initExportSteps(ctx *godog.ScenarioContext, w *csvWorld) {
	ctx.Step(`^a csv manager with template and storage deps$`, func() error {
		w.m = NewManager(w.sys, nil)
		w.m.SetTemplate(fakeTplSource{w})
		w.m.SetForms(fakeFormsSource{w})
		return nil
	})

	ctx.Step(`^a csv manager with no template dep$`, func() error {
		w.m = NewManager(w.sys, nil)
		w.m.SetForms(fakeFormsSource{w})
		return nil
	})

	ctx.Step(`^the template "([^"]*)" has fields:$`, func(tpl string, table *godog.Table) error {
		var fields []FieldSpec
		for i, row := range table.Rows {
			if i == 0 {
				continue // header
			}
			cells := row.Cells
			f := FieldSpec{Key: cells[0].Value, Type: cells[1].Value}
			if len(cells) > 2 {
				f.Label = cells[2].Value
			}
			if len(cells) > 3 {
				f.Options = parseOptions(cells[3].Value)
			}
			fields = append(fields, f)
		}
		w.tplFields[tpl] = fields
		return nil
	})

	ctx.Step(`^the form "([^"]*)" has data:$`, func(tpl string, table *godog.Table) error {
		data := map[string]any{}
		for i, row := range table.Rows {
			if i == 0 {
				continue
			}
			data[row.Cells[0].Value] = parseCell(row.Cells[1].Value)
		}
		w.forms[tpl] = append(w.forms[tpl], data)
		return nil
	})

	ctx.Step(`^I request the export schema for "([^"]*)" aligned on "([^"]*)"$`, func(tpl, align string) error {
		w.schema = w.m.ExportSchema(tpl, align)
		return nil
	})

	ctx.Step(`^I request the export schema for "([^"]*)" with no alignment$`, func(tpl string) error {
		w.schema = w.m.ExportSchema(tpl, "")
		return nil
	})

	ctx.Step(`^I export "([^"]*)" aligned on "([^"]*)" with columns "([^"]*)"$`, func(tpl, align, cols string) error {
		w.exportRes = w.m.Export(tpl, planFromColumns(cols, align))
		return nil
	})

	ctx.Step(`^I export "([^"]*)" with columns "([^"]*)"$`, func(tpl, cols string) error {
		w.exportRes = w.m.Export(tpl, planFromColumns(cols, ""))
		return nil
	})

	ctx.Step(`^the default columns are "([^"]*)"$`, func(want string) error {
		var got []string
		for _, c := range w.schema.Plan.Columns {
			if len(c.SourceKeys) > 0 {
				got = append(got, c.SourceKeys[0])
			}
		}
		if strings.Join(got, ",") != want {
			return fmt.Errorf("default columns = %q, want %q", strings.Join(got, ","), want)
		}
		return nil
	})

	ctx.Step(`^the echoed align source is "([^"]*)"$`, func(want string) error {
		if w.schema.Plan.AlignSource != want {
			return fmt.Errorf("align source = %q, want %q", w.schema.Plan.AlignSource, want)
		}
		return nil
	})

	ctx.Step(`^the alignable fields are "([^"]*)"$`, func(want string) error {
		var got []string
		for _, o := range w.schema.Alignable {
			got = append(got, o.Value)
		}
		if strings.Join(got, ",") != want {
			return fmt.Errorf("alignable = %q, want %q", strings.Join(got, ","), want)
		}
		return nil
	})

	ctx.Step(`^the source options include "([^"]*)"$`, func(key string) error {
		for _, o := range w.schema.Sources {
			if o.Value == key {
				return nil
			}
		}
		return fmt.Errorf("source options %v missing %q", w.schema.Sources, key)
	})

	ctx.Step(`^the source options do not include "([^"]*)"$`, func(key string) error {
		for _, o := range w.schema.Sources {
			if o.Value == key {
				return fmt.Errorf("source options unexpectedly include %q", key)
			}
		}
		return nil
	})

	ctx.Step(`^the schema reports an error$`, func() error {
		if w.schema.Error == "" {
			return fmt.Errorf("expected schema error, got none")
		}
		return nil
	})

	ctx.Step(`^the export has (\d+) data rows$`, func(want int) error {
		got := len(w.exportRes.Rows) - 1 // minus header
		if got != want {
			return fmt.Errorf("data rows = %d, want %d", got, want)
		}
		return nil
	})

	ctx.Step(`^the export header is "([^"]*)"$`, func(want string) error {
		if len(w.exportRes.Rows) == 0 {
			return fmt.Errorf("no rows")
		}
		got := strings.Join(w.exportRes.Rows[0], "|")
		if got != want {
			return fmt.Errorf("header = %q, want %q", got, want)
		}
		return nil
	})

	ctx.Step(`^data row (\d+) is "([^"]*)"$`, func(n int, want string) error {
		if n >= len(w.exportRes.Rows) {
			return fmt.Errorf("row %d out of range (%d rows)", n, len(w.exportRes.Rows))
		}
		got := strings.Join(w.exportRes.Rows[n], "|")
		if got != want {
			return fmt.Errorf("row %d = %q, want %q", n, got, want)
		}
		return nil
	})
}

// planFromColumns builds an ExportPlan from a comma list of source keys.
// Each column's header is its source key; alignSource is set verbatim.
func planFromColumns(cols, align string) ExportPlan {
	plan := ExportPlan{AlignSource: align}
	for c := range strings.SplitSeq(cols, ",") {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		plan.Columns = append(plan.Columns, ExportColumn{Header: c, SourceKeys: []string{c}})
	}
	return plan
}
