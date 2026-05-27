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
