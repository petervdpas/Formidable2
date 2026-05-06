package wiki

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/cucumber/godog"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initWikiScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			Output:   colorWriter(),
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fail()
	}
}

func colorWriter() io.Writer {
	if w, ok := any(os.Stdout).(io.Writer); ok {
		return w
	}
	return io.Discard
}

// world is the per-scenario state — kept tiny on purpose. The manager
// under test, the second one used for the port-conflict scenario, and
// the loose values we thread between Given/When/Then.
type world struct {
	m            *Manager
	mSecond      *Manager
	startErr     error
	stopErr      error
	rememberPort int
}

func (w *world) reset() {
	if w.m != nil {
		_ = w.m.Stop()
	}
	if w.mSecond != nil {
		_ = w.mSecond.Stop()
	}
	*w = world{}
}

func initWikiScenario(ctx *godog.ScenarioContext) {
	w := &world{}

	ctx.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		w.reset()
		return ctx, nil
	})

	ctx.Step(`^a wiki manager$`, func() error {
		w.m = NewManager(nil)
		return nil
	})

	ctx.Step(`^a custom handler returning "([^"]*)"$`, func(body string) error {
		w.m.SetHandler(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(rw, body)
		}))
		return nil
	})

	ctx.Step(`^I start the server on a random port$`, func() error {
		w.startErr = w.m.Start(0)
		return nil
	})

	ctx.Step(`^the server has started on a random port$`, func() error {
		if err := w.m.Start(0); err != nil {
			return fmt.Errorf("seed start: %w", err)
		}
		return nil
	})

	ctx.Step(`^I remember the bound port$`, func() error {
		s := w.m.Status()
		if s.Port == 0 {
			return fmt.Errorf("no port to remember")
		}
		w.rememberPort = s.Port
		return nil
	})

	ctx.Step(`^I stop the server$`, func() error {
		w.stopErr = w.m.Stop()
		return nil
	})

	ctx.Step(`^I start the server on the remembered port$`, func() error {
		w.startErr = w.m.Start(w.rememberPort)
		return nil
	})

	ctx.Step(`^a second manager tries to start on the remembered port$`, func() error {
		w.mSecond = NewManager(nil)
		w.startErr = w.mSecond.Start(w.rememberPort)
		return nil
	})

	// ── Thens ─────────────────────────────────────────────────────────

	ctx.Step(`^the server is not running$`, func() error {
		if w.m.Status().Running {
			return fmt.Errorf("expected not running")
		}
		return nil
	})

	ctx.Step(`^the reported port is zero$`, func() error {
		if got := w.m.Status().Port; got != 0 {
			return fmt.Errorf("port = %d, want 0", got)
		}
		return nil
	})

	ctx.Step(`^the server is running$`, func() error {
		if !w.m.Status().Running {
			return fmt.Errorf("expected running, startErr=%v", w.startErr)
		}
		return nil
	})

	ctx.Step(`^the reported port is non-zero$`, func() error {
		if w.m.Status().Port == 0 {
			return fmt.Errorf("expected non-zero port")
		}
		return nil
	})

	ctx.Step(`^the started-at timestamp is set$`, func() error {
		if w.m.Status().StartedAt.IsZero() {
			return fmt.Errorf("expected non-zero StartedAt")
		}
		return nil
	})

	ctx.Step(`^HTTP GET on "([^"]*)" returns a response$`, func(path string) error {
		port := w.m.Status().Port
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d%s", port, path))
		if err != nil {
			return fmt.Errorf("get: %w", err)
		}
		_ = resp.Body.Close()
		return nil
	})

	ctx.Step(`^HTTP GET on "([^"]*)" returns body "([^"]*)"$`, func(path, want string) error {
		port := w.m.Status().Port
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d%s", port, path))
		if err != nil {
			return fmt.Errorf("get: %w", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read body: %w", err)
		}
		if string(body) != want {
			return fmt.Errorf("body = %q, want %q", body, want)
		}
		return nil
	})

	ctx.Step(`^HTTP GET on "([^"]*)" fails$`, func(path string) error {
		port := w.rememberPort
		if port == 0 {
			port = 1 // random invalid; we expect the GET to fail
		}
		_, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d%s", port, path))
		if err == nil {
			return fmt.Errorf("expected error, got nil")
		}
		return nil
	})

	ctx.Step(`^no error is returned$`, func() error {
		if w.startErr != nil {
			return fmt.Errorf("startErr = %v", w.startErr)
		}
		if w.stopErr != nil {
			return fmt.Errorf("stopErr = %v", w.stopErr)
		}
		return nil
	})

	ctx.Step(`^a start error is returned$`, func() error {
		if w.startErr == nil {
			return fmt.Errorf("expected start error, got nil")
		}
		return nil
	})

	ctx.Step(`^the bound port matches the remembered port$`, func() error {
		got := w.m.Status().Port
		if got != w.rememberPort {
			return fmt.Errorf("port = %d, want %d", got, w.rememberPort)
		}
		return nil
	})

	ctx.Step(`^the status is not running with port zero$`, func() error {
		s := w.m.Status()
		if s.Running {
			return fmt.Errorf("expected not running")
		}
		if s.Port != 0 {
			return fmt.Errorf("port = %d, want 0", s.Port)
		}
		return nil
	})
}
