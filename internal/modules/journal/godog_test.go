package journal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/cucumber/godog"
	"github.com/petervdpas/formidable2/internal/modules/system"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initJournalScenario,
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

// captureEmitter records every Emit call so scenarios can assert on
// the events the journal raised.
type captureEmitter struct {
	mu     sync.Mutex
	events []capturedEvent
}

type capturedEvent struct {
	name string
	data Entry
}

func (c *captureEmitter) Emit(name string, data any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := data.(Entry); ok {
		c.events = append(c.events, capturedEvent{name: name, data: entry})
	}
}

func (c *captureEmitter) snapshot() []capturedEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]capturedEvent, len(c.events))
	copy(out, c.events)
	return out
}

type journalWorld struct {
	tmp        string
	sys        *system.Manager
	m          *Manager
	emitter    *captureEmitter
	initResult InitResult
	secondInit InitResult
}

func initJournalScenario(ctx *godog.ScenarioContext) {
	w := &journalWorld{}

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		dir, err := os.MkdirTemp("", "journal-godog-")
		if err != nil {
			return ctx, err
		}
		w.tmp = dir
		w.sys = nil
		w.m = nil
		w.emitter = nil
		w.initResult = InitResult{}
		w.secondInit = InitResult{}
		return ctx, nil
	})

	ctx.After(func(ctx context.Context, sc *godog.Scenario, _ error) (context.Context, error) {
		if w.tmp != "" {
			_ = os.RemoveAll(w.tmp)
		}
		return ctx, nil
	})

	// ── Background ────────────────────────────────────────────────────

	ctx.Step(`^a system manager rooted at a temp directory$`, func() error {
		w.sys = system.NewManager(w.tmp, nil)
		return nil
	})

	ctx.Step(`^a journal manager wrapping that system$`, func() error {
		w.m = NewManager(w.sys, nil, nil)
		return nil
	})

	// ── Givens ────────────────────────────────────────────────────────

	ctx.Step(`^the file "([^"]*)" with content "([^"]*)"$`, func(path, content string) error {
		// Resolve "literal-style" \n inside scenario strings to actual newlines.
		decoded := strings.ReplaceAll(content, `\n`, "\n")
		return w.sys.SaveFile(path, decoded)
	})

	ctx.Step(`^the file "([^"]*)" with content '([^']*)'$`, func(path, content string) error {
		decoded := strings.ReplaceAll(content, `\n`, "\n")
		return w.sys.SaveFile(path, decoded)
	})

	ctx.Step(`^an event sink is wired$`, func() error {
		w.emitter = &captureEmitter{}
		w.m = NewManager(w.sys, nil, w.emitter)
		return nil
	})

	// ── Whens ─────────────────────────────────────────────────────────

	ctx.Step(`^I configure the journal with backend "([^"]*)"$`, func(backend string) error {
		return w.m.Configure(w.tmp, backend)
	})

	ctx.Step(`^I initialize the journal$`, func() error {
		if w.initResult.Created || w.initResult.Reason != "" {
			w.secondInit = w.m.Init()
			return nil
		}
		w.initResult = w.m.Init()
		return nil
	})

	ctx.Step(`^I record op "([^"]*)" for "([^"]*)"$`, func(op, path string) error {
		full := filepath.Join(w.tmp, path)
		w.m.RecordOp(op, full, nil)
		return nil
	})

	ctx.Step(`^I record sync for backend "([^"]*)" with version "([^"]*)" and pushed (\d+)$`, func(backend, version string, pushed int) error {
		w.m.RecordSync(SyncRecord{Backend: backend, Version: version, Pushed: pushed})
		return nil
	})

	ctx.Step(`^I record remote seen for backend "([^"]*)" with version "([^"]*)"$`, func(backend, version string) error {
		w.m.RecordRemoteSeen(backend, version)
		return nil
	})

	// ── Thens ─────────────────────────────────────────────────────────

	ctx.Step(`^the file "([^"]*)" exists$`, func(path string) error {
		if _, err := os.Stat(filepath.Join(w.tmp, path)); err != nil {
			return fmt.Errorf("expected %q to exist: %v", path, err)
		}
		return nil
	})

	ctx.Step(`^the cursor for backend "([^"]*)" has ts "([^"]*)"$`, func(backend, want string) error {
		cur := w.m.ReadCursor()[backend]
		if cur.Ts != want {
			return fmt.Errorf("cursor[%s].ts = %q, want %q", backend, cur.Ts, want)
		}
		return nil
	})

	ctx.Step(`^the cursor for backend "([^"]*)" has version "([^"]*)"$`, func(backend, want string) error {
		cur := w.m.ReadCursor()[backend]
		if cur.Version != want {
			return fmt.Errorf("cursor[%s].version = %q, want %q", backend, cur.Version, want)
		}
		return nil
	})

	ctx.Step(`^the init result reports (\d+) entries created$`, func(want int) error {
		if w.initResult.Entries != want {
			return fmt.Errorf("init entries = %d, want %d", w.initResult.Entries, want)
		}
		if !w.initResult.Created {
			return fmt.Errorf("expected created=true, got false")
		}
		return nil
	})

	ctx.Step(`^the init result reason is "([^"]*)"$`, func(want string) error {
		if w.initResult.Reason != want {
			return fmt.Errorf("init reason = %q, want %q", w.initResult.Reason, want)
		}
		if w.initResult.Created {
			return fmt.Errorf("expected created=false when reason is %q", want)
		}
		return nil
	})

	ctx.Step(`^the second init reports created false$`, func() error {
		if w.secondInit.Created {
			return fmt.Errorf("expected second Init created=false, got true")
		}
		return nil
	})

	ctx.Step(`^the journal contains (\d+) baseline entries$`, func(want int) error {
		entries, err := readEntries(filepath.Join(w.tmp, ".changes.log"))
		if err != nil {
			return err
		}
		count := 0
		for _, e := range entries {
			if e.Op == "baseline" {
				count++
			}
		}
		if count != want {
			return fmt.Errorf("baseline count = %d, want %d", count, want)
		}
		return nil
	})

	ctx.Step(`^the baseline entries cover "([^"]*)"$`, func(path string) error {
		entries, err := readEntries(filepath.Join(w.tmp, ".changes.log"))
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.Op == "baseline" && e.Path == path {
				return nil
			}
		}
		return fmt.Errorf("baseline missing for %q (entries: %v)", path, paths(entries))
	})

	ctx.Step(`^pending for backend "([^"]*)" contains (\d+) entries$`, func(backend string, want int) error {
		got := w.m.Pending(backend)
		if got.Count != want {
			return fmt.Errorf("pending[%s].count = %d, want %d (paths=%v)", backend, got.Count, want, got.Paths)
		}
		if len(got.Paths) != want {
			return fmt.Errorf("pending[%s].paths len = %d, want %d", backend, len(got.Paths), want)
		}
		return nil
	})

	ctx.Step(`^pending for backend "([^"]*)" includes "([^"]*)" with op "([^"]*)"$`, func(backend, path, op string) error {
		got := w.m.Pending(backend)
		for _, p := range got.Paths {
			if p.Path == path && p.Op == op {
				return nil
			}
		}
		return fmt.Errorf("pending[%s] missing %q with op %q (have %v)", backend, path, op, got.Paths)
	})

	ctx.Step(`^the event sink received "([^"]*)" with op "([^"]*)" and path "([^"]*)"$`, func(eventName, op, path string) error {
		if w.emitter == nil {
			return fmt.Errorf("emitter not wired")
		}
		for _, ev := range w.emitter.snapshot() {
			if ev.name == eventName && ev.data.Op == op && ev.data.Path == path {
				return nil
			}
		}
		return fmt.Errorf("no matching event in %v", w.emitter.snapshot())
	})

	ctx.Step(`^the event sink received "([^"]*)" with op "([^"]*)" and backend "([^"]*)"$`, func(eventName, op, backend string) error {
		if w.emitter == nil {
			return fmt.Errorf("emitter not wired")
		}
		for _, ev := range w.emitter.snapshot() {
			if ev.name == eventName && ev.data.Op == op && ev.data.Backend == backend {
				return nil
			}
		}
		return fmt.Errorf("no matching event in %v", w.emitter.snapshot())
	})
}

func readEntries(path string) ([]Entry, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []Entry
	for line := range strings.SplitSeq(string(body), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		entry, err := parseLine(line)
		if err != nil || entry == nil {
			continue
		}
		out = append(out, *entry)
	}
	return out, nil
}

func paths(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Path)
	}
	return out
}

// silence linter on slices import in case scenarios don't all use it
var _ = slices.Contains[[]string]
