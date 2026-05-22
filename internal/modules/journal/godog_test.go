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
		w.m.RecordSync(backend, version, pushed, 0)
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

	ctx.Step(`^the cursor map has no entry for backend "([^"]*)"$`, func(backend string) error {
		if _, ok := w.m.ReadCursor()[backend]; ok {
			return fmt.Errorf("cursor map unexpectedly contains backend %q: %+v", backend, w.m.ReadCursor())
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

	// ── ensureGitignorePatterns scenarios ─────────────────────────────

	ctx.Step(`^the temp dir is the repo root with a \.git directory$`, func() error {
		return os.MkdirAll(filepath.Join(w.tmp, ".git"), 0o755)
	})

	ctx.Step(`^the context is a subdirectory "([^"]*)" with no gitignore$`, func(rel string) error {
		return os.MkdirAll(filepath.Join(w.tmp, rel), 0o755)
	})

	ctx.Step(`^I configure the journal pointing at the subdirectory with backend "([^"]*)"$`, func(backend string) error {
		// Expects the prior step to have created a "ctx" subdir under
		// w.tmp. The walk-up logic should still find the repo root via
		// the .git in w.tmp.
		return w.m.Configure(filepath.Join(w.tmp, "ctx"), backend)
	})

	ctx.Step(`^the gitignore at "([^"]*)" contains pattern "([^"]*)"$`, func(rel, pattern string) error {
		body, err := os.ReadFile(filepath.Join(w.tmp, rel))
		if err != nil {
			return fmt.Errorf("read %q: %w", rel, err)
		}
		for line := range strings.SplitSeq(string(body), "\n") {
			if line == pattern {
				return nil
			}
		}
		return fmt.Errorf("pattern %q not found as a line in %q:\n%s", pattern, rel, string(body))
	})

	ctx.Step(`^the gitignore at "([^"]*)" still contains "([^"]*)"$`, func(rel, want string) error {
		body, err := os.ReadFile(filepath.Join(w.tmp, rel))
		if err != nil {
			return fmt.Errorf("read %q: %w", rel, err)
		}
		if !strings.Contains(string(body), want) {
			return fmt.Errorf("missing %q in %q:\n%s", want, rel, string(body))
		}
		return nil
	})

	ctx.Step(`^the gitignore at "([^"]*)" starts with "([^"]*)"$`, func(rel, want string) error {
		decoded := strings.ReplaceAll(want, `\n`, "\n")
		body, err := os.ReadFile(filepath.Join(w.tmp, rel))
		if err != nil {
			return fmt.Errorf("read %q: %w", rel, err)
		}
		if !strings.HasPrefix(string(body), decoded) {
			return fmt.Errorf("expected prefix %q in:\n%s", decoded, string(body))
		}
		return nil
	})

	ctx.Step(`^the gitignore at the repo root contains pattern "([^"]*)"$`, func(pattern string) error {
		body, err := os.ReadFile(filepath.Join(w.tmp, ".gitignore"))
		if err != nil {
			return fmt.Errorf("read repo-root gitignore: %w", err)
		}
		for line := range strings.SplitSeq(string(body), "\n") {
			if line == pattern {
				return nil
			}
		}
		return fmt.Errorf("pattern %q not found as a line in repo-root gitignore:\n%s", pattern, string(body))
	})

	ctx.Step(`^no gitignore was created in the subdirectory$`, func() error {
		ctxGi := filepath.Join(w.tmp, "ctx", ".gitignore")
		if _, err := os.Stat(ctxGi); err == nil {
			return fmt.Errorf("subdirectory gitignore unexpectedly created at %q", ctxGi)
		}
		return nil
	})

	ctx.Step(`^no gitignore exists at "([^"]*)"$`, func(rel string) error {
		gi := filepath.Join(w.tmp, rel)
		if _, err := os.Stat(gi); err == nil {
			return fmt.Errorf("gitignore at %q should not exist", rel)
		}
		return nil
	})

	ctx.Step(`^the on-disk cursor file contains "([^"]*)"$`, func(want string) error {
		body, err := os.ReadFile(filepath.Join(w.tmp, ".changes.cursor"))
		if err != nil {
			return fmt.Errorf("read cursor file: %w", err)
		}
		// Scenario uses escaped quotes - accept literal embedded \"
		// from the feature file (godog passes the captured text raw).
		decoded := strings.ReplaceAll(want, `\"`, `"`)
		if !strings.Contains(string(body), decoded) {
			return fmt.Errorf("cursor file missing %q (decoded %q):\n%s", want, decoded, string(body))
		}
		return nil
	})

	ctx.Step(`^pattern "([^"]*)" appears exactly once in "([^"]*)"$`, func(pattern, rel string) error {
		body, err := os.ReadFile(filepath.Join(w.tmp, rel))
		if err != nil {
			return fmt.Errorf("read %q: %w", rel, err)
		}
		// Line-by-line equality so substring matches inside other
		// patterns (e.g. .changes.* inside **/.changes.*) don't
		// register as duplicates.
		got := 0
		for line := range strings.SplitSeq(string(body), "\n") {
			if line == pattern {
				got++
			}
		}
		if got != 1 {
			return fmt.Errorf("pattern %q appears %d times in %q (want 1):\n%s", pattern, got, rel, string(body))
		}
		return nil
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
