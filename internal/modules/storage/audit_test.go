package storage

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/petervdpas/formidable2/internal/modules/auth"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// ─────────────────────────────────────────────────────────────────────
// Sanitize — audit blocks (Created / Updated) round-trip + legacy migration
// ─────────────────────────────────────────────────────────────────────

func TestSanitize_NewShapeRoundTrips(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	envelope := map[string]any{
		"meta": map[string]any{
			"id":       "abc",
			"template": "basic",
			"created": map[string]any{
				"at":    "2026-05-01T10:00:00Z",
				"name":  "Alice",
				"email": "alice@a.com",
			},
			"updated": map[string]any{
				"at":    "2026-05-13T12:30:00Z",
				"name":  "Bob",
				"email": "bob@b.com",
			},
		},
		"data": map[string]any{"title": "Hi"},
	}
	out := Sanitize(envelope, fields, SanitizeOptions{})

	if out.Meta.Created.At != "2026-05-01T10:00:00Z" {
		t.Errorf("Created.At = %q", out.Meta.Created.At)
	}
	if out.Meta.Created.Name != "Alice" || out.Meta.Created.Email != "alice@a.com" {
		t.Errorf("Created = %+v", out.Meta.Created)
	}
	if out.Meta.Updated.At != "2026-05-13T12:30:00Z" {
		t.Errorf("Updated.At = %q", out.Meta.Updated.At)
	}
	if out.Meta.Updated.Name != "Bob" || out.Meta.Updated.Email != "bob@b.com" {
		t.Errorf("Updated = %+v", out.Meta.Updated)
	}
}

func TestSanitize_LegacyFlatMetaMigratesToAuditBlocks(t *testing.T) {
	// Legacy on-disk shape: flat author_name/author_email + flat
	// created/updated strings. Read-tolerance: hoist them into the
	// new Created/Updated blocks. Single legacy author covers both
	// blocks (we don't know who "last updated" was historically).
	fields := []template.Field{{Key: "title", Type: "text"}}
	envelope := map[string]any{
		"meta": map[string]any{
			"id":           "legacy-1",
			"template":     "basic",
			"author_name":  "Peter",
			"author_email": "peter@example.com",
			"created":      "2025-12-01T08:00:00Z",
			"updated":      "2026-01-15T18:00:00Z",
		},
		"data": map[string]any{"title": "Old"},
	}
	out := Sanitize(envelope, fields, SanitizeOptions{})

	if out.Meta.Created.At != "2025-12-01T08:00:00Z" {
		t.Errorf("Created.At = %q, want migrated from legacy created string", out.Meta.Created.At)
	}
	if out.Meta.Created.Name != "Peter" || out.Meta.Created.Email != "peter@example.com" {
		t.Errorf("Created identity not migrated: %+v", out.Meta.Created)
	}
	if out.Meta.Updated.At != "2026-01-15T18:00:00Z" {
		t.Errorf("Updated.At = %q, want migrated from legacy updated string", out.Meta.Updated.At)
	}
	if out.Meta.Updated.Name != "Peter" || out.Meta.Updated.Email != "peter@example.com" {
		t.Errorf("Updated identity not migrated: %+v", out.Meta.Updated)
	}
}

func TestSanitize_LegacyPartialMetaTolerated(t *testing.T) {
	// Legacy with author but no timestamps, or timestamps but no author.
	// Each path falls through to the sensible default.
	fields := []template.Field{{Key: "title", Type: "text"}}

	t.Run("author_without_timestamps", func(t *testing.T) {
		raw := map[string]any{
			"meta": map[string]any{
				"author_name":  "Solo",
				"author_email": "solo@x.com",
			},
			"data": map[string]any{"title": "X"},
		}
		out := Sanitize(raw, fields, SanitizeOptions{})
		if out.Meta.Created.Name != "Solo" || out.Meta.Updated.Name != "Solo" {
			t.Errorf("author should fill both blocks: created=%+v updated=%+v",
				out.Meta.Created, out.Meta.Updated)
		}
		if out.Meta.Created.At == "" || out.Meta.Updated.At == "" {
			t.Error("missing timestamps should default to now, got empty")
		}
	})

	t.Run("timestamps_without_author", func(t *testing.T) {
		raw := map[string]any{
			"meta": map[string]any{
				"created": "2025-11-01T00:00:00Z",
				"updated": "2025-11-02T00:00:00Z",
			},
			"data": map[string]any{"title": "X"},
		}
		out := Sanitize(raw, fields, SanitizeOptions{})
		if out.Meta.Created.At != "2025-11-01T00:00:00Z" {
			t.Errorf("Created.At lost: %q", out.Meta.Created.At)
		}
		if out.Meta.Updated.At != "2025-11-02T00:00:00Z" {
			t.Errorf("Updated.At lost: %q", out.Meta.Updated.At)
		}
		if out.Meta.Created.Name != "Unknown" || out.Meta.Updated.Name != "Unknown" {
			t.Errorf("missing author should fall back to Unknown")
		}
	})
}

