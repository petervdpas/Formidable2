package pdf

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	picoloom "github.com/alnah/picoloom/v2"
)

func captureLogger(t *testing.T) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	h := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(h), buf
}

// readLogEntries pulls every line of JSON-encoded slog output out of
// buf and decodes it. Drops empty lines.
func readLogEntries(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	out := []map[string]any{}
	for _, line := range strings.Split(buf.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line %q is not JSON: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func findEntry(entries []map[string]any, msg string) map[string]any {
	for _, e := range entries {
		if v, _ := e["msg"].(string); v == msg {
			return e
		}
	}
	return nil
}

func TestExport_Telemetry_SuccessAttrs(t *testing.T) {
	m, _, rdr, stg, _ := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|form-1.meta.json"] = "---\nstyle: technical\ncover:\n  title: Hello\n  template: classic\n---\n# Body\n"

	logger, buf := captureLogger(t)
	m.log = logger

	_, err := m.Export("tpl.yaml", "form-1.meta.json", ExportOpts{})
	if err != nil {
		t.Fatalf("Export err = %v", err)
	}

	entries := readLogEntries(t, buf)
	e := findEntry(entries, "pdf: exported")
	if e == nil {
		t.Fatalf("no 'pdf: exported' log line; got %d entries: %+v", len(entries), entries)
	}

	checks := map[string]any{
		"template":  "tpl.yaml",
		"datafile":  "form-1.meta.json",
		"theme":     "technical",
		"cover":     "classic",
		"has_cover": true,
	}
	for k, want := range checks {
		if got := e[k]; got != want {
			t.Errorf("log attr %q = %v, want %v", k, got, want)
		}
	}
	if _, ok := e["path"]; !ok {
		t.Errorf("log missing 'path' attr")
	}
	if v, _ := e["bytes"].(float64); v <= 0 {
		t.Errorf("log 'bytes' = %v, want > 0", e["bytes"])
	}
	if _, ok := e["duration_ms"]; !ok {
		t.Errorf("log missing 'duration_ms' attr")
	}
}

func TestExport_Telemetry_SuccessWithoutCoverIsNoise(t *testing.T) {
	m, _, rdr, stg, _ := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|form-1.meta.json"] = "# body without frontmatter"

	logger, buf := captureLogger(t)
	m.log = logger

	if _, err := m.Export("tpl.yaml", "form-1.meta.json", ExportOpts{}); err != nil {
		t.Fatalf("Export err = %v", err)
	}
	e := findEntry(readLogEntries(t, buf), "pdf: exported")
	if e == nil {
		t.Fatalf("no success log line")
	}
	if e["cover"] != "" {
		t.Errorf("cover attr = %v, want empty when no cover frontmatter", e["cover"])
	}
	if e["has_cover"] != false {
		t.Errorf("has_cover attr = %v, want false", e["has_cover"])
	}
	if e["theme"] != "" {
		t.Errorf("theme attr = %v, want empty when no style set", e["theme"])
	}
}

func TestExport_Telemetry_FailureCarriesCode(t *testing.T) {
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|form-1.meta.json"] = "# body"

	cf.convertOverride = func(_ context.Context, _ picoloom.Input) (*picoloom.ConvertResult, error) {
		return nil, picoloom.ErrCoverLogoNotFound
	}

	logger, buf := captureLogger(t)
	m.log = logger

	_, err := m.Export("tpl.yaml", "form-1.meta.json", ExportOpts{})
	if err == nil {
		t.Fatalf("Export err = nil, want failure")
	}
	var ee *ExportError
	if !errors.As(err, &ee) {
		t.Fatalf("err is not *ExportError: %T %v", err, err)
	}
	if ee.Code != CodeCoverLogoMissing {
		t.Errorf("err code = %q, want %q", ee.Code, CodeCoverLogoMissing)
	}

	e := findEntry(readLogEntries(t, buf), "pdf: export failed")
	if e == nil {
		t.Fatalf("no 'pdf: export failed' log line")
	}
	if e["code"] != string(CodeCoverLogoMissing) {
		t.Errorf("log code = %v, want %v", e["code"], CodeCoverLogoMissing)
	}
	if e["stage"] != "convert" {
		t.Errorf("log stage = %v, want convert", e["stage"])
	}
	if e["template"] != "tpl.yaml" {
		t.Errorf("log template = %v, want tpl.yaml", e["template"])
	}
	if _, ok := e["duration_ms"]; !ok {
		t.Errorf("failure log missing duration_ms")
	}
}

func TestExport_Telemetry_FailureAtRenderStage(t *testing.T) {
	m, _, rdr, stg, _ := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.err = errors.New("missing template")

	logger, buf := captureLogger(t)
	m.log = logger

	if _, err := m.Export("tpl.yaml", "form-1.meta.json", ExportOpts{}); err == nil {
		t.Fatalf("Export err = nil, want failure")
	}
	e := findEntry(readLogEntries(t, buf), "pdf: export failed")
	if e == nil {
		t.Fatalf("no failure log")
	}
	if e["code"] != string(CodeRenderFailed) {
		t.Errorf("code = %v, want render_failed", e["code"])
	}
	if e["stage"] != "render_markdown" {
		t.Errorf("stage = %v, want render_markdown", e["stage"])
	}
}
