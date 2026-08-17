package vault

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"

	"github.com/petervdpas/formidable2/internal/event"
)

// EventLocked fires whenever the vault locks, including when it locks itself
// after the idle timeout. Without it the UI keeps showing "unlocked" until the
// user's next action fails, which is the worst moment to find out.
const EventLocked = "vault:locked"

// DirName is the vault's home under AppRoot. Machine scoped and outside the
// context folder: a vault must never ride along with a git or gigot sync.
const DirName = "vault"

// MinPasswordLength is the shortest master password the app accepts. Argon2id
// buys time against a weak one; it does not make a four-character password
// safe. The library itself stays policy-free, so this is enforced here, at the
// boundary where a human types.
const MinPasswordLength = 8

// ErrWeakPassword is returned when a master password is shorter than
// MinPasswordLength.
var ErrWeakPassword = errors.New("vault: master password is too short")

// Policy is the rule set the UI must obey. It is served rather than restated:
// a minimum hardcoded in the frontend drifts the first time this changes.
type Policy struct {
	MinPasswordLength int `json:"min_password_length"`
	AutoLockMinutes   int `json:"auto_lock_minutes"`
}

// Status is what the settings panel renders. It is deliberately safe to fetch
// at any time: nothing here requires the vault to be unlocked.
type Status struct {
	Path     string `json:"path"`
	Exists   bool   `json:"exists"`
	Unlocked bool   `json:"unlocked"`
	Secrets  int    `json:"secrets"`
}

// Service is the Wails-bound facade for the vault.
//
// Secret values only ever travel inward. There is no GetSecret: the frontend
// can store, replace, delete and count credentials, but never read one back.
// Everything that needs a value (the api-client invoker) runs in Go and reads
// the vault directly, so a plaintext credential never crosses into JavaScript.
type Service struct {
	root string
	log  *slog.Logger
	emit event.Emitter

	mu sync.Mutex
	v  *Vault
}

// NewService binds a vault living at <appRoot>/vault. emit may be nil.
func NewService(appRoot string, log *slog.Logger, emit event.Emitter) *Service {
	return &Service{root: filepath.Join(appRoot, DirName), log: log, emit: emit}
}

// Dir is the vault directory, for the panel to show and for the composition
// root to build a Resolver against.
func (s *Service) Dir() string { return s.root }

// VaultStatus reports whether a vault exists, whether it is open, and how many
// secrets it holds. Names are filenames, so the count needs no key.
func (s *Service) VaultStatus() Status {
	st := Status{Path: s.root, Exists: Exists(s.root)}
	if !st.Exists {
		return st
	}
	v := s.current()
	st.Unlocked = v != nil && !v.IsLocked()
	if v != nil {
		if names, err := v.List(); err == nil {
			st.Secrets = len(names)
		}
	}
	return st
}

// InitializeVault creates the vault under <AppRoot>/vault and leaves it
// unlocked. It refuses when one already exists, since overwriting would
// discard every stored credential.
func (s *Service) InitializeVault(masterPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.v != nil {
		return ErrVaultExists
	}
	if err := checkPasswordPolicy(masterPassword); err != nil {
		return err
	}
	v, err := Create(s.root, masterPassword, OnLock(s.onLocked))
	if err != nil {
		return err
	}
	s.v = v
	return nil
}

// UnlockVault opens the vault with the master password. A wrong password
// returns ErrInvalidPassword and leaves the vault locked.
func (s *Service) UnlockVault(masterPassword string) error {
	s.mu.Lock()
	if s.v == nil {
		v, err := Open(s.root, OnLock(s.onLocked))
		if err != nil {
			s.mu.Unlock()
			return err
		}
		s.v = v
	}
	v := s.v
	s.mu.Unlock()

	return v.Unlock(masterPassword)
}

// LockVault zeroes the derived key. Safe to call when already locked.
func (s *Service) LockVault() {
	if v := s.current(); v != nil {
		v.Lock()
	}
}

// ListSecrets returns the stored entry names and their last-write times. No
// values, and no unlock required.
func (s *Service) ListSecrets() ([]Entry, error) {
	v := s.current()
	if v == nil {
		return nil, ErrVaultNotFound
	}
	return v.Entries()
}

// SetSecret stores or replaces a value under name.
func (s *Service) SetSecret(name, value string) error {
	v := s.current()
	if v == nil {
		return ErrVaultNotFound
	}
	return v.Set(name, value)
}

// DeleteSecret removes a stored value.
func (s *Service) DeleteSecret(name string) error {
	v := s.current()
	if v == nil {
		return ErrVaultNotFound
	}
	return v.Delete(name)
}

// HasSecret reports whether a value is stored under name, without unlocking.
func (s *Service) HasSecret(name string) bool {
	v := s.current()
	return v != nil && v.Has(name)
}

// ResolverFor hands out a prefixed view of a Service's vault, for the
// composition root to give to a consumer such as the api-client invoker.
//
// It is a package function rather than a method on purpose: Wails binds every
// exported method of a bound service, and this one is wiring, not UI. Keeping
// it off the method set keeps it out of the generated frontend API.
func ResolverFor(s *Service, prefix string) *Resolver {
	if s == nil {
		return nil
	}
	return NewLazyResolver(s.current, prefix)
}

// current returns the live vault, opening the header lazily so a vault created
// outside this process (or after startup) is picked up without a restart.
func (s *Service) current() *Vault {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.v != nil {
		return s.v
	}
	if !Exists(s.root) {
		return nil
	}
	v, err := Open(s.root, OnLock(s.onLocked))
	if err != nil {
		if s.log != nil && !errors.Is(err, ErrVaultNotFound) {
			s.log.Warn("vault: cannot open", "path", s.root, "err", err)
		}
		return nil
	}
	s.v = v
	return v
}

// onLocked runs on every lock, whether the user asked for it or the idle
// timer fired. It tells the frontend so a stale "unlocked" badge cannot
// outlive the key it describes.
func (s *Service) onLocked() {
	if s.log != nil {
		s.log.Info("vault: locked", "path", s.root)
	}
	event.Emit(s.emit, EventLocked, map[string]any{"path": s.root})
}

// VaultPolicy returns the password and auto-lock rules the UI must enforce. It
// is served rather than restated: a minimum hardcoded in the frontend drifts
// the first time this one changes.
func (s *Service) VaultPolicy() Policy {
	return Policy{
		MinPasswordLength: MinPasswordLength,
		AutoLockMinutes:   int(DefaultAutoLock.Minutes()),
	}
}

// ChangeMasterPassword re-encrypts the vault under a new password. The old one
// is required even on an unlocked vault, so an unattended app cannot be used
// to lock its owner out.
func (s *Service) ChangeMasterPassword(oldPassword, newPassword string) error {
	if err := checkPasswordPolicy(newPassword); err != nil {
		return err
	}
	v := s.current()
	if v == nil {
		return ErrVaultNotFound
	}
	return v.ChangePassword(oldPassword, newPassword)
}

func checkPasswordPolicy(password string) error {
	if len([]rune(password)) < MinPasswordLength {
		return fmt.Errorf("%w: use at least %d characters", ErrWeakPassword, MinPasswordLength)
	}
	return nil
}
