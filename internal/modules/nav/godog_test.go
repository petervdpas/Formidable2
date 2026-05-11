package nav

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/cucumber/godog"
	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initNavScenario,
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

// recordingConfig is a configWriter that captures every UpdateUserConfig
// call so scenarios can assert on what the nav Manager wrote.
type recordingConfig struct {
	mu      sync.Mutex
	updates []map[string]any
}

func (c *recordingConfig) UpdateUserConfig(p map[string]any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Defensive copy — shared maps could surprise us across scenarios.
	cp := make(map[string]any, len(p))
	for k, v := range p {
		cp[k] = v
	}
	c.updates = append(c.updates, cp)
	return nil
}

type recordedEvent struct {
	name string
	data any
}

type recordingEmitter struct {
	mu     sync.Mutex
	events []recordedEvent
}

func (e *recordingEmitter) Emit(name string, data any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, recordedEvent{name, data})
}

type navWorld struct {
	tmp      string
	sys      *system.Manager
	tplM     *template.Manager
	stoM     *storage.Manager
	cfg      *recordingConfig
	emit     *recordingEmitter
	m        *Manager
	parsed   *Target
	result   *Result
	resolveR *Result
}

func initNavScenario(ctx *godog.ScenarioContext) {
	w := &navWorld{}

	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		dir, err := os.MkdirTemp("", "nav-godog-")
		if err != nil {
			return ctx, err
		}
		*w = navWorld{tmp: dir}
		return ctx, nil
	})

	ctx.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w.tmp != "" {
			_ = os.RemoveAll(w.tmp)
		}
		return ctx, nil
	})

	// ── Background ────────────────────────────────────────────────────

	ctx.Step(`^a real system \+ template \+ storage stack$`, func() error {
		w.sys = system.NewManager(w.tmp, nil)
		w.tplM = template.NewManager(w.sys, "templates", nil)
		sfrM := sfr.NewManager(w.sys, nil)
		w.stoM = storage.NewManager(w.sys, sfrM, w.tplM, "storage", nil)
		w.cfg = &recordingConfig{}
		w.emit = &recordingEmitter{}
		w.m = NewManager(w.tplM, w.stoM, w.cfg, w.emit, nil, nil)
		return nil
	})

	ctx.Step(`^a template "([^"]*)" exists with a field "([^"]*)" of type "([^"]*)"$`,
		func(name, key, typ string) error {
			return w.tplM.SaveTemplate(name, &template.Template{
				Name: name, Filename: name,
				Fields: []template.Field{{Key: key, Type: typ}},
			})
		})

	ctx.Step(`^a form "([^"]*)" saved under "([^"]*)" with title "([^"]*)"$`,
		func(datafile, tpl, title string) error {
			r := w.stoM.SaveForm(tpl, datafile, map[string]any{"title": title})
			if !r.Success {
				return fmt.Errorf("save form failed: %s", r.Error)
			}
			return nil
		})

	// ── Parse ─────────────────────────────────────────────────────────

	ctx.Step(`^I parse "([^"]*)"$`, func(href string) error {
		w.parsed = ParseFormidableHref(href)
		return nil
	})

	ctx.Step(`^the parsed target template is "([^"]*)"$`, func(want string) error {
		if w.parsed == nil {
			return fmt.Errorf("parsed is nil")
		}
		if w.parsed.Template != want {
			return fmt.Errorf("template = %q, want %q", w.parsed.Template, want)
		}
		return nil
	})

	ctx.Step(`^the parsed target datafile is "([^"]*)"$`, func(want string) error {
		if w.parsed == nil {
			return fmt.Errorf("parsed is nil")
		}
		if w.parsed.Datafile != want {
			return fmt.Errorf("datafile = %q, want %q", w.parsed.Datafile, want)
		}
		return nil
	})

	ctx.Step(`^the parsed target fragment is "([^"]*)"$`, func(want string) error {
		if w.parsed == nil {
			return fmt.Errorf("parsed is nil")
		}
		if w.parsed.Fragment != want {
			return fmt.Errorf("fragment = %q, want %q", w.parsed.Fragment, want)
		}
		return nil
	})

	ctx.Step(`^the parsed target fragment is empty$`, func() error {
		if w.parsed == nil {
			return fmt.Errorf("parsed is nil")
		}
		if w.parsed.Fragment != "" {
			return fmt.Errorf("fragment should be empty, got %q", w.parsed.Fragment)
		}
		return nil
	})

	ctx.Step(`^the parse returns nil$`, func() error {
		if w.parsed != nil {
			return fmt.Errorf("expected nil, got %+v", w.parsed)
		}
		return nil
	})

	// ── Navigate ──────────────────────────────────────────────────────

	ctx.Step(`^I navigate to "([^"]*)"$`, func(href string) error {
		res, err := w.m.NavigateToFormidable(href)
		if err != nil {
			return err
		}
		w.result = res
		return nil
	})

	ctx.Step(`^the navigation succeeds$`, func() error {
		if w.result == nil || !w.result.Success {
			return fmt.Errorf("nav not successful: %+v", w.result)
		}
		return nil
	})

	ctx.Step(`^the navigation fails with a non-empty error$`, func() error {
		if w.result == nil {
			return fmt.Errorf("result is nil")
		}
		if w.result.Success {
			return fmt.Errorf("expected failure, got success")
		}
		if w.result.Error == "" {
			return fmt.Errorf("expected non-empty error message")
		}
		return nil
	})

	ctx.Step(`^the config reflects template "([^"]*)" and datafile "([^"]*)"$`,
		func(tpl, df string) error {
			if len(w.cfg.updates) == 0 {
				return fmt.Errorf("no config updates recorded")
			}
			latest := w.cfg.updates[len(w.cfg.updates)-1]
			if latest["selected_template"] != tpl {
				return fmt.Errorf("selected_template = %v, want %q", latest["selected_template"], tpl)
			}
			if latest["selected_data_file"] != df {
				return fmt.Errorf("selected_data_file = %v, want %q", latest["selected_data_file"], df)
			}
			return nil
		})

	ctx.Step(`^the config ribbon is "([^"]*)"$`, func(want string) error {
		if len(w.cfg.updates) == 0 {
			return fmt.Errorf("no config updates recorded")
		}
		latest := w.cfg.updates[len(w.cfg.updates)-1]
		if latest["context_ribbon"] != want {
			return fmt.Errorf("context_ribbon = %v, want %q", latest["context_ribbon"], want)
		}
		return nil
	})

	ctx.Step(`^a "([^"]*)" event was emitted with template "([^"]*)" and datafile "([^"]*)"$`,
		func(eventName, tpl, df string) error {
			for _, e := range w.emit.events {
				if e.name != eventName {
					continue
				}
				// Payload is *Target — accept either pointer or value-after-marshal.
				j, _ := json.Marshal(e.data)
				var got Target
				if err := json.Unmarshal(j, &got); err != nil {
					continue
				}
				if got.Template == tpl && got.Datafile == df {
					return nil
				}
			}
			return fmt.Errorf("no %q event with %q/%q in %v",
				eventName, tpl, df, w.emit.events)
		})

	ctx.Step(`^no config update was made$`, func() error {
		if len(w.cfg.updates) > 0 {
			return fmt.Errorf("expected no updates, got %d: %v",
				len(w.cfg.updates), w.cfg.updates)
		}
		return nil
	})

	ctx.Step(`^no "([^"]*)" event was emitted$`, func(name string) error {
		for _, e := range w.emit.events {
			if e.name == name {
				return fmt.Errorf("expected no %q events, got: %+v", name, w.emit.events)
			}
		}
		return nil
	})

	// ── Resolve ───────────────────────────────────────────────────────

	ctx.Step(`^I resolve "([^"]*)"$`, func(href string) error {
		res, err := w.m.ResolveFormidable(href)
		if err != nil {
			return err
		}
		w.resolveR = res
		return nil
	})

	ctx.Step(`^the resolution succeeds$`, func() error {
		if w.resolveR == nil || !w.resolveR.Success {
			return fmt.Errorf("resolve not successful: %+v", w.resolveR)
		}
		return nil
	})
}
