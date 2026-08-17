package vault

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestNormalizeCategories(t *testing.T) {
	got := NormalizeCategories([]string{
		"  zulu ", "alpha", "", "   ", "ALPHA", "Bravo", "alpha  ",
	})
	if strings.Join(got, ",") != "alpha,Bravo,zulu" {
		t.Fatalf("got %v; want trimmed, blank-free, case-insensitively deduped and sorted", got)
	}
}

func TestNormalizeCategories_KeepsFirstSeenCasing(t *testing.T) {
	got := NormalizeCategories([]string{"API-Client", "api-client"})
	if len(got) != 1 || got[0] != "API-Client" {
		t.Fatalf("got %v, want the first-seen casing kept", got)
	}
}

func TestCategories_EmptyVault(t *testing.T) {
	c, _ := newCatalog(t)
	got, err := c.Categories()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want none", got)
	}
}

// A category is in use the moment a secret carries it, whether or not anyone
// curated a list. The picker has to offer those too.
func TestCategories_IncludesOnesInUse(t *testing.T) {
	c, _ := newCatalog(t)
	for _, p := range [][2]string{{"git-remote", "origin"}, {"api-client", "northwind"}} {
		if _, err := c.Put(p[0], p[1], "v", ""); err != nil {
			t.Fatal(err)
		}
	}
	got, err := c.Categories()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "api-client,git-remote" {
		t.Fatalf("got %v", got)
	}
}

func TestCategories_MergesPersistedWithInUse(t *testing.T) {
	c, _ := newCatalog(t)
	if _, err := c.Put("api-client", "northwind", "v", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AddCategory("Azure"); err != nil {
		t.Fatal(err)
	}

	got, err := c.Categories()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "api-client,Azure" {
		t.Fatalf("got %v, want the curated name and the one in use", got)
	}
}

// Adding a category must make it offerable before any secret uses it, which is
// the whole point of being able to create one from the picker.
func TestAddCategory_OfferedBeforeAnySecretUsesIt(t *testing.T) {
	c, _ := newCatalog(t)
	got, err := c.AddCategory("Azure")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "Azure" {
		t.Fatalf("got %v", got)
	}

	list, _ := c.List()
	if len(list) != 0 {
		t.Fatalf("adding a category must not create a secret: %+v", list)
	}
}

func TestAddCategory_IsIdempotentAndCaseInsensitive(t *testing.T) {
	c, _ := newCatalog(t)
	if _, err := c.AddCategory("Azure"); err != nil {
		t.Fatal(err)
	}
	got, err := c.AddCategory("azure")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %v, want one entry", got)
	}
}

func TestAddCategory_RejectsBlank(t *testing.T) {
	c, _ := newCatalog(t)
	for _, name := range []string{"", "   "} {
		if _, err := c.AddCategory(name); !errors.Is(err, ErrInvalidName) {
			t.Errorf("AddCategory(%q) = %v, want ErrInvalidName", name, err)
		}
	}
}

// Curating the list must never orphan a secret: a category still in use stays
// on the list even if the caller leaves it out.
func TestSetCategories_KeepsOnesStillInUse(t *testing.T) {
	c, _ := newCatalog(t)
	if _, err := c.Put("api-client", "northwind", "v", ""); err != nil {
		t.Fatal(err)
	}

	got, err := c.SetCategories([]string{"Azure"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "api-client,Azure" {
		t.Fatalf("got %v; dropping api-client would hide the secret using it", got)
	}
}

func TestCategories_CatalogRecordIsNotListedAsASecret(t *testing.T) {
	c, _ := newCatalog(t)
	if _, err := c.AddCategory("Azure"); err != nil {
		t.Fatal(err)
	}
	list, foreign, err := c.ListWithForeign()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 || foreign != 0 {
		t.Fatalf("entries = %+v, foreign = %d; the catalog is not a user secret", list, foreign)
	}
}

func TestCategories_RequiresUnlock(t *testing.T) {
	c, _ := newCatalog(t)
	if _, err := c.AddCategory("Azure"); err != nil {
		t.Fatal(err)
	}
	c.Vault().Lock()
	if _, err := c.Categories(); !errors.Is(err, ErrLocked) {
		t.Fatalf("err = %v, want ErrLocked", err)
	}
}

// Wire format ------------------------------------------------------------

func TestCategoryCatalog_WireFormatMatchesTheDotNetSchema(t *testing.T) {
	c, _ := newCatalog(t)
	if _, err := c.SetCategories([]string{"Azure", "api-client"}); err != nil {
		t.Fatal(err)
	}

	raw, err := c.Vault().Get(reservedCatalogSlot)
	if err != nil {
		t.Fatal(err)
	}
	var probe map[string]any
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"SchemaVersion", "Categories", "UpdatedUtc"} {
		if _, ok := probe[field]; !ok {
			t.Errorf("missing PascalCase field %q; the .NET reader would not find it", field)
		}
	}
	if probe["SchemaVersion"] != float64(1) {
		t.Errorf("SchemaVersion = %v, want 1", probe["SchemaVersion"])
	}
}

func TestCategories_ReadsADotNetShapedCatalog(t *testing.T) {
	c, _ := newCatalog(t)
	written := `{"SchemaVersion":1,"Categories":["Azure","  Git  ",""],"UpdatedUtc":"2026-04-24T12:00:00Z"}`
	if err := c.Vault().Set(reservedCatalogSlot, written); err != nil {
		t.Fatal(err)
	}

	got, err := c.Categories()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "Azure,Git" {
		t.Fatalf("got %v, want the .NET list normalised", got)
	}
}

// A catalog this build cannot read must be reported, not treated as absent:
// treating it as empty would let the next write silently replace another
// tool's curated list.
func TestCategories_RefusesAnUnreadableCatalog(t *testing.T) {
	c, _ := newCatalog(t)
	for _, raw := range []string{
		`{not json`,
		`{"SchemaVersion":2,"Categories":["x"]}`,
	} {
		if err := c.Vault().Set(reservedCatalogSlot, raw); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Categories(); !errors.Is(err, ErrCategoryCatalogInvalid) {
			t.Errorf("Categories() = %v, want ErrCategoryCatalogInvalid for %q", err, raw)
		}
	}
}
