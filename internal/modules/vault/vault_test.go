package vault

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fastParams keeps the tests quick. Real vaults use DefaultParams; the crypto
// path is identical, only the work factor differs.
var fastParams = Params{MemoryKiB: 1024, Iterations: 1, Parallelism: 1}

func newTestVault(t *testing.T) (*Vault, string) {
	t.Helper()
	dir := t.TempDir()
	v, err := Create(dir, "correct horse battery staple", WithParams(fastParams))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(v.Lock)
	return v, dir
}

// Lifecycle -------------------------------------------------------------

func TestCreate_WritesHeaderAndOpensUnlocked(t *testing.T) {
	v, dir := newTestVault(t)
	if v.IsLocked() {
		t.Fatal("a freshly created vault must be unlocked")
	}
	if !Exists(dir) {
		t.Fatal("Exists is false right after Create")
	}
	if v.VaultID() == "" {
		t.Fatal("no vault id")
	}
}

func TestCreate_RefusesToOverwriteAnExistingVault(t *testing.T) {
	_, dir := newTestVault(t)
	if _, err := Create(dir, "another", WithParams(fastParams)); !errors.Is(err, ErrVaultExists) {
		t.Fatalf("err = %v, want ErrVaultExists; overwriting would discard every secret", err)
	}
}

func TestCreate_ToleratesAnExistingNonVaultDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(dir, "pw", WithParams(fastParams)); err != nil {
		t.Fatalf("unrelated files must not block creation: %v", err)
	}
}

func TestOpen_MissingVault(t *testing.T) {
	if _, err := Open(t.TempDir()); !errors.Is(err, ErrVaultNotFound) {
		t.Fatalf("err = %v, want ErrVaultNotFound", err)
	}
}

func TestOpen_ComesBackLocked(t *testing.T) {
	_, dir := newTestVault(t)
	v, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !v.IsLocked() {
		t.Fatal("Open must not unlock; the password has not been supplied yet")
	}
	if _, err := v.Get("anything"); !errors.Is(err, ErrLocked) {
		t.Fatalf("err = %v, want ErrLocked", err)
	}
}

// Unlocking -------------------------------------------------------------

func TestUnlock_RoundTrip(t *testing.T) {
	v, dir := newTestVault(t)
	if err := v.Set("github-token", "ghp_secret"); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.Unlock("correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Get("github-token")
	if err != nil {
		t.Fatal(err)
	}
	if got != "ghp_secret" {
		t.Fatalf("value = %q", got)
	}
}

func TestUnlock_WrongPassword(t *testing.T) {
	_, dir := newTestVault(t)
	v, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Unlock("wrong"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("err = %v, want ErrInvalidPassword", err)
	}
	if !v.IsLocked() {
		t.Fatal("a failed unlock must leave the vault locked")
	}
}

func TestUnlock_IsIdempotent(t *testing.T) {
	v, _ := newTestVault(t)
	if err := v.Unlock("does not matter"); err != nil {
		t.Fatalf("unlocking an unlocked vault must be a no-op: %v", err)
	}
	if v.IsLocked() {
		t.Fatal("vault locked itself")
	}
}

func TestUnlock_NormalisesThePassword(t *testing.T) {
	dir := t.TempDir()
	// The same text composed (U+00E9) and decomposed (e + U+0301).
	composed := "café"
	decomposed := "café"
	if composed == decomposed {
		t.Fatal("test inputs are not actually different byte sequences")
	}

	if _, err := Create(dir, composed, WithParams(fastParams)); err != nil {
		t.Fatal(err)
	}
	v, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Unlock(decomposed); err != nil {
		t.Fatalf("NFC normalisation should make these the same password: %v", err)
	}
}

func TestLock_ZeroesAndBlocks(t *testing.T) {
	v, _ := newTestVault(t)
	if err := v.Set("a", "b"); err != nil {
		t.Fatal(err)
	}
	v.Lock()
	if !v.IsLocked() {
		t.Fatal("still unlocked")
	}
	if _, err := v.Get("a"); !errors.Is(err, ErrLocked) {
		t.Fatalf("err = %v, want ErrLocked", err)
	}
	v.Lock() // idempotent
}

func TestLock_FiresTheCallbackOnce(t *testing.T) {
	dir := t.TempDir()
	var mu sync.Mutex
	calls := 0
	v, err := Create(dir, "pw", WithParams(fastParams), OnLock(func() {
		mu.Lock()
		calls++
		mu.Unlock()
	}))
	if err != nil {
		t.Fatal(err)
	}
	v.Lock()
	v.Lock()

	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("callback fired %d times, want 1", calls)
	}
}

