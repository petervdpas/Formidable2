package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/petervdpas/formidable2/internal/modules/pdf"
)

type fakeExporter struct {
	active  bool
	err     error
	gotOpts pdf.ExportOpts
	calls   int
}

func (f *fakeExporter) GetStatus() pdf.Status { return pdf.Status{Active: f.active} }

func (f *fakeExporter) ExportPDF(tpl, datafile string, opts pdf.ExportOpts) (pdf.Result, error) {
	f.calls++
	f.gotOpts = opts
	if f.err != nil {
		return pdf.Result{}, f.err
	}
	return pdf.Result{Path: "/out/" + datafile + ".pdf", Bytes: 2048, Duration: 1500 * time.Millisecond}, nil
}

func TestPluginPDFAdapter_ExportReturnsPathSizeAndMilliseconds(t *testing.T) {
	f := &fakeExporter{active: true}
	a := &pluginPDFAdapter{svc: f}
	got, err := a.ExportPDF("invoice.yaml", "inv-7", map[string]any{"output_path": "/tmp/inv-7.pdf", "disable_cover": true})
	if err != nil {
		t.Fatal(err)
	}
	if got["path"] != "/out/inv-7.pdf" || got["bytes"] != 2048 || got["duration_ms"] != int64(1500) {
		t.Errorf("result = %v", got)
	}
	if f.gotOpts.OutputPath != "/tmp/inv-7.pdf" || !f.gotOpts.DisableCover {
		t.Errorf("opts = %+v", f.gotOpts)
	}
	if !a.PDFActive() {
		t.Error("PDFActive = false with an active engine")
	}
}

func TestPluginPDFAdapter_NotSetUpSaysSoPlainly(t *testing.T) {
	cases := map[string]*pluginPDFAdapter{
		"service not wired yet": {},
		"engine not activated":  {svc: &fakeExporter{err: pdf.ErrPDFNotActivated}},
	}
	for name, a := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := a.ExportPDF("invoice.yaml", "inv-7", nil)
			if err == nil || !strings.HasPrefix(err.Error(), "engine_inactive: ") || strings.Contains(err.Error(), `{"code"`) {
				t.Fatalf("err = %v, want a flat engine_inactive error", err)
			}
			if a.PDFActive() {
				t.Error("PDFActive = true")
			}
		})
	}
	var none *pluginPDFAdapter
	if none.PDFActive() {
		t.Error("a nil adapter reads as active")
	}
}

func TestPluginPDFAdapter_UnknownOptionIsRefusedBeforeExporting(t *testing.T) {
	f := &fakeExporter{active: true}
	_, err := (&pluginPDFAdapter{svc: f}).ExportPDF("invoice.yaml", "inv-7", map[string]any{"output_pth": "/tmp/x.pdf"})
	if err == nil || !strings.Contains(err.Error(), "bad option") || f.calls != 0 {
		t.Fatalf("err = %v, calls = %d", err, f.calls)
	}
}

func TestPluginPDFAdapter_OtherExportErrorsKeepTheirCode(t *testing.T) {
	f := &fakeExporter{active: true, err: &pdf.ExportError{Code: pdf.CodeRenderTimeout, Message: "pdf: render timed out.", Hint: "Try again."}}
	_, err := (&pluginPDFAdapter{svc: f}).ExportPDF("invoice.yaml", "inv-7", nil)
	if err == nil || err.Error() != "render_timeout: pdf: render timed out. Try again." {
		t.Fatalf("err = %v", err)
	}
	f.err = errors.New("disk on fire")
	if _, err := (&pluginPDFAdapter{svc: f}).ExportPDF("invoice.yaml", "inv-7", nil); err == nil || !strings.Contains(err.Error(), "disk on fire") {
		t.Fatalf("err = %v", err)
	}
}
