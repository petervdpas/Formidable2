package csv

import (
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/system"
)

func newTestManager(t *testing.T) (*Manager, *system.Manager, string) {
	t.Helper()
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	return NewManager(sys, nil), sys, root
}

// ─────────────────────────────────────────────────────────────────────
// Pure helpers
// ─────────────────────────────────────────────────────────────────────

func TestPickDelimiter(t *testing.T) {
	cases := map[string]rune{
		"":   ',',
		",":  ',',
		";":  ';',
		"\t": '\t',
		"|":  '|',
	}
	for in, want := range cases {
		got := pickDelimiter(in)
		if got != want {
			t.Errorf("pickDelimiter(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPickDelimiter_MultiCharFallsToFirstRune(t *testing.T) {
	if got := pickDelimiter(",,,"); got != ',' {
		t.Errorf("pickDelimiter(\",,,\") = %q, want comma", got)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Preview edge cases not covered by the feature file
// ─────────────────────────────────────────────────────────────────────

func TestPreview_OnlyHeaderRow(t *testing.T) {
	m, sys, _ := newTestManager(t)
	_ = sys.SaveFile("h.csv", "a,b,c\n")
	pr, err := m.Preview("h.csv", ",")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if len(pr.Headers) != 3 {
		t.Errorf("headers = %v, want 3", pr.Headers)
	}
	if pr.RowCount != 0 {
		t.Errorf("rowCount = %d, want 0 (header-only)", pr.RowCount)
	}
}

func TestPreview_RaggedRowsAccepted(t *testing.T) {
	m, sys, _ := newTestManager(t)
	_ = sys.SaveFile("r.csv", "a,b,c\n1,2\n3,4,5,6\n")
	pr, err := m.Preview("r.csv", ",")
	if err != nil {
		t.Fatalf("Preview ragged: %v", err)
	}
	if pr.RowCount != 2 {
		t.Errorf("rowCount = %d, want 2", pr.RowCount)
	}
	if len(pr.Rows[0]) != 2 || len(pr.Rows[1]) != 4 {
		t.Errorf("ragged row lens = %d,%d", len(pr.Rows[0]), len(pr.Rows[1]))
	}
}

func TestPreview_PreservesEmptyCells(t *testing.T) {
	m, sys, _ := newTestManager(t)
	_ = sys.SaveFile("e.csv", "a,b,c\n,,\n1,,3\n")
	pr, _ := m.Preview("e.csv", ",")
	if len(pr.Rows) != 2 {
		t.Fatalf("rowCount = %d, want 2", pr.RowCount)
	}
	if pr.Rows[0][0] != "" || pr.Rows[0][1] != "" || pr.Rows[0][2] != "" {
		t.Errorf("empty row mangled: %v", pr.Rows[0])
	}
	if pr.Rows[1][1] != "" {
		t.Errorf("middle-empty cell mangled: %v", pr.Rows[1])
	}
}

func TestPreview_WhitespaceOnlyFile(t *testing.T) {
	m, sys, _ := newTestManager(t)
	_ = sys.SaveFile("w.csv", "   \n  \t\n")
	pr, err := m.Preview("w.csv", ",")
	if err != nil {
		t.Errorf("whitespace file should not error: %v", err)
	}
	if len(pr.Headers) != 0 || len(pr.Rows) != 0 {
		t.Errorf("expected empty result, got %+v", pr)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Write edge cases
// ─────────────────────────────────────────────────────────────────────

func TestWrite_QuotesNewlinesInsideCells(t *testing.T) {
	m, sys, _ := newTestManager(t)
	r := m.Write("nl.csv", [][]string{
		{"k", "v"},
		{"poem", "line1\nline2"},
	}, ",")
	if !r.Success {
		t.Fatalf("write: %+v", r)
	}
	body, _ := sys.LoadFile("nl.csv")
	if !strings.Contains(body, "\"line1\nline2\"") {
		t.Errorf("multiline cell not quoted: %q", body)
	}
}

func TestWrite_RoundTripPreservesData(t *testing.T) {
	m, _, _ := newTestManager(t)
	rows := [][]string{
		{"name", "address", "note"},
		{"Alice", "Main St, 1", `She said "hi"`},
		{"Bob", "Side Rd 2", ""},
	}
	r := m.Write("rt.csv", rows, ",")
	if !r.Success {
		t.Fatalf("write: %+v", r)
	}
	pr, err := m.Preview("rt.csv", ",")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if pr.RowCount != 2 {
		t.Errorf("rowCount = %d, want 2", pr.RowCount)
	}
	if pr.Rows[0][1] != "Main St, 1" || pr.Rows[0][2] != `She said "hi"` {
		t.Errorf("round-trip lost data: %v", pr.Rows[0])
	}
}

// ─────────────────────────────────────────────────────────────────────
// FS-failure path
// ─────────────────────────────────────────────────────────────────────

type stubFS struct {
	loadCalled bool
	loadErr    error
	saveCalled bool
	saveErr    error
	saved      string
}

func (s *stubFS) LoadFile(path string) (string, error) {
	s.loadCalled = true
	return "", s.loadErr
}

func (s *stubFS) SaveFile(path, content string) error {
	s.saveCalled = true
	s.saved = content
	return s.saveErr
}

func TestPreview_LoadErrorPropagates(t *testing.T) {
	stub := &stubFS{loadErr: errLoadBoom}
	m := NewManager(stub, nil)
	pr, err := m.Preview("x.csv", ",")
	if err == nil {
		t.Fatal("expected error from Preview")
	}
	if pr.Error == "" {
		t.Errorf("expected PreviewResult.Error to be set, got %+v", pr)
	}
}

func TestWrite_SaveErrorPropagates(t *testing.T) {
	stub := &stubFS{saveErr: errSaveBoom}
	m := NewManager(stub, nil)
	r := m.Write("x.csv", [][]string{{"a"}}, ",")
	if r.Success {
		t.Errorf("expected failure, got %+v", r)
	}
	if r.Error == "" {
		t.Errorf("expected error message, got %+v", r)
	}
}

var errLoadBoom = &boomErr{msg: "load failed"}
var errSaveBoom = &boomErr{msg: "save failed"}

type boomErr struct{ msg string }

func (e *boomErr) Error() string { return e.msg }
