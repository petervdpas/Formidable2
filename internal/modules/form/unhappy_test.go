package form

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// realStack wires system -> sfr -> storage -> form on a tempdir and also
// hands back the storage manager + tempdir so tests can inspect on-disk
// files. Distinct from round_trip_test.go's newRealStack, which returns
// only (form, template). Do not collapse the two: this one exposes paths.
func realStack(t *testing.T) (*Manager, *template.Manager, *storage.Manager, string) {
	t.Helper()
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	tplM := template.NewManager(sys, "templates", nil)
	sfrM := sfr.NewManager(sys, nil)
	stoM := storage.NewManager(sys, sfrM, tplM, "storage", nil)
	formM := NewManager(tplM, stoM, nil, nil)
	return formM, tplM, stoM, root
}

// formPath returns the absolute on-disk path of a saved form file.
func formPath(root, templateFilename, datafile string) string {
	stem := templateFilename[:len(templateFilename)-len(filepath.Ext(templateFilename))]
	return filepath.Join(root, "storage", stem, datafile)
}

// ─────────────────────────────────────────────────────────────────────
// Malformed form JSON on read: LoadForm returns nil, BuildView falls
// back to a defaults-filled unsaved view rather than erroring.
// ─────────────────────────────────────────────────────────────────────

func TestBuildView_MalformedJSONReadsAsUnsaved(t *testing.T) {
	m, tplM, _, root := realStack(t)
	if err := tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text", Default: "D"}},
	}); err != nil {
		t.Fatalf("save template: %v", err)
	}
	// Write garbage into the form's slot so the JSON decode fails.
	path := formPath(root, "t.yaml", "broken.meta.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not: valid json,,,"), 0o644); err != nil {
		t.Fatalf("write garbage: %v", err)
	}

	view, err := m.BuildView("t.yaml", "broken.meta.json")
	if err != nil {
		t.Fatalf("BuildView on malformed file should not error: %v", err)
	}
	if view.Saved {
		t.Errorf("malformed file should yield Saved=false, got true")
	}
	if got := view.Values["title"]; got != "D" {
		t.Errorf("default not injected on malformed read: want %q, got %v", "D", got)
	}
	if view.Datafile != "broken.meta.json" {
		t.Errorf("datafile lost: want %q, got %q", "broken.meta.json", view.Datafile)
	}
}

