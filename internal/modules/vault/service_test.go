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

	if err := s.SetSecret("api-client", "northwind", "bearer", ""); err != nil {
		t.Fatal(err)
	}
	if !s.HasSecret("api-client", "northwind") {
		t.Fatal("HasSecret is false after SetSecret")
	}
	if got := s.VaultStatus().Secrets; got != 1 {
		t.Fatalf("count = %d, want 1", got)
	}

	entries, err := s.ListSecrets()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Category != "api-client" || entries[0].Key != "northwind" {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].UpdatedUTC.IsZero() {
		t.Error("entries should carry a write time")
	}

	if err := s.DeleteSecret("api-client", "northwind"); err != nil {
		t.Fatal(err)
	}
	if s.HasSecret("api-client", "northwind") {
		t.Fatal("still present after DeleteSecret")
	}
}

func TestService_RevealSecret(t *testing.T) {
	s, _ := newTestService(t)
	if err := s.InitializeVault("master pw"); err != nil {
		t.Fatal(err)
	}
	const big = "-----BEGIN KEY-----\nline two\n-----END KEY-----"
	if err := s.SetSecret("c", "test", big, ""); err != nil {
		t.Fatal(err)
	}

	got, err := s.RevealSecret("c", "test")
	if err != nil {
		t.Fatal(err)
	}
	if got != big {
		t.Fatalf("revealed %q, want the stored value whole including newlines", got)
	}

	// A locked vault must refuse rather than return an empty string, which the
	// panel could not tell apart from a secret that really is empty.
	s.LockVault()
	if _, err := s.RevealSecret("c", "test"); !errors.Is(err, ErrLocked) {
		t.Fatalf("err = %v, want ErrLocked", err)
	}
}

func TestService_RevealMissingSecret(t *testing.T) {
	s, _ := newTestService(t)
	if err := s.InitializeVault("master pw"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RevealSecret("c", "never-stored"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestService_LockThenUnlock(t *testing.T) {
	s, _ := newTestService(t)
	if err := s.InitializeVault("master pw"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSecret("c", "k", "v", ""); err != nil {
		t.Fatal(err)
	}

	s.LockVault()
	if s.VaultStatus().Unlocked {
		t.Fatal("still unlocked")
	}
	if err := s.SetSecret("c", "k", "v2", ""); !errors.Is(err, ErrLocked) {
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
	if err := s.SetSecret("c", "k", "v", ""); !errors.Is(err, ErrVaultNotFound) {
		t.Errorf("SetSecret = %v, want ErrVaultNotFound", err)
	}
	if _, err := s.ListSecrets(); !errors.Is(err, ErrVaultNotFound) {
		t.Errorf("ListSecrets = %v, want ErrVaultNotFound", err)
	}
	if s.HasSecret("c", "k") {
		t.Error("HasSecret is true with no vault")
	}
	s.LockVault() // must not panic
}

// TestService_ResolverSurvivesALaterInitialize is the case that made the
// resolver lazy. The composition root builds one at startup, long before the
// user has created a vault; binding the vault eagerly would pin nil forever.
func TestService_ResolverSurvivesALaterInitialize(t *testing.T) {
	s, _ := newTestService(t)
	r := ResolverFor(s, "api-client")

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
	r := ResolverFor(s, "api-client")
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
	// Opaque slots keep identity inside the ciphertext, so a locked vault
	// cannot answer this and must not pretend otherwise.
	if r.Has("northwind") {
		t.Error("Has must not claim knowledge while locked")
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
	if _, err := NewCatalog(external).Put("c", "planted", "value", ""); err != nil {
		t.Fatal(err)
	}
	external.Lock()

	// The vault is found, but its contents cannot be counted until it is
	// open: identity lives inside the ciphertext.
	st := s.VaultStatus()
	if !st.Exists {
		t.Fatalf("status = %+v, want the externally created vault to be found", st)
	}
	if st.Secrets != 0 {
		t.Errorf("a locked vault must not report a count, got %d", st.Secrets)
	}

	if err := s.UnlockVault("external pw"); err != nil {
		t.Fatalf("could not open a vault created elsewhere: %v", err)
	}
	if got := s.VaultStatus().Secrets; got != 1 {
		t.Fatalf("count after unlock = %d, want 1", got)
	}
}
