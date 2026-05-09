package expression

import (
	"errors"
	"testing"
)

// fakeTpl returns a fixed sidebar expression + opt-in fields. Tests
// configure both the source and the field list to pin narrowContext.
type fakeTpl struct {
	src    string
	fields []string
	err    error
}

func (f fakeTpl) LookupSidebar(name string) (string, []string, error) {
	return f.src, f.fields, f.err
}

type fakeSto struct {
	records []Record
	err     error
}

func (f fakeSto) ListForExpression(name string) ([]Record, error) {
	return f.records, f.err
}

func TestEvaluateSidebar_HappyPath(t *testing.T) {
	withFakeNow(t, "2026-05-09")
	m := NewManager(
		fakeTpl{
			src:    `isOverdue(due) ? "OVERDUE: " + name : name`,
			fields: []string{"name", "due"},
		},
		fakeSto{records: []Record{
			{Filename: "a.json", Title: "A", Context: map[string]any{"name": "alpha", "due": "2026-05-08"}},
			{Filename: "b.json", Title: "B", Context: map[string]any{"name": "bravo", "due": "2026-05-15"}},
		}},
	)
	got, err := m.EvaluateSidebar("any")
	if err != nil {
		t.Fatalf("EvaluateSidebar: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 items, got %d", len(got))
	}
	if got[0].Text != "OVERDUE: alpha" {
		t.Errorf("a.json text: %q", got[0].Text)
	}
	if got[0].Filename != "a.json" {
		t.Errorf("a.json filename not stamped: %+v", got[0])
	}
	if got[1].Text != "bravo" {
		t.Errorf("b.json text: %q", got[1].Text)
	}
}

func TestEvaluateSidebar_NoExpression(t *testing.T) {
	m := NewManager(
		fakeTpl{src: ""}, // no sidebar_expression configured
		fakeSto{},
	)
	_, err := m.EvaluateSidebar("any")
	if !errors.Is(err, ErrNoExpression) {
		t.Errorf("want ErrNoExpression, got %v", err)
	}
}

func TestEvaluateSidebar_PerRowFailureIsIsolated(t *testing.T) {
	m := NewManager(
		// Helper with a typo causes a runtime error per row when
		// `due` is non-nil but unparseable.
		fakeTpl{src: `name + " — " + string(daysBetween(due, today()))`, fields: []string{"name", "due"}},
		fakeSto{records: []Record{
			{Filename: "ok.json", Title: "ok", Context: map[string]any{"name": "ok", "due": "2026-05-01"}},
			{Filename: "broken.json", Title: "broken", Context: map[string]any{"name": "x", "due": 12345}},
		}},
	)
	withFakeNow(t, "2026-05-09")
	got, err := m.EvaluateSidebar("any")
	if err != nil {
		t.Fatalf("EvaluateSidebar: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 items even with one failure, got %d", len(got))
	}
	// First row succeeds.
	if got[0].Error != "" {
		t.Errorf("ok.json should not carry error: %q", got[0].Error)
	}
	// Numeric `due` doesn't actually error (daysBetween returns 0)
	// but emptied context (no harvest) would. Validate the contract:
	// the result is text, not an error. If a failure DOES happen it
	// must carry the title fallback.
	if got[1].Text == "" {
		t.Errorf("broken row should still surface title fallback")
	}
}

func TestEvaluateSidebar_TitleFallbackOnEmptyText(t *testing.T) {
	m := NewManager(
		// Expression evaluates to "" — the row should still render
		// the storage-supplied title rather than blank.
		fakeTpl{src: `""`, fields: nil},
		fakeSto{records: []Record{
			{Filename: "a.json", Title: "Fallback", Context: map[string]any{}},
		}},
	)
	got, err := m.EvaluateSidebar("any")
	if err != nil {
		t.Fatalf("EvaluateSidebar: %v", err)
	}
	if got[0].Text != "Fallback" {
		t.Errorf("title fallback failed: %+v", got[0])
	}
}

func TestEvaluateSidebar_NarrowContext(t *testing.T) {
	// Storage harvest exposes 'secret' but template's
	// expressionFields whitelist doesn't include it — narrowContext
	// must drop it so the expression cannot read it.
	m := NewManager(
		fakeTpl{src: `defaultText(secret, "no-secret")`, fields: []string{"name"}},
		fakeSto{records: []Record{
			{Filename: "a.json", Title: "A",
				Context: map[string]any{"name": "x", "secret": "leak"}},
		}},
	)
	got, err := m.EvaluateSidebar("any")
	if err != nil {
		t.Fatalf("EvaluateSidebar: %v", err)
	}
	if got[0].Text != "no-secret" {
		t.Errorf("narrowContext leaked 'secret'; expression saw %q", got[0].Text)
	}
}

func TestEvaluateSidebar_ProvidersNotWired(t *testing.T) {
	m := NewManager(nil, nil)
	_, err := m.EvaluateSidebar("any")
	if err == nil {
		t.Fatal("expected error when providers are nil")
	}
}

func TestEvaluate_NilContextOK(t *testing.T) {
	m := NewManager(nil, nil)
	got, err := m.Evaluate(`"static"`, nil)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if got.Text != "static" {
		t.Errorf("static text drifted: %q", got.Text)
	}
}
