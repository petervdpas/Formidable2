package credential

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/zalando/go-keyring"
)

// MockInit swaps the platform keychain backend for an in-memory map
// for the duration of the test process. Call once at the top of any
// test that touches Manager - go-keyring resets cleanly between
// tests because each Set call overwrites whatever was there.
func init() {
	keyring.MockInit()
}

func TestSetAndGet_RoundTrip(t *testing.T) {
	m := NewManager()
	if err := m.Set("https://github.com/owner/repo.git", "ghp_abc123"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := m.Get("https://github.com/owner/repo.git")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "ghp_abc123" {
		t.Errorf("Get = %q, want ghp_abc123", got)
	}
}

func TestHas_ReportsTrueWhenSet(t *testing.T) {
	m := NewManager()
	const account = "https://github.com/owner/has.git"
	if m.Has(account) {
		t.Fatal("Has returned true before Set")
	}
	if err := m.Set(account, "x"); err != nil {
		t.Fatal(err)
	}
	if !m.Has(account) {
		t.Error("Has returned false after Set")
	}
}

func TestHas_FalseForMissingAccount(t *testing.T) {
	m := NewManager()
	if m.Has("https://example.com/missing.git") {
		t.Error("expected false for unknown account")
	}
}

func TestDelete_RemovesEntry(t *testing.T) {
	m := NewManager()
	const account = "https://github.com/owner/delete.git"
	if err := m.Set(account, "secret"); err != nil {
		t.Fatal(err)
	}
	if err := m.Delete(account); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if m.Has(account) {
		t.Error("Has true after Delete")
	}
}

func TestDelete_MissingIsNotError(t *testing.T) {
	// Idempotent delete - useful so the UI can call Forget without
	// caring whether an entry actually existed.
	m := NewManager()
	if err := m.Delete("https://example.com/never-existed.git"); err != nil {
		t.Errorf("Delete on missing account should be no-op, got %v", err)
	}
}

func TestSet_EmptyAccountRejected(t *testing.T) {
	m := NewManager()
	if err := m.Set("", "secret"); err == nil {
		t.Error("expected error for empty account")
	}
}

func TestSet_EmptySecretRejected(t *testing.T) {
	// Refuse to write an empty secret - that's almost always a bug
	// (the form was submitted before the user pasted the PAT).
	m := NewManager()
	if err := m.Set("https://github.com/owner/repo.git", ""); err == nil {
		t.Error("expected error for empty secret")
	}
}

func TestGet_MissingReturnsError(t *testing.T) {
	m := NewManager()
	if _, err := m.Get("https://example.com/none.git"); err == nil {
		t.Error("expected error for missing account")
	}
}

func TestSet_OverwritesExisting(t *testing.T) {
	m := NewManager()
	const account = "https://github.com/owner/overwrite.git"
	if err := m.Set(account, "old"); err != nil {
		t.Fatal(err)
	}
	if err := m.Set(account, "new"); err != nil {
		t.Fatal(err)
	}
	got, _ := m.Get(account)
	if got != "new" {
		t.Errorf("Get = %q, want new", got)
	}
}

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

// NOTE: headless. keyring.MockInit() runs in domain_test.go init(), so these
// exercise only the in-process Manager/Service logic over the in-memory mock.

func TestService_HasReflectsManagerState(t *testing.T) {
	m := NewManager()
	s := NewService(m)
	const account = "https://github.com/owner/svc-has.git"
	if got := s.Has(account); got.Found {
		t.Errorf("Service.Has before Set = %+v, want Found:false", got)
	}
	if err := m.Set(account, "tok"); err != nil {
		t.Fatal(err)
	}
	if got := s.Has(account); !got.Found {
		t.Errorf("Service.Has after Set = %+v, want Found:true", got)
	}
}

func TestService_SetEmptySecretRejected(t *testing.T) {
	s := NewService(NewManager())
	err := s.Set("https://github.com/owner/svc-empty.git", "")
	if err == nil || err.Error() != "credential: empty secret" {
		t.Errorf("Service.Set empty secret err = %v, want \"credential: empty secret\"", err)
	}
}

func TestService_DeleteThenHasFound(t *testing.T) {
	m := NewManager()
	s := NewService(m)
	const account = "https://github.com/owner/svc-del.git"
	if err := s.Set(account, "tok"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(account); err != nil {
		t.Fatalf("Service.Delete: %v", err)
	}
	if got := s.Has(account); got.Found {
		t.Errorf("Service.Has after Delete = %+v, want Found:false", got)
	}
}

func TestService_DeleteMissingReturnsNil(t *testing.T) {
	s := NewService(NewManager())
	if err := s.Delete("https://github.com/owner/svc-never.git"); err != nil {
		t.Errorf("Service.Delete missing err = %v, want nil", err)
	}
}

// Two Managers share the const service namespace; isolation is per-account,
// not per-Manager. A second Manager must read the first Manager's entry.
func TestManager_SharedNamespaceAcrossInstances(t *testing.T) {
	const account = "https://github.com/owner/shared-ns.git"
	if err := NewManager().Set(account, "ns-secret"); err != nil {
		t.Fatal(err)
	}
	got, err := NewManager().Get(account)
	if err != nil {
		t.Fatalf("second Manager Get: %v", err)
	}
	if got != "ns-secret" {
		t.Errorf("cross-Manager Get = %q, want ns-secret", got)
	}
}

// Distinct accounts under the same namespace stay isolated; the entry written
// at acctA must not leak into acctB.
func TestManager_DistinctAccountsIsolated(t *testing.T) {
	m := NewManager()
	const acctA = "personal.json:git:https://github.com/owner/repo.git"
	const acctB = "work.json:git:https://github.com/owner/repo.git"
	if err := m.Set(acctA, "tokA"); err != nil {
		t.Fatal(err)
	}
	if err := m.Set(acctB, "tokB"); err != nil {
		t.Fatal(err)
	}
	gotA, err := m.Get(acctA)
	if err != nil {
		t.Fatalf("Get acctA: %v", err)
	}
	gotB, err := m.Get(acctB)
	if err != nil {
		t.Fatalf("Get acctB: %v", err)
	}
	if gotA != "tokA" {
		t.Errorf("acctA = %q, want tokA", gotA)
	}
	if gotB != "tokB" {
		t.Errorf("acctB = %q, want tokB", gotB)
	}
}

// Get must surface the same ErrNotFound sentinel that callers branch on to
// prompt for a fresh PAT, not a wrapped or message-only error.
func TestManager_GetMissingIsSentinelNotFound(t *testing.T) {
	m := NewManager()
	_, err := m.Get("https://github.com/owner/sentinel-absent.git")
	if !errors.Is(err, keyring.ErrNotFound) {
		t.Errorf("Get missing err = %v, want keyring.ErrNotFound (errors.Is)", err)
	}
}

// Concurrent Has over an already-seeded set: the mock map is read-only here,
// so reads are race-safe. Each lookup must report Found:true for its own seed.
// Writes are serialized first because Manager holds no mutex and the mock map
// is unsynchronized (see suspectedBugs domain.go).
func TestService_ConcurrentHasRaceSafe(t *testing.T) {
	m := NewManager()
	s := NewService(m)
	const workers = 32
	accounts := make([]string, workers)
	for i := 0; i < workers; i++ {
		accounts[i] = "https://github.com/owner/svc-conc-" + string(rune('a'+i%26))
		if err := m.Set(accounts[i], "tok"); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if got := s.Has(accounts[i]); !got.Found {
				t.Errorf("worker %d Has = %+v, want Found:true", i, got)
			}
		}(i)
	}
	wg.Wait()
}
