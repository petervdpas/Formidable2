package credential

import (
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
