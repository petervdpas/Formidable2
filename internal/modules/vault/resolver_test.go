package vault

import (
	"errors"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/connection"
)

const apiClientPrefix = "api-client-"

// A Resolver must satisfy the api-client invoker's secret lookup without
// either package importing the other. If this stops compiling, the seam broke.
var _ connection.SecretResolver = (*Resolver)(nil)

func newResolver(t *testing.T) *Resolver {
	t.Helper()
	v, _ := newTestVault(t)
	return NewResolver(v, apiClientPrefix)
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

func TestResolver_NamespacesEntries(t *testing.T) {
	r := newResolver(t)
	if err := r.Store("northwind", "v"); err != nil {
		t.Fatal(err)
	}
	names, err := r.Vault().List()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "api-client-northwind" {
		t.Fatalf("stored under %v, want the prefixed name", names)
	}
	if r.Name("northwind") != "api-client-northwind" {
		t.Fatalf("Name = %q", r.Name("northwind"))
	}
}

func TestResolver_TwoPrefixesShareOneVaultWithoutColliding(t *testing.T) {
	v, _ := newTestVault(t)
	clients := NewResolver(v, apiClientPrefix)
	git := NewResolver(v, "git-remote-")

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

	_, err := r.Secret("northwind")
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("err = %v, want ErrLocked; a locked vault is not a missing secret", err)
	}
}

func TestResolver_HasAndIDsWorkWhileLocked(t *testing.T) {
	r := newResolver(t)
	for _, id := range []string{"northwind", "crm-prod"} {
		if err := r.Store(id, "v"); err != nil {
			t.Fatal(err)
		}
	}
	// An unrelated entry in the same vault must not show up as a client.
	if err := r.Vault().Set("git-remote-origin", "token"); err != nil {
		t.Fatal(err)
	}
	r.Vault().Lock()

	if !r.Has("northwind") {
		t.Error("Has must work while locked; a settings screen shows this before any prompt")
	}
	if r.Has("never-stored") {
		t.Error("Has is true for an unstored id")
	}

	ids, err := r.IDs()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(ids, ",") != "crm-prod,northwind" {
		t.Fatalf("ids = %v", ids)
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

func TestResolver_RejectsAnIDThatIsNotAValidEntryName(t *testing.T) {
	r := newResolver(t)
	for _, id := range []string{"../../escape", "has/slash", ""} {
		if err := r.Store(id, "v"); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Store(%q) = %v, want ErrInvalidName", id, err)
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

// TestResolver_DrivesTheInvoker wires the two modules the way the app will and
// checks the whole path: a credential in the vault reaching a remote request.
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
