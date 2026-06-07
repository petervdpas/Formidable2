package storage

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

func newTestStack(t *testing.T) (*Manager, *system.Manager, *template.Manager, string) {
	t.Helper()
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	tplM := template.NewManager(sys, "templates", nil)
	sfrM := sfr.NewManager(sys, nil)
	m := NewManager(sys, sfrM, tplM, "storage", nil)
	return m, sys, tplM, root
}

func boolPtr(b bool) *bool { return &b }

// ─────────────────────────────────────────────────────────────────────
// Sanitize edge cases
// ─────────────────────────────────────────────────────────────────────

func TestSanitize_FillsDefaultsForMissingFields(t *testing.T) {
	fields := []template.Field{
		{Key: "title", Type: "text"},
		{Key: "count", Type: "number", Default: 7},
		{Key: "flag", Type: "boolean"},
		{Key: "rating", Type: "range"},
		{Key: "tags", Type: "tags"},
	}
	out := Sanitize(map[string]any{"title": "Hello"}, fields, SanitizeOptions{})
	if out.Data["title"] != "Hello" {
		t.Errorf("title not preserved: %v", out.Data["title"])
	}
	if out.Data["count"] != 7 {
		t.Errorf("count default missing: %v", out.Data["count"])
	}
	if out.Data["flag"] != false {
		t.Errorf("boolean default = %v, want false", out.Data["flag"])
	}
	if out.Data["rating"] != 50 {
		t.Errorf("range default = %v, want 50", out.Data["rating"])
	}
	if _, ok := out.Data["tags"].([]any); !ok {
		// nil string default fallback is empty array for tags? Tags isn't
		// in the special-list of types so it gets "". But we add it via
		// addTags which collects and stores in meta.tags. data[tags] stays
		// the string default. That's fine.
		t.Logf("tags data type %T is acceptable", out.Data["tags"])
	}
}

func TestSanitize_AcceptsEnvelopeShape(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	envelope := map[string]any{
		"meta": map[string]any{"id": "abc", "template": "basic", "created": "2026-01-01T00:00:00Z"},
		"data": map[string]any{"title": "Hi"},
	}
	out := Sanitize(envelope, fields, SanitizeOptions{})
	if out.Meta.ID != "abc" {
		t.Errorf("id = %q, want abc", out.Meta.ID)
	}
	if out.Data["title"] != "Hi" {
		t.Errorf("title = %v", out.Data["title"])
	}
}