func TestSanitize_OptsBlocksOverrideRawMeta(t *testing.T) {
	// SaveForm passes opts.Created from prev to lock the creator across
	// edits, and opts.Updated to stamp the current profile. opts must
	// beat anything in raw (which could be stale or attacker-controlled).
	fields := []template.Field{{Key: "title", Type: "text"}}
	envelope := map[string]any{
		"meta": map[string]any{
			"created": map[string]any{
				"at":    "9999-12-31T00:00:00Z",
				"name":  "Evil",
				"email": "evil@x.com",
			},
		},
		"data": map[string]any{"title": "X"},
	}
	opts := SanitizeOptions{
		Created: AuditEntry{At: "2026-05-01T10:00:00Z", Name: "Alice", Email: "alice@a.com"},
		Updated: AuditEntry{At: "2026-05-13T12:00:00Z", Name: "Bob", Email: "bob@b.com"},
	}
	out := Sanitize(envelope, fields, opts)
	if out.Meta.Created.Name != "Alice" {
		t.Errorf("opts.Created should win, got %+v", out.Meta.Created)
	}
	if out.Meta.Updated.Name != "Bob" {
		t.Errorf("opts.Updated should win, got %+v", out.Meta.Updated)
	}
}

func TestSanitize_EmitsOnlyNewShapeOnWrite(t *testing.T) {
	// The serialised JSON must NOT carry legacy flat keys. Catches the
	// "added new fields but kept old ones" footgun.
	fields := []template.Field{{Key: "title", Type: "text"}}
	out := Sanitize(map[string]any{"title": "X"}, fields, SanitizeOptions{
		Created: AuditEntry{At: "2026-05-01T10:00:00Z", Name: "A", Email: "a@x.com"},
		Updated: AuditEntry{At: "2026-05-02T10:00:00Z", Name: "B", Email: "b@x.com"},
	})
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	js := string(b)
	for _, banned := range []string{`"author_name"`, `"author_email"`} {
		if strings.Contains(js, banned) {
			t.Errorf("encoded JSON contains legacy key %s: %s", banned, js)
		}
	}
	// Created/Updated must marshal as objects, not strings.
	if !strings.Contains(js, `"created":{`) || !strings.Contains(js, `"updated":{`) {
		t.Errorf("expected nested created/updated objects, got: %s", js)
	}
}

func TestSanitize_NewShapeWinsOverLegacyFlatInSameMeta(t *testing.T) {
	// If both new and legacy keys are present on the same record (mid-
	// migration disk state), new shape wins. Legacy keys are treated as
	// fallback only.
	fields := []template.Field{{Key: "title", Type: "text"}}
	envelope := map[string]any{
		"meta": map[string]any{
			"author_name":  "Old",
			"author_email": "old@x.com",
			"created": map[string]any{
				"at":    "2026-05-01T10:00:00Z",
				"name":  "New",
				"email": "new@x.com",
			},
		},
		"data": map[string]any{"title": "X"},
	}
	out := Sanitize(envelope, fields, SanitizeOptions{})
	if out.Meta.Created.Name != "New" {
		t.Errorf("new shape must win over legacy flat author: %+v", out.Meta.Created)
	}
}

// ─────────────────────────────────────────────────────────────────────
// SaveForm — author provider + Created preservation on edit
// ─────────────────────────────────────────────────────────────────────

