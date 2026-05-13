package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestMigrateTemplateMeta_RewritesLegacyFlatFile(t *testing.T) {
	m, _, tplM, root := newTestStack(t)
	// Provider should be IGNORED — migration must preserve original
	// identity, not stamp the current actor. We set a clearly-wrong
	// provider here so a regression that calls SaveForm (which would
	// restamp Updated) shows up immediately.
	m.SetAuthorProvider(func() (string, string) { return "Should Not Appear", "wrong@x.com" })

	_ = tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "basic", Filename: "basic.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})

	legacy := `{
  "meta": {
    "id": "legacy-1",
    "template": "basic",
    "author_name": "Original Peter",
    "author_email": "peter@example.com",
    "created": "2025-10-01T00:00:00Z",
    "updated": "2025-10-05T00:00:00Z"
  },
  "data": {"title": "legacy"}
}`
	path := filepath.Join(root, "storage", "basic", "legacy-1.meta.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	res, err := m.MigrateTemplateMeta("basic.yaml")
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Total != 1 || res.Migrated != 1 || res.Skipped != 0 {
		t.Errorf("counts off: %+v", res)
	}
	if len(res.Errors) != 0 {
		t.Errorf("unexpected errors: %v", res.Errors)
	}

	f := m.LoadForm("basic.yaml", "legacy-1")
	if f == nil {
		t.Fatal("load nil")
	}
	if f.Meta.Created.At != "2025-10-01T00:00:00Z" {
		t.Errorf("Created.At lost: %q", f.Meta.Created.At)
	}
	if f.Meta.Created.Name != "Original Peter" || f.Meta.Created.Email != "peter@example.com" {
		t.Errorf("Created identity lost: %+v", f.Meta.Created)
	}
	// Crucial: Updated must reflect the LEGACY author, not the current
	// provider. Migration is structural; it doesn't claim authorship.
	if f.Meta.Updated.Name != "Original Peter" || f.Meta.Updated.Email != "peter@example.com" {
		t.Errorf("Updated identity should be legacy author, got %+v", f.Meta.Updated)
	}
	if f.Meta.Updated.At != "2025-10-05T00:00:00Z" {
		t.Errorf("Updated.At should preserve legacy: %q", f.Meta.Updated.At)
	}

	// And on disk: legacy keys must be gone.
	raw, _ := os.ReadFile(path)
	js := string(raw)
	for _, banned := range []string{`"author_name"`, `"author_email"`} {
		if strings.Contains(js, banned) {
			t.Errorf("disk still has legacy key %s: %s", banned, js)
		}
	}
}

func TestMigrateTemplateMeta_SkipsAlreadyNewShape(t *testing.T) {
	m, _, tplM, root := newTestStack(t)
	m.SetAuthorProvider(func() (string, string) { return "Bob", "bob@b.com" })

	_ = tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "basic", Filename: "basic.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})

	// SaveForm writes a fresh new-shape file. After it lands, MigrateTemplateMeta
	// must touch nothing (idempotent + no mtime churn).
	if r := m.SaveForm("basic.yaml", "fresh", map[string]any{"title": "X"}); !r.Success {
		t.Fatalf("seed save: %s", r.Error)
	}

	path := filepath.Join(root, "storage", "basic", "fresh.meta.json")
	statBefore, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	time.Sleep(10 * time.Millisecond) // give mtime room to differ if we wrote

	res, err := m.MigrateTemplateMeta("basic.yaml")
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Migrated != 0 || res.Skipped != 1 {
		t.Errorf("expected skipped 1, got %+v", res)
	}

	statAfter, _ := os.Stat(path)
	if !statAfter.ModTime().Equal(statBefore.ModTime()) {
		t.Errorf("mtime changed for already-new file: %v → %v",
			statBefore.ModTime(), statAfter.ModTime())
	}
}

func TestMigrateTemplateMeta_MissingFolderIsNoOp(t *testing.T) {
	m, _, _, _ := newTestStack(t)
	res, err := m.MigrateTemplateMeta("ghost.yaml")
	if err != nil {
		t.Fatalf("missing template should not error, got %v", err)
	}
	if res.Total != 0 || res.Migrated != 0 || res.Skipped != 0 || len(res.Errors) != 0 {
		t.Errorf("missing folder should yield zero counts, got %+v", res)
	}
}

func TestMigrateTemplateMeta_CorruptFileIsCountedAsError(t *testing.T) {
	m, _, tplM, root := newTestStack(t)
	_ = tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "basic", Filename: "basic.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})

	path := filepath.Join(root, "storage", "basic", "broken.meta.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	res, err := m.MigrateTemplateMeta("basic.yaml")
	if err != nil {
		t.Fatalf("migrate should not return top-level error, got %v", err)
	}
	if len(res.Errors) == 0 {
		t.Errorf("expected at least one per-file error, got %+v", res)
	}
	// Total counts every file attempt; Migrated/Skipped should not over-count.
	if res.Total != 1 {
		t.Errorf("Total = %d, want 1", res.Total)
	}
}

func TestMigrateTemplateMeta_MixedFolder(t *testing.T) {
	m, _, tplM, root := newTestStack(t)
	_ = tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "basic", Filename: "basic.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})

	// Two legacy files + one fresh new-shape file.
	legacy := func(name, author string) {
		body := map[string]any{
			"meta": map[string]any{
				"id":           name,
				"template":     "basic",
				"author_name":  author,
				"author_email": author + "@x.com",
				"created":      "2025-01-01T00:00:00Z",
				"updated":      "2025-01-02T00:00:00Z",
			},
			"data": map[string]any{"title": name},
		}
		b, _ := json.Marshal(body)
		p := filepath.Join(root, "storage", "basic", name+".meta.json")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	legacy("alpha", "Alice")
	legacy("beta", "Bob")
	m.SetAuthorProvider(func() (string, string) { return "Carol", "carol@x.com" })
	if r := m.SaveForm("basic.yaml", "gamma", map[string]any{"title": "G"}); !r.Success {
		t.Fatalf("seed gamma: %s", r.Error)
	}

	res, err := m.MigrateTemplateMeta("basic.yaml")
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if res.Total != 3 {
		t.Errorf("Total = %d, want 3", res.Total)
	}
	if res.Migrated != 2 {
		t.Errorf("Migrated = %d, want 2", res.Migrated)
	}
	if res.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", res.Skipped)
	}

	// Spot-check: alpha kept its identity, didn't get Carol stamped.
	alpha := m.LoadForm("basic.yaml", "alpha")
	if alpha == nil || alpha.Meta.Updated.Name != "Alice" {
		t.Errorf("alpha Updated.Name = %q, want Alice", alpha.Meta.Updated.Name)
	}
}
