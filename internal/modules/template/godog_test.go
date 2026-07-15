package template

import (
	"context"
	"errors"
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
	tmp      string
	sys      *system.Manager
	m        *Manager
	list     []string
	loaded   *Template
	loadErr  error
	tmpl     *Template
	errors   []ValidationError
	desc     Descriptor
	descErr  error
	items    []ItemField
	itemsErr error
	saveErr  error
	registry []FieldDescriptor
	yamlBlob []byte
	reloaded *Template

	// Generator state (used by generator.feature only)
	genFields []Field
	genOut    string
	genShapes []ShapeInfo
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

	ctx.Step(`^a template in project mode with fields:$`, func(table *godog.Table) error {
		w.tmpl = &Template{
			Name:        "Test",
			Filename:    "test.yaml",
			ProjectMode: true,
			Fields:      tableToFields(table),
		}
		return nil
	})

	ctx.Step(`^the template has facet "([^"]*)" with icon "([^"]*)" and options:$`, func(key, icon string, table *godog.Table) error {
		if w.tmpl == nil {
			return fmt.Errorf("no template under construction")
		}
		w.tmpl.Facets = append(w.tmpl.Facets, Facet{
			Key:     key,
			Icon:    icon,
			Options: tableToFacetOptions(table),
		})
		return nil
	})

	ctx.Step(`^the template has facet "([^"]*)" with icon "([^"]*)" and no options$`, func(key, icon string) error {
		if w.tmpl == nil {
			return fmt.Errorf("no template under construction")
		}
		w.tmpl.Facets = append(w.tmpl.Facets, Facet{Key: key, Icon: icon})
		return nil
	})

	ctx.Step(`^the template has (\d+) facets$`, func(n int) error {
		if w.tmpl == nil {
			return fmt.Errorf("no template under construction")
		}
		facets := make([]Facet, n)
		for i := 0; i < n; i++ {
			facets[i] = Facet{
				Key:     fmt.Sprintf("facet_%02d", i),
				Icon:    "fa-flag",
				Options: []FacetOption{{Label: fmt.Sprintf("OPT_%d", i), Color: "red"}},
			}
		}
		w.tmpl.Facets = facets
		return nil
	})

	ctx.Step(`^the reloaded template has (\d+) facet(?:s)?$`, func(want int) error {
		if w.reloaded == nil {
			return fmt.Errorf("no reloaded template")
		}
		if got := len(w.reloaded.Facets); got != want {
			return fmt.Errorf("facets len = %d, want %d", got, want)
		}
		return nil
	})

	ctx.Step(`^reloaded facet (\d+) has key "([^"]*)" and icon "([^"]*)"$`, func(idx int, key, icon string) error {
		if w.reloaded == nil || idx >= len(w.reloaded.Facets) {
			return fmt.Errorf("facet %d not available", idx)
		}
		f := w.reloaded.Facets[idx]
		if f.Key != key || f.Icon != icon {
			return fmt.Errorf("facet[%d] = (key:%s, icon:%s), want (key:%s, icon:%s)", idx, f.Key, f.Icon, key, icon)
		}
		return nil
	})

	ctx.Step(`^reloaded facet (\d+) option (\d+) is "([^"]*)" colored "([^"]*)"$`, func(fIdx, oIdx int, label, color string) error {
		if w.reloaded == nil || fIdx >= len(w.reloaded.Facets) {
			return fmt.Errorf("facet %d not available", fIdx)
		}
		f := w.reloaded.Facets[fIdx]
		if oIdx >= len(f.Options) {
			return fmt.Errorf("facet %d option %d not available", fIdx, oIdx)
		}
		o := f.Options[oIdx]
		if o.Label != label || o.Color != color {
			return fmt.Errorf("facet[%d].options[%d] = (%s,%s), want (%s,%s)", fIdx, oIdx, o.Label, o.Color, label, color)
		}
		return nil
	})

	ctx.Step(`^I reload a template authored with legacy flag_definitions:$`, func(table *godog.Table) error {
		opts := tableToFacetOptions(table)
		var lines []string
		lines = append(lines, "name: Legacy", "filename: legacy.yaml", "fields:", "  - key: title", "    type: text", "flag_definitions:")
		for _, o := range opts {
			lines = append(lines, fmt.Sprintf("  - label: %s", o.Label))
			lines = append(lines, fmt.Sprintf("    color: %s", o.Color))
		}
		yamlSrc := strings.Join(lines, "\n")
		var tpl Template
		if err := unmarshalYAML([]byte(yamlSrc), &tpl); err != nil {
			return fmt.Errorf("unmarshal legacy yaml: %w", err)
		}
		w.reloaded = &tpl
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

	ctx.Step(`^I save the current template as "([^"]*)"$`, func(name string) error {
		w.saveErr = w.m.SaveTemplate(name, w.tmpl)
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

	ctx.Step(`^the save returned a validation error of type "([^"]*)"$`, func(want string) error {
		if w.saveErr == nil {
			return fmt.Errorf("expected save error, got nil")
		}
		var verr *ValidationFailedError
		if !errors.As(w.saveErr, &verr) {
			return fmt.Errorf("expected *ValidationFailedError, got %T: %v", w.saveErr, w.saveErr)
		}
		for _, ve := range verr.Errors {
			if ve.Type == want {
				return nil
			}
		}
		return fmt.Errorf("expected error type %q in %v", want, summarizeErrors(verr.Errors))
	})

	ctx.Step(`^the item fields request returned an error$`, func() error {
		if w.itemsErr == nil {
			return fmt.Errorf("expected item-fields error, got %+v", w.items)
		}
		return nil
	})

	// ── Field-type registry + per-type validation ────────────────────

	ctx.Step(`^a template with one guid field "([^"]*)" with collapsible true$`,
		func(key string) error {
			b := true
			w.tmpl = &Template{
				Name: "T", Filename: "t.yaml",
				Fields: []Field{{Key: key, Type: "guid", Collapsible: &b}},
			}
			return nil
		})

	ctx.Step(`^a template with one number field "([^"]*)" with format "([^"]*)"$`,
		func(key, format string) error {
			w.tmpl = &Template{
				Name: "T", Filename: "t.yaml",
				Fields: []Field{{Key: key, Type: "number", Format: format}},
			}
			return nil
		})

	ctx.Step(`^a template with one list field "([^"]*)" with collapsible true$`,
		func(key string) error {
			b := true
			w.tmpl = &Template{
				Name: "T", Filename: "t.yaml",
				Fields: []Field{{Key: key, Type: "list", Collapsible: &b}},
			}
			return nil
		})

	ctx.Step(`^a template with a loopstart field "([^"]*)" carrying summary_field "([^"]*)"$`,
		func(key, summary string) error {
			w.tmpl = &Template{
				Name: "T", Filename: "t.yaml",
				Fields: []Field{
					{Key: key, Type: "loopstart", SummaryField: summary},
					{Key: "name", Type: "text"},
					{Key: key, Type: "loopstop"},
				},
			}
			return nil
		})

	ctx.Step(`^validation reports a "([^"]*)" error for key "([^"]*)" and attr "([^"]*)"$`,
		func(kind, key, attr string) error {
			w.errors = w.m.Validate(w.tmpl)
			for _, e := range w.errors {
				if e.Type != kind || e.Key != key {
					continue
				}
				if got, _ := e.Detail["attr"].(string); got == attr {
					return nil
				}
			}
			return fmt.Errorf("expected %s/%s/%s, got %v",
				kind, key, attr, summarizeErrors(w.errors))
		})

	// ── Field-type registry surface ──────────────────────────────────

	ctx.Step(`^I read the field-type registry$`, func() error {
		w.registry = AllFieldTypes()
		return nil
	})

	ctx.Step(`^the registry contains "([^"]*)"$`, func(id string) error {
		for _, d := range w.registry {
			if d.ID == id {
				return nil
			}
		}
		return fmt.Errorf("registry missing %q", id)
	})

	ctx.Step(`^the registry first id is "([^"]*)"$`, func(want string) error {
		if len(w.registry) == 0 {
			return fmt.Errorf("registry is empty")
		}
		if w.registry[0].ID != want {
			return fmt.Errorf("first id = %q, want %q", w.registry[0].ID, want)
		}
		return nil
	})

	ctx.Step(`^the registry size is (\d+)$`, func(want int) error {
		if len(w.registry) != want {
			return fmt.Errorf("registry size = %d, want %d", len(w.registry), want)
		}
		return nil
	})

	// ── YAML round-trip ──────────────────────────────────────────────

	ctx.Step(`^I marshal the template and reload it$`, func() error {
		b, err := marshalYAML(w.tmpl)
		if err != nil {
			return err
		}
		w.yamlBlob = b
		var got Template
		if err := unmarshalYAML(b, &got); err != nil {
			return err
		}
		w.reloaded = &got
		return nil
	})

	ctx.Step(`^the loaded field "([^"]*)" has collapsible true$`, func(key string) error {
		if w.reloaded == nil {
			return fmt.Errorf("no reloaded template")
		}
		for _, f := range w.reloaded.Fields {
			if f.Key != key {
				continue
			}
			if f.Collapsible == nil || !*f.Collapsible {
				return fmt.Errorf("collapsible = %v, want true", f.Collapsible)
			}
			return nil
		}
		return fmt.Errorf("field %q not in reloaded fields", key)
	})

	ctx.Step(`^the loaded field "([^"]*)" has facet_key "([^"]*)" and format "([^"]*)"$`, func(key, wantFK, wantFmt string) error {
		if w.reloaded == nil {
			return fmt.Errorf("no reloaded template")
		}
		for _, f := range w.reloaded.Fields {
			if f.Key != key {
				continue
			}
			if f.FacetKey != wantFK {
				return fmt.Errorf("facet_key = %q, want %q", f.FacetKey, wantFK)
			}
			if f.Format != wantFmt {
				return fmt.Errorf("format = %q, want %q", f.Format, wantFmt)
			}
			return nil
		}
		return fmt.Errorf("field %q not in reloaded fields", key)
	})

	ctx.Step(`^the marshaled YAML does not contain "([^"]*)"$`, func(unwanted string) error {
		if strings.Contains(string(w.yamlBlob), unwanted) {
			return fmt.Errorf("YAML should not contain %q; got:\n%s", unwanted, w.yamlBlob)
		}
		return nil
	})

	// ── Generator (generator.feature) ────────────────────────────────

	ctx.Step(`^a fresh generator world$`, func() error {
		w.genFields = nil
		w.genOut = ""
		w.genShapes = nil
		return nil
	})

	ctx.Step(`^no fields$`, func() error {
		w.genFields = nil
		return nil
	})

	ctx.Step(`^the fields:$`, func(table *godog.Table) error {
		w.genFields = tableToFields(table)
		return nil
	})

	ctx.Step(`^I generate with shape "([^"]*)"$`, func(shape string) error {
		w.genOut = GenerateMarkdownTemplate(
			Shape(shape),
			GeneratorOptions{ImgMode: ImgURL, WrapLoops: true},
			w.genFields,
		)
		return nil
	})

	ctx.Step(`^I generate with shape "([^"]*)" and image mode "([^"]*)"$`,
		func(shape, mode string) error {
			w.genOut = GenerateMarkdownTemplate(
				Shape(shape),
				GeneratorOptions{ImgMode: ImgMode(mode), WrapLoops: true},
				w.genFields,
			)
			return nil
		})

	ctx.Step(`^I generate with shape "([^"]*)" and wrap loops "([^"]*)"$`,
		func(shape, wrap string) error {
			w.genOut = GenerateMarkdownTemplate(
				Shape(shape),
				GeneratorOptions{ImgMode: ImgURL, WrapLoops: wrap == "true"},
				w.genFields,
			)
			return nil
		})

	ctx.Step(`^I read the shape catalog$`, func() error {
		w.genShapes = Shapes()
		return nil
	})

	ctx.Step(`^the generated output is empty$`, func() error {
		if w.genOut != "" {
			return fmt.Errorf("expected empty output, got %q", w.genOut)
		}
		return nil
	})

	ctx.Step(`^the catalog has (\d+) entries$`, func(want int) error {
		if len(w.genShapes) != want {
			return fmt.Errorf("catalog len = %d, want %d", len(w.genShapes), want)
		}
		return nil
	})

	ctx.Step(`^the catalog contains shape "([^"]*)"$`, func(id string) error {
		for _, s := range w.genShapes {
			if string(s.ID) == id {
				return nil
			}
		}
		return fmt.Errorf("catalog missing shape %q", id)
	})

	ctx.Step(`^every catalog entry has a non-empty label and description$`, func() error {
		for _, s := range w.genShapes {
			if s.Label == "" || s.Description == "" {
				return fmt.Errorf("shape %q has empty label or description", s.ID)
			}
		}
		return nil
	})

	ctx.Step(`^the output starts with "(.*)"$`, func(prefix string) error {
		decoded := decodeFeatureLiteral(prefix)
		if !strings.HasPrefix(w.genOut, decoded) {
			return fmt.Errorf("output does not start with %q; got:\n%s", decoded, w.genOut)
		}
		return nil
	})

	ctx.Step(`^the output does not start with "(.*)"$`, func(prefix string) error {
		decoded := decodeFeatureLiteral(prefix)
		if strings.HasPrefix(w.genOut, decoded) {
			return fmt.Errorf("output should NOT start with %q; got:\n%s", decoded, w.genOut)
		}
		return nil
	})

	ctx.Step(`^the output contains "(.*)"$`, func(needle string) error {
		decoded := decodeFeatureLiteral(needle)
		if !strings.Contains(w.genOut, decoded) {
			return fmt.Errorf("output missing %q; got:\n%s", decoded, w.genOut)
		}
		return nil
	})

	ctx.Step(`^the output does not contain "(.*)"$`, func(needle string) error {
		decoded := decodeFeatureLiteral(needle)
		if strings.Contains(w.genOut, decoded) {
			return fmt.Errorf("output should NOT contain %q; got:\n%s", decoded, w.genOut)
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
		if i, ok := headers["label"]; ok {
			f.Label = r.Cells[i].Value
		}
		if i, ok := headers["facet_key"]; ok {
			f.FacetKey = r.Cells[i].Value
		}
		if i, ok := headers["format"]; ok {
			f.Format = r.Cells[i].Value
		}
		out = append(out, f)
	}
	return out
}

func tableToFacetOptions(table *godog.Table) []FacetOption {
	rows := table.Rows
	if len(rows) < 2 {
		return nil
	}
	headers := make(map[string]int, len(rows[0].Cells))
	for i, c := range rows[0].Cells {
		headers[c.Value] = i
	}
	out := make([]FacetOption, 0, len(rows)-1)
	for _, r := range rows[1:] {
		opt := FacetOption{}
		if i, ok := headers["label"]; ok {
			opt.Label = r.Cells[i].Value
		}
		if i, ok := headers["color"]; ok {
			opt.Color = r.Cells[i].Value
		}
		out = append(out, opt)
	}
	return out
}

// decodeFeatureLiteral unescapes \n and \" in a step parameter so
// generator scenarios can quote handlebars helpers naturally
// (godog's gherkin parser does not unescape these by default).
func decodeFeatureLiteral(s string) string {
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\"`, `"`)
	return s
}

func summarizeErrors(errs []ValidationError) []string {
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = e.Type
	}
	return out
}
