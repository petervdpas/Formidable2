package plugin

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// fakePDF stands in for the PDF export service.
type fakePDF struct {
	active  bool
	err     error
	calls   int
	gotTpl  string
	gotFile string
	gotOpts map[string]any
}

func (f *fakePDF) PDFActive() bool { return f.active }

func (f *fakePDF) ExportPDF(template, datafile string, opts map[string]any) (map[string]any, error) {
	f.calls++
	f.gotTpl, f.gotFile, f.gotOpts = template, datafile, opts
	if f.err != nil {
		return nil, f.err
	}
	return map[string]any{"path": "/tmp/out/invoice-7.pdf", "bytes": float64(1234), "duration_ms": float64(88)}, nil
}

func runPDFScript(t *testing.T, f PDFAccess, body string) (RunResult, error) {
	t.Helper()
	return runScript(scriptOpts{Source: "function run(ctx)\n" + body + "\nend", Fn: "run", PDF: f})
}

func TestPDF_ExportReturnsThePathOfTheFile(t *testing.T) {
	f := &fakePDF{active: true}
	res, err := runPDFScript(t, f, `
		local out = formidable.pdf.export("invoice.yaml", "invoice-7.meta.json")
		return { path = out.path, bytes = out.bytes, ms = out.duration_ms, active = formidable.pdf.active() }`)
	if err != nil {
		t.Fatal(err)
	}
	got := res.Value.(map[string]any)
	if got["path"] != "/tmp/out/invoice-7.pdf" || got["bytes"] != float64(1234) || got["ms"] != float64(88) || got["active"] != true {
		t.Errorf("result = %v", got)
	}
	if f.gotTpl != "invoice.yaml" || f.gotFile != "invoice-7.meta.json" || f.gotOpts != nil {
		t.Errorf("call = %s/%s %v", f.gotTpl, f.gotFile, f.gotOpts)
	}
}

func TestPDF_ExportPassesOptions(t *testing.T) {
	f := &fakePDF{active: true}
	if _, err := runPDFScript(t, f, `
		return formidable.pdf.export("invoice.yaml", "a.meta.json", { output_path = "/tmp/x.pdf", disable_cover = true })`); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"output_path": "/tmp/x.pdf", "disable_cover": true}
	if !reflect.DeepEqual(f.gotOpts, want) {
		t.Errorf("opts = %v, want %v", f.gotOpts, want)
	}
}

func TestPDF_ExportErrorIsRaisedAndCatchable(t *testing.T) {
	f := &fakePDF{err: errors.New("engine_inactive: PDF export is not set up")}
	res, err := runPDFScript(t, f, `
		local ok, msg = pcall(formidable.pdf.export, "invoice.yaml", "a.meta.json")
		return { ok = ok, msg = tostring(msg), active = formidable.pdf.active() }`)
	if err != nil {
		t.Fatal(err)
	}
	got := res.Value.(map[string]any)
	if got["ok"] != false || got["active"] != false || !strings.Contains(got["msg"].(string), "pdf.export: engine_inactive: PDF export is not set up") {
		t.Errorf("result = %v", got)
	}
}

func TestPDF_BadArguments(t *testing.T) {
	for name, call := range map[string]string{
		"no datafile":         `formidable.pdf.export("invoice.yaml")`,
		"empty template":      `formidable.pdf.export("", "a.meta.json")`,
		"empty datafile":      `formidable.pdf.export("invoice.yaml", "")`,
		"options not a table": `formidable.pdf.export("invoice.yaml", "a.meta.json", "fast")`,
		"options are a list":  `formidable.pdf.export("invoice.yaml", "a.meta.json", { "fast" })`,
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakePDF{active: true}
			if _, err := runPDFScript(t, f, "return "+call); err == nil {
				t.Fatal("accepted")
			}
			if f.calls != 0 {
				t.Error("the export ran despite bad arguments")
			}
		})
	}
}

func TestPDF_NotConfigured(t *testing.T) {
	for _, call := range []string{`formidable.pdf.active()`, `formidable.pdf.export("a.yaml", "b.meta.json")`} {
		if _, err := runPDFScript(t, nil, "return "+call); err == nil || !strings.Contains(err.Error(), "pdf: not configured") {
			t.Errorf("%s: err = %v", call, err)
		}
	}
}

func pdfManifest(requires string) string {
	return `{
		"manifest_version": 1, "id": "demo", "name": "Demo", "version": "0.1.0",
		"requires_pdf": ` + requires + `,
		"commands": [{"id": "run", "label": "Run"}]
	}`
}

func TestLoadManifest_RequiresPDFRoundtrip(t *testing.T) {
	dir := writePlugin(t, t.TempDir(), "demo", pdfManifest("true"), "function run() end")
	m, err := LoadManifest(dir)
	if err != nil || !m.RequiresPDF {
		t.Fatalf("RequiresPDF = %v, err = %v", m.RequiresPDF, err)
	}
	raw, _ := SerializeManifest(m)
	if !strings.Contains(string(raw), `"requires_pdf": true`) {
		t.Errorf("serialized manifest lost the flag: %s", raw)
	}
	plain, _ := SerializeManifest(DefaultManifest("x"))
	if strings.Contains(string(plain), "requires_pdf") {
		t.Error("a plugin that does not need PDF export must not carry the key")
	}
}

func TestRun_RefusesUpFrontWhenPDFExportIsNeededButNotSetUp(t *testing.T) {
	m, pluginsDir := newTestManager(t)
	f := &fakePDF{}
	m.deps.PDF = f
	writePlugin(t, pluginsDir, "demo", pdfManifest("true"), "function run() return 1 end")
	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Run("demo", "run", nil); !errors.Is(err, ErrPDFNotActive) {
		t.Fatalf("err = %v, want ErrPDFNotActive", err)
	}
	if got := NewService(m).Run("demo", "run", nil); got.Kind != "pdf_not_active" {
		t.Errorf("Kind = %q, want pdf_not_active", got.Kind)
	}
	f.active = true
	if res, err := m.Run("demo", "run", nil); err != nil || res.Value != float64(1) {
		t.Fatalf("with PDF export set up: %v, %v", res.Value, err)
	}
	m.deps.PDF = nil
	if _, err := m.Run("demo", "run", nil); !errors.Is(err, ErrPDFNotActive) {
		t.Errorf("no PDF service wired: err = %v", err)
	}
}

func TestRun_PluginThatDoesNotDeclarePDFRunsWithoutIt(t *testing.T) {
	m, pluginsDir := newTestManager(t)
	m.deps.PDF = &fakePDF{}
	writePlugin(t, pluginsDir, "demo", pdfManifest("false"), `function run()
		local ok = pcall(formidable.pdf.export, "a.yaml", "b.meta.json")
		return { active = formidable.pdf.active(), ok = ok }
	end`)
	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	res, err := m.Run("demo", "run", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := res.Value.(map[string]any); got["active"] != false {
		t.Errorf("result = %v", got)
	}
}
