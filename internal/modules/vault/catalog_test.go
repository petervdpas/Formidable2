package vault

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func newCatalog(t *testing.T) (*Catalog, string) {
	t.Helper()
	v, dir := newTestVault(t)
	return NewCatalog(v), dir
}

// Identity is opaque on disk ------------------------------------------

func TestCatalog_FilenamesLeakNothing(t *testing.T) {
	c, dir := newCatalog(t)
	if _, err := c.Put("api-client", "northwind", "bearer-token", ""); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(filepath.Join(dir, secretsDirName))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("files = %d, want 1", len(entries))
	}
	name := strings.TrimSuffix(entries[0].Name(), secretExt)
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(name) {
		t.Fatalf("filename %q is not an opaque 32-hex slot", name)
	}
	for _, leak := range []string{"api-client", "northwind", "bearer"} {
		if strings.Contains(entries[0].Name(), leak) {
			t.Errorf("filename leaks %q", leak)
		}
	}
}

func TestCatalog_PayloadCarriesIdentityNotThePath(t *testing.T) {
	c, dir := newCatalog(t)
	if _, err := c.Put("api-client", "northwind", "bearer-token", "prod key"); err != nil {
		t.Fatal(err)
	}
	files, _ := os.ReadDir(filepath.Join(dir, secretsDirName))
	raw, err := os.ReadFile(filepath.Join(dir, secretsDirName, files[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	// The record on disk is ciphertext: neither the identity nor the value is
	// readable without the key.
	for _, leak := range []string{"api-client", "northwind", "bearer-token", "prod key"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("record leaks %q in plaintext", leak)
		}
	}
}

// CRUD ------------------------------------------------------------------

func TestCatalog_RoundTrip(t *testing.T) {
	c, _ := newCatalog(t)

	entry, err := c.Put("api-client", "northwind", "bearer-token", "prod")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Category != "api-client" || entry.Key != "northwind" || entry.Description != "prod" {
		t.Fatalf("entry = %+v", entry)
	}

	got, err := c.Get("api-client", "northwind")
	if err != nil || got != "bearer-token" {
		t.Fatalf("Get = %q, %v", got, err)
	}
	if !c.Has("api-client", "northwind") {
		t.Error("Has is false after Put")
	}
	if c.Has("api-client", "nope") {
		t.Error("Has is true for an unstored key")
	}
}

func TestCatalog_ReplaceKeepsSlotAndCreatedTime(t *testing.T) {
	c, _ := newCatalog(t)
	first, err := c.Put("api-client", "northwind", "v1", "")
	if err != nil {
		t.Fatal(err)
	}

	second, err := c.Put("api-client", "northwind", "v2", "")
	if err != nil {
		t.Fatal(err)
	}
	if second.Slot != first.Slot {
		t.Errorf("slot changed on replace: %q -> %q", first.Slot, second.Slot)
	}
	if !second.CreatedUTC.Equal(first.CreatedUTC) {
		t.Errorf("created time changed on replace: %v -> %v", first.CreatedUTC, second.CreatedUTC)
	}

	got, _ := c.Get("api-client", "northwind")
	if got != "v2" {
		t.Fatalf("value = %q, want v2", got)
	}
	list, _ := c.List()
	if len(list) != 1 {
		t.Fatalf("replace created a duplicate: %+v", list)
	}
}

func TestCatalog_CategoriesAreSeparateNamespaces(t *testing.T) {
	c, _ := newCatalog(t)
	if _, err := c.Put("api-client", "shared", "client-secret", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Put("git-remote", "shared", "git-token", ""); err != nil {
		t.Fatal(err)
	}

	if got, _ := c.Get("api-client", "shared"); got != "client-secret" {
		t.Errorf("api-client value = %q", got)
	}
	if got, _ := c.Get("git-remote", "shared"); got != "git-token" {
		t.Errorf("git-remote value = %q", got)
	}
}

func TestCatalog_IdentityIsCaseInsensitive(t *testing.T) {
	c, _ := newCatalog(t)
	if _, err := c.Put("API-Client", "Northwind", "v1", ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.Get("api-client", "northwind"); got != "v1" {
		t.Fatalf("case-differing lookup missed: %q", got)
	}
	// And a differently-cased write replaces rather than duplicating.
	if _, err := c.Put("api-client", "NORTHWIND", "v2", ""); err != nil {
		t.Fatal(err)
	}
	list, _ := c.List()
	if len(list) != 1 {
		t.Fatalf("case difference created a duplicate: %+v", list)
	}
}

func TestCatalog_Delete(t *testing.T) {
	c, _ := newCatalog(t)
	if _, err := c.Put("api-client", "northwind", "v", ""); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete("api-client", "northwind"); err != nil {
		t.Fatal(err)
	}
	if c.Has("api-client", "northwind") {
		t.Fatal("still present after Delete")
	}
	if err := c.Delete("api-client", "northwind"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestCatalog_GetMissing(t *testing.T) {
	c, _ := newCatalog(t)
	if _, err := c.Get("api-client", "never"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestCatalog_RejectsEmptyIdentity(t *testing.T) {
	c, _ := newCatalog(t)
	for _, tc := range [][2]string{{"", "key"}, {"cat", ""}, {"  ", "key"}, {"cat", "  "}} {
		if _, err := c.Put(tc[0], tc[1], "v", ""); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Put(%q,%q) = %v, want ErrInvalidName; the .NET reader refuses these too", tc[0], tc[1], err)
		}
	}
}

func TestCatalog_ListSortedByCategoryThenKey(t *testing.T) {
	c, _ := newCatalog(t)
	for _, p := range [][2]string{
		{"git-remote", "origin"}, {"api-client", "zulu"}, {"api-client", "alpha"},
	} {
		if _, err := c.Put(p[0], p[1], "v", ""); err != nil {
			t.Fatal(err)
		}
	}
	list, err := c.List()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range list {
		got = append(got, e.Category+"/"+e.Key)
	}
	if strings.Join(got, ",") != "api-client/alpha,api-client/zulu,git-remote/origin" {
		t.Fatalf("order = %v", got)
	}
}

// Locked and foreign behaviour -----------------------------------------

func TestCatalog_ListRequiresUnlock(t *testing.T) {
	c, _ := newCatalog(t)
	if _, err := c.Put("api-client", "northwind", "v", ""); err != nil {
		t.Fatal(err)
	}
	c.Vault().Lock()

	// Opaque slots put identity inside the ciphertext, so unlike the plain
	// vault this cannot answer while locked. It must say so rather than
	// report an empty catalog.
	if _, err := c.List(); !errors.Is(err, ErrLocked) {
		t.Fatalf("List = %v, want ErrLocked", err)
	}
	if _, err := c.Get("api-client", "northwind"); !errors.Is(err, ErrLocked) {
		t.Fatalf("Get = %v, want ErrLocked", err)
	}
	if c.Has("api-client", "northwind") {
		t.Error("Has must not claim knowledge while locked")
	}
}

func TestCatalog_SkipsTheReservedCatalogSlot(t *testing.T) {
	c, _ := newCatalog(t)
	if _, err := c.Put("api-client", "northwind", "v", ""); err != nil {
		t.Fatal(err)
	}
	// The category catalog TaskBlaster keeps is not a user secret.
	if err := c.Vault().Set(reservedCatalogSlot, `{"Categories":["api-client"]}`); err != nil {
		t.Fatal(err)
	}

	list, foreign, err := c.ListWithForeign()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("entries = %+v", list)
	}
	if foreign != 0 {
		t.Errorf("the reserved slot must be skipped, not counted as foreign (got %d)", foreign)
	}
}

func TestCatalog_CountsRecordsItCannotRead(t *testing.T) {
	c, _ := newCatalog(t)
	if _, err := c.Put("api-client", "northwind", "v", ""); err != nil {
		t.Fatal(err)
	}
	// A legacy plain-value record, or one from a schema this build predates.
	if err := c.Vault().Set(NewSlot(), "just a bare value"); err != nil {
		t.Fatal(err)
	}

	list, foreign, err := c.ListWithForeign()
	if err != nil {
		t.Fatalf("one unreadable record must not fail the whole listing: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("entries = %+v", list)
	}
	if foreign != 1 {
		t.Errorf("foreign = %d, want 1; silently dropping it would under-report", foreign)
	}
}

// Wire format ------------------------------------------------------------

func TestEnvelope_WireFormatMatchesTheDotNetSchema(t *testing.T) {
	env := NewEnvelope("api-client", "northwind", "bearer", "prod", nowForTest())
	raw, err := MarshalEnvelope(env)
	if err != nil {
		t.Fatal(err)
	}

	var probe map[string]any
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		"SchemaVersion", "Category", "Key", "Value", "Description", "CreatedUtc", "UpdatedUtc",
	} {
		if _, ok := probe[field]; !ok {
			t.Errorf("missing PascalCase field %q; the .NET reader would not find it", field)
		}
	}
	if probe["SchemaVersion"] != float64(1) {
		t.Errorf("SchemaVersion = %v, want 1", probe["SchemaVersion"])
	}
	if strings.Contains(raw, "\n") {
		t.Error("the .NET writer emits compact JSON; indentation would still parse but diverges needlessly")
	}
}

func TestEnvelope_DescriptionIsExplicitNullWhenAbsent(t *testing.T) {
	raw, err := MarshalEnvelope(NewEnvelope("c", "k", "v", "", nowForTest()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, `"Description":null`) {
		t.Fatalf("want an explicit null description, got: %s", raw)
	}
}

func TestParseEnvelope_RejectsWhatTheDotNetReaderRejects(t *testing.T) {
	cases := map[string]string{
		"empty":           "",
		"malformed":       "{not json",
		"wrong version":   `{"SchemaVersion":2,"Category":"c","Key":"k","Value":"v"}`,
		"no category":     `{"SchemaVersion":1,"Category":"","Key":"k","Value":"v"}`,
		"blank category":  `{"SchemaVersion":1,"Category":"  ","Key":"k","Value":"v"}`,
		"no key":          `{"SchemaVersion":1,"Category":"c","Key":"","Value":"v"}`,
		"bare value":      "just a token",
		"missing version": `{"Category":"c","Key":"k","Value":"v"}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseEnvelope(raw); !errors.Is(err, ErrEnvelopeInvalid) {
				t.Fatalf("err = %v, want ErrEnvelopeInvalid", err)
			}
		})
	}
}

func TestParseEnvelope_AcceptsADotNetShapedRecord(t *testing.T) {
	// Exactly what System.Text.Json emits for the TaskBlaster record.
	raw := `{"SchemaVersion":1,"Category":"Azure","Key":"prod-sql",` +
		`"Value":"Server=tcp:example;","Description":"production",` +
		`"CreatedUtc":"2026-04-24T12:00:00Z","UpdatedUtc":"2026-04-24T12:34:56.1234567Z"}`
	env, err := ParseEnvelope(raw)
	if err != nil {
		t.Fatal(err)
	}
	if env.Category != "Azure" || env.Key != "prod-sql" || env.Value != "Server=tcp:example;" {
		t.Fatalf("env = %+v", env)
	}
	if env.DescriptionOr() != "production" {
		t.Errorf("description = %q", env.DescriptionOr())
	}
	if env.UpdatedUTC.Year() != 2026 {
		t.Errorf("timestamp did not parse: %v", env.UpdatedUTC)
	}
}

func TestNewSlot_IsThirtyTwoLowercaseHexAndUnique(t *testing.T) {
	seen := map[string]bool{}
	re := regexp.MustCompile(`^[0-9a-f]{32}$`)
	for range 200 {
		s := NewSlot()
		if !re.MatchString(s) {
			t.Fatalf("slot %q is not 32 lowercase hex", s)
		}
		if seen[s] {
			t.Fatalf("duplicate slot %q", s)
		}
		seen[s] = true
	}
}

// nowForTest is a fixed instant so wire-format assertions do not depend on the
// clock.
func nowForTest() time.Time {
	return time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
}
