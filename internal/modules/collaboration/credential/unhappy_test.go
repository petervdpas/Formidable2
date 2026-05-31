package credential

import (
	"strings"
	"sync"
	"testing"

	"github.com/zalando/go-keyring"
)

// NOTE: This package runs headless. domain_test.go calls keyring.MockInit()
// in init(), swapping the OS keychain for an in-process map. These tests only
// exercise the in-process Manager logic plus that mock, never a real keychain.

func TestGet_MissingReturnsErrNotFound(t *testing.T) {
	m := NewManager()
	_, err := m.Get("https://example.com/absent.git")
	if err != keyring.ErrNotFound {
		t.Errorf("Get missing err = %v, want keyring.ErrNotFound", err)
	}
}

func TestGet_WrongKeyAfterSet(t *testing.T) {
	m := NewManager()
	const right = "https://github.com/owner/right.git"
	const wrong = "https://github.com/owner/wrong.git"
	if err := m.Set(right, "ghp_correct"); err != nil {
		t.Fatal(err)
	}
	_, err := m.Get(wrong)
	if err != keyring.ErrNotFound {
		t.Errorf("Get wrong key err = %v, want keyring.ErrNotFound", err)
	}
}

func TestGet_EmptyAccountErrorMessage(t *testing.T) {
	m := NewManager()
	_, err := m.Get("")
	if err == nil || err.Error() != "credential: empty account" {
		t.Errorf("Get empty account err = %v, want \"credential: empty account\"", err)
	}
}

func TestGet_WhitespaceAccountRejected(t *testing.T) {
	// "   " trims to empty so the guard must reject it before hitting keyring.
	m := NewManager()
	_, err := m.Get("   ")
	if err == nil || err.Error() != "credential: empty account" {
		t.Errorf("Get whitespace account err = %v, want \"credential: empty account\"", err)
	}
}

func TestSet_EmptyAccountErrorMessage(t *testing.T) {
	m := NewManager()
	err := m.Set("", "secret")
	if err == nil || err.Error() != "credential: empty account" {
		t.Errorf("Set empty account err = %v, want \"credential: empty account\"", err)
	}
}

func TestSet_WhitespaceAccountRejected(t *testing.T) {
	m := NewManager()
	err := m.Set("\t \n", "secret")
	if err == nil || err.Error() != "credential: empty account" {
		t.Errorf("Set whitespace account err = %v, want \"credential: empty account\"", err)
	}
}

func TestSet_EmptySecretErrorMessage(t *testing.T) {
	m := NewManager()
	err := m.Set("https://github.com/owner/empty.git", "")
	if err == nil || err.Error() != "credential: empty secret" {
		t.Errorf("Set empty secret err = %v, want \"credential: empty secret\"", err)
	}
}

func TestSet_WhitespaceSecretRejected(t *testing.T) {
	// A whitespace-only secret is treated as empty: a fat-fingered all-space PAT
	// must not store. The guard TrimSpaces before the empty check.
	m := NewManager()
	const account = "https://github.com/owner/ws-secret.git"
	if err := m.Set(account, "  \t "); err == nil {
		t.Fatal("Set whitespace-only secret err = nil, want rejection")
	}
	if m.Has(account) {
		t.Error("whitespace-only secret was stored, want nothing persisted")
	}
}

func TestSet_OversizedSecretRoundTrips(t *testing.T) {
	// The in-process mock imposes no size cap, unlike some real backends.
	// Assert exact length preservation without printing the material.
	m := NewManager()
	const account = "https://github.com/owner/big.git"
	const n = 1 << 16
	big := strings.Repeat("a", n)
	if err := m.Set(account, big); err != nil {
		t.Fatalf("Set oversized err = %v, want nil", err)
	}
	got, err := m.Get(account)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != n {
		t.Errorf("oversized secret length = %d, want %d", len(got), n)
	}
	if got != big {
		t.Error("oversized secret round-trip mismatch")
	}
}

func TestSet_OverwriteThenGetReturnsLatest(t *testing.T) {
	m := NewManager()
	const account = "https://github.com/owner/seq.git"
	for _, v := range []string{"v1", "v2", "v3"} {
		if err := m.Set(account, v); err != nil {
			t.Fatalf("Set %s: %v", v, err)
		}
	}
	got, err := m.Get(account)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "v3" {
		t.Errorf("Get after 3 overwrites = %q, want v3", got)
	}
}

func TestDelete_EmptyAccountReturnsNil(t *testing.T) {
	// Empty account short-circuits to nil before touching keyring.
	m := NewManager()
	if err := m.Delete(""); err != nil {
		t.Errorf("Delete empty account err = %v, want nil", err)
	}
}

func TestDelete_WhitespaceAccountReturnsNil(t *testing.T) {
	m := NewManager()
	if err := m.Delete("   "); err != nil {
		t.Errorf("Delete whitespace account err = %v, want nil", err)
	}
}

func TestDelete_TwiceIsIdempotent(t *testing.T) {
	m := NewManager()
	const account = "https://github.com/owner/twice.git"
	if err := m.Set(account, "x"); err != nil {
		t.Fatal(err)
	}
	if err := m.Delete(account); err != nil {
		t.Fatalf("first Delete: %v", err)
	}
	if err := m.Delete(account); err != nil {
		t.Errorf("second Delete err = %v, want nil (idempotent)", err)
	}
	if m.Has(account) {
		t.Error("Has true after double Delete")
	}
}

func TestHas_EmptyAccountFalse(t *testing.T) {
	m := NewManager()
	if m.Has("") {
		t.Error("Has(\"\") = true, want false")
	}
}

func TestHas_WhitespaceAccountFalse(t *testing.T) {
	m := NewManager()
	if m.Has("  \t ") {
		t.Error("Has whitespace = true, want false")
	}
}

func TestGet_AfterDeleteReturnsErrNotFound(t *testing.T) {
	m := NewManager()
	const account = "https://github.com/owner/gone.git"
	if err := m.Set(account, "tok"); err != nil {
		t.Fatal(err)
	}
	if err := m.Delete(account); err != nil {
		t.Fatal(err)
	}
	_, err := m.Get(account)
	if err != keyring.ErrNotFound {
		t.Errorf("Get after Delete err = %v, want keyring.ErrNotFound", err)
	}
}

func TestConcurrentWrites_RaceSafe(t *testing.T) {
	// Concurrent Set on distinct accounts must serialize keyring access so the
	// shared backend map is never written by two goroutines at once. Under -race
	// this fails without the package-level lock guarding the keyring calls.
	m := NewManager()
	const workers = 32
	accounts := make([]string, workers)
	secrets := make([]string, workers)
	for i := 0; i < workers; i++ {
		accounts[i] = "https://github.com/owner/conc-" + string(rune('a'+i%26)) + strings.Repeat("z", i)
		secrets[i] = "secret-" + strings.Repeat("s", i+1)
	}

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := m.Set(accounts[i], secrets[i]); err != nil {
				t.Errorf("worker %d Set: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	// Every concurrent write landed and round-trips to its own value.
	for i := 0; i < workers; i++ {
		got, err := m.Get(accounts[i])
		if err != nil {
			t.Errorf("worker %d Get: %v", i, err)
			continue
		}
		if got != secrets[i] {
			t.Errorf("worker %d secret length = %d, want %d", i, len(got), len(secrets[i]))
		}
	}
}