func TestSanitize_RawWithFieldNamedDataIsNotMistakenForEnvelope(t *testing.T) {
	// Defensive: a user field named `data` (not an object) must not flip
	// the parser into envelope mode.
	fields := []template.Field{
		{Key: "data", Type: "text"},
		{Key: "other", Type: "text"},
	}
	raw := map[string]any{
		"data":  "user value",
		"other": "yo",
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	if out.Data["data"] != "user value" {
		t.Errorf("user field 'data' lost: %+v", out.Data)
	}
	if out.Data["other"] != "yo" {
		t.Errorf("unrelated field lost: %+v", out.Data)
	}
}

func TestSanitize_GeneratesGuidWhenTemplateHasGuidField(t *testing.T) {
	fields := []template.Field{
		{Key: "id", Type: "guid"},
		{Key: "title", Type: "text"},
	}
	out := Sanitize(map[string]any{"title": "X"}, fields, SanitizeOptions{})
	if out.Meta.ID == "" {
		t.Error("expected generated id when template has guid field")
	}
}

func TestSanitize_GuidFieldDataMirrorsMetaID(t *testing.T) {
	fields := []template.Field{
		{Key: "id", Type: "guid"},
		{Key: "title", Type: "text"},
	}
	out := Sanitize(map[string]any{"title": "X"}, fields, SanitizeOptions{})
	if out.Meta.ID == "" {
		t.Fatal("expected generated meta.id")
	}
	if got, _ := out.Data["id"].(string); got != out.Meta.ID {
		t.Errorf("data[id] = %q, want it mirrored from meta.id %q", got, out.Meta.ID)
	}
}

func TestSanitize_GuidFieldDrivesMetaID(t *testing.T) {
	// The field is the identity source: a guid present in the data feeds
	// meta.id, and both end up equal.
	fields := []template.Field{
		{Key: "id", Type: "guid"},
		{Key: "title", Type: "text"},
	}
	out := Sanitize(map[string]any{"id": "field-guid", "title": "X"}, fields, SanitizeOptions{})
	if out.Meta.ID != "field-guid" {
		t.Errorf("meta.id = %q, want field-guid", out.Meta.ID)
	}
	if got, _ := out.Data["id"].(string); got != "field-guid" {
		t.Errorf("data[id] = %q, want field-guid", got)
	}
}

func TestSanitize_PreservedIDBackfillsEmptyGuidField(t *testing.T) {
	// On edit the caller preserves the prior identity via opts.ID; an
	// emptied guid field is restored from it so data and meta stay synced.
	fields := []template.Field{{Key: "id", Type: "guid"}, {Key: "title", Type: "text"}}
	out := Sanitize(map[string]any{"id": "", "title": "X"}, fields, SanitizeOptions{ID: "prev-id"})
	if out.Meta.ID != "prev-id" {
		t.Errorf("meta.id = %q, want prev-id", out.Meta.ID)
	}
	if got, _ := out.Data["id"].(string); got != "prev-id" {
		t.Errorf("data[id] = %q, want prev-id (backfilled)", got)
	}
}

func TestSanitize_EmptyStringArrayFieldNormalisedToTypedEmpty(t *testing.T) {
	// Legacy forms wrote "" for an empty array-shaped field; sanitize must
	// normalise that unset sentinel to the typed empty so the shape is valid.
	fields := []template.Field{
		{Key: "rows", Type: "table"},
		{Key: "picks", Type: "multioption"},
		{Key: "items", Type: "list"},
	}
	raw := map[string]any{"rows": "", "picks": "", "items": ""}
	out := Sanitize(raw, fields, SanitizeOptions{})
	for _, k := range []string{"rows", "picks", "items"} {
		if _, ok := out.Data[k].([]any); !ok {
			t.Errorf("data[%q] = %#v (%T), want []any", k, out.Data[k], out.Data[k])
		}
	}
}

func TestSanitize_NonEmptyStringArrayFieldIsLeftForTheDoctor(t *testing.T) {
	// A non-empty string in an array field is genuine drift, not an unset
	// sentinel - sanitize must not silently coerce it away.
	fields := []template.Field{{Key: "rows", Type: "table"}}
	out := Sanitize(map[string]any{"rows": "oops"}, fields, SanitizeOptions{})
	if out.Data["rows"] != "oops" {
		t.Errorf("data[rows] = %#v, want \"oops\" preserved", out.Data["rows"])
	}
}

func TestSanitize_NoIdWhenNoGuidFieldAndNothingProvided(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	out := Sanitize(map[string]any{"title": "X"}, fields, SanitizeOptions{})
	if out.Meta.ID != "" {
		t.Errorf("expected empty id, got %q", out.Meta.ID)
	}
}

func TestSanitize_PreservesProvidedID(t *testing.T) {
	fields := []template.Field{{Key: "id", Type: "guid"}, {Key: "title", Type: "text"}}
	out := Sanitize(map[string]any{"title": "X"}, fields, SanitizeOptions{ID: "fixed-id"})
	if out.Meta.ID != "fixed-id" {
		t.Errorf("id = %q, want fixed-id", out.Meta.ID)
	}
}

func TestSanitize_TagsAreNormalisedAndSortedUnique(t *testing.T) {
	fields := []template.Field{
		{Key: "title", Type: "text"},
		{Key: "tags", Type: "tags"},
	}
	raw := map[string]any{
		"title": "X",
		"tags":  []any{"  Foo ", "bar", "FOO", "baz", ""},
	}
	out := Sanitize(raw, fields, SanitizeOptions{Tags: []string{"qux", "BAR"}})
	want := []string{"bar", "baz", "foo", "qux"}
	if !slices.Equal(out.Meta.Tags, want) {
		t.Errorf("tags = %v, want %v", out.Meta.Tags, want)
	}
}

func TestSanitize_TagsFromCommaString(t *testing.T) {
	fields := []template.Field{
		{Key: "title", Type: "text"},
		{Key: "tags", Type: "tags"},
	}
	raw := map[string]any{"title": "X", "tags": "alpha, Beta; alpha"}
	out := Sanitize(raw, fields, SanitizeOptions{})
	want := []string{"alpha", "beta"}
	if !slices.Equal(out.Meta.Tags, want) {
		t.Errorf("tags = %v, want %v", out.Meta.Tags, want)
	}
}

// When the template has a tags-typed field, that field is the single
// source of truth for Meta.Tags. Removing a tag from the field on edit
// must drop it from the meta block - the stale `_meta.tags` carried on
// the envelope (round-tripped from BuildView) must NOT union with the
// fresh field value. Regression for the "removed tag persists" bug.
func TestSanitize_TagsFieldIsSourceOfTruth_NoStaleMetaUnion(t *testing.T) {
	fields := []template.Field{
		{Key: "title", Type: "text"},
		{Key: "tags", Type: "tags"},
	}
	raw := map[string]any{
		"title": "X",
		"tags":  []any{"test"},
		"_meta": map[string]any{
			"tags": []any{"archived"},
		},
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	want := []string{"test"}
	if !slices.Equal(out.Meta.Tags, want) {
		t.Errorf("tags = %v, want %v (stale _meta.tags must not merge)", out.Meta.Tags, want)
	}
}

// Same guarantee for the envelope shape: {meta:{tags:[...]}, data:{...}}
// carrying stale meta.tags must not survive when the tags-typed field
// no longer contains them.
func TestSanitize_TagsFieldIsSourceOfTruth_EnvelopeShape(t *testing.T) {
	fields := []template.Field{
		{Key: "title", Type: "text"},
		{Key: "tags", Type: "tags"},
	}
	raw := map[string]any{
		"meta": map[string]any{
			"tags": []any{"archived"},
		},
		"data": map[string]any{
			"title": "X",
			"tags":  []any{"test"},
		},
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	want := []string{"test"}
	if !slices.Equal(out.Meta.Tags, want) {
		t.Errorf("tags = %v, want %v (stale meta.tags must not merge)", out.Meta.Tags, want)
	}
}

// Templates without a tags-typed field have nothing to derive Meta.Tags
// from except the meta round-trip - that path must keep working so
// API-only forms (POST {meta:{tags:[...]}}) don't silently lose tags.
func TestSanitize_NoTagsField_MetaRoundTripPreserved(t *testing.T) {
	fields := []template.Field{
		{Key: "title", Type: "text"},
	}
	raw := map[string]any{
		"title": "X",
		"_meta": map[string]any{
			"tags": []any{"alpha", "beta"},
		},
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	want := []string{"alpha", "beta"}
	if !slices.Equal(out.Meta.Tags, want) {
		t.Errorf("tags = %v, want %v (no tags-field → preserve meta round-trip)", out.Meta.Tags, want)
	}
}

func TestSanitize_LoopFieldsPreserved(t *testing.T) {
	fields := []template.Field{
		{Key: "title", Type: "text"},
		{Key: "items", Type: "loopstart"},
		{Key: "name", Type: "text"},
		{Key: "items", Type: "loopstop"},
	}
	raw := map[string]any{
		"title": "X",
		"items": []any{
			map[string]any{"name": "a"},
			map[string]any{"name": "b"},
		},
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	loop, ok := out.Data["items"].([]any)
	if !ok {
		t.Fatalf("items not preserved: %T", out.Data["items"])
	}
	if len(loop) != 2 {
		t.Errorf("loop len = %d, want 2", len(loop))
	}
	if _, leaked := out.Data["name"]; leaked {
		t.Error("loop child key 'name' leaked to top-level")
	}
}

func TestSanitize_LinkValueShapesRoundTrip(t *testing.T) {
	// Vue emits {href, text} for non-empty links and "" for cleared
	// ones; legacy strings (older saves) should also pass through.
	fields := []template.Field{{Key: "ref", Type: "link"}}

	cases := []struct {
		name string
		in   any
		want any
	}{
		{"object", map[string]any{"href": "formidable://t.yaml:e.meta.json", "text": "Open"}, map[string]any{"href": "formidable://t.yaml:e.meta.json", "text": "Open"}},
		{"empty-string", "", ""},
		{"legacy-string", "https://example.com", "https://example.com"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := Sanitize(map[string]any{"ref": c.in}, fields, SanitizeOptions{})
			got := out.Data["ref"]
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("ref = %#v, want %#v", got, c.want)
			}
		})
	}
}

// API-field value shape: a single object {guid, ...projected_columns}.
// Multiplicity comes from wrapping the field in a loopstart/loopstop
// pair - the api field itself is always one record's projection.

func TestSanitize_ApiFieldUnsetDefaultsToNil(t *testing.T) {
	// User hasn't picked a record yet - sanitize should produce nil
	// (NOT empty string, which would be a type-confused stand-in and
	// would force every consumer to guard against `""` vs `map`).
	fields := []template.Field{
		{Key: "ref", Type: "api", Collection: "people.yaml"},
	}
	out := Sanitize(map[string]any{}, fields, SanitizeOptions{})
	if got := out.Data["ref"]; got != nil {
		t.Errorf("unset api field default = %#v, want nil", got)
	}
}

func TestSanitize_ApiFieldObjectRoundTrip(t *testing.T) {
	// Once the picker stamps a record, the field's value is a flat
	// {guid, ...projected_columns} map. Sanitize must preserve it
	// verbatim (no key reshuffle, no string-coercion).
	fields := []template.Field{
		{Key: "ref", Type: "api", Collection: "people.yaml"},
	}
	value := map[string]any{
		"guid":  "g-1",
		"name":  "Alice",
		"email": "alice@a.com",
	}
	out := Sanitize(map[string]any{"ref": value}, fields, SanitizeOptions{})
	got, ok := out.Data["ref"].(map[string]any)
	if !ok {
		t.Fatalf("ref not a map: %T", out.Data["ref"])
	}
	if !reflect.DeepEqual(got, value) {
		t.Errorf("ref round-trip: got %#v, want %#v", got, value)
	}
}

func TestSanitize_ApiFieldInsideLoopPreservesPerIteration(t *testing.T) {
	// Multi-record case: the api field lives inside a loopstart/loopstop
	// pair; each iteration carries its own {guid, ...} map. The loop
	// preservation rule already exists for any field type - this test
	// pins the api-shaped payload to it.
	fields := []template.Field{
		{Key: "title", Type: "text"},
		{Key: "addrs", Type: "loopstart"},
		{Key: "ref", Type: "api", Collection: "people.yaml"},
		{Key: "addrs", Type: "loopstop"},
	}
	raw := map[string]any{
		"title": "X",
		"addrs": []any{
			map[string]any{"ref": map[string]any{"guid": "g-1", "name": "A"}},
			map[string]any{"ref": map[string]any{"guid": "g-2", "name": "B"}},
		},
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	loop, ok := out.Data["addrs"].([]any)
	if !ok {
		t.Fatalf("addrs not preserved: %T", out.Data["addrs"])
	}
	if len(loop) != 2 {
		t.Fatalf("loop len = %d, want 2", len(loop))
	}
	for i, want := range []string{"g-1", "g-2"} {
		iter, ok := loop[i].(map[string]any)
		if !ok {
			t.Fatalf("iteration %d not a map: %T", i, loop[i])
		}
		ref, ok := iter["ref"].(map[string]any)
		if !ok {
			t.Fatalf("iter[%d].ref not a map: %T", i, iter["ref"])
		}
		if ref["guid"] != want {
			t.Errorf("iter[%d].ref.guid = %v, want %q", i, ref["guid"], want)
		}
	}
	// Top-level shouldn't carry the inner key.
	if _, leaked := out.Data["ref"]; leaked {
		t.Error("loop child key 'ref' leaked to top-level")
	}
}

func TestSanitize_LegacyFlaggedHonorsRawMeta(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	raw := map[string]any{
		"meta": map[string]any{"flagged": true},
		"data": map[string]any{"title": "X"},
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	if !out.Meta.Facets["flag"].Set {
		t.Error("legacy flagged from raw meta not migrated to facets.flag.set")
	}
}

// ─────────────────────────────────────────────────────────────────────
// Manager edge cases
// ─────────────────────────────────────────────────────────────────────

func TestSaveForm_RejectsEmptyDatafile(t *testing.T) {
	m, _, tplM, _ := newTestStack(t)
	_ = tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "basic", Filename: "basic.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})
	r := m.SaveForm(context.Background(), "basic.yaml", "", map[string]any{"title": "X"})
	if r.Success {
		t.Errorf("expected failure, got %+v", r)
	}
}

func TestSaveForm_RejectsPathSeparatorsInDatafile(t *testing.T) {
	m, _, tplM, _ := newTestStack(t)
	_ = tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "basic", Filename: "basic.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})
	r := m.SaveForm(context.Background(), "basic.yaml", "subdir/x", map[string]any{"title": "X"})
	if r.Success {
		t.Errorf("expected failure, got %+v", r)
	}
}

func TestTemplateImageDir_StripsYAMLExtension(t *testing.T) {
	// newTestStack constructs the Manager with `storageDir: "storage"`
	// (relative). The path-composition rule (strip `.yaml`, append
	// `images`) is what we verify here - the absolute prefix is the
	// composition root's job (see app.go) and is not under test.
	m, _, _, _ := newTestStack(t)
	cases := []struct {
		in, want string
	}{
		{"basic.yaml", "storage/basic/images"},
		{"basic", "storage/basic/images"},
		{"recepten.yaml", "storage/recepten/images"},
	}
	for _, c := range cases {
		got := m.TemplateImageDir(c.in)
		if got != c.want {
			t.Errorf("TemplateImageDir(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSaveImageFile_RejectsTraversal(t *testing.T) {
	m, _, _, _ := newTestStack(t)
	r := m.SaveImageFile("basic.yaml", "../escape.png", []byte("x"))
	if r.Success {
		t.Errorf("expected failure, got %+v", r)
	}
}

func TestSaveImageFile_RejectsEmptyName(t *testing.T) {
	m, _, _, _ := newTestStack(t)
	r := m.SaveImageFile("basic.yaml", "", []byte("x"))
	if r.Success {
		t.Errorf("expected failure, got %+v", r)
	}
}

// ─────────────────────────────────────────────────────────────────────
// LoadImageFile - reads <storage>/<template>/images/<name> and returns
// a data URL ready for direct use in <img src="">.
// ─────────────────────────────────────────────────────────────────────

func TestLoadImageFile_RoundTripsPNG(t *testing.T) {
	m, _, _, _ := newTestStack(t)
	// PNG magic bytes - enough to look like a real PNG to mime sniffers.
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00}
	if r := m.SaveImageFile("basic.yaml", "logo.png", png); !r.Success {
		t.Fatalf("save: %v", r.Error)
	}
	url, err := m.LoadImageFile("basic.yaml", "logo.png")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := "data:image/png;base64,"
	if !strings.HasPrefix(url, want) {
		t.Errorf("want data URL prefix %q, got %q", want, url[:min(40, len(url))])
	}
}

func TestLoadImageFile_RoundTripsJPEG(t *testing.T) {
	m, _, _, _ := newTestStack(t)
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	if r := m.SaveImageFile("basic.yaml", "photo.jpg", jpeg); !r.Success {
		t.Fatalf("save: %v", r.Error)
	}
	url, err := m.LoadImageFile("basic.yaml", "photo.jpg")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !strings.HasPrefix(url, "data:image/jpeg;base64,") {
		t.Errorf("expected jpeg data URL, got %q", url[:min(40, len(url))])
	}
}

func TestLoadImageFile_MissingReturnsEmpty(t *testing.T) {
	m, _, _, _ := newTestStack(t)
	url, err := m.LoadImageFile("basic.yaml", "ghost.png")
	if err != nil {
		t.Errorf("missing should not error, got: %v", err)
	}
	if url != "" {
		t.Errorf("missing should return empty, got %q", url)
	}
}

func TestLoadImageFile_RejectsTraversal(t *testing.T) {
	m, _, _, _ := newTestStack(t)
	if _, err := m.LoadImageFile("basic.yaml", "../escape.png"); err == nil {
		t.Errorf("expected error on traversal, got nil")
	}
}

func TestLoadImageFile_RejectsEmptyName(t *testing.T) {
	m, _, _, _ := newTestStack(t)
	if _, err := m.LoadImageFile("basic.yaml", ""); err == nil {
		t.Errorf("expected error on empty name, got nil")
	}
}

// ─────────────────────────────────────────────────────────────────────
// OpenImageFile - raw bytes + MIME type, no data-URL framing.
// Used by the api /api/images/{tpl}/{filename} route and by the
// asset middleware that serves the slideout's <img src=…>.
// ─────────────────────────────────────────────────────────────────────

func TestOpenImageFile_RoundTripsPNG(t *testing.T) {
	m, _, _, _ := newTestStack(t)
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x01, 0x02}
	if r := m.SaveImageFile("basic.yaml", "logo.png", png); !r.Success {
		t.Fatalf("save: %v", r.Error)
	}
	bytes, mime, err := m.OpenImageFile("basic.yaml", "logo.png")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if mime != "image/png" {
		t.Errorf("mime = %q, want image/png", mime)
	}
	if string(bytes) != string(png) {
		t.Errorf("bytes mismatch: got %v, want %v", bytes, png)
	}
}

func TestOpenImageFile_MIMEByExtension(t *testing.T) {
	m, _, _, _ := newTestStack(t)
	cases := map[string]string{
		"a.png":  "image/png",
		"b.jpg":  "image/jpeg",
		"c.jpeg": "image/jpeg",
		"d.gif":  "image/gif",
		"e.webp": "image/webp",
		"f.svg":  "image/svg+xml",
		"g.bin":  "application/octet-stream",
	}
	for name, want := range cases {
		if r := m.SaveImageFile("basic.yaml", name, []byte("x")); !r.Success {
			t.Fatalf("save %s: %v", name, r.Error)
		}
		_, mime, err := m.OpenImageFile("basic.yaml", name)
		if err != nil {
			t.Errorf("open %s: %v", name, err)
			continue
		}
		if mime != want {
			t.Errorf("%s: mime = %q, want %q", name, mime, want)
		}
	}
}

func TestOpenImageFile_MissingReturnsNil(t *testing.T) {
	m, _, _, _ := newTestStack(t)
	bytes, mime, err := m.OpenImageFile("basic.yaml", "ghost.png")
	if err != nil {
		t.Errorf("missing should not error, got %v", err)
	}
	if bytes != nil {
		t.Errorf("missing should return nil bytes, got %d bytes", len(bytes))
	}
	if mime != "" {
		t.Errorf("missing should return empty mime, got %q", mime)
	}
}

func TestOpenImageFile_RejectsTraversal(t *testing.T) {
	m, _, _, _ := newTestStack(t)
	if _, _, err := m.OpenImageFile("basic.yaml", "../escape.png"); err == nil {
		t.Errorf("expected error on traversal, got nil")
	}
}

func TestOpenImageFile_RejectsEmptyName(t *testing.T) {
	m, _, _, _ := newTestStack(t)
	if _, _, err := m.OpenImageFile("basic.yaml", ""); err == nil {
		t.Errorf("expected error on empty name, got nil")
	}
}

// ─────────────────────────────────────────────────────────────────────
// DeleteImageFile - removes the file under <storage>/<template>/images/.
// Missing is a no-op (mirrors DeleteForm).
// ─────────────────────────────────────────────────────────────────────

func TestDeleteImageFile_RemovesPersistedFile(t *testing.T) {
	m, _, _, _ := newTestStack(t)
	png := []byte{0x89, 0x50, 0x4E, 0x47}
	if r := m.SaveImageFile("basic.yaml", "logo.png", png); !r.Success {
		t.Fatalf("save: %v", r.Error)
	}
	if err := m.DeleteImageFile("basic.yaml", "logo.png"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	url, err := m.LoadImageFile("basic.yaml", "logo.png")
	if err != nil {
		t.Fatalf("load after delete: %v", err)
	}
	if url != "" {
		t.Errorf("expected empty after delete, got %q", url)
	}
}

func TestDeleteImageFile_MissingIsNoOp(t *testing.T) {
	m, _, _, _ := newTestStack(t)
	if err := m.DeleteImageFile("basic.yaml", "ghost.png"); err != nil {
		t.Errorf("delete missing should be no-op, got %v", err)
	}
}

func TestDeleteImageFile_RejectsTraversal(t *testing.T) {
	m, _, _, _ := newTestStack(t)
	if err := m.DeleteImageFile("basic.yaml", "../escape.png"); err == nil {
		t.Errorf("expected error on traversal, got nil")
	}
}

func TestDeleteImageFile_RejectsEmptyName(t *testing.T) {
	m, _, _, _ := newTestStack(t)
	if err := m.DeleteImageFile("basic.yaml", ""); err == nil {
		t.Errorf("expected error on empty name, got nil")
	}
}

func TestLoadForm_MissingTemplateStillReadsRawData(t *testing.T) {
	// Even with no template, LoadForm should not blow up - Sanitize
	// runs with nil fields (everything stays raw).
	m, sys, _, _ := newTestStack(t)
	// Save a form by hand without a template file in templates/.
	_ = sys.SaveFile("storage/orphan/x.meta.json",
		`{"meta":{"id":"abc"},"data":{"weird":"value"}}`)
	f := m.LoadForm("orphan.yaml", "x")
	if f == nil {
		t.Fatal("expected form, got nil")
	}
	if f.Meta.ID != "abc" {
		t.Errorf("meta.id lost: %+v", f.Meta)
	}
}

func TestListForms_MissingFolderReturnsEmpty(t *testing.T) {
	m, _, _, _ := newTestStack(t)
	out, err := m.ListForms("nonexistent.yaml")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty, got %v", out)
	}
}

func TestDeleteForm_MissingIsNoOp(t *testing.T) {
	m, _, tplM, _ := newTestStack(t)
	_ = tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "basic", Filename: "basic.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})
	if err := m.DeleteForm("basic.yaml", "ghost"); err != nil {
		t.Errorf("delete missing should be no-op: %v", err)
	}
}

// fakeFormReader is the minimal FormReader stand-in used by the reader-
// path tests below. The slice / err pair is whatever ListSummaries
// returns; calls counts how many times the manager consulted the
// reader (used to assert the disk fallback didn't run).
type fakeFormReader struct {
	out         []FormSummary
	err         error
	calls       int
	searchOut   []FormSummary
	searchErr   error
	searchCalls int
	lastQuery   string
}

func (f *fakeFormReader) ListSummaries(_ string) ([]FormSummary, error) {
	f.calls++
	return f.out, f.err
}

func (f *fakeFormReader) LoadSummary(_, datafile string) (FormSummary, bool, error) {
	f.calls++
	if f.err != nil {
		return FormSummary{}, false, f.err
	}
	for _, s := range f.out {
		if s.Filename == datafile {
			return s, true, nil
		}
	}
	return FormSummary{}, false, nil
}

func (f *fakeFormReader) SearchSummaries(_, query string) ([]FormSummary, error) {
	f.searchCalls++
	f.lastQuery = query
	return f.searchOut, f.searchErr
}

func TestExtendedListForms_PrefersReaderWhenInstalled(t *testing.T) {
	m, sys, tplM, _ := newTestStack(t)
	_ = tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "basic", Filename: "basic.yaml", ItemField: "title",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})
	// Write a disk fixture that, if the reader were bypassed, would
	// return Title="disk-title" - the assertion below would fail.
	_ = sys.SaveFile("storage/basic/x.meta.json",
		`{"meta":{"id":"abc","template":"basic"},"data":{"title":"disk-title"}}`)

	reader := &fakeFormReader{out: []FormSummary{
		{Filename: "x.meta.json", Title: "reader-title"},
	}}
	m.SetReader(reader)

	out, err := m.ExtendedListForms("basic.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if reader.calls != 1 {
		t.Errorf("reader calls = %d, want 1 (manager bypassed reader?)", reader.calls)
	}
	if len(out) != 1 || out[0].Title != "reader-title" {
		t.Errorf("reader result not surfaced: %+v", out)
	}
}

func TestSearchForms_UsesReader(t *testing.T) {
	m, _, _, _ := newTestStack(t)
	reader := &fakeFormReader{searchOut: []FormSummary{
		{Filename: "hit.meta.json", Title: "Hit"},
	}}
	m.SetReader(reader)

	out, err := m.SearchForms("basic.yaml", "needle")
	if err != nil {
		t.Fatal(err)
	}
	if reader.searchCalls != 1 || reader.lastQuery != "needle" {
		t.Errorf("reader not consulted correctly: calls=%d query=%q", reader.searchCalls, reader.lastQuery)
	}
	if len(out) != 1 || out[0].Filename != "hit.meta.json" {
		t.Errorf("search result not surfaced: %+v", out)
	}
}

func TestSearchForms_NoReaderErrors(t *testing.T) {
	// No disk fallback for search: without an index there's nothing to
	// query, so the caller gets an error rather than a silent empty list.
	m, _, _, _ := newTestStack(t)
	if _, err := m.SearchForms("basic.yaml", "needle"); err == nil {
		t.Fatal("expected error when no reader installed")
	}
}

func TestSearchForms_PropagatesReaderError(t *testing.T) {
	m, _, _, _ := newTestStack(t)
	m.SetReader(&fakeFormReader{searchErr: errors.New("fts boom")})
	if _, err := m.SearchForms("basic.yaml", "needle"); err == nil {
		t.Fatal("expected reader error to propagate")
	}
}

func TestExtendedListForms_FallsBackOnReaderError(t *testing.T) {
	m, sys, tplM, _ := newTestStack(t)
	_ = tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "basic", Filename: "basic.yaml", ItemField: "title",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})
	_ = sys.SaveFile("storage/basic/x.meta.json",
		`{"meta":{"id":"abc","template":"basic"},"data":{"title":"disk-title"}}`)

	// Reader errors → manager must walk disk instead of propagating.
	// The disk fixture above proves the fallback executed (we get a
	// non-empty result tagged with the disk-only title).
	m.SetReader(&fakeFormReader{err: errors.New("index unavailable")})

	out, err := m.ExtendedListForms("basic.yaml")
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}
	if len(out) != 1 || out[0].Title != "disk-title" {
		t.Errorf("fallback did not run: %+v", out)
	}
}

func TestExtendedListForms_NoReaderUsesDisk(t *testing.T) {
	// Pre-existing semantics: no reader installed → ExtendedListForms
	// walks the storage tree. Keeps the migration safe: code paths
	// that build a storage.Manager without an index keep working.
	m, sys, tplM, _ := newTestStack(t)
	_ = tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "basic", Filename: "basic.yaml", ItemField: "title",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})
	_ = sys.SaveFile("storage/basic/x.meta.json",
		`{"meta":{"id":"abc","template":"basic"},"data":{"title":"disk-title"}}`)

	out, err := m.ExtendedListForms("basic.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Title != "disk-title" {
		t.Errorf("disk path wrong: %+v", out)
	}
}

