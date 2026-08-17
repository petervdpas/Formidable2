package vault

import (
	"errors"
	"path/filepath"
	"testing"
)

func newTestService(t *testing.T) (*Service, string) {
	t.Helper()
	root := t.TempDir()
	s := NewService(root, nil, nil)
	t.Cleanup(s.LockVault)
	return s, root
}

func TestService_DefaultsToAppRootVault(t *testing.T) {
	root := t.TempDir()
	s := NewService(root, nil, nil)
	if want := filepath.Join(root, "vault"); s.Dir() != want {
		t.Fatalf("Dir = %q, want %q", s.Dir(), want)
	}
	if DirName != "vault" {
		t.Fatalf("DirName = %q", DirName)
	}
}

func TestService_StatusBeforeAnyVaultExists(t *testing.T) {
	s, _ := newTestService(t)
	st := s.VaultStatus()
	if st.Exists || st.Unlocked || st.Secrets != 0 {
		t.Fatalf("status = %+v, want an empty slate", st)
	}
	if st.Path == "" {
		t.Error("status must always carry the path, so the panel can show it")
	}
}

func TestService_InitializeThenStatus(t *testing.T) {
	s, _ := newTestService(t)
	if err := s.InitializeVault("master pw"); err != nil {
		t.Fatal(err)
	}
	st := s.VaultStatus()
	if !st.Exists || !st.Unlocked {
		t.Fatalf("status = %+v, want an existing unlocked vault", st)
	}
	if err := s.InitializeVault("another"); !errors.Is(err, ErrVaultExists) {
		t.Fatalf("err = %v, want ErrVaultExists", err)
	}
}

func TestService_SecretsRoundTripAndCount(t *testing.T) {
	s, _ := newTestService(t)
	if err := s.InitializeVault("master pw"); err != nil {
		t.Fatal(err)
	}

	if err := s.SetSecret("api-client-northwind", "bearer"); err != nil {
		t.Fatal(err)
	}
	if !s.HasSecret("api-client-northwind") {
		t.Fatal("HasSecret is false after SetSecret")
	}
	if got := s.VaultStatus().Secrets; got != 1 {
		t.Fatalf("count = %d, want 1", got)
	}

	entries, err := s.ListSecrets()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "api-client-northwind" {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].UpdatedUTC.IsZero() {
		t.Error("entries should carry a write time")
	}

	if err := s.DeleteSecret("api-client-northwind"); err != nil {
		t.Fatal(err)
	}
	if s.HasSecret("api-client-northwind") {
		t.Fatal("still present after DeleteSecret")
	}
}

func TestService_LockThenUnlock(t *testing.T) {
	s, _ := newTestService(t)
	if err := s.InitializeVault("master pw"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSecret("k", "v"); err != nil {
		t.Fatal(err)
	}

	s.LockVault()
	if s.VaultStatus().Unlocked {
		t.Fatal("still unlocked")
	}
	if err := s.SetSecret("k", "v2"); !errors.Is(err, ErrLocked) {
		t.Fatalf("err = %v, want ErrLocked", err)
	}

	if err := s.UnlockVault("wrong"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("err = %v, want ErrInvalidPassword", err)
	}
	if err := s.UnlockVault("master pw"); err != nil {
		t.Fatal(err)
	}
	if !s.VaultStatus().Unlocked {
		t.Fatal("unlock did not take")
	}
}

func TestService_UnlockWithNoVault(t *testing.T) {
	s, _ := newTestService(t)
	if err := s.UnlockVault("pw"); !errors.Is(err, ErrVaultNotFound) {
		t.Fatalf("err = %v, want ErrVaultNotFound", err)
	}
}

func TestService_OperationsBeforeAVaultExists(t *testing.T) {
	s, _ := newTestService(t)
	if err := s.SetSecret("k", "v"); !errors.Is(err, ErrVaultNotFound) {
		t.Errorf("SetSecret = %v, want ErrVaultNotFound", err)
	}
	if _, err := s.ListSecrets(); !errors.Is(err, ErrVaultNotFound) {
		t.Errorf("ListSecrets = %v, want ErrVaultNotFound", err)
	}
	if s.HasSecret("k") {
		t.Error("HasSecret is true with no vault")
	}
	s.LockVault() // must not panic
}

// TestService_ResolverSurvivesALaterInitialize is the case that made the
// resolver lazy. The composition root builds one at startup, long before the
// user has created a vault; binding the vault eagerly would pin nil forever.
func TestService_ResolverSurvivesALaterInitialize(t *testing.T) {
	s, _ := newTestService(t)
	r := ResolverFor(s, "api-client-")

	if r.Has("northwind") {
		t.Fatal("nothing is stored yet")
	}
	if _, err := r.Secret("northwind"); !errors.Is(err, ErrVaultNotFound) {
		t.Fatalf("err = %v, want ErrVaultNotFound before the vault exists", err)
	}

	if err := s.InitializeVault("master pw"); err != nil {
		t.Fatal(err)
	}
	if err := r.Store("northwind", "bearer"); err != nil {
		t.Fatalf("the resolver did not pick up the new vault: %v", err)
	}
	got, err := r.Secret("northwind")
	if err != nil || got != "bearer" {
		t.Fatalf("Secret = %q, %v", got, err)
	}
}

func TestService_ResolverTracksLockState(t *testing.T) {
	s, _ := newTestService(t)
	r := ResolverFor(s, "api-client-")
	if err := s.InitializeVault("master pw"); err != nil {
		t.Fatal(err)
	}
	if err := r.Store("northwind", "bearer"); err != nil {
		t.Fatal(err)
	}

	s.LockVault()
	if _, err := r.Secret("northwind"); !errors.Is(err, ErrLocked) {
		t.Fatalf("err = %v, want ErrLocked", err)
	}
	if !r.Has("northwind") {
		t.Error("Has must keep working while locked")
	}

	if err := s.UnlockVault("master pw"); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.Secret("northwind"); got != "bearer" {
		t.Fatalf("secret = %q after re-unlock", got)
	}
}

// TestService_PicksUpAVaultCreatedOutsideThisProcess covers the shared-vault
// case: another tool writes the directory while Formidable is running.
func TestService_PicksUpAVaultCreatedOutsideThisProcess(t *testing.T) {
	s, root := newTestService(t)
	if s.VaultStatus().Exists {
		t.Fatal("no vault yet")
	}

	external, err := Create(filepath.Join(root, DirName), "external pw", WithParams(fastParams))
	if err != nil {
		t.Fatal(err)
	}
	if err := external.Set("planted", "value"); err != nil {
		t.Fatal(err)
	}
	external.Lock()

	st := s.VaultStatus()
	if !st.Exists || st.Secrets != 1 {
		t.Fatalf("status = %+v, want the externally created vault", st)
	}
	if err := s.UnlockVault("external pw"); err != nil {
		t.Fatalf("could not open a vault created elsewhere: %v", err)
	}
}
