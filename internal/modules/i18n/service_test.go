package i18n

import (
	"slices"
	"testing"
)

// The Service is a thin Wails-facing wrapper around Manager. These
// tests mirror the Manager's contract through the Service layer so any
// future change (e.g. swapping the underlying type, adding caching,
// renaming methods) keeps the Wails surface stable.

func TestService_DelegatesLoadBundle(t *testing.T) {
	t.Parallel()
	m, _ := NewManager(nil)
	s := NewService(m)

	b, err := s.LoadBundle("en")
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}
	if _, ok := b["status.ready"]; !ok {
		t.Errorf("missing status.ready via service")
	}
}

func TestService_LoadBundle_UnknownErrors(t *testing.T) {
	t.Parallel()
	m, _ := NewManager(nil)
	s := NewService(m)
	if _, err := s.LoadBundle("klingon"); err == nil {
		t.Errorf("expected error for unknown locale via service")
	}
}

func TestService_AvailableLocales(t *testing.T) {
	t.Parallel()
	m, _ := NewManager(nil)
	s := NewService(m)
	locs := s.AvailableLocales()
	if !slices.Contains(locs, "en") || !slices.Contains(locs, "nl") {
		t.Errorf("AvailableLocales missing en/nl: %v", locs)
	}
}

func TestService_DefaultLocale(t *testing.T) {
	t.Parallel()
	m, _ := NewManager(nil)
	s := NewService(m)
	if got := s.DefaultLocale(); got != "en" {
		t.Errorf("DefaultLocale = %q, want en", got)
	}
}
