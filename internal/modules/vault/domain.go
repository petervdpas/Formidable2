package vault

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// DefaultAutoLock is how long an unlocked vault stays unlocked without use.
// The derived key sits in process memory while unlocked, so the timeout is
// what bounds that window.
const DefaultAutoLock = 15 * time.Minute

// Option configures a vault at Create or Open.
type Option func(*Vault)

// WithParams sets the Argon2id work factors used by Create. Open reads the
// parameters from the header instead, so this is inert there.
func WithParams(p Params) Option {
	return func(v *Vault) { v.params = p }
}

// WithAutoLock sets the idle timeout. Zero or negative disables auto-locking.
func WithAutoLock(d time.Duration) Option {
	return func(v *Vault) { v.autoLock = d }
}

// OnLock registers a callback fired whenever the vault locks, so a UI can drop
// back to a password prompt without polling.
func OnLock(fn func()) Option {
	return func(v *Vault) { v.onLock = fn }
}

// Vault is one vault directory. All methods are safe for concurrent use.
type Vault struct {
	dir      string
	params   Params
	autoLock time.Duration
	onLock   func()

	mu     sync.Mutex
	head   header
	key    []byte
	timer  *time.Timer
	nameMu sync.Map
}

// Exists reports whether dir holds a vault, so a caller can decide between an
// unlock prompt and a create prompt before touching anything.
func Exists(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, headerFileName))
	return err == nil && !st.IsDir()
}

// Create writes a new vault under dir and returns it unlocked. It refuses to
// overwrite an existing vault: doing so would discard every secret in it.
// A directory holding unrelated files is fine.
func Create(dir, password string, opts ...Option) (*Vault, error) {
	v := newVault(dir, opts...)
	if Exists(dir) {
		return nil, ErrVaultExists
	}
	if err := os.MkdirAll(filepath.Join(dir, secretsDirName), 0o700); err != nil {
		return nil, err
	}

	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	vaultID := uuid.NewString()
	key := deriveKey(password, salt, v.params)

	canary, err := seal(key, vaultID, canarySlot, canaryPlaintext)
	if err != nil {
		zero(key)
		return nil, err
	}

	v.head = header{
		Version:    formatVersion,
		VaultID:    vaultID,
		CreatedUTC: utcTime{time.Now().UTC()},
		KDF: kdfHeader{
			Algorithm:   algorithmArgon2id,
			MemoryKiB:   v.params.MemoryKiB,
			Iterations:  v.params.Iterations,
			Parallelism: v.params.Parallelism,
			Salt:        base64.StdEncoding.EncodeToString(salt),
		},
		Canary: canary,
	}
	if err := writeJSON(v.headerPath(), v.head); err != nil {
		zero(key)
		return nil, err
	}

	v.key = key
	v.touch()
	return v, nil
}

// Open reads an existing vault's header. The vault comes back locked; call
// Unlock with the master password before any data operation. Opening eagerly
// means a wrong path surfaces before the user is asked to type anything.
func Open(dir string, opts ...Option) (*Vault, error) {
	v := newVault(dir, opts...)
	if !Exists(dir) {
		return nil, ErrVaultNotFound
	}
	head, err := readHeader(v.headerPath())
	if err != nil {
		return nil, err
	}
	v.head = head
	return v, nil
}

func newVault(dir string, opts ...Option) *Vault {
	v := &Vault{dir: dir, params: DefaultParams, autoLock: DefaultAutoLock}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// Unlock derives the master key and verifies it against the header canary.
// Unlocking an already-unlocked vault is a no-op and does not re-check the
// password: the holder of the reference has already proven it once.
func (v *Vault) Unlock(password string) error {
	v.mu.Lock()
	if v.key != nil {
		v.mu.Unlock()
		v.touch()
		return nil
	}
	head := v.head
	v.mu.Unlock()

	salt, err := base64.StdEncoding.DecodeString(head.KDF.Salt)
	if err != nil || len(salt) != saltLen {
		return corrupt(v.headerPath(), "salt is not a valid 16-byte value", err)
	}
	if head.KDF.Algorithm != "" && head.KDF.Algorithm != algorithmArgon2id {
		return corrupt(v.headerPath(), "unsupported KDF "+head.KDF.Algorithm, nil)
	}

	// Derivation is the expensive step and needs no lock: it reads only the
	// header copy taken above, so a concurrent unlock costs CPU, not safety.
	key := deriveKey(password, salt, Params{
		MemoryKiB:   head.KDF.MemoryKiB,
		Iterations:  head.KDF.Iterations,
		Parallelism: head.KDF.Parallelism,
	})

	_, ok, err := open(key, head.VaultID, canarySlot, head.Canary, v.headerPath())
	if err != nil {
		zero(key)
		return err
	}
	if !ok {
		zero(key)
		return ErrInvalidPassword
	}

	v.mu.Lock()
	if v.key == nil {
		v.key = key
	} else {
		zero(key)
	}
	v.mu.Unlock()
	v.touch()
	return nil
}

// Lock zeroes the derived key. Safe to call when already locked.
func (v *Vault) Lock() {
	v.mu.Lock()
	had := v.key != nil
	if v.key != nil {
		zero(v.key)
		v.key = nil
	}
	if v.timer != nil {
		v.timer.Stop()
		v.timer = nil
	}
	fn := v.onLock
	v.mu.Unlock()

	if had && fn != nil {
		fn()
	}
}

// IsLocked reports whether a data operation would need an unlock first.
func (v *Vault) IsLocked() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.key == nil
}

