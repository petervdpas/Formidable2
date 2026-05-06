package form

import (
	"reflect"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// End-to-end round-trip tests — the existing fakeStorage stashes data
// verbatim, so it can't catch regressions in storage.Sanitize or in
// JSON disk I/O. These tests wire the real system → sfr → storage →
// form stack on a tempdir so SaveValues actually serializes to JSON
// and BuildView re-reads from disk.

func newRealStack(t *testing.T) (*Manager, *template.Manager) {
	t.Helper()
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	tplM := template.NewManager(sys, "templates", nil)
	sfrM := sfr.NewManager(sys, nil)
	stoM := storage.NewManager(sys, sfrM, tplM, "storage", nil)
	formM := NewManager(tplM, stoM, nil, nil)
	return formM, tplM
}

func TestForm_LinkObjectRoundTripsThroughDisk(t *testing.T) {
	m, tplM := newRealStack(t)

	// Template with a single link field — keeps the focus on shape
	// preservation, no other fields to mask the regression.
	if err := tplM.SaveTemplate("links.yaml", &template.Template{
		Name: "links", Filename: "links.yaml",
		Fields: []template.Field{
			{Key: "ref", Type: "link"},
		},
	}); err != nil {
		t.Fatalf("save template: %v", err)
	}

	want := map[string]any{
		"href": "formidable://links.yaml:other.meta.json",
		"text": "Open the other one",
	}
	res, err := m.SaveValues("links.yaml", SavePayload{
		Datafile: "self.meta.json",
		Values:   map[string]any{"ref": want},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if !res.Saved {
		t.Fatalf("save returned unsaved view: %+v", res)
	}

	// SaveValues round-trips by re-reading from disk; verify the
	// returned view holds the same shape.
	if got, ok := res.Values["ref"].(map[string]any); !ok {
		t.Fatalf("ref not an object on disk: %T %v", res.Values["ref"], res.Values["ref"])
	} else if !reflect.DeepEqual(got, want) {
		t.Errorf("ref shape changed: got %v, want %v", got, want)
	}

	// Independently re-open via BuildView so we test the read path
	// distinct from save's returned view.
	view, err := m.BuildView("links.yaml", "self.meta.json")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	got, ok := view.Values["ref"].(map[string]any)
	if !ok {
		t.Fatalf("ref not an object after BuildView: %T", view.Values["ref"])
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("BuildView lost link shape: got %v, want %v", got, want)
	}
}

func TestForm_LinkLegacyStringRoundTrips(t *testing.T) {
	// Legacy templates may have stored a bare string in the link
	// slot. The new Vue field accepts either shape, but the storage
	// layer must continue to round-trip strings without quietly
	// upgrading them to {href, text} (that would mask data in saves
	// of forms users haven't touched).
	m, tplM := newRealStack(t)
	_ = tplM.SaveTemplate("links.yaml", &template.Template{
		Name: "links", Filename: "links.yaml",
		Fields: []template.Field{{Key: "ref", Type: "link"}},
	})
	res, err := m.SaveValues("links.yaml", SavePayload{
		Datafile: "self.meta.json",
		Values:   map[string]any{"ref": "https://example.com/old"},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if got, want := res.Values["ref"], "https://example.com/old"; got != want {
		t.Errorf("legacy string changed: got %v (%T), want %v", got, got, want)
	}
}

func TestForm_LinkEmptyStaysEmpty(t *testing.T) {
	// Empty value (cleared link) round-trips as "" — the canonical
	// empty form for the link field.
	m, tplM := newRealStack(t)
	_ = tplM.SaveTemplate("links.yaml", &template.Template{
		Name: "links", Filename: "links.yaml",
		Fields: []template.Field{{Key: "ref", Type: "link"}},
	})
	res, err := m.SaveValues("links.yaml", SavePayload{
		Datafile: "self.meta.json",
		Values:   map[string]any{"ref": ""},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if got, want := res.Values["ref"], ""; got != want {
		t.Errorf("empty link changed: got %v (%T), want empty string", got, got)
	}
}