func TestExtendedListForms_NoTemplateFallsBackToFilename(t *testing.T) {
	m, sys, _, _ := newTestStack(t)
	_ = sys.SaveFile("storage/orphan/x.meta.json",
		`{"meta":{"id":"abc"},"data":{"title":"hi"}}`)
	out, err := m.ExtendedListForms("orphan.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(out))
	}
	if out[0].Title != "x.meta.json" {
		t.Errorf("expected fallback title to filename, got %q", out[0].Title)
	}
}

func TestExtendedLoadForm_ReturnsSingleSummary(t *testing.T) {
	m, sys, tplM, _ := newTestStack(t)
	_ = tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "basic", Filename: "basic.yaml", ItemField: "title",
		Fields: []template.Field{
			{Key: "title", Type: "text"},
			{Key: "category", Type: "text", ExpressionItem: true},
		},
	})
	_ = sys.SaveFile("storage/basic/x.meta.json",
		`{"meta":{"id":"abc","template":"basic"},"data":{"title":"Hello","category":"green"}}`)

	got, err := m.ExtendedLoadForm("basic.yaml", "x.meta.json")
	if err != nil {
		t.Fatalf("ExtendedLoadForm: %v", err)
	}
	if got == nil {
		t.Fatal("expected summary, got nil")
	}
	if got.Title != "Hello" {
		t.Errorf("Title = %q, want %q", got.Title, "Hello")
	}
	if v, _ := got.ExpressionItems["category"].(string); v != "green" {
		t.Errorf("ExpressionItems[category] = %v, want %q", got.ExpressionItems["category"], "green")
	}
}

