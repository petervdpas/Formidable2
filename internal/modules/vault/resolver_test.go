package vault

import (
	"errors"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/connection"
)

const apiClientCategory = "api-client"

// A Resolver must satisfy the api-client invoker's secret lookup without
// either package importing the other. If this stops compiling, the seam broke.
var _ connection.SecretResolver = (*Resolver)(nil)

func newResolver(t *testing.T) *Resolver {
	t.Helper()
	v, _ := newTestVault(t)
	return NewResolver(v, apiClientCategory)
}

func TestResolver_RoundTrip(t *testing.T) {
	r := newResolver(t)

	if err := r.Store("northwind", "odata-bearer"); err != nil {
		t.Fatal(err)
	}
	got, err := r.Secret("northwind")
	if err != nil {
		t.Fatal(err)
	}
	if got != "odata-bearer" {
		t.Fatalf("secret = %q", got)
	}
}

func TestResolver_StoresUnderItsCategoryWithAnOpaqueSlot(t *testing.T) {
	r := newResolver(t)
	if err := r.Store("northwind", "v"); err != nil {
		t.Fatal(err)
	}

	slots, err := r.Vault().List()
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 1 {
		t.Fatalf("slots = %v", slots)
	}
	if strings.Contains(slots[0], "northwind") || strings.Contains(slots[0], "api-client") {
		t.Fatalf("slot %q leaks the identity", slots[0])
	}

	entries, err := NewCatalog(r.Vault()).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Category != apiClientCategory || entries[0].Key != "northwind" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestResolver_CategoriesShareOneVaultWithoutColliding(t *testing.T) {
	v, _ := newTestVault(t)
	clients := NewResolver(v, apiClientCategory)
	git := NewResolver(v, "git-remote")

	if err := clients.Store("shared", "client-secret"); err != nil {
		t.Fatal(err)
	}
	if err := git.Store("shared", "git-token"); err != nil {
		t.Fatal(err)
	}

	if got, _ := clients.Secret("shared"); got != "client-secret" {
		t.Errorf("client secret = %q", got)
	}
	if got, _ := git.Secret("shared"); got != "git-token" {
		t.Errorf("git secret = %q", got)
	}
}

func TestResolver_MissingSecret(t *testing.T) {
	r := newResolver(t)
	if _, err := r.Secret("never-stored"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestResolver_LockedVaultSaysSoRatherThanNotFound(t *testing.T) {
	r := newResolver(t)
	if err := r.Store("northwind", "v"); err != nil {
		t.Fatal(err)
	}
	r.Vault().Lock()

	if _, err := r.Secret("northwind"); !errors.Is(err, ErrLocked) {
		t.Fatalf("err = %v, want ErrLocked; a locked vault is not a missing secret", err)
	}
}

// Has and IDs now need an unlocked vault: identity lives inside the ciphertext,
// so a locked vault genuinely cannot answer. Saying no is honest; guessing yes
// would be worse than useless.
func TestResolver_HasAndIDsNeedAnUnlockedVault(t *testing.T) {
	r := newResolver(t)
	for _, id := range []string{"northwind", "crm-prod"} {
		if err := r.Store(id, "v"); err != nil {
			t.Fatal(err)
		}
	}
	// Another category in the same vault must not show up as a client.
	if _, err := NewCatalog(r.Vault()).Put("git-remote", "origin", "token", ""); err != nil {
		t.Fatal(err)
	}

	ids, err := r.IDs()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(ids, ",") != "crm-prod,northwind" {
		t.Fatalf("ids = %v, want only this category, sorted", ids)
	}
	if !r.Has("northwind") {
		t.Error("Has is false for a stored id")
	}
	if r.Has("never-stored") {
		t.Error("Has is true for an unstored id")
	}

	r.Vault().Lock()
	if r.Has("northwind") {
		t.Error("Has must not claim knowledge while locked")
	}
	if _, err := r.IDs(); !errors.Is(err, ErrLocked) {
		t.Errorf("IDs = %v, want ErrLocked", err)
	}
}

func TestResolver_Forget(t *testing.T) {
	r := newResolver(t)
	if err := r.Store("northwind", "v"); err != nil {
		t.Fatal(err)
	}
	if err := r.Forget("northwind"); err != nil {
		t.Fatal(err)
	}
	if r.Has("northwind") {
		t.Fatal("still present after Forget")
	}
	if err := r.Forget("northwind"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound on a second Forget", err)
	}
}

func TestResolver_RejectsAnEmptyID(t *testing.T) {
	r := newResolver(t)
	for _, id := range []string{"", "   "} {
		if err := r.Store(id, "v"); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Store(%q) = %v, want ErrInvalidName", id, err)
		}
	}
}

// An id that could never have been a filename is fine now, because it never
// becomes one. Opaque slots removed a whole class of naming constraint.
func TestResolver_AcceptsIDsThatCouldNotBeFilenames(t *testing.T) {
	r := newResolver(t)
	for _, id := range []string{"a/b", "../escape", "has space", "a:b"} {
		if err := r.Store(id, "v"); err != nil {
			t.Errorf("Store(%q) = %v; the id is not a path any more", id, err)
			continue
		}
		if got, err := r.Secret(id); err != nil || got != "v" {
			t.Errorf("Secret(%q) = %q, %v", id, got, err)
		}
	}
}

func TestResolver_NilIsSafe(t *testing.T) {
	var r *Resolver
	if _, err := r.Secret("x"); err == nil {
		t.Error("a nil resolver must error rather than panic")
	}
	if r.Has("x") {
		t.Error("a nil resolver has nothing")
	}
	if err := (&Resolver{}).Store("x", "y"); err == nil {
		t.Error("a resolver with no vault must error")
	}
}

func TestResolver_SatisfiesTheInvokerContract(t *testing.T) {
	r := newResolver(t)
	if err := r.Store("northwind", "bearer-from-vault"); err != nil {
		t.Fatal(err)
	}

	var resolver connection.SecretResolver = r
	got, err := resolver.Secret("northwind")
	if err != nil {
		t.Fatal(err)
	}
	if got != "bearer-from-vault" {
		t.Fatalf("the invoker would receive %q", got)
	}
}
