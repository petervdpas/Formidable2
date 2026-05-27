package csv

import (
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

func initImportSteps(ctx *godog.ScenarioContext, w *csvWorld) {
	ctx.Step(`^I request mappable fields for "([^"]*)"$`, func(tpl string) error {
		fs, err := w.m.MappableFieldsForTemplate(tpl)
		w.mappable = fs
		return err
	})

	ctx.Step(`^the mappable field keys are "([^"]*)"$`, func(want string) error {
		var got []string
		for _, f := range w.mappable {
			got = append(got, f.Key)
		}
		if strings.Join(got, ",") != want {
			return fmt.Errorf("mappable = %q, want %q", strings.Join(got, ","), want)
		}
		return nil
	})

	ctx.Step(`^I suggest mappings for headers "([^"]*)"$`, func(headers string) error {
		w.suggestions = SuggestMappings(strings.Split(headers, ","), w.mappable)
		return nil
	})

	ctx.Step(`^header "([^"]*)" maps to field "([^"]*)"$`, func(header, field string) error {
		for _, s := range w.suggestions {
			if s.Header == header {
				if s.FieldKey != field {
					return fmt.Errorf("header %q -> %q, want %q", header, s.FieldKey, field)
				}
				return nil
			}
		}
		return fmt.Errorf("no suggestion for header %q", header)
	})

	ctx.Step(`^header "([^"]*)" maps to nothing$`, func(header string) error {
		for _, s := range w.suggestions {
			if s.Header == header {
				if s.FieldKey != "" {
					return fmt.Errorf("header %q unexpectedly maps to %q", header, s.FieldKey)
				}
				return nil
			}
		}
		return fmt.Errorf("no suggestion entry for header %q", header)
	})

	ctx.Step(`^I apply transform "([^"]*)" param "([^"]*)" to "([^"]*)"$`, func(rule, param, val string) error {
		w.transformed = Apply(val, rule, param, ModeStorage)
		return nil
	})

	ctx.Step(`^the transformed value is "([^"]*)"$`, func(want string) error {
		if w.transformed != want {
			return fmt.Errorf("transformed = %q, want %q", w.transformed, want)
		}
		return nil
	})

	ctx.Step(`^I coerce "([^"]*)" as "([^"]*)"$`, func(raw, fieldType string) error {
		w.coerced = Coerce(raw, fieldType, nil)
		return nil
	})

	ctx.Step(`^the coerced value is "([^"]*)"$`, func(want string) error {
		if got := fmt.Sprintf("%v", w.coerced); got != want {
			return fmt.Errorf("coerced = %q (%T), want %q", got, w.coerced, want)
		}
		return nil
	})

	ctx.Step(`^I import the last export of "([^"]*)" aligned on "([^"]*)" grouped by "([^"]*)"$`, func(tpl, align, group string) error {
		rows := w.exportRes.Rows
		if len(rows) == 0 {
			return fmt.Errorf("no export to import")
		}
		headers := rows[0]
		cols := make([]ImportColumn, 0, len(headers))
		for _, h := range headers {
			cols = append(cols, ImportColumn{Header: h, Target: h})
		}
		plan := ImportPlan{Columns: cols, AlignSource: align, GroupKey: group}
		w.importForms = BuildImportForms(plan, headers, rows[1:], w.tplFields[tpl])
		return nil
	})

	ctx.Step(`^the import yields (\d+) form\(s\)$`, func(want int) error {
		if len(w.importForms) != want {
			return fmt.Errorf("import forms = %d, want %d", len(w.importForms), want)
		}
		return nil
	})

	ctx.Step(`^import form (\d+) field "([^"]*)" equals "([^"]*)"$`, func(idx int, key, want string) error {
		if idx >= len(w.importForms) {
			return fmt.Errorf("form %d out of range (%d)", idx, len(w.importForms))
		}
		got := fmt.Sprintf("%v", w.importForms[idx].Data[key])
		if got != want {
			return fmt.Errorf("form %d field %q = %q, want %q", idx, key, got, want)
		}
		return nil
	})

	ctx.Step(`^import form (\d+) table "([^"]*)" has (\d+) rows$`, func(idx int, key string, want int) error {
		if idx >= len(w.importForms) {
			return fmt.Errorf("form %d out of range (%d)", idx, len(w.importForms))
		}
		arr, ok := w.importForms[idx].Data[key].([]any)
		if !ok {
			return fmt.Errorf("form %d field %q is %T, want []any", idx, key, w.importForms[idx].Data[key])
		}
		if len(arr) != want {
			return fmt.Errorf("form %d table %q has %d rows, want %d", idx, key, len(arr), want)
		}
		return nil
	})

	ctx.Step(`^the coerced list is "([^"]*)"$`, func(want string) error {
		arr, ok := w.coerced.([]any)
		if !ok {
			return fmt.Errorf("coerced is %T, want []any", w.coerced)
		}
		var got []string
		for _, v := range arr {
			got = append(got, fmt.Sprintf("%v", v))
		}
		if strings.Join(got, ",") != want {
			return fmt.Errorf("coerced list = %q, want %q", strings.Join(got, ","), want)
		}
		return nil
	})
}
