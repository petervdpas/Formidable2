package credential

import (
	"errors"
	"strings"

	"github.com/zalando/go-keyring"
)

// service is the keychain "vendor" namespace shared by every
// Formidable credential. Backends differentiate via the account
// name (e.g. a Git remote URL, a GiGot subscription endpoint).
const service = "Formidable"

// Manager is the read/write entry point for the OS keychain.
// Stateless — go-keyring talks directly to the platform-native
// store and we just adapt its API. Tests inject keyring.MockInit()
// at package init, so domain_test.go runs in-memory.
type Manager struct{}

// NewManager constructs the credential Manager. No options yet —
// the only knob we'd want (an alternate "service" namespace) is
// likely overkill; one shared namespace keeps the OS UI tidy and
// avoids stranded entries when the app gets renamed.
func NewManager() *Manager { return &Manager{} }

// Set stores secret under the given account name, overwriting any
// existing entry. Empty account or empty secret are rejected — both
// are almost always UI bugs (form submitted before the PAT was
// pasted, missing remote URL).
func (m *Manager) Set(account, secret string) error {
	if strings.TrimSpace(account) == "" {
		return errors.New("credential: empty account")
	}
	if secret == "" {
		return errors.New("credential: empty secret")
	}
	return keyring.Set(service, account, secret)
}

// Get returns the secret stored under account, or an error if no
// entry exists (or the platform keychain is unavailable). Callers
// should treat ErrNotFound as "user must re-enter the PAT", not as
// a fatal app error.
func (m *Manager) Get(account string) (string, error) {
	if strings.TrimSpace(account) == "" {
		return "", errors.New("credential: empty account")
	}
	return keyring.Get(service, account)
}

// Has reports whether a non-empty entry exists under account. False
// when missing, when the keychain rejected the read, or when account
// is empty — the caller's correct response to all three is "prompt
// for a PAT", so collapsing them to a single bool is fine.
func (m *Manager) Has(account string) bool {
	if strings.TrimSpace(account) == "" {
		return false
	}
	v, err := keyring.Get(service, account)
	return err == nil && v != ""
}

// Delete removes the entry for account. Idempotent — deleting a
// missing entry is not an error. The UI calls this from a future
// "Forget token" action; the soft-fail behaviour means it can be
// invoked unconditionally without the UI having to check Has first.
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
