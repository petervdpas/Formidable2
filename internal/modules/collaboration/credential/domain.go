package credential

import (
	"errors"
	"strings"

	"github.com/zalando/go-keyring"
)

// service is the shared keychain namespace; backends differentiate via the account name.
const service = "Formidable"

// Manager is the read/write entry point for the OS keychain.
type Manager struct{}

// NewManager constructs the credential Manager.
func NewManager() *Manager { return &Manager{} }

// Set stores secret under the given account name, overwriting any existing entry.
func (m *Manager) Set(account, secret string) error {
	if strings.TrimSpace(account) == "" {
		return errors.New("credential: empty account")
	}
	if secret == "" {
		return errors.New("credential: empty secret")
	}
	return keyring.Set(service, account, secret)
}

// Get returns the secret stored under account. Treat ErrNotFound as "user must re-enter the PAT", not fatal.
func (m *Manager) Get(account string) (string, error) {
	if strings.TrimSpace(account) == "" {
		return "", errors.New("credential: empty account")
	}
	return keyring.Get(service, account)
}

// Has reports whether a non-empty entry exists under account.
func (m *Manager) Has(account string) bool {
	if strings.TrimSpace(account) == "" {
		return false
	}
	v, err := keyring.Get(service, account)
	return err == nil && v != ""
}

// Delete removes the entry for account. Idempotent: deleting a missing entry is not an error.
func (m *Manager) Delete(account string) error {
	if strings.TrimSpace(account) == "" {
		return nil
	}
	err := keyring.Delete(service, account)
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
