package fonts

import (
	"errors"
	"fmt"
	"os"
	"path"
	"testing"
	"testing/fstest"

	"github.com/cucumber/godog"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initFontsScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			Output:   os.Stdout,
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fail()
	}
}

type fontsWorld struct {
	fs      *memFS
	factory fstest.MapFS
	m       *Manager
	loaded  []byte
	lastErr error
}

func (w *fontsWorld) reset() {
	w.fs = newMemFS()
	w.factory = fstest.MapFS{}
	w.m = NewManager(w.fs)
	w.m.seedFS = w.factory // override the (empty) real embed with the scenario's
	w.loaded = nil
	w.lastErr = nil
}

func (w *fontsWorld) find(name string) (FontInfo, bool) {
	list, _ := w.m.List()
	for _, f := range list {
		if f.Filename == name {
			return f, true
		}
	}
	return FontInfo{}, false
}

func initFontsScenario(ctx *godog.ScenarioContext) {
	w := &fontsWorld{}

	ctx.Given(`^an empty fonts library$`, func() error { w.reset(); return nil })

	ctx.Given(`^a saved font "([^"]*)" with bytes "([^"]*)"$`, func(name, body string) error {
		return w.m.Save(name, []byte(body))
	})
	ctx.Given(`^a factory font "([^"]*)" with bytes "([^"]*)"$`, func(name, body string) error {
		w.factory[path.Join(factoryDir, name)] = &fstest.MapFile{Data: []byte(body)}
		return nil
	})

	ctx.When(`^I save font "([^"]*)" with bytes "([^"]*)"$`, func(name, body string) error {
		w.lastErr = w.m.Save(name, []byte(body))
		return nil
	})
	ctx.When(`^I load font "([^"]*)"$`, func(name string) error {
		w.loaded, w.lastErr = w.m.Load(name)
		return nil
	})
	ctx.When(`^I delete font "([^"]*)"$`, func(name string) error {
		w.lastErr = w.m.Delete(name)
		return nil
	})
	ctx.When(`^I scaffold the fonts library$`, func() error {
		w.lastErr = w.m.Scaffold()
		return nil
	})
	ctx.When(`^I restore default fonts$`, func() error {
		w.lastErr = w.m.Scaffold()
		return nil
	})

	ctx.Then(`^the font list is empty$`, func() error {
		if list, _ := w.m.List(); len(list) != 0 {
			return fmt.Errorf("expected empty list, got %d", len(list))
		}
		return nil
	})
	ctx.Then(`^the font list contains "([^"]*)"$`, func(name string) error {
		if _, ok := w.find(name); !ok {
			return fmt.Errorf("font %q not in list", name)
		}
		return nil
	})
	ctx.Then(`^the font list does not contain "([^"]*)"$`, func(name string) error {
		if _, ok := w.find(name); ok {
			return fmt.Errorf("font %q still in list", name)
		}
		return nil
	})
	ctx.Then(`^the font "([^"]*)" has family "([^"]*)"$`, func(name, family string) error {
		fi, ok := w.find(name)
		if !ok {
			return fmt.Errorf("font %q not in list", name)
		}
		if fi.Family != family {
			return fmt.Errorf("family = %q, want %q", fi.Family, family)
		}
		return nil
	})
	ctx.Then(`^the font "([^"]*)" is a seed$`, func(name string) error {
		fi, ok := w.find(name)
		if !ok {
			return fmt.Errorf("font %q not in list", name)
		}
		if !fi.IsSeed {
			return fmt.Errorf("font %q should be flagged as a seed", name)
		}
		return nil
	})
	ctx.Then(`^the loaded font bytes equal "([^"]*)"$`, func(body string) error {
		if string(w.loaded) != body {
			return fmt.Errorf("loaded %q, want %q", w.loaded, body)
		}
		return nil
	})
	ctx.Then(`^no font error occurred$`, func() error {
		if w.lastErr != nil {
			return fmt.Errorf("unexpected error: %v", w.lastErr)
		}
		return nil
	})
	ctx.Then(`^the font save is rejected as invalid$`, func() error {
		if !errors.Is(w.lastErr, ErrInvalidFont) {
			return fmt.Errorf("expected ErrInvalidFont, got %v", w.lastErr)
		}
		return nil
	})
}
