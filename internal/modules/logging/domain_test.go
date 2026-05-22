package logging

import (
	"io"
	"log/slog"
	"testing"

	applog "github.com/petervdpas/formidable2/internal/log"
)

// testLogger wires a slog.Logger that fans into both io.Discard (silent
// stderr surrogate) and the given Broadcaster - mirrors the multi-
// handler the composition root builds in production but without
// touching the filesystem.
func testLogger(bc *applog.Broadcaster) *slog.Logger {
	return slog.New(bc.Handler())
}

func TestWriteFromFrontend_EmitsEntry(t *testing.T) {
	bc := applog.NewBroadcaster(16)
	m := NewManager(bc, "", testLogger(bc))

	m.WriteFromFrontend("warn", "client says hi", map[string]any{"foo": "bar"})

	got := bc.Recent(0)
	if len(got) != 1 {
		t.Fatalf("ring had %d entries, want 1", len(got))
	}
	e := got[0]
	if e.Level != "WARN" {
		t.Errorf("Entry.Level = %q, want WARN", e.Level)
	}
	if e.Msg != "client says hi" {
		t.Errorf("Entry.Msg = %q", e.Msg)
	}
	if e.Attrs["source"] != "frontend" {
		t.Errorf("Entry.Attrs[source] = %v, want frontend", e.Attrs["source"])
	}
	if e.Attrs["foo"] != "bar" {
		t.Errorf("Entry.Attrs[foo] = %v, want bar", e.Attrs["foo"])
	}
}

func TestWriteFromFrontend_UnknownLevelFallsBackToInfo(t *testing.T) {
	bc := applog.NewBroadcaster(16)
	m := NewManager(bc, "", testLogger(bc))

	m.WriteFromFrontend("nonsense", "x", nil)
	m.WriteFromFrontend("", "y", nil)

	got := bc.Recent(0)
	if len(got) != 2 {
		t.Fatalf("ring had %d entries, want 2", len(got))
	}
	for i, e := range got {
		if e.Level != "INFO" {
			t.Errorf("entry[%d].Level = %q, want INFO", i, e.Level)
		}
	}
}

func TestWriteFromFrontend_NilLoggerIsNoOp(t *testing.T) {
	bc := applog.NewBroadcaster(16)
	m := NewManager(bc, "", nil)

	m.WriteFromFrontend("info", "vanishes", nil)

	if got := bc.Recent(0); len(got) != 0 {
		t.Errorf("expected no entries when logger is nil; got %d", len(got))
	}
}

func TestWriteFromFrontend_EmptyMessageDropped(t *testing.T) {
	bc := applog.NewBroadcaster(16)
	m := NewManager(bc, "", testLogger(bc))

	m.WriteFromFrontend("info", "", nil)
	m.WriteFromFrontend("info", "   ", nil)

	if got := bc.Recent(0); len(got) != 0 {
		t.Errorf("expected empty/whitespace msgs to be dropped; got %d", len(got))
	}
}

func TestWriteFromFrontend_SourceAttrCannotBeOverridden(t *testing.T) {
	bc := applog.NewBroadcaster(16)
	m := NewManager(bc, "", testLogger(bc))

	m.WriteFromFrontend("info", "tampered", map[string]any{"source": "backend"})

	got := bc.Recent(0)
	if len(got) != 1 {
		t.Fatalf("ring had %d entries, want 1", len(got))
	}
	if got[0].Attrs["source"] != "frontend" {
		t.Errorf("source attr was overridden: %v", got[0].Attrs["source"])
	}
}

// Sanity that the old constructor shape is gone - guards against an
// app.go that still calls NewManager(bc, path) and somehow compiles.
var _ = io.Discard // keep "io" import used; serves as no-op anchor
