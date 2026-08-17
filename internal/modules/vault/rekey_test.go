package vault

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedForRekey fills a vault with a few secrets and returns the expected
// contents, so every re-key test can assert nothing was lost.
func seedForRekey(t *testing.T, v *Vault) map[string]string {
	t.Helper()
	want := map[string]string{
		"api-client-northwind": "odata-bearer-token",
		"git-remote-origin":    "ghp_something",
		"multi.line":           "-----BEGIN KEY-----\nline two\nline three\n-----END KEY-----",
		"empty":                "",
	}
	for name, value := range want {
		if err := v.Set(name, value); err != nil {
			t.Fatalf("Set %s: %v", name, err)
		}
	}
	return want
}

// assertReadable checks every wanted secret still decrypts, without insisting
// on the exact entry count.
func assertReadable(t *testing.T, v *Vault, want map[string]string) {
	t.Helper()
	for name, expect := range want {
		got, err := v.Get(name)
		if err != nil {
			t.Errorf("Get(%q): %v", name, err)
			continue
		}
		if got != expect {
			t.Errorf("Get(%q) = %q, want %q", name, got, expect)
		}
	}
}

func assertContents(t *testing.T, v *Vault, want map[string]string) {
	t.Helper()
	names, err := v.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != len(want) {
		t.Fatalf("names = %v, want %d entries", names, len(want))
	}
	assertReadable(t, v, want)
}

func TestChangePassword_RoundTrip(t *testing.T) {
	v, dir := newTestVault(t)
	want := seedForRekey(t, v)
	oldID := v.VaultID()

	if err := v.ChangePassword("correct horse battery staple", "a brand new master"); err != nil {
		t.Fatal(err)
	}

	// Still usable in place, without a re-unlock.
	assertContents(t, v, want)
	if v.IsLocked() {
		t.Error("re-key should leave the vault unlocked")
	}
	if v.VaultID() != oldID {
		t.Error("the vault id must survive a re-key; it is baked into every record's AAD")
	}

	// And from a fresh open, under the new password only.
	reopened, err := Open(dir, WithAutoLock(0))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Lock)
	if err := reopened.Unlock("correct horse battery staple"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("old password still works: %v", err)
	}
	if err := reopened.Unlock("a brand new master"); err != nil {
		t.Fatalf("new password rejected: %v", err)
	}
	assertContents(t, reopened, want)
}

func TestChangePassword_UsesAFreshSalt(t *testing.T) {
	v, dir := newTestVault(t)
	before, err := readHeader(filepath.Join(dir, headerFileName))
	if err != nil {
		t.Fatal(err)
	}
	if err := v.ChangePassword("correct horse battery staple", "a brand new master"); err != nil {
		t.Fatal(err)
	}
	after, err := readHeader(filepath.Join(dir, headerFileName))
	if err != nil {
		t.Fatal(err)
	}
	if after.KDF.Salt == before.KDF.Salt {
		t.Fatal("the salt was reused; a re-key must derive from fresh material")
	}
	if after.Canary.Ciphertext == before.Canary.Ciphertext {
		t.Fatal("the canary was not re-sealed")
	}
}

func TestChangePassword_WrongOldPasswordChangesNothing(t *testing.T) {
	v, _ := newTestVault(t)
	want := seedForRekey(t, v)

	err := v.ChangePassword("not the old one", "a brand new master")
	if !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("err = %v, want ErrInvalidPassword", err)
	}
	assertContents(t, v, want)

	// The original password must still open it.
	if err := v.Unlock("correct horse battery staple"); err != nil {
		t.Fatalf("original password broken by a failed re-key: %v", err)
	}
}

func TestChangePassword_RequiresAnUnlockedVault(t *testing.T) {
	v, _ := newTestVault(t)
	v.Lock()
	if err := v.ChangePassword("correct horse battery staple", "new one"); !errors.Is(err, ErrLocked) {
		t.Fatalf("err = %v, want ErrLocked", err)
	}
}

func TestChangePassword_RefusesAnEmptyNewPassword(t *testing.T) {
	v, _ := newTestVault(t)
	if err := v.ChangePassword("correct horse battery staple", ""); err == nil {
		t.Fatal("an empty new password must be refused")
	}
}