func newTestStackWithAuthor(t *testing.T, name, email string) (*Manager, *template.Manager) {
	t.Helper()
	m, _, tplM, _ := newTestStack(t)
	m.SetAuthorProvider(func() (string, string) { return name, email })
	return m, tplM
}

func TestSaveForm_NewFormStampsCreatedAndUpdatedFromProvider(t *testing.T) {
	m, tplM := newTestStackWithAuthor(t, "Alice", "alice@a.com")
	_ = tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "basic", Filename: "basic.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})

	before := time.Now().UTC()
	r := m.SaveForm(context.Background(), "basic.yaml", "f1", map[string]any{"title": "Hi"})
	if !r.Success {
		t.Fatalf("save: %s", r.Error)
	}

	f := m.LoadForm("basic.yaml", "f1")
	if f == nil {
		t.Fatal("load returned nil")
	}
	if f.Meta.Created.Name != "Alice" || f.Meta.Created.Email != "alice@a.com" {
		t.Errorf("Created not stamped from provider: %+v", f.Meta.Created)
	}
	if f.Meta.Updated.Name != "Alice" || f.Meta.Updated.Email != "alice@a.com" {
		t.Errorf("Updated not stamped from provider: %+v", f.Meta.Updated)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, f.Meta.Created.At)
	if err != nil {
		t.Fatalf("Created.At not RFC3339Nano: %q", f.Meta.Created.At)
	}
	if createdAt.Before(before.Add(-time.Second)) {
		t.Errorf("Created.At %v predates save time %v", createdAt, before)
	}
}

func TestSaveForm_EditPreservesCreatedRestampsUpdated(t *testing.T) {
	m, tplM := newTestStackWithAuthor(t, "Alice", "alice@a.com")
	_ = tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "basic", Filename: "basic.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})

	if r := m.SaveForm(context.Background(), "basic.yaml", "f1", map[string]any{"title": "v1"}); !r.Success {
		t.Fatalf("first save: %s", r.Error)
	}
	first := m.LoadForm("basic.yaml", "f1")
	if first == nil {
		t.Fatal("first load nil")
	}
	firstCreated := first.Meta.Created
	firstUpdated := first.Meta.Updated

	// Switch profile mid-life; common case: shared repo, second user
	// edits a record originally authored by the first.
	m.SetAuthorProvider(func() (string, string) { return "Bob", "bob@b.com" })
	time.Sleep(2 * time.Millisecond)
	if r := m.SaveForm(context.Background(), "basic.yaml", "f1", map[string]any{"title": "v2"}); !r.Success {
		t.Fatalf("edit save: %s", r.Error)
	}

	after := m.LoadForm("basic.yaml", "f1")
	if after == nil {
		t.Fatal("after load nil")
	}
	if after.Meta.Created != firstCreated {
		t.Errorf("Created changed across edit: was %+v, now %+v", firstCreated, after.Meta.Created)
	}
	if after.Meta.Updated.Name != "Bob" || after.Meta.Updated.Email != "bob@b.com" {
		t.Errorf("Updated should reflect editor Bob, got %+v", after.Meta.Updated)
	}
	if after.Meta.Updated.At == firstUpdated.At {
		t.Errorf("Updated.At should advance, stuck at %q", after.Meta.Updated.At)
	}
}

func TestSaveForm_LegacyOnDiskIsMigratedThenPreservedOnEdit(t *testing.T) {
	// Form was written by old Formidable with flat author_*/created/updated.
	// First edit by a new profile must: migrate Created.* from the legacy
	// author, then restamp Updated.* with the editor's identity.
	m, tplM := newTestStackWithAuthor(t, "Bob", "bob@b.com")
	_ = tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "basic", Filename: "basic.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})

	// Hand-write a legacy-shaped form. Bypass SaveForm so we don't
	// accidentally migrate-then-overwrite during the seed step.
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
	if err := m.fs.SaveFile("storage/basic/legacy-1.meta.json", legacy); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Bob edits it.
	if r := m.SaveForm(context.Background(), "basic.yaml", "legacy-1", map[string]any{"title": "edited"}); !r.Success {
		t.Fatalf("edit save: %s", r.Error)
	}

	f := m.LoadForm("basic.yaml", "legacy-1")
	if f == nil {
		t.Fatal("load nil")
	}
	if f.Meta.Created.At != "2025-10-01T00:00:00Z" {
		t.Errorf("Created.At not migrated/preserved: %q", f.Meta.Created.At)
	}
	if f.Meta.Created.Name != "Original Peter" || f.Meta.Created.Email != "peter@example.com" {
		t.Errorf("Created identity not migrated: %+v", f.Meta.Created)
	}
	if f.Meta.Updated.Name != "Bob" || f.Meta.Updated.Email != "bob@b.com" {
		t.Errorf("Updated should be editor Bob, got %+v", f.Meta.Updated)
	}
}

