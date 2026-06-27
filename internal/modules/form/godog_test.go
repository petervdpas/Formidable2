package form

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/cucumber/godog"
	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initFormScenario,
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

type formWorld struct {
	tmp     string
	tplM    *template.Manager
	stoM    *storage.Manager
	m       *Manager
	view    *FormView
	saved   *FormView
	saveErr error
	copied  *FormView
	copyErr error
}

func initFormScenario(ctx *godog.ScenarioContext) {
	w := &formWorld{}

	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		dir, err := os.MkdirTemp("", "form-godog-")
		if err != nil {
			return ctx, err
		}
		*w = formWorld{tmp: dir}
		return ctx, nil
	})

	ctx.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w.tmp != "" {
			_ = os.RemoveAll(w.tmp)
		}
		return ctx, nil
	})

	// ── Background ────────────────────────────────────────────────────

	ctx.Step(`^a real form stack on a temp directory$`, func() error {
		sys := system.NewManager(w.tmp, nil)
		w.tplM = template.NewManager(sys, "templates", nil)
		sfrM := sfr.NewManager(sys, nil)
		w.stoM = storage.NewManager(sys, sfrM, w.tplM, "storage", nil)
		w.m = NewManager(w.tplM, w.stoM, nil, nil)
		return nil
	})

	// ── Template setup helpers ────────────────────────────────────────

	saveTpl := func(filename string, fields []template.Field) error {
		return w.tplM.SaveTemplate(filename, &template.Template{
			Name: filename, Filename: filename, Fields: fields,
		})
	}

	ctx.Step(`^a template "([^"]*)" with a text field "([^"]*)"$`, func(file, key string) error {
		return saveTpl(file, []template.Field{{Key: key, Type: "text"}})
	})

	ctx.Step(`^a template "([^"]*)" with a link field "([^"]*)"$`, func(file, key string) error {
		return saveTpl(file, []template.Field{{Key: key, Type: "link"}})
	})

	ctx.Step(`^a template "([^"]*)" with a date field "([^"]*)"$`, func(file, key string) error {
		return saveTpl(file, []template.Field{{Key: key, Type: "date"}})
	})

	ctx.Step(`^a template "([^"]*)" with a guid field "([^"]*)" and a text field "([^"]*)"$`,
		func(file, guidKey, textKey string) error {
			return saveTpl(file, []template.Field{
				{Key: guidKey, Type: "guid"},
				{Key: textKey, Type: "text"},
			})
		})

	ctx.Step(`^a template "([^"]*)" with a loop "([^"]*)" containing field "([^"]*)" of type "([^"]*)"$`,
		func(file, loopKey, innerKey, innerType string) error {
			return saveTpl(file, []template.Field{
				{Key: loopKey, Type: "loopstart"},
				{Key: innerKey, Type: innerType},
				{Key: loopKey, Type: "loopstop"},
			})
		})

	// ── Whens ─────────────────────────────────────────────────────────

	ctx.Step(`^I build the view for template "([^"]*)" with no datafile$`,
		func(tpl string) error {
			v, err := w.m.BuildView(tpl, "")
			if err != nil {
				return err
			}
			w.view = v
			return nil
		})

	tableToValues := func(table *godog.Table) map[string]any {
		m := map[string]any{}
		for _, r := range table.Rows[1:] {
			m[r.Cells[0].Value] = r.Cells[1].Value
		}
		return m
	}

	ctx.Step(`^I save form "([^"]*)" under "([^"]*)" with values:$`,
		func(datafile, tpl string, table *godog.Table) error {
			payload := SavePayload{Datafile: datafile, Values: tableToValues(table)}
			res, err := w.m.SaveValues(tpl, payload)
			w.saveErr = err
			if err == nil {
				w.saved = res
			}
			return nil
		})

	ctx.Step(`^I save form "([^"]*)" under "([^"]*)" with link "([^"]*)" of href "([^"]*)" and text "([^"]*)"$`,
		func(datafile, tpl, key, href, text string) error {
			payload := SavePayload{
				Datafile: datafile,
				Values: map[string]any{
					key: map[string]any{"href": href, "text": text},
				},
			}
			res, err := w.m.SaveValues(tpl, payload)
			w.saveErr = err
			if err == nil {
				w.saved = res
			}
			return nil
		})

	ctx.Step(`^I save form "([^"]*)" under "([^"]*)" with loop "([^"]*)" entries:$`,
		func(datafile, tpl, loopKey string, table *godog.Table) error {
			headers := []string{}
			for _, c := range table.Rows[0].Cells {
				headers = append(headers, c.Value)
			}
			entries := []any{}
			for _, r := range table.Rows[1:] {
				entry := map[string]any{}
				for i, c := range r.Cells {
					entry[headers[i]] = c.Value
				}
				entries = append(entries, entry)
			}
			payload := SavePayload{
				Datafile: datafile,
				Values:   map[string]any{loopKey: entries},
			}
			res, err := w.m.SaveValues(tpl, payload)
			w.saveErr = err
			if err == nil {
				w.saved = res
			}
			return nil
		})

	ctx.Step(`^I delete form "([^"]*)" under "([^"]*)"$`, func(datafile, tpl string) error {
		return w.m.DeleteForm(tpl, datafile)
	})

	ctx.Step(`^I copy form "([^"]*)" to "([^"]*)" under "([^"]*)"$`,
		func(src, dst, tpl string) error {
			v, err := w.m.CopyForm(tpl, src, dst)
			w.copyErr = err
			if err == nil {
				w.copied = v
			}
			return nil
		})

	// ── Thens ─────────────────────────────────────────────────────────

	ctx.Step(`^the view is unsaved$`, func() error {
		if w.view == nil {
			return fmt.Errorf("no view built")
		}
		if w.view.Saved {
			return fmt.Errorf("view should be unsaved")
		}
		return nil
	})

	ctx.Step(`^the view value "([^"]*)" is "([^"]*)"$`, func(key, want string) error {
		if w.view == nil {
			return fmt.Errorf("no view")
		}
		got, ok := w.view.Values[key]
		if !ok {
			return fmt.Errorf("key %q absent", key)
		}
		s := fmt.Sprintf("%v", got)
		if s != want {
			return fmt.Errorf("%q = %q, want %q", key, s, want)
		}
		return nil
	})

	ctx.Step(`^the saved view value "([^"]*)" is "([^"]*)"$`, func(key, want string) error {
		if w.saved == nil {
			return fmt.Errorf("no saved view (saveErr=%v)", w.saveErr)
		}
		got := fmt.Sprintf("%v", w.saved.Values[key])
		if got != want {
			return fmt.Errorf("%q = %q, want %q", key, got, want)
		}
		return nil
	})

	ctx.Step(`^the saved link "([^"]*)" has href "([^"]*)"$`, func(key, want string) error {
		obj, ok := saveLinkObj(w, key)
		if !ok {
			return fmt.Errorf("link %q not an object", key)
		}
		if got, _ := obj["href"].(string); got != want {
			return fmt.Errorf("href = %q, want %q", got, want)
		}
		return nil
	})

	ctx.Step(`^the saved link "([^"]*)" has text "([^"]*)"$`, func(key, want string) error {
		obj, ok := saveLinkObj(w, key)
		if !ok {
			return fmt.Errorf("link %q not an object", key)
		}
		if got, _ := obj["text"].(string); got != want {
			return fmt.Errorf("text = %q, want %q", got, want)
		}
		return nil
	})

	ctx.Step(`^reopening "([^"]*)" yields title "([^"]*)"$`,
		func(datafile, want string) error {
			v, err := w.m.BuildView("basic.yaml", datafile)
			if err != nil {
				return err
			}
			if got := fmt.Sprintf("%v", v.Values["title"]); got != want {
				return fmt.Errorf("title = %q, want %q", got, want)
			}
			return nil
		})

	ctx.Step(`^reopening "([^"]*)" returns an unsaved view$`,
		func(datafile string) error {
			v, err := w.m.BuildView("basic.yaml", datafile)
			if err != nil {
				return err
			}
			if v.Saved {
				return fmt.Errorf("expected unsaved view, got saved=%v", v)
			}
			return nil
		})

	ctx.Step(`^reopening "([^"]*)" yields loop "([^"]*)" with (\d+) entries$`,
		func(datafile, loopKey string, want int) error {
			v, err := w.m.BuildView("loops.yaml", datafile)
			if err != nil {
				return err
			}
			arr, ok := v.Values[loopKey].([]any)
			if !ok {
				return fmt.Errorf("loop %q not an array: %T", loopKey, v.Values[loopKey])
			}
			if len(arr) != want {
				return fmt.Errorf("loop %q size = %d, want %d", loopKey, len(arr), want)
			}
			w.view = v
			return nil
		})

	ctx.Step(`^loop "([^"]*)" entry (\d+) has name "([^"]*)"$`,
		func(loopKey string, idx int, want string) error {
			arr, ok := w.view.Values[loopKey].([]any)
			if !ok {
				return fmt.Errorf("loop missing")
			}
			entry, ok := arr[idx].(map[string]any)
			if !ok {
				return fmt.Errorf("entry %d not a map", idx)
			}
			if got := fmt.Sprintf("%v", entry["name"]); got != want {
				return fmt.Errorf("entry[%d].name = %q, want %q", idx, got, want)
			}
			return nil
		})

	ctx.Step(`^the copy has a fresh id$`, func() error {
		if w.copied == nil {
			return fmt.Errorf("no copied view (copyErr=%v)", w.copyErr)
		}
		if w.copied.Meta.ID == "" {
			return fmt.Errorf("copy has no id")
		}
		if w.saved == nil {
			return fmt.Errorf("no source view to compare against")
		}
		if w.copied.Meta.ID == w.saved.Meta.ID {
			return fmt.Errorf("copy id %q must differ from source id", w.copied.Meta.ID)
		}
		return nil
	})

	ctx.Step(`^the copy value "([^"]*)" is "([^"]*)"$`, func(key, want string) error {
		if w.copied == nil {
			return fmt.Errorf("no copied view (copyErr=%v)", w.copyErr)
		}
		if got := fmt.Sprintf("%v", w.copied.Values[key]); got != want {
			return fmt.Errorf("copy %q = %q, want %q", key, got, want)
		}
		return nil
	})

	ctx.Step(`^the original "([^"]*)" under "([^"]*)" keeps its id$`,
		func(datafile, tpl string) error {
			if w.saved == nil {
				return fmt.Errorf("no source view captured")
			}
			v, err := w.m.BuildView(tpl, datafile)
			if err != nil {
				return err
			}
			if v.Meta.ID != w.saved.Meta.ID {
				return fmt.Errorf("original id changed: %q -> %q", w.saved.Meta.ID, v.Meta.ID)
			}
			return nil
		})

	ctx.Step(`^the save returns an error$`, func() error {
		if w.saveErr == nil {
			return fmt.Errorf("expected save error, got nil; saved=%+v", w.saved)
		}
		return nil
	})

	ctx.Step(`^listing forms under "([^"]*)" yields (\d+) entries$`,
		func(tpl string, want int) error {
			summaries, err := w.m.ListForms(tpl)
			if err != nil {
				return err
			}
			if len(summaries) != want {
				return fmt.Errorf("got %d summaries, want %d", len(summaries), want)
			}
			return nil
		})
}

func saveLinkObj(w *formWorld, key string) (map[string]any, bool) {
	if w.saved == nil {
		return nil, false
	}
	obj, ok := w.saved.Values[key].(map[string]any)
	return obj, ok
}
