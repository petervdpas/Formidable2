package pdf

import (
	"errors"
	"testing"
)

func TestNewManagerDefaultStatus(t *testing.T) {
	m := NewManager(nil)
	s := m.Status()
	if s.Active {
		t.Errorf("fresh manager active=true, want false")
	}
	if s.Source != SourceUnset {
		t.Errorf("fresh manager source=%q, want %q", s.Source, SourceUnset)
	}
	if s.BrowserBin != "" || s.Version != "" {
		t.Errorf("fresh manager status not zero: %+v", s)
	}
	if !s.ActivatedAt.IsZero() {
		t.Errorf("fresh manager activated_at not zero: %v", s.ActivatedAt)
	}
}

func TestActivateReturnsNotActivated(t *testing.T) {
	m := NewManager(nil)
	_, err := m.Activate(ActivateOpts{})
	if !errors.Is(err, ErrPDFNotActivated) {
		t.Errorf("Activate err = %v, want ErrPDFNotActivated", err)
	}
	if m.Status().Active {
		t.Errorf("Activate flipped status active=true, want false")
	}
}

func TestDeactivateReturnsNotActivated(t *testing.T) {
	m := NewManager(nil)
	err := m.Deactivate()
	if !errors.Is(err, ErrPDFNotActivated) {
		t.Errorf("Deactivate err = %v, want ErrPDFNotActivated", err)
	}
}

func TestExportReturnsNotActivated(t *testing.T) {
	for _, tc := range []struct {
		name string
		guid string
		opts ExportOpts
	}{
		{"with guid", "abc-123", ExportOpts{}},
		{"empty guid", "", ExportOpts{}},
		{"with options", "abc-123", ExportOpts{OutputPath: "/tmp/x.pdf", Style: "technical"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewManager(nil)
			res, err := m.Export(tc.guid, tc.opts)
			if !errors.Is(err, ErrPDFNotActivated) {
				t.Errorf("Export err = %v, want ErrPDFNotActivated", err)
			}
			if res != (Result{}) {
				t.Errorf("Export result = %+v, want zero value", res)
			}
		})
	}
}

func TestNewServiceNilManagerPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("NewService(nil) did not panic")
		}
	}()
	_ = NewService(nil)
}

func TestServiceMirrorsManager(t *testing.T) {
	svc := NewService(NewManager(nil))
	if svc.GetStatus().Active {
		t.Errorf("fresh service status active=true")
	}
	if _, err := svc.Activate(ActivateOpts{}); !errors.Is(err, ErrPDFNotActivated) {
		t.Errorf("svc.Activate err = %v", err)
	}
	if err := svc.Deactivate(); !errors.Is(err, ErrPDFNotActivated) {
		t.Errorf("svc.Deactivate err = %v", err)
	}
	if _, err := svc.ExportPDF("g", ExportOpts{}); !errors.Is(err, ErrPDFNotActivated) {
		t.Errorf("svc.ExportPDF err = %v", err)
	}
}