// TestChangePassword_KeepsRecordsItCannotRead is the bug TaskBlaster has: its
// re-key skips entries it cannot decode and then deletes the backup, so a
// foreign record is lost for good. Aborting is the safe answer.
func TestChangePassword_AbortsOnAnUnreadableRecord(t *testing.T) {
	v, dir := newTestVault(t)
	want := seedForRekey(t, v)

	bad := filepath.Join(dir, secretsDirName, "corrupted"+secretExt)
	if err := os.WriteFile(bad, []byte(`{"version":1,"nonce":"AAAA","ciphertext":"AA","tag":"AA"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	err := v.ChangePassword("correct horse battery staple", "a brand new master")
	if err == nil {
		t.Fatal("a record that cannot be read must stop the re-key, not be dropped")
	}
	if !strings.Contains(err.Error(), "corrupted") {
		t.Errorf("the error should name the offending record, got: %v", err)
	}

	// Everything readable is untouched and still under the old password. The
	// count is not checked here: the planted corrupt file is a real .secret
	// entry and legitimately shows up in List.
	assertReadable(t, v, want)
	reopened, err2 := Open(dir, WithAutoLock(0))
	if err2 != nil {
		t.Fatal(err2)
	}
	t.Cleanup(reopened.Lock)
	if err := reopened.Unlock("correct horse battery staple"); err != nil {
		t.Fatalf("the old password stopped working after an aborted re-key: %v", err)
	}
}

// TestChangePassword_PreservesForeignFiles covers the shared-vault case: files
// this code did not write must survive, or another tool's data disappears.
func TestChangePassword_PreservesForeignFiles(t *testing.T) {
	v, dir := newTestVault(t)
	want := seedForRekey(t, v)

	stray := filepath.Join(dir, secretsDirName, "notes.txt")
	if err := os.WriteFile(stray, []byte("someone else put this here"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := v.ChangePassword("correct horse battery staple", "a brand new master"); err != nil {
		t.Fatal(err)
	}

	assertContents(t, v, want)
	got, err := os.ReadFile(stray)
	if err != nil {
		t.Fatalf("a file this package did not write was destroyed: %v", err)
	}
	if string(got) != "someone else put this here" {
		t.Errorf("stray file content changed: %q", got)
	}
}

func TestChangePassword_LeavesNoStagingDirectories(t *testing.T) {
	v, dir := newTestVault(t)
	seedForRekey(t, v)
	if err := v.ChangePassword("correct horse battery staple", "a brand new master"); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".rekey") {
			t.Fatalf("left staging behind: %s", e.Name())
		}
	}
}

func TestChangePassword_EmptyVault(t *testing.T) {
	v, _ := newTestVault(t)
	if err := v.ChangePassword("correct horse battery staple", "a brand new master"); err != nil {
		t.Fatalf("re-keying an empty vault must work: %v", err)
	}
	if err := v.Unlock("a brand new master"); err != nil {
		t.Fatal(err)
	}
}

func TestChangePassword_SecretsStayReadableAfterRepeatedRekeys(t *testing.T) {
	v, _ := newTestVault(t)
	want := seedForRekey(t, v)

	passwords := []string{"correct horse battery staple", "second one", "third one", "fourth one"}
	for i := 1; i < len(passwords); i++ {
		if err := v.ChangePassword(passwords[i-1], passwords[i]); err != nil {
			t.Fatalf("re-key %d: %v", i, err)
		}
		assertContents(t, v, want)
	}
}

// Service level --------------------------------------------------------

func TestService_ChangeMasterPassword(t *testing.T) {
	s, _ := newTestService(t)
	if err := s.InitializeVault("master pw"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSecret("api-client", "northwind", "bearer", ""); err != nil {
		t.Fatal(err)
	}

	if err := s.ChangeMasterPassword("master pw", "a longer new one"); err != nil {
		t.Fatal(err)
	}
	if !s.HasSecret("api-client", "northwind") {
		t.Fatal("secret lost in the re-key")
	}

	s.LockVault()
	if err := s.UnlockVault("master pw"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("old password still opens the vault: %v", err)
	}
	if err := s.UnlockVault("a longer new one"); err != nil {
		t.Fatal(err)
	}
}

func TestService_ChangeMasterPasswordEnforcesPolicy(t *testing.T) {
	s, _ := newTestService(t)
	if err := s.InitializeVault("master pw"); err != nil {
		t.Fatal(err)
	}
	if err := s.ChangeMasterPassword("master pw", "short"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("err = %v, want ErrWeakPassword", err)
	}
	// The old password must still work after a rejected change.
	s.LockVault()
	if err := s.UnlockVault("master pw"); err != nil {
		t.Fatal(err)
	}
}

func TestService_InitializeEnforcesPolicy(t *testing.T) {
	s, _ := newTestService(t)
	if err := s.InitializeVault("short"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("err = %v, want ErrWeakPassword", err)
	}
	if s.VaultStatus().Exists {
		t.Fatal("a rejected password must not leave a vault behind")
	}
}

func TestService_ChangeMasterPasswordNeedsAVault(t *testing.T) {
	s, _ := newTestService(t)
	if err := s.ChangeMasterPassword("a", "a longer new one"); !errors.Is(err, ErrVaultNotFound) {
		t.Fatalf("err = %v, want ErrVaultNotFound", err)
	}
}

// TestVaultPolicy_IsTheSingleSourceOfTruth pins the rule the frontend must
// read rather than restate. A hardcoded minimum in Vue drifts the first time
// this changes.
func TestVaultPolicy_IsTheSingleSourceOfTruth(t *testing.T) {
	s, _ := newTestService(t)
	p := s.VaultPolicy()
	if p.MinPasswordLength != MinPasswordLength {
		t.Errorf("policy min = %d, want %d", p.MinPasswordLength, MinPasswordLength)
	}
	if p.MinPasswordLength <= 0 {
		t.Error("a zero minimum would let the frontend accept anything")
	}
	if p.AutoLockMinutes != int(DefaultAutoLock.Minutes()) {
		t.Errorf("policy auto-lock = %d, want %d", p.AutoLockMinutes, int(DefaultAutoLock.Minutes()))
	}
}
