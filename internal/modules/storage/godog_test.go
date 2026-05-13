package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/cucumber/godog"
	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initStorageScenario,
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

type storageWorld struct {
	tmp          string
	sys          *system.Manager
	tplM         *template.Manager
	tpl          *template.Template
	m            *Manager
	formList     []string
	loaded       *Form
	saveResult   SaveResult
	saveImageRes SaveResult
	extendedList []FormSummary
	capturedID   string
	lastTpl      string
	lastDatafile string
}

func initStorageScenario(ctx *godog.ScenarioContext) {
	w := &storageWorld{}

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		dir, err := os.MkdirTemp("", "storage-godog-")
		if err != nil {
			return ctx, err
		}
		w.tmp = dir
		w.sys = nil
		w.tplM = nil
		w.tpl = nil
		w.m = nil
		w.formList = nil
		w.loaded = nil
		w.saveResult = SaveResult{}
		w.saveImageRes = SaveResult{}
		w.extendedList = nil
		w.capturedID = ""
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

	ctx.Step(`^a storage manager wrapping that system$`, func() error {
		w.tplM = template.NewManager(w.sys, "templates", nil)
		sfrM := sfr.NewManager(w.sys, nil)
		w.m = NewManager(w.sys, sfrM, w.tplM, "storage", nil)
		return nil
	})

	// ── Givens ────────────────────────────────────────────────────────

	ctx.Step(`^the template "([^"]*)" has no forms yet$`, func(name string) error {
		// Default template with one text field "title".
		w.tpl = &template.Template{
			Name:     name,
			Filename: name + ".yaml",
			Fields: []template.Field{
				{Key: "title", Type: "text"},
			},
		}
		return w.tplM.SaveTemplate(w.tpl.Filename, w.tpl)
	})

	ctx.Step(`^a basic template with a "([^"]*)" number field defaulting to (\d+)$`, func(key string, def int) error {
		w.tpl = &template.Template{
			Name:     "basic",
			Filename: "basic.yaml",
			Fields: []template.Field{
				{Key: "title", Type: "text"},
				{Key: key, Type: "number", Default: def},
			},
		}
		return w.tplM.SaveTemplate(w.tpl.Filename, w.tpl)
	})

	ctx.Step(`^a basic template with item_field set to "([^"]*)"$`, func(itemField string) error {
		w.tpl = &template.Template{
			Name:      "basic",
			Filename:  "basic.yaml",
			ItemField: itemField,
			Fields: []template.Field{
				{Key: "title", Type: "text"},
			},
		}
		return w.tplM.SaveTemplate(w.tpl.Filename, w.tpl)
	})

	ctx.Step(`^a saved form "([^"]*)" / "([^"]*)"$`, func(tmplFile, datafile string) error {
		w.saveResult = w.m.SaveForm(context.Background(), tmplFile, datafile, map[string]any{"title": "stub"})
		if !w.saveResult.Success {
			return fmt.Errorf("seed save failed: %s", w.saveResult.Error)
		}
		return nil
	})

	ctx.Step(`^a saved form "([^"]*)" / "([^"]*)" with title "([^"]*)"$`, func(tmplFile, datafile, title string) error {
		w.saveResult = w.m.SaveForm(context.Background(), tmplFile, datafile, map[string]any{"title": title})
		if !w.saveResult.Success {
			return fmt.Errorf("seed save failed: %s", w.saveResult.Error)
		}
		return nil
	})

	ctx.Step(`^a saved form "([^"]*)" / "([^"]*)" with empty title$`, func(tmplFile, datafile string) error {
		w.saveResult = w.m.SaveForm(context.Background(), tmplFile, datafile, map[string]any{"title": ""})
		if !w.saveResult.Success {
			return fmt.Errorf("seed save failed: %s", w.saveResult.Error)
		}
		return nil
	})

	// ── Whens ─────────────────────────────────────────────────────────

	ctx.Step(`^I list forms for "([^"]*)"$`, func(tmplFile string) error {
		out, err := w.m.ListForms(tmplFile)
		if err != nil {
			return err
		}
		w.formList = out
		return nil
	})

	ctx.Step(`^I save a form "([^"]*)" / "([^"]*)" with data:$`, func(tmplFile, datafile string, table *godog.Table) error {
		data := tableToData(table)
		w.saveResult = w.m.SaveForm(context.Background(), tmplFile, datafile, data)
		w.lastTpl, w.lastDatafile = tmplFile, datafile
		return nil
	})

	ctx.Step(`^I load form "([^"]*)" / "([^"]*)"$`, func(tmplFile, datafile string) error {
		w.loaded = w.m.LoadForm(tmplFile, datafile)
		return nil
	})

	ctx.Step(`^I save a form "([^"]*)" / "([^"]*)" with raw meta flag_state "([^"]*)"$`, func(tmplFile, datafile, state string) error {
		raw := map[string]any{
			"_meta": map[string]any{"flag_state": state},
		}
		w.saveResult = w.m.SaveForm(context.Background(), tmplFile, datafile, raw)
		w.lastTpl, w.lastDatafile = tmplFile, datafile
		return nil
	})

	ctx.Step(`^I save a form "([^"]*)" / "([^"]*)" with raw meta flagged (true|false) and flag_state "([^"]*)"$`, func(tmplFile, datafile, flaggedStr, state string) error {
		raw := map[string]any{
			"_meta": map[string]any{
				"flagged":    flaggedStr == "true",
				"flag_state": state,
			},
		}
		w.saveResult = w.m.SaveForm(context.Background(), tmplFile, datafile, raw)
		w.lastTpl, w.lastDatafile = tmplFile, datafile
		return nil
	})

	ctx.Step(`^the loaded form's meta has flag_state "([^"]*)"$`, func(want string) error {
		f := loadFormByDatafile(w)
		if f == nil {
			return fmt.Errorf("no form to inspect")
		}
		if f.Meta.FlagState != want {
			return fmt.Errorf("flag_state = %q, want %q", f.Meta.FlagState, want)
		}
		return nil
	})

	ctx.Step(`^the loaded form's meta has flagged (true|false)$`, func(want string) error {
		f := loadFormByDatafile(w)
		if f == nil {
			return fmt.Errorf("no form to inspect")
		}
		got := "false"
		if f.Meta.Flagged {
			got = "true"
		}
		if got != want {
			return fmt.Errorf("flagged = %s, want %s", got, want)
		}
		return nil
	})

	ctx.Step(`^I delete form "([^"]*)" / "([^"]*)"$`, func(tmplFile, datafile string) error {
		return w.m.DeleteForm(tmplFile, datafile)
	})

	ctx.Step(`^I save image bytes "([^"]*)" to "([^"]*)" as "([^"]*)"$`, func(hex, tmplFile, name string) error {
		bytes := []byte(hex)
		w.saveImageRes = w.m.SaveImageFile(tmplFile, name, bytes)
		return nil
	})

	ctx.Step(`^I request the extended list for "([^"]*)"$`, func(tmplFile string) error {
		out, err := w.m.ExtendedListForms(tmplFile)
		if err != nil {
			return err
		}
		w.extendedList = out
		return nil
	})

	ctx.Step(`^I capture the form's id$`, func() error {
		f := w.m.LoadForm("basic.yaml", "form-1")
		if f == nil {
			return fmt.Errorf("form-1 missing")
		}
		w.capturedID = f.Meta.ID
		return nil
	})

	// ── Thens ─────────────────────────────────────────────────────────

	ctx.Step(`^the form list is empty$`, func() error {
		if len(w.formList) != 0 {
			return fmt.Errorf("expected empty list, got %v", w.formList)
		}
		return nil
	})

	ctx.Step(`^the form list for "([^"]*)" contains "([^"]*)"$`, func(tmplFile, name string) error {
		out, err := w.m.ListForms(tmplFile)
		if err != nil {
			return err
		}
		if !slices.Contains(out, name) {
			return fmt.Errorf("%q not in %v", name, out)
		}
		return nil
	})

	ctx.Step(`^the form list for "([^"]*)" does not contain "([^"]*)"$`, func(tmplFile, name string) error {
		out, err := w.m.ListForms(tmplFile)
		if err != nil {
			return err
		}
		if slices.Contains(out, name) {
			return fmt.Errorf("%q should NOT be in %v", name, out)
		}
		return nil
	})

	ctx.Step(`^loading "([^"]*)" / "([^"]*)" returns data field "([^"]*)" equal to "([^"]*)"$`, func(tmplFile, datafile, key, want string) error {
		f := w.m.LoadForm(tmplFile, datafile)
		if f == nil {
			return fmt.Errorf("form not loaded")
		}
		got, ok := f.Data[key].(string)
		if !ok || got != want {
			return fmt.Errorf("data[%s] = %v, want %q", key, f.Data[key], want)
		}
		return nil
	})

	ctx.Step(`^loading "([^"]*)" / "([^"]*)" returns data field "([^"]*)" equal to (\d+)$`, func(tmplFile, datafile, key string, want int) error {
		f := w.m.LoadForm(tmplFile, datafile)
		if f == nil {
			return fmt.Errorf("form not loaded")
		}
		switch v := f.Data[key].(type) {
		case int:
			if v != want {
				return fmt.Errorf("data[%s] = %d, want %d", key, v, want)
			}
		case float64:
			if int(v) != want {
				return fmt.Errorf("data[%s] = %v, want %d", key, v, want)
			}
		default:
			return fmt.Errorf("data[%s] is %T, not number", key, v)
		}
		return nil
	})

	ctx.Step(`^the loaded form's meta has a non-empty "([^"]*)" timestamp$`, func(field string) error {
		if w.loaded == nil {
			// load on demand for this scenario chain
			w.loaded = w.m.LoadForm("basic.yaml", "form-1")
		}
		if w.loaded == nil {
			return fmt.Errorf("form not loaded")
		}
		var v string
		switch field {
		case "created":
			v = w.loaded.Meta.Created.At
		case "updated":
			v = w.loaded.Meta.Updated.At
		default:
			return fmt.Errorf("unknown meta field %q", field)
		}
		if v == "" {
			return fmt.Errorf("expected non-empty %s", field)
		}
		return nil
	})

	ctx.Step(`^the loaded form is nil$`, func() error {
		if w.loaded != nil {
			return fmt.Errorf("expected nil, got %+v", w.loaded)
		}
		return nil
	})

	ctx.Step(`^the file "([^"]*)" exists$`, func(path string) error {
		if _, err := os.Stat(filepath.Join(w.tmp, path)); err != nil {
			return fmt.Errorf("expected %q to exist: %v", path, err)
		}
		return nil
	})

	ctx.Step(`^the saved image result is success$`, func() error {
		if !w.saveImageRes.Success {
			return fmt.Errorf("expected success, got %+v", w.saveImageRes)
		}
		return nil
	})

	ctx.Step(`^the extended list has (\d+) entries$`, func(want int) error {
		if len(w.extendedList) != want {
			return fmt.Errorf("extended list len = %d, want %d", len(w.extendedList), want)
		}
		return nil
	})

	ctx.Step(`^the extended entry for "([^"]*)" has title "([^"]*)"$`, func(filename, want string) error {
		for _, e := range w.extendedList {
			if e.Filename == filename {
				if e.Title != want {
					return fmt.Errorf("title for %q = %q, want %q", filename, e.Title, want)
				}
				return nil
			}
		}
		return fmt.Errorf("filename %q not in extended list", filename)
	})

	ctx.Step(`^the form's id matches the captured id$`, func() error {
		f := w.m.LoadForm("basic.yaml", "form-1")
		if f == nil {
			return fmt.Errorf("form not loaded")
		}
		if f.Meta.ID != w.capturedID {
			return fmt.Errorf("id changed from %q to %q", w.capturedID, f.Meta.ID)
		}
		return nil
	})

	ctx.Step(`^the save returned an error$`, func() error {
		if w.saveResult.Success {
			return fmt.Errorf("expected save failure, got success: %+v", w.saveResult)
		}
		return nil
	})
}

func loadFormByDatafile(w *storageWorld) *Form {
	if w.lastTpl == "" || w.lastDatafile == "" {
		return nil
	}
	return w.m.LoadForm(w.lastTpl, w.lastDatafile)
}

func tableToData(table *godog.Table) map[string]any {
	rows := table.Rows
	if len(rows) < 2 {
		return map[string]any{}
	}
	headers := make(map[string]int, len(rows[0].Cells))
	for i, c := range rows[0].Cells {
		headers[c.Value] = i
	}
	out := map[string]any{}
	for _, r := range rows[1:] {
		k := r.Cells[headers["key"]].Value
		v := r.Cells[headers["value"]].Value
		out[k] = v
	}
	return out
}
