package pdf

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"time"

	"github.com/cucumber/godog"
)

// coverImagesWorld carries state across the cover-image-library godog
// scenarios. Mirrors coverArchiveWorld's shape: each scenario gets a
// fresh memFS-backed Manager/Service pair so background assertions
// don't bleed across scenarios.
type coverImagesWorld struct {
	svc       *Service
	mgr       *Manager
	mem       *memFS
	actionErr error

	// Last List / Load result snapshot - set by When-steps, read by
	// the matching Then-steps.
	listed []CoverImageDescriptor
	loaded []byte
}

func (w *coverImagesWorld) reset() { *w = coverImagesWorld{} }

func initCoverImagesScenario(ctx *godog.ScenarioContext) {
	w := &coverImagesWorld{}

	ctx.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		w.reset()
		return ctx, nil
	})

	scaffold := func() error {
		w.mem = newMemFS()
		w.mgr = &Manager{
			log:    slog.Default(),
			store:  &store{fs: w.mem, log: slog.Default()},
			status: Status{Source: SourceUnset},
			nowFn:  func() time.Time { return time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC) },
		}
		w.svc = NewService(w.mgr)
		return scaffoldCovers(w.mem, slog.Default())
	}

	ctx.Step(`^a pdf cover image library scaffolded on disk$`, scaffold)

	ctx.Step(`^the cover image library is scaffolded again$`, func() error {
		return scaffoldCovers(w.mem, slog.Default())
	})

	ctx.Step(`^a cover image "([^"]*)" exists with bytes "([^"]*)"$`, func(name, body string) error {
		path := onDiskCoversDir + "/" + coverImagesSubdir + "/" + name
		w.mem.files[path] = body
		return nil
	})

	ctx.Step(`^I ListCoverImages$`, func() error {
		w.listed, w.actionErr = w.svc.ListCoverImages()
		return nil
	})

	ctx.Step(`^I SaveCoverImage "([^"]*)" with bytes "([^"]*)"$`, func(name, body string) error {
		enc := base64.StdEncoding.EncodeToString([]byte(body))
		w.actionErr = w.svc.SaveCoverImage(name, enc)
		w.listed, _ = w.svc.ListCoverImages()
		return nil
	})

	ctx.Step(`^I LoadCoverImage "([^"]*)"$`, func(name string) error {
		enc, err := w.svc.LoadCoverImage(name)
		w.actionErr = err
		if err == nil {
			raw, decErr := base64.StdEncoding.DecodeString(enc)
			if decErr != nil {
				return fmt.Errorf("decode base64 from LoadCoverImage: %w", decErr)
			}
			w.loaded = raw
		}
		return nil
	})

	ctx.Step(`^I DeleteCoverImage "([^"]*)"$`, func(name string) error {
		w.actionErr = w.svc.DeleteCoverImage(name)
		w.listed, _ = w.svc.ListCoverImages()
		return nil
	})

	ctx.Step(`^the cover image action returned no error$`, func() error {
		if w.actionErr != nil {
			return fmt.Errorf("got %v, want nil", w.actionErr)
		}
		return nil
	})

	ctx.Step(`^the cover image action returned an error$`, func() error {
		if w.actionErr == nil {
			return fmt.Errorf("got nil, want an error")
		}
		return nil
	})

	ctx.Step(`^the cover image list contains "([^"]*)"$`, func(name string) error {
		got, err := w.svc.ListCoverImages()
		if err != nil {
			return err
		}
		for _, e := range got {
			if e.Name == name {
				return nil
			}
		}
		return fmt.Errorf("cover image %q not in list %v", name, descNames(got))
	})

	ctx.Step(`^the cover image list does not contain "([^"]*)"$`, func(name string) error {
		got, err := w.svc.ListCoverImages()
		if err != nil {
			return err
		}
		for _, e := range got {
			if e.Name == name {
				return fmt.Errorf("cover image %q still present in list %v", name, descNames(got))
			}
		}
		return nil
	})

	ctx.Step(`^cover image "([^"]*)" is flagged as a seed$`, func(name string) error {
		return assertSeedFlag(w.svc, name, true)
	})

	ctx.Step(`^cover image "([^"]*)" is not flagged as a seed$`, func(name string) error {
		return assertSeedFlag(w.svc, name, false)
	})

	ctx.Step(`^the loaded cover image bytes equal "([^"]*)"$`, func(want string) error {
		if string(w.loaded) != want {
			return fmt.Errorf("loaded = %q, want %q", w.loaded, want)
		}
		return nil
	})
}

func descNames(in []CoverImageDescriptor) []string {
	out := make([]string, 0, len(in))
	for _, e := range in {
		out = append(out, e.Name)
	}
	return out
}

func assertSeedFlag(svc *Service, name string, wantSeed bool) error {
	got, err := svc.ListCoverImages()
	if err != nil {
		return err
	}
	for _, e := range got {
		if e.Name == name {
			if e.IsSeed != wantSeed {
				return fmt.Errorf("cover image %q IsSeed = %v, want %v",
					name, e.IsSeed, wantSeed)
			}
			return nil
		}
	}
	return fmt.Errorf("cover image %q not found while checking seed flag (have %v)",
		name, descNames(got))
}

