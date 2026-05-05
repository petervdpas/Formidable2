package i18n

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"testing"

	"github.com/cucumber/godog"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initI18nScenario,
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

type i18nWorld struct {
	m         *Manager
	bundle    map[string]any
	locales   []string
	lastErr   error
}

func initI18nScenario(ctx *godog.ScenarioContext) {
	w := &i18nWorld{}

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		w.m = nil
		w.bundle = nil
		w.locales = nil
		w.lastErr = nil
		return ctx, nil
	})

	ctx.Step(`^a fresh i18n manager$`, func() error {
		m, err := NewManager(nil)
		if err != nil {
			return err
		}
		w.m = m
		return nil
	})

	ctx.Step(`^the default locale is "([^"]*)"$`, func(want string) error {
		got := w.m.DefaultLocale()
		if got != want {
			return fmt.Errorf("DefaultLocale = %q, want %q", got, want)
		}
		return nil
	})

	ctx.Step(`^the locale "([^"]*)" is available$`, func(loc string) error {
		if !w.m.HasLocale(loc) {
			return fmt.Errorf("locale %q not loaded", loc)
		}
		return nil
	})

	ctx.Step(`^I load the bundle for "([^"]*)"$`, func(loc string) error {
		w.bundle, w.lastErr = w.m.LoadBundle(loc)
		return nil
	})

	ctx.Step(`^the bundle contains key "([^"]*)"$`, func(key string) error {
		if w.bundle == nil {
			return errors.New("no bundle loaded")
		}
		if _, ok := w.bundle[key]; !ok {
			return fmt.Errorf("bundle missing key %q", key)
		}
		return nil
	})

	ctx.Step(`^an i18n error occurred$`, func() error {
		if w.lastErr == nil {
			return errors.New("expected an error, got nil")
		}
		return nil
	})

	ctx.Step(`^I list available locales$`, func() error {
		w.locales = w.m.AvailableLocales()
		return nil
	})

	ctx.Step(`^the locale list is sorted alphabetically$`, func() error {
		if !sort.StringsAreSorted(w.locales) {
			return fmt.Errorf("not sorted: %v", w.locales)
		}
		return nil
	})

	ctx.Step(`^the locale list contains "([^"]*)"$`, func(loc string) error {
		if !slices.Contains(w.locales, loc) {
			return fmt.Errorf("locale %q not in %v", loc, w.locales)
		}
		return nil
	})
}