func TestAutoLock_LocksWhenIdle(t *testing.T) {
	dir := t.TempDir()
	v, err := Create(dir, "pw", WithParams(fastParams), WithAutoLock(60*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if v.IsLocked() {
		t.Fatal("locked too early")
	}
	time.Sleep(200 * time.Millisecond)
	if !v.IsLocked() {
		t.Fatal("the idle timeout did not lock the vault")
	}
}

func TestAutoLock_UseKeepsItOpen(t *testing.T) {
	dir := t.TempDir()
	v, err := Create(dir, "pw", WithParams(fastParams), WithAutoLock(150*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	for range 4 {
		time.Sleep(50 * time.Millisecond)
		if err := v.Set("busy", "value"); err != nil {
			t.Fatalf("vault locked under active use: %v", err)
		}
	}
	if v.IsLocked() {
		t.Fatal("an actively used vault must not lock")
	}
}

func TestAutoLock_ZeroDisablesIt(t *testing.T) {
	dir := t.TempDir()
	v, err := Create(dir, "pw", WithParams(fastParams), WithAutoLock(0))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(120 * time.Millisecond)
	if v.IsLocked() {
		t.Fatal("auto-lock should be disabled")
	}
}

// Data operations -------------------------------------------------------

func TestSetGetDelete(t *testing.T) {
	v, _ := newTestVault(t)

	if err := v.Set("api-key", "s3cret"); err != nil {
		t.Fatal(err)
	}
	if !v.Has("api-key") {
		t.Fatal("Has is false after Set")
	}
	got, err := v.Get("api-key")
	if err != nil || got != "s3cret" {
		t.Fatalf("Get = %q, %v", got, err)
	}

	if err := v.Set("api-key", "rotated"); err != nil {
		t.Fatal(err)
	}
	got, _ = v.Get("api-key")
	if got != "rotated" {
		t.Fatalf("overwrite failed: %q", got)
	}

	if err := v.Delete("api-key"); err != nil {
		t.Fatal(err)
	}
	if v.Has("api-key") {
		t.Fatal("Has is true after Delete")
	}
	if _, err := v.Get("api-key"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if err := v.Delete("api-key"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleting twice = %v, want ErrNotFound", err)
	}
}

func TestSet_EmptyAndLargeValues(t *testing.T) {
	v, _ := newTestVault(t)
	big := strings.Repeat("x", 512*1024)

	for name, want := range map[string]string{"empty": "", "big": big, "unicode": "wachtwoord-één-\U0001F510"} {
		if err := v.Set(name, want); err != nil {
			t.Fatalf("Set %s: %v", name, err)
		}
		got, err := v.Get(name)
		if err != nil {
			t.Fatalf("Get %s: %v", name, err)
		}
		if got != want {
			t.Errorf("%s round-trip lost data (%d vs %d bytes)", name, len(got), len(want))
		}
	}
}

func TestList_SortedAndSkipsStrays(t *testing.T) {
	v, dir := newTestVault(t)
	for _, n := range []string{"zulu", "alpha", "mike"} {
		if err := v.Set(n, "v"); err != nil {
			t.Fatal(err)
		}
	}
	stray := filepath.Join(dir, secretsDirName, "readme.txt")
	if err := os.WriteFile(stray, []byte("ignore"), 0o600); err != nil {
		t.Fatal(err)
	}

	names, err := v.List()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(names, ",") != "alpha,mike,zulu" {
		t.Fatalf("names = %v", names)
	}
}

func TestList_WorksWhileLocked(t *testing.T) {
	v, _ := newTestVault(t)
	if err := v.Set("visible", "v"); err != nil {
		t.Fatal(err)
	}
	v.Lock()

	names, err := v.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "visible" {
		t.Fatalf("names = %v; names are filenames, so listing needs no key", names)
	}
}

func TestEntries_CarryUpdateTimes(t *testing.T) {
	v, _ := newTestVault(t)
	before := time.Now().UTC().Add(-time.Second)
	if err := v.Set("stamped", "v"); err != nil {
		t.Fatal(err)
	}
	entries, err := v.Entries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].UpdatedUTC.Before(before) {
		t.Fatalf("updated = %v, want a recent timestamp", entries[0].UpdatedUTC)
	}
}

func TestList_EmptyVault(t *testing.T) {
	v, _ := newTestVault(t)
	names, err := v.List()
	if err != nil || len(names) != 0 {
		t.Fatalf("names = %v, err = %v", names, err)
	}
}

// Names -----------------------------------------------------------------

func TestValidateName(t *testing.T) {
	ok := []string{"a", "api-key", "api_key", "api.key", "Azure1", "café"}
	for _, n := range ok {
		if err := validateName(n); err != nil {
			t.Errorf("validateName(%q) = %v, want nil", n, err)
		}
	}
	bad := []string{"", "   ", "a/b", "a\\b", "../escape", ".", "..", "a b", "a:b", "a*b", "a\x00b"}
	for _, n := range bad {
		if err := validateName(n); err == nil {
			t.Errorf("validateName(%q) = nil, want an error", n)
		}
	}
}

func TestOperations_RejectBadNamesBeforeTouchingDisk(t *testing.T) {
	v, _ := newTestVault(t)
	for _, n := range []string{"", "../../etc/passwd", "a/b"} {
		if err := v.Set(n, "x"); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Set(%q) = %v, want ErrInvalidName", n, err)
		}
		if _, err := v.Get(n); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Get(%q) = %v, want ErrInvalidName", n, err)
		}
		if err := v.Delete(n); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Delete(%q) = %v, want ErrInvalidName", n, err)
		}
	}
}

// Tamper resistance -----------------------------------------------------

func TestTamper_FlippedCiphertextByteFailsAuthentication(t *testing.T) {
	v, dir := newTestVault(t)
	if err := v.Set("target", "original"); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, secretsDirName, "target"+secretExt)
	var rec record
	if err := readJSON(path, &rec); err != nil {
		t.Fatal(err)
	}
	rec.Ciphertext = flipBase64Byte(t, rec.Ciphertext)
	if err := writeJSON(path, rec); err != nil {
		t.Fatal(err)
	}

	_, err := v.Get("target")
	var ce *CorruptError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want a CorruptError", err)
	}
}

