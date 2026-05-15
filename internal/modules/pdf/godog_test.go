package pdf

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: func(ctx *godog.ScenarioContext) {
			initPDFScenario(ctx)
			initFrontmatterScenario(ctx)
		},
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

type pdfWorld struct {
	svc       *Service
	mgr       *Manager
	mem       *memFS
	fs        fakeFS
	vers      fakeVersions
	status    Status
	actionErr error
	result    Result
}

func (w *pdfWorld) reset() { *w = pdfWorld{} }

func initPDFScenario(ctx *godog.ScenarioContext) {
	w := &pdfWorld{}

	ctx.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		w.reset()
		return ctx, nil
	})

	ctx.Step(`^a pdf service with a fresh manager$`, func() error {
		w.mem = newMemFS()
		w.fs = fakeFS{}
		w.vers = fakeVersions{}
		w.mgr = &Manager{
			log:    slog.Default(),
			store:  &store{fs: w.mem, log: slog.Default()},
			prober: &prober{fs: w.fs, versions: w.vers, goos: "linux", cacheRoot: "/cache/rod/browser"},
			nowFn:  func() time.Time { return time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC) },
			status: Status{Source: SourceUnset},
		}
		w.svc = NewService(w.mgr)
		return nil
	})

	ctx.Step(`^a fake browser exists at "([^"]*)" reporting version "([^"]*)"$`, func(path, ver string) error {
		w.fs[path] = true
		w.vers[path] = struct {
			version string
			err     error
		}{version: ver, err: nil}
		return nil
	})

	ctx.Step(`^the service has been activated with BrowserBin "([^"]*)"$`, func(path string) error {
		_, err := w.svc.Activate(ActivateOpts{BrowserBin: path})
		return err
	})

	ctx.Step(`^I ask Status from the service$`, func() error {
		w.status = w.svc.GetStatus()
		return nil
	})

	ctx.Step(`^I Activate through the service with no overrides$`, func() error {
		w.status, w.actionErr = w.svc.Activate(ActivateOpts{})
		return nil
	})

	ctx.Step(`^I Activate through the service with BrowserBin "([^"]*)"$`, func(path string) error {
		w.status, w.actionErr = w.svc.Activate(ActivateOpts{BrowserBin: path})
		return nil
	})

	ctx.Step(`^I Deactivate through the service$`, func() error {
		w.actionErr = w.svc.Deactivate()
		w.status = w.svc.GetStatus()
		return nil
	})

	ctx.Step(`^I ExportPDF through the service for form "([^"]*)" with no options$`, func(guid string) error {
		w.result, w.actionErr = w.svc.ExportPDF(guid, ExportOpts{})
		w.status = w.svc.GetStatus()
		return nil
	})

	ctx.Step(`^the status reports not active$`, func() error {
		if w.status.Active {
			return fmt.Errorf("status.Active = true, want false")
		}
		return nil
	})

	ctx.Step(`^the status reports active$`, func() error {
		if !w.status.Active {
			return fmt.Errorf("status.Active = false, want true")
		}
		return nil
	})

	ctx.Step(`^the status source is "([^"]*)"$`, func(want string) error {
		if string(w.status.Source) != want {
			return fmt.Errorf("status.Source = %q, want %q", w.status.Source, want)
		}
		return nil
	})

	ctx.Step(`^the status browser bin is empty$`, func() error {
		if w.status.BrowserBin != "" {
			return fmt.Errorf("status.BrowserBin = %q, want empty", w.status.BrowserBin)
		}
		return nil
	})

	ctx.Step(`^the status browser bin is "([^"]*)"$`, func(want string) error {
		if w.status.BrowserBin != want {
			return fmt.Errorf("status.BrowserBin = %q, want %q", w.status.BrowserBin, want)
		}
		return nil
	})

	ctx.Step(`^the status version is empty$`, func() error {
		if w.status.Version != "" {
			return fmt.Errorf("status.Version = %q, want empty", w.status.Version)
		}
		return nil
	})

	ctx.Step(`^the service action returned ErrPDFNotActivated$`, func() error {
		if !errors.Is(w.actionErr, ErrPDFNotActivated) {
			return fmt.Errorf("got %v, want ErrPDFNotActivated", w.actionErr)
		}
		return nil
	})

	ctx.Step(`^the service action returned ErrNoBrowserFound$`, func() error {
		if !errors.Is(w.actionErr, ErrNoBrowserFound) {
			return fmt.Errorf("got %v, want ErrNoBrowserFound", w.actionErr)
		}
		return nil
	})

	ctx.Step(`^the service action returned ErrInvalidBrowserBin$`, func() error {
		if !errors.Is(w.actionErr, ErrInvalidBrowserBin) {
			return fmt.Errorf("got %v, want ErrInvalidBrowserBin", w.actionErr)
		}
		return nil
	})

	ctx.Step(`^the service action returned no error$`, func() error {
		if w.actionErr != nil {
			return fmt.Errorf("got err %v, want nil", w.actionErr)
		}
		return nil
	})

	ctx.Step(`^the export result is empty$`, func() error {
		if w.result != (Result{}) {
			return fmt.Errorf("result = %+v, want zero value", w.result)
		}
		return nil
	})
}