// VaultID is the identifier bound into every record's AAD.
func (v *Vault) VaultID() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.head.VaultID
}

// Get returns the value stored under name.
func (v *Vault) Get(name string) (string, error) {
	if err := validateName(name); err != nil {
		return "", err
	}
	key, vaultID, err := v.session()
	if err != nil {
		return "", err
	}
	unlock := v.lockName(name)
	defer unlock()

	path := v.secretPath(name)
	var rec record
	if err := readJSON(path, &rec); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrNotFound
		}
		return "", err
	}
	if rec.Algorithm != "" && rec.Algorithm != algorithmAESGCM {
		return "", corrupt(path, "unsupported algorithm "+rec.Algorithm, nil)
	}

	plaintext, ok, err := open(key, vaultID, name, rec, path)
	if err != nil {
		return "", err
	}
	if !ok {
		// The key already passed the header canary, so a failure here is the
		// record itself: tampered, or lifted in from another vault.
		return "", corrupt(path, "record does not authenticate against this vault", nil)
	}
	return string(plaintext), nil
}

// Set writes name, replacing any existing value. Every write takes a fresh
// nonce, which is safe because a record is always rewritten whole.
func (v *Vault) Set(name, value string) error {
	if err := validateName(name); err != nil {
		return err
	}
	key, vaultID, err := v.session()
	if err != nil {
		return err
	}
	unlock := v.lockName(name)
	defer unlock()

	rec, err := seal(key, vaultID, name, []byte(value))
	if err != nil {
		return err
	}
	rec.Version = formatVersion
	rec.Algorithm = algorithmAESGCM
	rec.UpdatedUTC = utcTime{time.Now().UTC()}
	return writeJSON(v.secretPath(name), rec)
}

// Delete removes name. Deleting a missing secret reports ErrNotFound so a
// caller can tell a real removal from a no-op.
func (v *Vault) Delete(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	if _, _, err := v.session(); err != nil {
		return err
	}
	unlock := v.lockName(name)
	defer unlock()

	err := os.Remove(v.secretPath(name))
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	return err
}

// Has reports whether a secret is stored under name, without decrypting it.
func (v *Vault) Has(name string) bool {
	if validateName(name) != nil {
		return false
	}
	_, err := os.Stat(v.secretPath(name))
	return err == nil
}

// List returns the stored secret names, sorted. Names are the filenames, so
// listing needs no key; the values stay sealed.
func (v *Vault) List() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(v.dir, secretsDirName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), secretExt) {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), secretExt))
	}
	slices.Sort(names)
	return names, nil
}

// Entries is List plus each record's last-write time, for a management view
// that wants to show age without unsealing anything.
func (v *Vault) Entries() ([]Entry, error) {
	names, err := v.List()
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(names))
	for _, name := range names {
		e := Entry{Name: name}
		var rec record
		if err := readJSON(v.secretPath(name), &rec); err == nil {
			e.UpdatedUTC = rec.UpdatedUTC.Time
		}
		out = append(out, e)
	}
	return out, nil
}

// session returns the live key and vault id, or ErrLocked. It also resets the
// idle timer, so an active vault does not lock under the user's hands.
func (v *Vault) session() ([]byte, string, error) {
	v.mu.Lock()
	key, vaultID := v.key, v.head.VaultID
	v.mu.Unlock()
	if key == nil {
		return nil, "", ErrLocked
	}
	v.touch()
	return key, vaultID, nil
}

// touch restarts the auto-lock countdown.
func (v *Vault) touch() {
	if v.autoLock <= 0 {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.key == nil {
		return
	}
	if v.timer != nil {
		v.timer.Stop()
	}
	v.timer = time.AfterFunc(v.autoLock, v.Lock)
}

// lockName serialises operations on one secret, so a concurrent read never
// sees a half-replaced file on platforms where rename is not atomic over an
// open handle.
func (v *Vault) lockName(name string) func() {
	actual, _ := v.nameMu.LoadOrStore(name, &sync.Mutex{})
	mu := actual.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func (v *Vault) headerPath() string { return filepath.Join(v.dir, headerFileName) }

func (v *Vault) secretPath(name string) string {
	return filepath.Join(v.dir, secretsDirName, name+secretExt)
}

// validateName keeps a secret name usable as a filename on every platform, and
// incidentally forecloses traversal: no separator survives the check.
func validateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return ErrInvalidName
	}
	for _, ch := range name {
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) {
			continue
		}
		if ch == '-' || ch == '_' || ch == '.' {
			continue
		}
		return ErrInvalidName
	}
	// "." and ".." would resolve to a directory rather than a record.
	if strings.Trim(name, ".") == "" {
		return ErrInvalidName
	}
	return nil
}

func readHeader(path string) (header, error) {
	var h header
	if err := readJSON(path, &h); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return header{}, ErrVaultNotFound
		}
		return header{}, err
	}
	if h.VaultID == "" {
		return header{}, corrupt(path, "header has no vault id", nil)
	}
	return h, nil
}

func readJSON(path string, out any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, out); err != nil {
		var ce *CorruptError
		if errors.As(err, &ce) {
			return err
		}
		return corrupt(path, "not valid JSON", err)
	}
	return nil
}

// writeJSON writes indented JSON through a temp file and a rename, at 0600.
// The payload is ciphertext, so the mode is belt and braces rather than the
// thing that protects it, but a secrets directory has no business being
// world-readable.
func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
