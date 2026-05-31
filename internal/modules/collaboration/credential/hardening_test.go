package credential

import (
	"errors"
	"sync"
	"testing"

	"github.com/zalando/go-keyring"
)

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
