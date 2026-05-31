package credential

import (
	"errors"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"
)

// service is the shared keychain namespace; backends differentiate via the account name.
const service = "Formidable"

// keyringMu serializes access to the process-global keyring backend. Real OS
// keychains serialize internally; the test mock is an unsynchronized map, so
// concurrent Set/Get would race without this.
var keyringMu sync.Mutex

// Manager is the read/write entry point for the OS keychain.
type Manager struct{}

// NewManager constructs the credential Manager.
func NewManager() *Manager { return &Manager{} }

// Set stores secret under the given account name, overwriting any existing entry.
func (m *Manager) Set(account, secret string) error {
	if strings.TrimSpace(account) == "" {
		return errors.New("credential: empty account")
	}
	if strings.TrimSpace(secret) == "" {
		return errors.New("credential: empty secret")
	}
	keyringMu.Lock()
	defer keyringMu.Unlock()
	return keyring.Set(service, account, secret)
}

// Get returns the secret stored under account. Treat ErrNotFound as "user must re-enter the PAT", not fatal.
func (m *Manager) Get(account string) (string, error) {
	if strings.TrimSpace(account) == "" {
		return "", errors.New("credential: empty account")
	}
	keyringMu.Lock()
	defer keyringMu.Unlock()
	return keyring.Get(service, account)
}

// Has reports whether a non-empty entry exists under account.
func (m *Manager) Has(account string) bool {
	if strings.TrimSpace(account) == "" {
		return false
	}
	keyringMu.Lock()
	defer keyringMu.Unlock()
	v, err := keyring.Get(service, account)
	return err == nil && v != ""
}

// Delete removes the entry for account. Idempotent: deleting a missing entry is not an error.
func (m *Manager) Delete(account string) error {
	if strings.TrimSpace(account) == "" {
		return nil
	}
	keyringMu.Lock()
	defer keyringMu.Unlock()
	err := keyring.Delete(service, account)
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