// A non-array JSON top level (valid JSON, wrong shape) is also tolerated:
// LoadForm requires a map, anything else reads as nil -> unsaved.
func TestBuildView_NonObjectJSONReadsAsUnsaved(t *testing.T) {
	m, tplM, _, root := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "n", Type: "number", Default: 7}},
	})
	path := formPath(root, "t.yaml", "arr.meta.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("[1,2,3]"), 0o644); err != nil {
		t.Fatalf("write array: %v", err)
	}
	view, err := m.BuildView("t.yaml", "arr.meta.json")
	if err != nil {
		t.Fatalf("BuildView: %v", err)
	}
	if view.Saved {
		t.Errorf("array-shaped JSON should be Saved=false")
	}
	if got := view.Values["n"]; got != 7 {
		t.Errorf("number default not injected: want 7, got %v", got)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Missing file: never written, BuildView synthesizes an unsaved view.
// ─────────────────────────────────────────────────────────────────────

func TestBuildView_MissingFileIsUnsavedWithDefaults(t *testing.T) {
	m, tplM, _, root := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text", Default: "Z"}},
	})
	// Confirm the file genuinely does not exist before the call.
	path := formPath(root, "t.yaml", "ghost.meta.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("precondition: file should not exist, stat err=%v", err)
	}
	view, err := m.BuildView("t.yaml", "ghost.meta.json")
	if err != nil {
		t.Fatalf("BuildView missing file should not error: %v", err)
	}
	if view.Saved {
		t.Errorf("missing file should be Saved=false")
	}
	if got := view.Values["title"]; got != "Z" {
		t.Errorf("default not injected: want %q, got %v", "Z", got)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Save-no-collateral-damage: saving one form must not touch a sibling
// form's file (mtime + bytes unchanged).
// ─────────────────────────────────────────────────────────────────────

func TestSaveValues_DoesNotTouchSiblingForm(t *testing.T) {
	m, tplM, _, root := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})

	// Save form B first, then snapshot its file.
	if _, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "b.meta.json",
		Values:   map[string]any{"title": "Bee"},
	}); err != nil {
		t.Fatalf("save B: %v", err)
	}
	bPath := formPath(root, "t.yaml", "b.meta.json")
	bInfo, err := os.Stat(bPath)
	if err != nil {
		t.Fatalf("stat B: %v", err)
	}
	bBytes, err := os.ReadFile(bPath)
	if err != nil {
		t.Fatalf("read B: %v", err)
	}
	bMtime := bInfo.ModTime()

	// Ensure the clock advances so an accidental rewrite would change mtime.
	time.Sleep(20 * time.Millisecond)

	// Now save form A twice (create then update).
	if _, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "a.meta.json",
		Values:   map[string]any{"title": "Aye"},
	}); err != nil {
		t.Fatalf("save A create: %v", err)
	}
	if _, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "a.meta.json",
		Values:   map[string]any{"title": "Aye-2"},
	}); err != nil {
		t.Fatalf("save A update: %v", err)
	}

	// B must be byte-for-byte and mtime identical: untouched.
	afterInfo, err := os.Stat(bPath)
	if err != nil {
		t.Fatalf("re-stat B: %v", err)
	}
	if !afterInfo.ModTime().Equal(bMtime) {
		t.Errorf("sibling B mtime changed: before=%v after=%v", bMtime, afterInfo.ModTime())
	}
	afterBytes, err := os.ReadFile(bPath)
	if err != nil {
		t.Fatalf("re-read B: %v", err)
	}
	if string(afterBytes) != string(bBytes) {
		t.Errorf("sibling B content changed.\nbefore:\n%s\nafter:\n%s", bBytes, afterBytes)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Concurrent saves under -race: distinct datafiles in parallel all land,
// each readable with its own value, no data races.
// ─────────────────────────────────────────────────────────────────────

func TestSaveValues_ConcurrentDistinctFiles(t *testing.T) {
	m, tplM, _, _ := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})

	const n = 12
	want := make(map[string]string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		df := "row" + string(rune('a'+i)) + ".meta.json"
		val := "v" + string(rune('a'+i))
		want[df] = val
		wg.Add(1)
		go func(df, val string) {
			defer wg.Done()
			if _, err := m.SaveValues("t.yaml", SavePayload{
				Datafile: df,
				Values:   map[string]any{"title": val},
			}); err != nil {
				t.Errorf("concurrent save %s: %v", df, err)
			}
		}(df, val)
	}
	wg.Wait()

	// Every file must be present with its own value, no cross-contamination.
	for df, val := range want {
		view, err := m.BuildView("t.yaml", df)
		if err != nil {
			t.Fatalf("build %s: %v", df, err)
		}
		if !view.Saved {
			t.Errorf("%s not saved", df)
		}
		if got := view.Values["title"]; got != val {
			t.Errorf("%s title: want %q, got %v", df, val, got)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────
// Bad / out-of-range facet selections.
//
// The form + storage layers do NOT validate a facet selection against
// the template's declared option set, nor that the facet key exists.
// These tests pin the ACTUAL current behavior: the selection round-trips
// verbatim. See suspectedBugs in the report.
// ─────────────────────────────────────────────────────────────────────

func TestSaveValues_OutOfRangeFacetSelectionRoundTripsVerbatim(t *testing.T) {
	m, tplM, _, _ := realStack(t)
	// Template declares facet "status" with options "open"/"closed".
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

	// Selected value "bogus" is not among the declared options.
	res, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "f.meta.json",
		Values:   map[string]any{"title": "x"},
		Meta: storage.FormMeta{
			Facets: map[string]storage.FacetState{
				"status": {Set: true, Selected: "bogus"},
			},
		},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok := res.Meta.Facets["status"]
	if !ok {
		t.Fatalf("status facet dropped on save; facets=%+v", res.Meta.Facets)
	}
	if !got.Set || got.Selected != "bogus" {
		t.Errorf("out-of-range selection not preserved: want {Set:true Selected:bogus}, got %+v", got)
	}

	// Confirm it also survives a fresh read from disk.
	view, err := m.BuildView("t.yaml", "f.meta.json")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if s := view.Meta.Facets["status"]; s.Selected != "bogus" {
		t.Errorf("BuildView lost out-of-range facet: %+v", view.Meta.Facets["status"])
	}
}

func TestSaveValues_UndeclaredFacetKeyRoundTripsVerbatim(t *testing.T) {
	m, tplM, _, _ := realStack(t)
	// Template declares no facets at all.
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})

	res, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "f.meta.json",
		Values:   map[string]any{"title": "x"},
		Meta: storage.FormMeta{
			Facets: map[string]storage.FacetState{
				"ghostkey": {Set: true, Selected: "whatever"},
			},
		},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok := res.Meta.Facets["ghostkey"]
	if !ok {
		t.Fatalf("undeclared facet key dropped; facets=%+v", res.Meta.Facets)
	}
	if !got.Set || got.Selected != "whatever" {
		t.Errorf("undeclared facet not preserved: want {Set:true Selected:whatever}, got %+v", got)
	}
}

// An empty Selected with Set=true is stamped-but-unselected: the canonical
// boundary between "facet touched" and "facet has a value". It must round
// trip as {Set:true, Selected:""} rather than collapsing to absent.
func TestSaveValues_FacetSetButEmptySelectionRoundTrips(t *testing.T) {
	m, tplM, _, _ := realStack(t)
	_ = tplM.SaveTemplate("t.yaml", &template.Template{
		Name: "t", Filename: "t.yaml",
		Facets: []template.Facet{{
			Key:     "status",
			Icon:    "fa-flag",
			Options: []template.FacetOption{{Label: "OPEN", Color: "green"}},
		}},
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})
	res, err := m.SaveValues("t.yaml", SavePayload{
		Datafile: "f.meta.json",
		Values:   map[string]any{"title": "x"},
		Meta: storage.FormMeta{
			Facets: map[string]storage.FacetState{
				"status": {Set: true, Selected: ""},
			},
		},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok := res.Meta.Facets["status"]
	if !ok {
		t.Fatalf("set-but-empty facet dropped; facets=%+v", res.Meta.Facets)
	}
	if !got.Set || got.Selected != "" {
		t.Errorf("set-but-empty facet changed: want {Set:true Selected:\"\"}, got %+v", got)
	}
}
