package about

import (
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/i18n"
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
	s := NewService(nil)
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
	if got.Website != Website {
		t.Errorf("Info.Website = %q, want %q", got.Website, Website)
	}
}

func TestWebsiteConstant(t *testing.T) {
	if !strings.HasPrefix(Website, "https://") {
		t.Errorf("Website %q must be an https URL", Website)
	}
}

func TestServiceOpenWebsite(t *testing.T) {
	var got string
	s := NewService(func(url string) error {
		got = url
		return nil
	})
	if err := s.OpenWebsite(); err != nil {
		t.Fatalf("OpenWebsite: %v", err)
	}
	if got != Website {
		t.Errorf("opener received %q, want %q", got, Website)
	}
}

func TestServiceOpenWebsiteNoOpener(t *testing.T) {
	s := NewService(nil)
	if err := s.OpenWebsite(); err == nil {
		t.Error("OpenWebsite with nil opener should error")
	}
}

func TestServiceGetInfoIsStable(t *testing.T) {
	s := NewService(nil)
	a, b := s.GetInfo(), s.GetInfo()
	if a != b {
		t.Errorf("GetInfo not stable across calls: %+v vs %+v", a, b)
	}
}

func TestNewServiceNotNil(t *testing.T) {
	if NewService(nil) == nil {
		t.Fatal("NewService returned nil")
	}
}

func TestLibrariesNonEmptyAndUnique(t *testing.T) {
	if len(Libraries) == 0 {
		t.Fatal("Libraries must not be empty - About panel needs a credits list")
	}
	seen := make(map[string]bool, len(Libraries))
	for _, l := range Libraries {
		if l.ID == "" {
			t.Errorf("library has empty ID: %+v", l)
		}
		if l.Name == "" {
			t.Errorf("library %q has empty display Name", l.ID)
		}
		if seen[l.ID] {
			t.Errorf("duplicate library ID %q", l.ID)
		}
		seen[l.ID] = true
	}
}

// TestLibraries_EveryIDHasDescriptionInEveryLocale fails when a new
// Library entry lands without the matching i18n
// `workspace.information.about.thanks.lib.<id>.desc` string in every
// shipped locale - and inversely, when an orphan desc string lingers
// for an ID that has been removed from Libraries. Drift caught at
// build time instead of as a missing label in the running app.
func TestLibraries_EveryIDHasDescriptionInEveryLocale(t *testing.T) {
	m, err := i18n.NewManager(nil)
	if err != nil {
		t.Fatalf("i18n.NewManager: %v", err)
	}
	const prefix = "workspace.information.about.thanks.lib."
	const suffix = ".desc"

	want := make(map[string]struct{}, len(Libraries))
	for _, l := range Libraries {
		want[prefix+l.ID+suffix] = struct{}{}
	}

	for _, locale := range []string{"en", "nl"} {
		bundle, err := m.LoadBundle(locale)
		if err != nil {
			t.Fatalf("LoadBundle(%q): %v", locale, err)
		}
		for key := range want {
			v, ok := bundle[key]
			if !ok {
				t.Errorf("locale %q: missing key %q (Library ID present but no description)", locale, key)
				continue
			}
			if s, _ := v.(string); strings.TrimSpace(s) == "" {
				t.Errorf("locale %q: key %q is empty - provide a real description", locale, key)
			}
		}
		// Orphan check: a desc key whose ID is not in Libraries means
		// either Libraries was trimmed and the locale wasn't, or an
		// ID was renamed and only one side moved.
		for key := range bundle {
			if !strings.HasPrefix(key, prefix) || !strings.HasSuffix(key, suffix) {
				continue
			}
			if _, ok := want[key]; !ok {
				t.Errorf("locale %q: orphan key %q (no matching Library entry)", locale, key)
			}
		}
	}
}

func TestGetLibrariesReturnsCopy(t *testing.T) {
	s := NewService(nil)
	got := s.GetLibraries()
	if len(got) != len(Libraries) {
		t.Fatalf("GetLibraries length = %d, want %d", len(got), len(Libraries))
	}
	// Mutating the returned slice must not leak into the package-level
	// canonical list - guards against a bound caller stomping the
	// credits at runtime.
	if len(got) > 0 {
		original := Libraries[0].Name
		got[0].Name = "tampered-in-caller"
		if Libraries[0].Name != original {
			t.Errorf("caller mutation leaked: Libraries[0].Name = %q", Libraries[0].Name)
		}
	}
}