func TestTamper_RecordFromAnotherVaultIsRejected(t *testing.T) {
	a, dirA := newTestVault(t)
	if err := a.Set("shared", "from-a"); err != nil {
		t.Fatal(err)
	}

	dirB := t.TempDir()
	b, err := Create(dirB, "different password", WithParams(fastParams))
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Set("shared", "from-b"); err != nil {
		t.Fatal(err)
	}

	// Swap A's record into B. Same name, same format, different vault id in
	// the AAD, so it must not decrypt.
	src, err := os.ReadFile(filepath.Join(dirA, secretsDirName, "shared"+secretExt))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirB, secretsDirName, "shared"+secretExt), src, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := b.Get("shared"); err == nil {
		t.Fatal("a record lifted from another vault must not decrypt")
	}
}

func TestTamper_RenamedRecordIsRejected(t *testing.T) {
	v, dir := newTestVault(t)
	if err := v.Set("low-privilege", "boring"); err != nil {
		t.Fatal(err)
	}
	if err := v.Set("admin-token", "powerful"); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(dir, secretsDirName, "low-privilege"+secretExt)
	dst := filepath.Join(dir, secretsDirName, "admin-token"+secretExt)
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := v.Get("admin-token"); err == nil {
		t.Fatal("the secret name is in the AAD, so a renamed record must not decrypt")
	}
}

func TestCorrupt_MalformedRecordsAreNotReportedAsBadPasswords(t *testing.T) {
	v, dir := newTestVault(t)
	if err := v.Set("broken", "value"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, secretsDirName, "broken"+secretExt)

	cases := map[string]record{
		"short nonce": {Nonce: "AAAA", Ciphertext: "AAAA", Tag: strings.Repeat("A", 24)},
		"short tag":   {Nonce: strings.Repeat("A", 16), Ciphertext: "AAAA", Tag: "AAAA"},
		"bad base64":  {Nonce: "!!!!", Ciphertext: "AAAA", Tag: strings.Repeat("A", 24)},
		"bad algorithm": {Algorithm: "rot13", Nonce: strings.Repeat("A", 16),
			Ciphertext: "AAAA", Tag: strings.Repeat("A", 24)},
	}
	for name, rec := range cases {
		t.Run(name, func(t *testing.T) {
			if err := writeJSON(path, rec); err != nil {
				t.Fatal(err)
			}
			_, err := v.Get("broken")
			var ce *CorruptError
			if !errors.As(err, &ce) {
				t.Fatalf("err = %v, want a CorruptError", err)
			}
			if errors.Is(err, ErrInvalidPassword) {
				t.Fatal("a corrupt record must never read as a wrong password")
			}
		})
	}
}