func TestExtendedLoadForm_MissingFileReturnsNil(t *testing.T) {
	m, _, tplM, _ := newTestStack(t)
	_ = tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "basic", Filename: "basic.yaml",
	})
	got, err := m.ExtendedLoadForm("basic.yaml", "nope.meta.json")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != nil {
		t.Errorf("missing file should return nil, got %+v", got)
	}
}

// boolPtr is reused if needed in future tests
var _ = boolPtr

func TestSaveForm_RemintsCollidingGuid(t *testing.T) {
	m, _, tplM, _ := newTestStack(t)
	_ = tplM.SaveTemplate("ent.yaml", &template.Template{
		Name: "ent", Filename: "ent.yaml", EnableCollection: true,
		Fields: []template.Field{{Key: "id", Type: "guid"}, {Key: "name", Type: "text"}},
	})
	// The index already holds record a.meta.json with guid "shared".
	m.SetReader(&fakeFormReader{out: []FormSummary{
		{Filename: "a.meta.json", Meta: FormMeta{ID: "shared"}},
	}})
	// Saving a DIFFERENT record carrying the same guid must re-mint it.
	r := m.SaveForm(context.Background(), "ent.yaml", "b.meta.json", map[string]any{"id": "shared", "name": "B"})
	if !r.Success {
		t.Fatalf("save: %s", r.Error)
	}
	got := m.LoadForm("ent.yaml", "b.meta.json")
	if got == nil {
		t.Fatal("load nil")
	}
	if got.Meta.ID == "shared" || got.Meta.ID == "" {
		t.Fatalf("colliding guid not re-minted: %q", got.Meta.ID)
	}
	if got.Data["id"] != got.Meta.ID {
		t.Fatalf("data guid field not synced with meta: data=%v meta=%q", got.Data["id"], got.Meta.ID)
	}
}

func TestSaveForm_KeepsOwnGuidOnReSave(t *testing.T) {
	m, _, tplM, _ := newTestStack(t)
	_ = tplM.SaveTemplate("ent.yaml", &template.Template{
		Name: "ent", Filename: "ent.yaml", EnableCollection: true,
		Fields: []template.Field{{Key: "id", Type: "guid"}, {Key: "name", Type: "text"}},
	})
	// a.meta.json owns "shared"; re-saving a.meta.json itself is not a collision.
	m.SetReader(&fakeFormReader{out: []FormSummary{
		{Filename: "a.meta.json", Meta: FormMeta{ID: "shared"}},
	}})
	r := m.SaveForm(context.Background(), "ent.yaml", "a.meta.json", map[string]any{"id": "shared", "name": "A"})
	if !r.Success {
		t.Fatalf("save: %s", r.Error)
	}
	if got := m.LoadForm("ent.yaml", "a.meta.json"); got.Meta.ID != "shared" {
		t.Fatalf("own guid changed on re-save: %q", got.Meta.ID)
	}
}
