package connection

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"net/http/httptest"

	"github.com/cucumber/godog"
	"gopkg.in/yaml.v3"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initConnectionScenario,
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

type connWorld struct {
	fs      *memFS
	mgr     *Manager
	spec    SpecInfo
	specErr error
	saveErr error
	lastID  string

	remote  *remote
	srv     *httptest.Server
	inv     *Invoker
	page    *Page
	item    *Item
	callErr error
}

func initConnectionScenario(ctx *godog.ScenarioContext) {
	w := &connWorld{}

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		*w = connWorld{}
		return ctx, nil
	})

	ctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		if w.srv != nil {
			w.srv.Close()
		}
		return ctx, nil
	})

	ctx.Step(`^an empty connection store$`, func() error {
		w.fs = newMemFS()
		w.mgr = NewManager(w.fs, nil)
		return nil
	})

	ctx.Step(`^the spec "([^"]*)" is imported:$`, func(name string, doc *godog.DocString) error {
		w.spec, w.specErr = w.mgr.ImportSpec(name, []byte(doc.Content))
		return w.specErr
	})

	ctx.Step(`^I import the spec "([^"]*)":$`, func(name string, doc *godog.DocString) error {
		w.spec, w.specErr = w.mgr.ImportSpec(name, []byte(doc.Content))
		return nil
	})

	ctx.Step(`^the import succeeded$`, func() error {
		if w.specErr != nil {
			return fmt.Errorf("import failed: %w", w.specErr)
		}
		return nil
	})

	ctx.Step(`^the import failed$`, func() error {
		if w.specErr == nil {
			return errors.New("expected the import to fail")
		}
		return nil
	})

	ctx.Step(`^the spec is stored as "([^"]*)"$`, func(file string) error {
		if w.spec.File != file {
			return fmt.Errorf("spec file = %q, want %q", w.spec.File, file)
		}
		if !w.fs.FileExists("api-clients/specs/" + file) {
			return fmt.Errorf("%q is not on disk", file)
		}
		return nil
	})

	ctx.Step(`^the catalog has (\d+) operations$`, func(n int) error {
		if got := len(w.spec.Catalog.Operations); got != n {
			return fmt.Errorf("operation count = %d, want %d", got, n)
		}
		return nil
	})

	ctx.Step(`^the catalog lists operation "([^"]*)"$`, func(id string) error {
		if _, ok := w.spec.Catalog.Op(id); !ok {
			return fmt.Errorf("operation %q is not in the catalog", id)
		}
		return nil
	})

	ctx.Step(`^I save the connection:$`, func(doc *godog.DocString) error {
		var c Connection
		if err := yaml.Unmarshal([]byte(doc.Content), &c); err != nil {
			return fmt.Errorf("scenario connection is not valid YAML: %w", err)
		}
		w.lastID = c.ID
		w.saveErr = w.mgr.Save(&c)
		return nil
	})

	ctx.Step(`^the save succeeded$`, func() error {
		if w.saveErr != nil {
			return fmt.Errorf("save failed: %w", w.saveErr)
		}
		return nil
	})

	ctx.Step(`^the save was rejected$`, func() error {
		if w.saveErr == nil {
			return errors.New("expected the save to be rejected")
		}
		return nil
	})

	ctx.Step(`^the save failed with "([^"]*)"$`, func(want string) error {
		var vfe *ValidationFailedError
		if !errors.As(w.saveErr, &vfe) {
			return fmt.Errorf("want a validation failure, got %v", w.saveErr)
		}
		if !hasType(vfe.Errors, want) {
			return fmt.Errorf("want a %q issue, got: %s", want, types(vfe.Errors))
		}
		return nil
	})

	ctx.Step(`^connection "([^"]*)" is listed as ok$`, func(id string) error {
		list, err := w.mgr.List()
		if err != nil {
			return err
		}
		for _, s := range list {
			if s.ID != id {
				continue
			}
			if !s.OK {
				return fmt.Errorf("connection %q is not ok: %s", id, s.Error)
			}
			return nil
		}
		return fmt.Errorf("connection %q is not listed", id)
	})

	ctx.Step(`^no connection "([^"]*)" is stored$`, func(id string) error {
		if w.fs.FileExists("api-clients/" + id + ".yaml") {
			return fmt.Errorf("connection %q is still on disk", id)
		}
		return nil
	})

	ctx.Step(`^I delete connection "([^"]*)"$`, func(id string) error {
		return w.mgr.Delete(id)
	})

	ctx.Step(`^the spec "([^"]*)" is still stored$`, func(file string) error {
		if !w.fs.FileExists("api-clients/specs/" + file) {
			return fmt.Errorf("spec %q was removed while still in use", file)
		}
		return nil
	})

	ctx.Step(`^the spec "([^"]*)" is gone$`, func(file string) error {
		if w.fs.FileExists("api-clients/specs/" + file) {
			return fmt.Errorf("spec %q outlived its last connection", file)
		}
		return nil
	})

	initInvokeSteps(ctx, w)

	ctx.Step(`^the stored definition does not contain a secret$`, func() error {
		raw, err := w.fs.LoadFile("api-clients/" + w.lastID + ".yaml")
		if err != nil {
			return err
		}
		for _, word := range []string{"secret", "token", "password", "credential"} {
			if strings.Contains(strings.ToLower(raw), word) {
				return fmt.Errorf("definition mentions %q; secrets belong in the keychain:\n%s", word, raw)
			}
		}
		return nil
	})
}
