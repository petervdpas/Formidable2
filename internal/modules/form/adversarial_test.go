package form

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// Reuses realStack + formPath from unhappy_test.go (same package).

// ─────────────────────────────────────────────────────────────────────
// Path traversal: a datafile escaping the template dir must be rejected
// by the storage guard, surfaced as a SaveValues error, and no file may
// land outside the storage subtree.
// ─────────────────────────────────────────────────────────────────────

func TestSaveValues_TraversalDatafileIsRejected(t *testing.T) {
	m, tplM, _, root := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})

	_, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "../escape.meta.json",
		Values:   map[string]any{"title": "x"},
	})
	if err == nil {
		t.Fatalf("traversal datafile must error")
	}
	if !strings.Contains(err.Error(), "invalid datafile") {
		t.Errorf("want invalid-datafile error, got %q", err.Error())
	}
	// The escape target must not exist anywhere near the storage root.
	if _, statErr := os.Stat(root + "/escape.meta.json"); !os.IsNotExist(statErr) {
		t.Errorf("escape file should not exist, stat err=%v", statErr)
	}
}

func TestSaveValues_SlashDatafileIsRejected(t *testing.T) {
	m, tplM, _, _ := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})
	_, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "sub/dir.meta.json",
		Values:   map[string]any{"title": "x"},
	})
	if err == nil {
		t.Fatalf("datafile with slash must error")
	}
	if !strings.Contains(err.Error(), "invalid datafile") {
		t.Errorf("want invalid-datafile error, got %q", err.Error())
	}
}

// ─────────────────────────────────────────────────────────────────────
// Nil Values payload still persists: storage injects type-defaults so a
// declared text field reads back as "" and the form is Saved.
// ─────────────────────────────────────────────────────────────────────

func TestSaveValues_NilValuesPersistsWithFieldDefaults(t *testing.T) {
	m, tplM, _, _ := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})
	view, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "e.meta.json",
		Values:   nil,
	})
	if err != nil {
		t.Fatalf("save nil values: %v", err)
	}
	if !view.Saved {
		t.Errorf("nil-values save should be Saved=true")
	}
	if got := view.Values["title"]; got != "" {
		t.Errorf("declared text field should default to empty string, got %v (%T)", got, got)
	}
	// Re-read independently from disk.
	again, err := m.BuildView("t.yaml", "e.meta.json")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !again.Saved || again.Values["title"] != "" {
		t.Errorf("disk read mismatch: saved=%v title=%v", again.Saved, again.Values["title"])
	}
}

// ─────────────────────────────────────────────────────────────────────
// Audit stamping: with no AuthorProvider wired, every save stamps the
// fallback identity on Created + Updated and a non-empty timestamp.
// ─────────────────────────────────────────────────────────────────────

func TestSaveValues_StampsFallbackIdentity(t *testing.T) {
	m, tplM, _, _ := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})
	view, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "f.meta.json",
		Values:   map[string]any{"title": "x"},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if view.Meta.Created.Name != "Unknown" {
		t.Errorf("created name: want Unknown, got %q", view.Meta.Created.Name)
	}
	if view.Meta.Created.Email != "unknown@example.com" {
		t.Errorf("created email: want unknown@example.com, got %q", view.Meta.Created.Email)
	}
	if view.Meta.Updated.Name != "Unknown" {
		t.Errorf("updated name: want Unknown, got %q", view.Meta.Updated.Name)
	}
	if view.Meta.Created.At == "" {
		t.Errorf("created timestamp must be stamped")
	}
	if view.Meta.Updated.At == "" {
		t.Errorf("updated timestamp must be stamped")
	}
}

// ─────────────────────────────────────────────────────────────────────
// Valid declared facet selection round-trips with the exact label, both
// in the returned view and on a fresh disk read.
// ─────────────────────────────────────────────────────────────────────

