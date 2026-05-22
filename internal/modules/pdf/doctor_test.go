package pdf

import (
	"context"
	"errors"
	"testing"
	"time"

	picoloom "github.com/alnah/picoloom/v2"
)

func TestManager_LastExport_EmptyBeforeAnyCall(t *testing.T) {
	m, _, _, _, _ := newActiveManager(t)

	snap := m.LastExport()
	if snap.LastSuccess != nil {
		t.Errorf("LastSuccess = %+v, want nil before any export", snap.LastSuccess)
	}
	if snap.LastFailure != nil {
		t.Errorf("LastFailure = %+v, want nil before any export", snap.LastFailure)
	}
}

func TestManager_LastExport_CapturesSuccess(t *testing.T) {
	m, _, rdr, stg, _ := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|form-1.meta.json"] = "---\nstyle: technical\ncover:\n  title: Hi\n  template: classic\n---\n# body\n"

	if _, err := m.Export("tpl.yaml", "form-1.meta.json", ExportOpts{}); err != nil {
		t.Fatalf("Export: %v", err)
	}

	snap := m.LastExport()
	if snap.LastSuccess == nil {
		t.Fatalf("LastSuccess is nil after a successful export")
	}
	if snap.LastFailure != nil {
		t.Errorf("LastFailure should still be nil; got %+v", snap.LastFailure)
	}
	got := snap.LastSuccess
	if got.Template != "tpl.yaml" || got.Datafile != "form-1.meta.json" {
		t.Errorf("identity wrong: template=%q datafile=%q", got.Template, got.Datafile)
	}
	if got.Theme != "technical" || got.Cover != "classic" || !got.HasCover {
		t.Errorf("telemetry attrs wrong: theme=%q cover=%q has_cover=%v",
			got.Theme, got.Cover, got.HasCover)
	}
	if got.Path == "" || got.Bytes <= 0 {
		t.Errorf("path/bytes wrong: path=%q bytes=%d", got.Path, got.Bytes)
	}
	if got.Code != "" || got.Stage != "" || got.Err != "" {
		t.Errorf("failure-only fields leaked into success: code=%q stage=%q err=%q",
			got.Code, got.Stage, got.Err)
	}
	if got.At.IsZero() {
		t.Errorf("At is zero - must be stamped from m.nowFn()")
	}
}

func TestManager_LastExport_CapturesFailure(t *testing.T) {
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|form-1.meta.json"] = "# body"
	cf.convertOverride = func(_ context.Context, _ picoloom.Input) (*picoloom.ConvertResult, error) {
		return nil, picoloom.ErrCoverLogoNotFound
	}

	if _, err := m.Export("tpl.yaml", "form-1.meta.json", ExportOpts{}); err == nil {
		t.Fatalf("Export err = nil, want failure")
	}

	snap := m.LastExport()
	if snap.LastSuccess != nil {
		t.Errorf("LastSuccess should be nil; got %+v", snap.LastSuccess)
	}
	if snap.LastFailure == nil {
		t.Fatalf("LastFailure is nil after a failed export")
	}
	got := snap.LastFailure
	if got.Code != string(CodeCoverLogoMissing) {
		t.Errorf("Code = %q, want %q", got.Code, CodeCoverLogoMissing)
	}
	if got.Stage != "convert" {
		t.Errorf("Stage = %q, want convert", got.Stage)
	}
	if got.Err == "" {
		t.Errorf("Err empty; want underlying picoloom message")
	}
	if got.Template != "tpl.yaml" || got.Datafile != "form-1.meta.json" {
		t.Errorf("identity wrong: %+v", got)
	}
	if got.At.IsZero() {
		t.Errorf("At is zero")
	}
}

func TestManager_LastExport_PreservesBothSlots(t *testing.T) {
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|good.meta.json"] = "# body"
	rdr.md["tpl.yaml|bad.meta.json"] = "# body"

	// Success first.
	if _, err := m.Export("tpl.yaml", "good.meta.json", ExportOpts{}); err != nil {
		t.Fatalf("good export: %v", err)
	}
	// Then a failure on a different form.
	cf.convertOverride = func(_ context.Context, _ picoloom.Input) (*picoloom.ConvertResult, error) {
		return nil, picoloom.ErrPDFGeneration
	}
	if _, err := m.Export("tpl.yaml", "bad.meta.json", ExportOpts{}); err == nil {
		t.Fatalf("bad export err = nil")
	}

	snap := m.LastExport()
	if snap.LastSuccess == nil || snap.LastSuccess.Datafile != "good.meta.json" {
		t.Errorf("LastSuccess wrong: %+v", snap.LastSuccess)
	}
	if snap.LastFailure == nil || snap.LastFailure.Datafile != "bad.meta.json" {
		t.Errorf("LastFailure wrong: %+v", snap.LastFailure)
	}
	if snap.LastFailure.Code != string(CodePDFGenerationFailed) {
		t.Errorf("LastFailure.Code = %q, want %q",
			snap.LastFailure.Code, CodePDFGenerationFailed)
	}
}

func TestManager_LastExport_MostRecentSuccessWins(t *testing.T) {
	m, _, rdr, stg, _ := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|a.meta.json"] = "# a"
	rdr.md["tpl.yaml|b.meta.json"] = "# b"

	clock := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	m.nowFn = func() time.Time {
		clock = clock.Add(time.Second)
		return clock
	}

	if _, err := m.Export("tpl.yaml", "a.meta.json", ExportOpts{}); err != nil {
		t.Fatalf("a: %v", err)
	}
	if _, err := m.Export("tpl.yaml", "b.meta.json", ExportOpts{}); err != nil {
		t.Fatalf("b: %v", err)
	}

	snap := m.LastExport()
	if snap.LastSuccess.Datafile != "b.meta.json" {
		t.Errorf("LastSuccess.Datafile = %q, want b.meta.json", snap.LastSuccess.Datafile)
	}
}

func TestManager_LastExport_FailureAtRenderStageRecorded(t *testing.T) {
	m, _, rdr, stg, _ := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.err = errors.New("template not found")

	if _, err := m.Export("tpl.yaml", "x.meta.json", ExportOpts{}); err == nil {
		t.Fatalf("err = nil, want failure")
	}
	got := m.LastExport().LastFailure
	if got == nil {
		t.Fatalf("LastFailure nil")
	}
	if got.Code != string(CodeRenderFailed) || got.Stage != "render_markdown" {
		t.Errorf("got code=%q stage=%q; want render_failed/render_markdown", got.Code, got.Stage)
	}
}

func TestService_LastExport_DelegatesToManager(t *testing.T) {
	m, _, rdr, stg, _ := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|x.meta.json"] = "# body"
	if _, err := m.Export("tpl.yaml", "x.meta.json", ExportOpts{}); err != nil {
		t.Fatalf("export: %v", err)
	}

	svc := NewService(m)
	snap := svc.LastExport()
	if snap.LastSuccess == nil || snap.LastSuccess.Datafile != "x.meta.json" {
		t.Errorf("service snapshot wrong: %+v", snap)
	}
}
