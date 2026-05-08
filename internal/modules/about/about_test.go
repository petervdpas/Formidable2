package about

import (
	"strings"
	"testing"
)

func TestInfoConstants(t *testing.T) {
	if Name != "Formidable" {
		t.Errorf("Name = %q, want %q", Name, "Formidable")
	}
	if Author == "" {
		t.Error("Author must not be empty")
	}
	if Tagline == "" {
		t.Error("Tagline must not be empty")
	}
	if Version == "" {
		t.Error("Version must not be empty")
	}
	parts := strings.Split(Version, ".")
	if len(parts) < 2 {
		t.Errorf("Version %q is not a dotted version string", Version)
	}
}

func TestServiceGetInfo(t *testing.T) {
	s := NewService()
	got := s.GetInfo()

	if got.Name != Name {
		t.Errorf("Info.Name = %q, want %q", got.Name, Name)
	}
	if got.Version != Version {
		t.Errorf("Info.Version = %q, want %q", got.Version, Version)
	}
	if got.Tagline != Tagline {
		t.Errorf("Info.Tagline = %q, want %q", got.Tagline, Tagline)
	}
	if got.Author != Author {
		t.Errorf("Info.Author = %q, want %q", got.Author, Author)
	}
}

func TestServiceGetInfoIsStable(t *testing.T) {
	s := NewService()
	a, b := s.GetInfo(), s.GetInfo()
	if a != b {
		t.Errorf("GetInfo not stable across calls: %+v vs %+v", a, b)
	}
}

func TestNewServiceNotNil(t *testing.T) {
	if NewService() == nil {
		t.Fatal("NewService returned nil")
	}
}