func TestCorrupt_UnreadableHeader(t *testing.T) {
	_, dir := newTestVault(t)
	if err := os.WriteFile(filepath.Join(dir, headerFileName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Open(dir)
	var ce *CorruptError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want a CorruptError", err)
	}
}

func TestCorrupt_HeaderWithoutAVaultID(t *testing.T) {
	_, dir := newTestVault(t)
	if err := os.WriteFile(filepath.Join(dir, headerFileName), []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("a header with no vault id cannot bind any AAD")
	}
}

// On-disk shape ---------------------------------------------------------

func TestOnDisk_HeaderMatchesTheDocumentedShape(t *testing.T) {
	_, dir := newTestVault(t)
	b, err := os.ReadFile(filepath.Join(dir, headerFileName))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"version", "vaultId", "createdUtc", "kdf", "canary"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("header is missing %q", key)
		}
	}
	kdf, _ := raw["kdf"].(map[string]any)
	for _, key := range []string{"algorithm", "memoryKiB", "iterations", "parallelism", "salt"} {
		if _, ok := kdf[key]; !ok {
			t.Errorf("kdf is missing %q", key)
		}
	}
	if kdf["algorithm"] != algorithmArgon2id {
		t.Errorf("algorithm = %v", kdf["algorithm"])
	}
	canary, _ := raw["canary"].(map[string]any)
	for _, key := range []string{"nonce", "ciphertext", "tag"} {
		if _, ok := canary[key]; !ok {
			t.Errorf("canary is missing %q", key)
		}
	}
	if _, present := canary["version"]; present {
		t.Error("the canary carries no version field in the documented format")
	}
}

func TestOnDisk_RecordMatchesTheDocumentedShape(t *testing.T) {
	v, dir := newTestVault(t)
	if err := v.Set("shaped", "value"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, secretsDirName, "shaped"+secretExt))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"version", "algorithm", "nonce", "ciphertext", "tag", "updatedUtc"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("record is missing %q", key)
		}
	}
	if raw["algorithm"] != algorithmAESGCM {
		t.Errorf("algorithm = %v", raw["algorithm"])
	}
}

func TestOnDisk_SecretsAreNotWorldReadable(t *testing.T) {
	v, dir := newTestVault(t)
	if err := v.Set("private", "value"); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(dir, secretsDirName, "private"+secretExt))
	if err != nil {
		t.Fatal(err)
	}
	if perm := st.Mode().Perm(); perm&0o077 != 0 {
		t.Fatalf("mode = %o, want no group or other bits", perm)
	}
}

func TestOnDisk_NoPlaintextLeaksIntoTheRecord(t *testing.T) {
	v, dir := newTestVault(t)
	const secret = "hunter2-do-not-leak"
	if err := v.Set("leaky", secret); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, secretsDirName, "leaky"+secretExt))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), secret) {
		t.Fatal("the plaintext is sitting in the record file")
	}
}

func TestOnDisk_EveryWriteTakesAFreshNonce(t *testing.T) {
	v, dir := newTestVault(t)
	path := filepath.Join(dir, secretsDirName, "rotating"+secretExt)
	seen := map[string]bool{}

	for range 8 {
		if err := v.Set("rotating", "same value every time"); err != nil {
			t.Fatal(err)
		}
		var rec record
		if err := readJSON(path, &rec); err != nil {
			t.Fatal(err)
		}
		if seen[rec.Nonce] {
			t.Fatal("a nonce was reused; GCM key recovery starts here")
		}
		seen[rec.Nonce] = true
	}
}

func TestOnDisk_NoTempFilesSurviveAWrite(t *testing.T) {
	v, dir := newTestVault(t)
	if err := v.Set("clean", "value"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, secretsDirName))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Fatalf("left a temp file behind: %s", e.Name())
		}
	}
}

// Concurrency -----------------------------------------------------------

func TestConcurrent_ReadsAndWrites(t *testing.T) {
	v, _ := newTestVault(t)
	if err := v.Set("shared", "initial"); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := range 24 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			switch i % 4 {
			case 0:
				_ = v.Set("shared", "value")
			case 1:
				_, _ = v.Get("shared")
			case 2:
				_, _ = v.List()
			case 3:
				_ = v.Has("shared")
			}
		}(i)
	}
	wg.Wait()

	got, err := v.Get("shared")
	if err != nil {
		t.Fatalf("vault damaged by concurrent access: %v", err)
	}
	if got != "initial" && got != "value" {
		t.Fatalf("value = %q, want one of the written values whole", got)
	}
}

func TestConcurrent_UnlockRace(t *testing.T) {
	_, dir := newTestVault(t)
	v, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = v.Unlock("correct horse battery staple")
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("unlock %d: %v", i, err)
		}
	}
	if v.IsLocked() {
		t.Fatal("vault ended up locked")
	}
}

// flipBase64Byte mutates one byte of a base64 payload and re-encodes it.
func flipBase64Byte(t *testing.T, encoded string) string {
	t.Helper()
	raw, err := decodeBase64(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 {
		t.Fatal("nothing to flip")
	}
	raw[0] ^= 0xFF
	return encodeBase64(raw)
}