// ─────────────────────────────────────────────────────────────────────
// SaveForm — ctx-scoped auth.Identity wins over the AuthorProvider
// (the HTTP API path threads request context; Wails IPC does not)
// ─────────────────────────────────────────────────────────────────────

func TestSaveForm_CtxIdentityWinsOverAuthorProvider(t *testing.T) {
	m, tplM := newTestStackWithAuthor(t, "Active Profile", "profile@example.com")
	_ = tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "basic", Filename: "basic.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})

	ctx := auth.WithIdentity(context.Background(), auth.Identity{
		Kind: auth.KindSubscription, Subject: "sub-7",
		Name: "External Caller", Email: "external@api.com",
	})

	if r := m.SaveForm(ctx, "basic.yaml", "f1", map[string]any{"title": "Hi"}); !r.Success {
		t.Fatalf("save: %s", r.Error)
	}
	f := m.LoadForm("basic.yaml", "f1")
	if f == nil {
		t.Fatal("load nil")
	}
	if f.Meta.Created.Name != "External Caller" || f.Meta.Updated.Name != "External Caller" {
		t.Errorf("ctx Identity should beat AuthorProvider, got Created=%+v Updated=%+v",
			f.Meta.Created, f.Meta.Updated)
	}
	if f.Meta.Created.Email != "external@api.com" {
		t.Errorf("Email not from ctx: %+v", f.Meta.Created)
	}
}

func TestSaveForm_InvalidCtxIdentityFallsBackToProvider(t *testing.T) {
	m, tplM := newTestStackWithAuthor(t, "Active Profile", "profile@example.com")
	_ = tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "basic", Filename: "basic.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})

	// Identity with missing Subject is !Valid() — stamp must ignore it
	// and fall through to the AuthorProvider rather than silently
	// attribute the write to a malformed caller.
	ctx := auth.WithIdentity(context.Background(), auth.Identity{
		Kind: auth.KindDesktop, Name: "Suspicious", Email: "x@x.com",
	})
	if r := m.SaveForm(ctx, "basic.yaml", "f1", map[string]any{"title": "X"}); !r.Success {
		t.Fatalf("save: %s", r.Error)
	}
	f := m.LoadForm("basic.yaml", "f1")
	if f.Meta.Updated.Name != "Active Profile" {
		t.Errorf("invalid ctx Identity must fall back to provider, got %+v", f.Meta.Updated)
	}
}

func TestSaveForm_NoProviderFallsBackToUnknown(t *testing.T) {
	// Direct storage.Manager usage without composition-root wiring
	// (early-boot tests, integrity tools, ad-hoc scripts) must still
	// produce a valid form. Identity falls back to Unknown.
	m, _, tplM, _ := newTestStack(t)
	_ = tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "basic", Filename: "basic.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})

	if r := m.SaveForm(context.Background(), "basic.yaml", "f1", map[string]any{"title": "X"}); !r.Success {
		t.Fatalf("save: %s", r.Error)
	}
	f := m.LoadForm("basic.yaml", "f1")
	if f.Meta.Created.Name != "Unknown" || f.Meta.Created.Email != "unknown@example.com" {
		t.Errorf("no-provider Created should be Unknown: %+v", f.Meta.Created)
	}
	if f.Meta.Updated.Name != "Unknown" {
		t.Errorf("no-provider Updated should be Unknown: %+v", f.Meta.Updated)
	}
}
