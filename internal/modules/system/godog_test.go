package system

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// TestFeatures wires the godog suite into `go test`. Discovers .feature
// files in ./features automatically.
func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initSystemScenario,
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

// systemWorld holds the per-scenario state. Each scenario gets a fresh
// instance via Before so cases can't leak into each other.
type systemWorld struct {
	tmp      string
	m        *Manager
	journal  *captureJournal
	lastErr  error
	lastLoad string
}

type captureJournal struct {
	ops []string
}

func (c *captureJournal) RecordOp(op, path string, _ map[string]any) {
	c.ops = append(c.ops, op)
	_ = path
}

func initSystemScenario(ctx *godog.ScenarioContext) {
	w := &systemWorld{}

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		w.tmp = ""
		w.m = nil
		w.journal = nil
		w.lastErr = nil
		w.lastLoad = ""
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
		dir, err := os.MkdirTemp("", "system-godog-")
		if err != nil {
			return err
		}
		w.tmp = dir
		w.m = NewManager(dir, nil)
		return nil
	})

	ctx.Step(`^the file "([^"]*)" with content "([^"]*)"$`, func(path, content string) error {
		return w.m.SaveFile(path, content)
	})

	ctx.Step(`^a journal stub is wired$`, func() error {
		w.journal = &captureJournal{}
		w.m.SetJournal(w.journal)
		return nil
	})

	// ── Whens ─────────────────────────────────────────────────────────

	ctx.Step(`^I save "([^"]*)" with content "([^"]*)"$`, func(path, content string) error {
		w.lastErr = w.m.SaveFile(path, content)
		return nil
	})

	ctx.Step(`^I delete "([^"]*)"$`, func(path string) error {
		w.lastErr = w.m.DeleteFile(path)
		return nil
	})

	ctx.Step(`^I copy "([^"]*)" to "([^"]*)" with overwrite (true|false)$`, func(from, to, ov string) error {
		w.lastErr = w.m.CopyFile(from, to, ov == "true")
		return nil
	})

	ctx.Step(`^I empty the folder "([^"]*)"$`, func(path string) error {
		w.lastErr = w.m.EmptyFolder(path)
		return nil
	})

	ctx.Step(`^I delete the folder "([^"]*)"$`, func(path string) error {
		w.lastErr = w.m.DeleteFolder(path)
		return nil
	})

	ctx.Step(`^I append to "([^"]*)" the content "([^"]*)"$`, func(path, content string) error {
		decoded := strings.ReplaceAll(content, `\n`, "\n")
		w.lastErr = w.m.AppendFile(path, decoded)
		return nil
	})

	ctx.Step(`^I load "([^"]*)"$`, func(path string) error {
		_, w.lastErr = w.m.LoadFile(path)
		return nil
	})

	ctx.Step(`^I execute the command "([^"]*)"$`, func(cmd string) error {
		_, w.lastErr = w.m.ExecuteCommand(cmd)
		return nil
	})

	// ── Thens ─────────────────────────────────────────────────────────

	ctx.Step(`^the file "([^"]*)" exists$`, func(path string) error {
		if !w.m.FileExists(path) {
			return fmt.Errorf("expected file %q to exist", path)
		}
		return nil
	})

	ctx.Step(`^the file "([^"]*)" does not exist$`, func(path string) error {
		if w.m.FileExists(path) {
			return fmt.Errorf("expected file %q to NOT exist", path)
		}
		return nil
	})

	ctx.Step(`^loading "([^"]*)" returns "([^"]*)"$`, func(path, want string) error {
		got, err := w.m.LoadFile(path)
		if err != nil {
			return err
		}
		decoded := strings.ReplaceAll(want, `\n`, "\n")
		if got != decoded {
			return fmt.Errorf("loaded %q, want %q", got, decoded)
		}
		w.lastLoad = got
		return nil
	})

	ctx.Step(`^no error occurred$`, func() error {
		if w.lastErr != nil {
			return fmt.Errorf("expected no error, got: %v", w.lastErr)
		}
		return nil
	})

	ctx.Step(`^an error occurred$`, func() error {
		if w.lastErr == nil {
			return fmt.Errorf("expected an error, got nil")
		}
		return nil
	})

	ctx.Step(`^the load returns an error$`, func() error {
		if w.lastErr == nil {
			return fmt.Errorf("expected load error, got nil")
		}
		return nil
	})

	ctx.Step(`^the folder "([^"]*)" does not exist$`, func(path string) error {
		full := w.m.ResolvePath(path)
		if _, err := os.Stat(full); err == nil {
			return fmt.Errorf("expected folder %q to NOT exist", path)
		}
		return nil
	})

	ctx.Step(`^the folder "([^"]*)" exists$`, func(path string) error {
		full := w.m.ResolvePath(path)
		info, err := os.Stat(full)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return errors.New("expected directory")
		}
		return nil
	})

	ctx.Step(`^the folder "([^"]*)" is empty$`, func(path string) error {
		entries, err := os.ReadDir(w.m.ResolvePath(path))
		if err != nil {
			return err
		}
		if len(entries) != 0 {
			return fmt.Errorf("expected empty, got %d entries", len(entries))
		}
		return nil
	})

	ctx.Step(`^the journal recorded operations ([a-z, ]+)$`, func(list string) error {
		want := splitCSV(list)
		if w.journal == nil {
			return errors.New("journal not wired")
		}
		if len(w.journal.ops) != len(want) {
			return fmt.Errorf("journal got %v, want %v", w.journal.ops, want)
		}
		for i, op := range w.journal.ops {
			if op != want[i] {
				return fmt.Errorf("journal[%d] = %q, want %q", i, op, want[i])
			}
		}
		return nil
	})
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
