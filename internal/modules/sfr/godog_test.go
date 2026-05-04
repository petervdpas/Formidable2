package sfr

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/cucumber/godog"
	"github.com/petervdpas/formidable2/internal/modules/system"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initSfrScenario,
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

type sfrWorld struct {
	tmp        string
	sys        *system.Manager
	m          *Manager
	saveResult SaveResult
	listResult []string
	loadResult any
	loadErr    error
}

func initSfrScenario(ctx *godog.ScenarioContext) {
	w := &sfrWorld{}

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		dir, err := os.MkdirTemp("", "sfr-godog-")
		if err != nil {
			return ctx, err
		}
		w.tmp = dir
		w.sys = nil
		w.m = nil
		w.saveResult = SaveResult{}
		w.listResult = nil
		w.loadResult = nil
		w.loadErr = nil
		return ctx, nil
	})

	ctx.After(func(ctx context.Context, sc *godog.Scenario, _ error) (context.Context, error) {
		if w.tmp != "" {
			_ = os.RemoveAll(w.tmp)
		}
		return ctx, nil
	})

	// ── Givens ────────────────────────────────────────────────────────

	ctx.Step(`^a system manager rooted at a temp directory$`, func() error {
		w.sys = system.NewManager(w.tmp, nil)
		return nil
	})

	ctx.Step(`^an sfr manager wrapping that system$`, func() error {
		w.m = NewManager(w.sys, nil)
		return nil
	})

	ctx.Step(`^the file "([^"]*)" with content "([^"]*)"$`, func(path, content string) error {
		return w.sys.SaveFile(path, content)
	})

	ctx.Step(`^a saved entry under "([^"]*)" with base "([^"]*)" and data (.*)$`, func(dir, base, dataJSON string) error {
		var data any
		if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
			return err
		}
		r := w.m.SaveFromBase(dir, base, data, Options{})
		if !r.Success {
			return fmt.Errorf("seed save failed: %s", r.Error)
		}
		return nil
	})

	// ── Whens ─────────────────────────────────────────────────────────

	ctx.Step(`^I save under "([^"]*)" with base "([^"]*)" and data (.*)$`, func(dir, base, dataJSON string) error {
		var data any
		if dataJSON != "" {
			if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
				return err
			}
		}
		w.saveResult = w.m.SaveFromBase(dir, base, data, Options{})
		return nil
	})

	ctx.Step(`^I load under "([^"]*)" with base "([^"]*)"$`, func(dir, base string) error {
		w.loadResult, w.loadErr = w.m.LoadFromBase(dir, base, Options{})
		return nil
	})

	ctx.Step(`^I delete under "([^"]*)" with base "([^"]*)"$`, func(dir, base string) error {
		return w.m.DeleteFromBase(dir, base, Options{})
	})

	ctx.Step(`^I list files under "([^"]*)"$`, func(dir string) error {
		out, err := w.m.ListFiles(dir, "")
		if err != nil {
			return err
		}
		w.listResult = out
		return nil
	})

	ctx.Step(`^I list files under "([^"]*)" with extension "([^"]*)"$`, func(dir, ext string) error {
		out, err := w.m.ListFiles(dir, ext)
		if err != nil {
			return err
		}
		w.listResult = out
		return nil
	})

	// ── Thens ─────────────────────────────────────────────────────────

	ctx.Step(`^the file "([^"]*)" exists$`, func(path string) error {
		if _, err := os.Stat(filepath.Join(w.tmp, path)); err != nil {
			return fmt.Errorf("expected %q to exist: %v", path, err)
		}
		return nil
	})

	ctx.Step(`^the file "([^"]*)" does not exist$`, func(path string) error {
		if _, err := os.Stat(filepath.Join(w.tmp, path)); err == nil {
			return fmt.Errorf("expected %q to NOT exist", path)
		}
		return nil
	})

	ctx.Step(`^the loaded JSON has field "([^"]*)" equal to "([^"]*)"$`, func(field, want string) error {
		obj, ok := w.loadResult.(map[string]any)
		if !ok {
			return fmt.Errorf("loaded result is not a JSON object: %T", w.loadResult)
		}
		got, ok := obj[field].(string)
		if !ok {
			return fmt.Errorf("field %q is not a string: %v", field, obj[field])
		}
		if got != want {
			return fmt.Errorf("field %q = %q, want %q", field, got, want)
		}
		return nil
	})

	ctx.Step(`^the loaded JSON has field "([^"]*)" equal to (\d+)$`, func(field string, want int) error {
		obj, ok := w.loadResult.(map[string]any)
		if !ok {
			return fmt.Errorf("loaded result is not a JSON object: %T", w.loadResult)
		}
		// JSON numbers come through as float64
		gotF, ok := obj[field].(float64)
		if !ok {
			return fmt.Errorf("field %q is not a number: %v", field, obj[field])
		}
		if int(gotF) != want {
			return fmt.Errorf("field %q = %v, want %d", field, gotF, want)
		}
		return nil
	})

	ctx.Step(`^the list contains "([^"]*)"$`, func(name string) error {
		if !slices.Contains(w.listResult, name) {
			return fmt.Errorf("%q not in %v", name, w.listResult)
		}
		return nil
	})

	ctx.Step(`^the list does not contain "([^"]*)"$`, func(name string) error {
		if slices.Contains(w.listResult, name) {
			return fmt.Errorf("%q should NOT be in %v", name, w.listResult)
		}
		return nil
	})

	ctx.Step(`^the load returns an error$`, func() error {
		if w.loadErr == nil {
			return fmt.Errorf("expected an error, got nil; result=%v", w.loadResult)
		}
		return nil
	})

	ctx.Step(`^the save result is a failure$`, func() error {
		if w.saveResult.Success {
			return fmt.Errorf("expected save failure, got success: %+v", w.saveResult)
		}
		return nil
	})
}
