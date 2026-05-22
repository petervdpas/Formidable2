package monitor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/petervdpas/formidable2/internal/modules/system"
)

// fakeCtx satisfies journalContext for tests without dragging in the
// journal package.
type fakeCtx struct{ folder string }

func (f *fakeCtx) ContextFolder() string { return f.folder }

// writeJournalLog plants a .changes.log file at root with the given
// raw body. system.Manager isn't used for the write because we want to
// control exact JSONL content (including malformed lines).
func writeJournalLog(t *testing.T, root, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, ".changes.log"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────

func TestTemplateFromPath(t *testing.T) {
	cases := map[string]string{
		"templates/recepten.yaml":              "recepten",
		"templates/people.yaml":                "people",
		"storage/recepten/brood.meta.json":     "recepten",
		"storage/recepten/images/photo.png":    "recepten",
		"templates":                            "",
		"templates/":                           "",
		"storage":                              "",
		"random.txt":                           "",
		"":                                     "",
	}
	for in, want := range cases {
		if got := templateFromPath(in); got != want {
			t.Errorf("templateFromPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseJournalLine_HandlesMalformed(t *testing.T) {
	cases := []string{
		"",
		"   ",
		"not json",
		`{"ts":"","op":"create","path":"x"}`,
		`{"ts":"bogus","op":"create","path":"x"}`,
		`{"op":"create"}`,
		`{"ts":"2026-05-09T10:00:00Z"}`,
	}
	for _, line := range cases {
		if _, ok := parseJournalLine(line); ok {
			t.Errorf("expected malformed reject for %q", line)
		}
	}
}

func TestParseJournalLine_GoodMutation(t *testing.T) {
	line := `{"ts":"2026-05-09T10:00:00Z","op":"create","path":"templates/recepten.yaml","bytes":42}`
	ev, ok := parseJournalLine(line)
	if !ok {
		t.Fatal("expected parse success")
	}
	if ev.Dims["op"] != "create" {
		t.Errorf("op = %q", ev.Dims["op"])
	}
	if ev.Dims["template"] != "recepten" {
		t.Errorf("template = %q, want recepten", ev.Dims["template"])
	}
	if ev.Dims["path"] != "templates/recepten.yaml" {
		t.Errorf("path = %q", ev.Dims["path"])
	}
	if ev.Value != 1 {
		t.Errorf("value = %v, want 1", ev.Value)
	}
}

func TestParseJournalLine_SyncEntry(t *testing.T) {
	line := `{"ts":"2026-05-09T10:00:00Z","op":"sync","backend":"git","version":"abc","pushed":3}`
	ev, ok := parseJournalLine(line)
	if !ok {
		t.Fatal("expected parse success")
	}
	if ev.Dims["op"] != "sync" || ev.Dims["backend"] != "git" {
		t.Errorf("dims = %+v", ev.Dims)
	}
	if _, hasPath := ev.Dims["path"]; hasPath {
		t.Errorf("sync entry should not carry path: %+v", ev.Dims)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Source-level behavior
// ─────────────────────────────────────────────────────────────────────

func TestJournalSource_NoContextReturnsNoEvents(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	src := NewJournalSource(&fakeCtx{folder: ""}, sys)
	events := src.Events(time.Time{}, time.Time{})
	if len(events) != 0 {
		t.Errorf("expected 0 events with empty context, got %d", len(events))
	}
}

func TestJournalSource_MissingLogReturnsNoEvents(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	src := NewJournalSource(&fakeCtx{folder: root}, sys)
	if got := src.Events(time.Time{}, time.Time{}); len(got) != 0 {
		t.Errorf("missing log should yield no events, got %d", len(got))
	}
}

func TestJournalSource_NilDepsAreSafe(t *testing.T) {
	src := NewJournalSource(nil, nil)
	if got := src.Events(time.Time{}, time.Time{}); got != nil {
		t.Errorf("nil deps should return nil, got %v", got)
	}
	// Source identity still works.
	if src.Name() != "journal" {
		t.Errorf("Name() = %q", src.Name())
	}
}

func TestJournalSource_ProjectsValidEntriesAndSkipsBad(t *testing.T) {
	root := t.TempDir()
	body := strings.Join([]string{
		`{"ts":"2026-05-09T10:00:00Z","op":"create","path":"templates/recepten.yaml"}`,
		`not-a-line`,
		`{"ts":"2026-05-09T11:00:00Z","op":"sync","backend":"git","version":"abc","pushed":1}`,
		``,
		`{"ts":"2026-05-09T12:00:00Z","op":"update","path":"storage/recepten/brood.meta.json"}`,
	}, "\n")
	writeJournalLog(t, root, body)

	sys := system.NewManager(root, nil)
	src := NewJournalSource(&fakeCtx{folder: root}, sys)
	got := src.Events(time.Time{}, time.Time{})
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (got: %+v)", len(got), got)
	}
	if got[0].Dims["op"] != "create" {
		t.Errorf("first op = %q", got[0].Dims["op"])
	}
	if got[1].Dims["op"] != "sync" || got[1].Dims["backend"] != "git" {
		t.Errorf("second event dims = %+v", got[1].Dims)
	}
	if got[2].Dims["template"] != "recepten" {
		t.Errorf("third event template = %q", got[2].Dims["template"])
	}
}

func TestJournalSource_ClipsToFromTo(t *testing.T) {
	root := t.TempDir()
	body := strings.Join([]string{
		`{"ts":"2026-05-09T08:00:00Z","op":"create","path":"templates/a.yaml"}`,
		`{"ts":"2026-05-09T10:00:00Z","op":"update","path":"templates/a.yaml"}`,
		`{"ts":"2026-05-09T12:00:00Z","op":"delete","path":"templates/a.yaml"}`,
	}, "\n")
	writeJournalLog(t, root, body)

	sys := system.NewManager(root, nil)
	src := NewJournalSource(&fakeCtx{folder: root}, sys)

	from := time.Date(2026, 5, 9, 9, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 9, 11, 0, 0, 0, time.UTC)
	got := src.Events(from, to)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (just the 10:00 entry)", len(got))
	}
	if got[0].Dims["op"] != "update" {
		t.Errorf("op = %q", got[0].Dims["op"])
	}
}

func TestJournalSource_EndToEndWithManager(t *testing.T) {
	// Wire JournalSource into a Manager and run a Query - proves the
	// Source/Manager seam works end-to-end with realistic data.
	root := t.TempDir()
	body := strings.Join([]string{
		`{"ts":"2026-05-09T10:00:00Z","op":"create","path":"templates/recepten.yaml"}`,
		`{"ts":"2026-05-09T10:30:00Z","op":"create","path":"storage/recepten/brood.meta.json"}`,
		`{"ts":"2026-05-09T11:00:00Z","op":"update","path":"storage/recepten/brood.meta.json"}`,
	}, "\n")
	writeJournalLog(t, root, body)

	sys := system.NewManager(root, nil)
	mgr := NewManager()
	mgr.Register(NewJournalSource(&fakeCtx{folder: root}, sys))

	res, err := mgr.Run(Query{Source: "journal", GroupBy: []string{"op"}})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"create": 2, "update": 1}
	for _, s := range res.Series {
		op := s.Key["op"]
		if s.Total != want[op] {
			t.Errorf("op=%s total = %v, want %v", op, s.Total, want[op])
		}
	}
}

func TestJournalSource_ReportsDeclaredDimsAndIdentity(t *testing.T) {
	src := NewJournalSource(nil, nil)
	if src.Name() != "journal" {
		t.Errorf("Name() = %q", src.Name())
	}
	if src.Kind() != "mutation" {
		t.Errorf("Kind() = %q", src.Kind())
	}
	want := []string{"op", "backend", "path", "template"}
	if got := src.Dims(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Dims() = %v, want %v", got, want)
	}
}
