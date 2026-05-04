package template

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/petervdpas/formidable2/internal/modules/system"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initTemplateScenario,
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

type tmplWorld struct {
	tmp        string
	sys        *system.Manager
	m          *Manager
	list       []string
	loaded     *Template
	loadErr    error
	tmpl       *Template
	errors     []ValidationError
	desc       Descriptor
	descErr    error
	items      []ItemField
	itemsErr   error
	saveErr    error
}

func initTemplateScenario(ctx *godog.ScenarioContext) {
	w := &tmplWorld{}

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		dir, err := os.MkdirTemp("", "tmpl-godog-")
		if err != nil {
			return ctx, err
		}
		w.tmp = dir
		w.sys = nil
		w.m = nil
		w.list = nil
		w.loaded = nil
		w.loadErr = nil
		w.tmpl = nil
		w.errors = nil
		w.desc = Descriptor{}
		w.descErr = nil
		w.items = nil
		return ctx, nil
	})

	ctx.After(func(ctx context.Context, sc *godog.Scenario, _ error) (context.Context, error) {
		if w.tmp != "" {
			_ = os.RemoveAll(w.tmp)
		}
		return ctx, nil
	})

	// ── Background ────────────────────────────────────────────────────

	ctx.Step(`^a system manager rooted at a temp directory$`, func() error {
		w.sys = system.NewManager(w.tmp, nil)
		return nil
	})

	ctx.Step(`^a template manager rooted under "([^"]*)"$`, func(dir string) error {
		w.m = NewManager(w.sys, dir, nil)
		return nil
	})

	// ── Givens ────────────────────────────────────────────────────────

	ctx.Step(`^a template "([^"]*)" exists$`, func(name string) error {
		body := `name: Other
filename: ` + name + `
fields:
  - key: id
    type: guid
  - key: title
    type: text
`
		return w.sys.SaveFile("templates/"+name, body)
	})

	ctx.Step(`^a template with fields:$`, func(table *godog.Table) error {
		w.tmpl = &Template{
			Name:     "Test",
			Filename: "test.yaml",
			Fields:   tableToFields(table),
		}
		return nil
	})

	ctx.Step(`^a template with collections enabled and fields:$`, func(table *godog.Table) error {
		w.tmpl = &Template{
			Name:             "Test",
			Filename:         "test.yaml",
			EnableCollection: true,
			Fields:           tableToFields(table),
		}
		return nil
	})

	ctx.Step(`^a template with an api field with no collection$`, func() error {
		w.tmpl = &Template{
			Name:     "Test",
			Filename: "test.yaml",
			Fields: []Field{
				{Key: "ref", Type: "api"},
			},
		}
		return nil
	})

	ctx.Step(`^a template with an api field with map keys "([^"]*)" "([^"]*)"$`, func(k1, k2 string) error {
		w.tmpl = &Template{
			Name:     "Test",
			Filename: "test.yaml",
			Fields: []Field{
				{
					Key: "ref", Type: "api", Collection: "x",
					Map: []APIMap{{Key: k1}, {Key: k2}},
				},
			},
		}
		return nil
	})

	ctx.Step(`^the file "([^"]*)" with content "([^"]*)"$`, func(path, content string) error {
		decoded := strings.ReplaceAll(content, `\n`, "\n")
		return w.sys.SaveFile(path, decoded)
	})

	// ── Whens ─────────────────────────────────────────────────────────

	ctx.Step(`^I list templates$`, func() error {
		out, err := w.m.ListTemplates()
		if err != nil {
			return err
		}
		w.list = out
		return nil
	})

	ctx.Step(`^I save a template named "([^"]*)" with the following yaml:$`, func(name string, body *godog.DocString) error {
		var t Template
		if err := unmarshalYAML([]byte(body.Content), &t); err != nil {
			return err
		}
		return w.m.SaveTemplate(name, &t)
	})

	ctx.Step(`^I load template "([^"]*)"$`, func(name string) error {
		w.loaded, w.loadErr = w.m.LoadTemplate(name)
		return nil
	})

	ctx.Step(`^loading "([^"]*)" returns a template named "([^"]*)"$`, func(file, name string) error {
		w.loaded, w.loadErr = w.m.LoadTemplate(file)
		if w.loadErr != nil {
			return fmt.Errorf("load: %v", w.loadErr)
		}
		if w.loaded.Name != name {
			return fmt.Errorf("name = %q, want %q", w.loaded.Name, name)
		}
		return nil
	})

	ctx.Step(`^I delete template "([^"]*)"$`, func(name string) error {
		return w.m.DeleteTemplate(name)
	})

	ctx.Step(`^I seed the basic template$`, func() error {
		return w.m.SeedBasicIfEmpty()
	})

	ctx.Step(`^I request the descriptor for "([^"]*)"$`, func(name string) error {
		w.desc, w.descErr = w.m.GetDescriptor(name, "/storage/path")
		return nil
	})

	ctx.Step(`^I list templates from a nonexistent folder$`, func() error {
		// Replace the manager with one rooted at a non-existent folder.
		w.m = NewManager(w.sys, "no-such-templates", nil)
		out, err := w.m.ListTemplates()
		if err != nil {
			return err
		}
		w.list = out
		return nil
	})

	ctx.Step(`^I save the test template with empty name$`, func() error {
		w.saveErr = w.m.SaveTemplate("", &Template{Name: "X", Fields: []Field{}})
		return nil
	})

	ctx.Step(`^I save a nil template named "([^"]*)"$`, func(name string) error {
		w.saveErr = w.m.SaveTemplate(name, nil)
		return nil
	})

	ctx.Step(`^I request item fields for "([^"]*)"$`, func(name string) error {
		w.items, w.itemsErr = w.m.GetItemFields(name)
		return nil
	})

	// ── Thens ─────────────────────────────────────────────────────────

	ctx.Step(`^the template list is empty$`, func() error {
		out, err := w.m.ListTemplates()
		if err != nil {
			return err
		}
		if len(out) != 0 {
			return fmt.Errorf("expected empty list, got %v", out)
		}
		return nil
	})

	ctx.Step(`^the template list contains "([^"]*)"$`, func(name string) error {
		out, err := w.m.ListTemplates()
		if err != nil {
			return err
		}
		if !slices.Contains(out, name) {
			return fmt.Errorf("%q not in %v", name, out)
		}
		return nil
	})

	ctx.Step(`^the template list does not contain "([^"]*)"$`, func(name string) error {
		out, err := w.m.ListTemplates()
		if err != nil {
			return err
		}
		if slices.Contains(out, name) {
			return fmt.Errorf("%q should NOT be in %v", name, out)
		}
		return nil
	})

	ctx.Step(`^the template has (\d+) field$`, func(want int) error {
		if w.loaded == nil {
			return fmt.Errorf("no template loaded")
		}
		if len(w.loaded.Fields) != want {
			return fmt.Errorf("fields len = %d, want %d", len(w.loaded.Fields), want)
		}
		return nil
	})

	ctx.Step(`^field (\d+) has key "([^"]*)" and type "([^"]*)"$`, func(idx int, key, typ string) error {
		if w.loaded == nil || idx >= len(w.loaded.Fields) {
			return fmt.Errorf("field %d not available", idx)
		}
		f := w.loaded.Fields[idx]
		if f.Key != key || f.Type != typ {
			return fmt.Errorf("field[%d] = (%s,%s), want (%s,%s)", idx, f.Key, f.Type, key, typ)
		}
		return nil
	})

	ctx.Step(`^the load returns an error$`, func() error {
		if w.loadErr == nil {
			return fmt.Errorf("expected error, got nil; loaded=%+v", w.loaded)
		}
		return nil
	})

	ctx.Step(`^validation reports a "([^"]*)" error$`, func(want string) error {
		w.errors = w.m.Validate(w.tmpl)
		for _, e := range w.errors {
			if e.Type == want {
				return nil
			}
		}
		return fmt.Errorf("expected %q in errors %v", want, summarizeErrors(w.errors))
	})

	ctx.Step(`^validation reports an "([^"]*)" error$`, func(want string) error {
		w.errors = w.m.Validate(w.tmpl)
		for _, e := range w.errors {
			if e.Type == want {
				return nil
			}
		}
		return fmt.Errorf("expected %q in errors %v", want, summarizeErrors(w.errors))
	})

	ctx.Step(`^validation reports no errors$`, func() error {
		w.errors = w.m.Validate(w.tmpl)
		if len(w.errors) > 0 {
			return fmt.Errorf("expected no errors, got %v", summarizeErrors(w.errors))
		}
		return nil
	})

	ctx.Step(`^the item fields are "([^"]*)"$`, func(csv string) error {
		// First save the template so GetItemFields can read it
		if err := w.m.SaveTemplate("test.yaml", w.tmpl); err != nil {
			return err
		}
		items, err := w.m.GetItemFields("test.yaml")
		if err != nil {
			return err
		}
		w.items = items
		got := make([]string, 0, len(items))
		for _, it := range items {
			got = append(got, it.Key)
		}
		want := strings.Split(csv, ",")
		if !slices.Equal(got, want) {
			return fmt.Errorf("item fields = %v, want %v", got, want)
		}
		return nil
	})

	ctx.Step(`^the descriptor name is "([^"]*)"$`, func(want string) error {
		if w.descErr != nil {
			return w.descErr
		}
		if w.desc.Name != want {
			return fmt.Errorf("descriptor.Name = %q, want %q", w.desc.Name, want)
		}
		return nil
	})

	ctx.Step(`^the descriptor has a non-empty storage location$`, func() error {
		if w.desc.StorageLocation == "" {
			return fmt.Errorf("expected non-empty StorageLocation")
		}
		return nil
	})

	ctx.Step(`^the descriptor request returned an error$`, func() error {
		if w.descErr == nil {
			return fmt.Errorf("expected descriptor error, got %+v", w.desc)
		}
		return nil
	})

	ctx.Step(`^the save returned an error$`, func() error {
		if w.saveErr == nil {
			return fmt.Errorf("expected save error, got nil")
		}
		return nil
	})

	ctx.Step(`^the item fields request returned an error$`, func() error {
		if w.itemsErr == nil {
			return fmt.Errorf("expected item-fields error, got %+v", w.items)
		}
		return nil
	})
}

func tableToFields(table *godog.Table) []Field {
	rows := table.Rows
	if len(rows) < 2 {
		return nil
	}
	headers := make(map[string]int, len(rows[0].Cells))
	for i, c := range rows[0].Cells {
		headers[c.Value] = i
	}
	out := make([]Field, 0, len(rows)-1)
	for _, r := range rows[1:] {
		f := Field{}
		if i, ok := headers["key"]; ok {
			f.Key = r.Cells[i].Value
		}
		if i, ok := headers["type"]; ok {
			f.Type = r.Cells[i].Value
		}
		out = append(out, f)
	}
	return out
}

func summarizeErrors(errs []ValidationError) []string {
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = e.Type
	}
	return out
}