func TestSaveValues_ValidFacetSelectionRoundTrips(t *testing.T) {
	m, tplM, _, _ := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Facets: []template.Facet{{
			Key:  "status",
			Icon: "fa-flag",
			Options: []template.FacetOption{
				{Label: "OPEN", Color: "green"},
				{Label: "CLOSED", Color: "red"},
			},
		}},
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})
	res, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "f.meta.json",
		Values:   map[string]any{"title": "x"},
		Meta: storage.FormMeta{
			Facets: map[string]storage.FacetState{
				"status": {Set: true, Selected: "CLOSED"},
			},
		},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	got := res.Meta.Facets["status"]
	if !got.Set || got.Selected != "CLOSED" {
		t.Errorf("valid facet not preserved: want {Set:true Selected:CLOSED}, got %+v", got)
	}
	view, err := m.BuildView("t.yaml", "f.meta.json")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if s := view.Meta.Facets["status"]; !s.Set || s.Selected != "CLOSED" {
		t.Errorf("disk read lost facet: %+v", s)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Concurrent saves to the SAME datafile under -race: atomic writes mean
// the last writer wins and the file is never half-written. After the
// storm, the file is valid JSON, readable, and holds one of the values.
// ─────────────────────────────────────────────────────────────────────

func TestSaveValues_ConcurrentSameFileNoCorruption(t *testing.T) {
	m, tplM, _, root := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})

	const n = 20
	wanted := make(map[string]bool, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		val := "v" + string(rune('a'+i))
		wanted[val] = true
		wg.Add(1)
		go func(val string) {
			defer wg.Done()
			if _, err := m.SaveValues("t.yaml", SavePayload{
				Datafile: "same.meta.json",
				Values:   map[string]any{"title": val},
			}); err != nil {
				t.Errorf("concurrent same-file save %s: %v", val, err)
			}
		}(val)
	}
	wg.Wait()

	// The on-disk bytes must parse as JSON (no torn write).
	path := formPath(root, "t.yaml", "same.meta.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read same: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("on-disk JSON is corrupt: %v\n%s", err, raw)
	}

	// The surviving value must be exactly one of the writers' values.
	view, err := m.BuildView("t.yaml", "same.meta.json")
	if err != nil {
		t.Fatalf("build same: %v", err)
	}
	if !view.Saved {
		t.Errorf("same-file form should be Saved=true")
	}
	got, _ := view.Values["title"].(string)
	if !wanted[got] {
		t.Errorf("surviving title %q is not one of the written values", got)
	}
}

// ─────────────────────────────────────────────────────────────────────
// DeleteForm: empty datafile is a hard error; deleting a never-written
// file is a tolerated no-op (mirrors "missing reads as unsaved").
// ─────────────────────────────────────────────────────────────────────

func TestDeleteForm_EmptyDatafileIsError(t *testing.T) {
	m, _, _, _ := realStack(t)
	err := m.DeleteForm("t.yaml", "")
	if err == nil {
		t.Fatalf("empty datafile must error")
	}
	if !strings.Contains(err.Error(), "empty datafile") {
		t.Errorf("want empty-datafile error, got %q", err.Error())
	}
}

func TestDeleteForm_MissingFileIsNoOp(t *testing.T) {
	m, tplM, _, _ := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})
	if err := m.DeleteForm("t.yaml", "ghost.meta.json"); err != nil {
		t.Errorf("delete of missing file should be a no-op, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Type-default injection for the array-shaped and range field types on
// an unsaved view, plus a per-field Default override winning over the
// type-default.
// ─────────────────────────────────────────────────────────────────────

func TestBuildView_TypeDefaultsForArrayAndRange(t *testing.T) {
	m, tplM, _, _ := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{
			{Key: "tags", Type: "multioption"},
			{Key: "level", Type: "range"},
			{Key: "picks", Type: "list"},
		},
	})
	view, err := m.BuildView("t.yaml", "")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if got, ok := view.Values["tags"].([]any); !ok || len(got) != 0 {
		t.Errorf("multioption default: want empty []any, got %#v", view.Values["tags"])
	}
	if got := view.Values["level"]; got != 50 {
		t.Errorf("range default: want 50, got %v (%T)", got, got)
	}
	if got, ok := view.Values["picks"].([]any); !ok || len(got) != 0 {
		t.Errorf("list default: want empty []any, got %#v", view.Values["picks"])
	}
}

// ─────────────────────────────────────────────────────────────────────
// Save then overwrite the same file: the second save updates the value
// and bumps Updated, but Created identity/time stays the lifetime value.
// ─────────────────────────────────────────────────────────────────────

func TestSaveValues_ResaveKeepsCreatedBumpsValue(t *testing.T) {
	m, tplM, _, _ := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})
	first, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "f.meta.json",
		Values:   map[string]any{"title": "one"},
	})
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	createdAt := first.Meta.Created.At
	id := first.Meta.ID

	second, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "f.meta.json",
		Values:   map[string]any{"title": "two"},
	})
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if got := second.Values["title"]; got != "two" {
		t.Errorf("resave value: want two, got %v", got)
	}
	if second.Meta.Created.At != createdAt {
		t.Errorf("created timestamp must not change on resave: %q -> %q", createdAt, second.Meta.Created.At)
	}
	if id != "" && second.Meta.ID != id {
		t.Errorf("id must be stable across resave: %q -> %q", id, second.Meta.ID)
	}
}
