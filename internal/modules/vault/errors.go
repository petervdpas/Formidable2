package vault

import "errors"

// Sentinel errors. Callers distinguish "wrong password" from "no vault" from
// "locked" to decide whether to re-prompt, offer creation, or just unlock.
var (
	// ErrLocked is returned by any data operation on a locked vault.
	ErrLocked = errors.New("vault: locked")

	// ErrInvalidPassword means the derived key failed the header canary.
	ErrInvalidPassword = errors.New("vault: invalid master password")

	// ErrNotFound means no secret is stored under that name.
	ErrNotFound = errors.New("vault: secret not found")

	// ErrVaultNotFound means the directory holds no vault.json.
	ErrVaultNotFound = errors.New("vault: no vault at that path")

	// ErrVaultExists means Create was pointed at a directory that already
	// holds a vault. Overwriting one would destroy every secret in it.
	ErrVaultExists = errors.New("vault: a vault already exists at that path")

	// ErrInvalidName means the secret name is empty or holds a character that
	// cannot appear in a filename.
	ErrInvalidName = errors.New("vault: invalid secret name")
)

// CorruptError reports a vault whose files are present but unreadable: bad
// JSON, wrong-length crypto fields, a tag that does not authenticate against
// an otherwise-correct key. It is deliberately distinct from
// ErrInvalidPassword so the UI never tells someone to retype a good password.
type CorruptError struct {
	Path   string
	Detail string
	Cause  error
}

func (e *CorruptError) Error() string {
	msg := "vault: corrupt"
	if e.Path != "" {
		msg += " " + e.Path
	}
	if e.Detail != "" {
		msg += ": " + e.Detail
	}
	return msg
}

func (e *CorruptError) Unwrap() error { return e.Cause }

func corrupt(path, detail string, cause error) error {
	return &CorruptError{Path: path, Detail: detail, Cause: cause}
}
