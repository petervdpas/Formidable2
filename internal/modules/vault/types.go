// Package vault is an encrypted, portable secrets vault: a plain directory of
// files, keyed by a master password, that is not bound to any OS account.
//
// The on-disk format is byte-compatible with SecretBlast (the .NET library
// behind TaskBlaster), so one vault directory can be opened by either. That
// compatibility is a tested property, not an aspiration: vault_interop_test.go
// drives the real SecretBlast build and round-trips a vault in both directions.
//
// Formidable deliberately does not use the OS keychain here. A vault travels
// with the user between machines, survives a reinstall, and can be shared with
// the other tools that already read this format. The OS-keychain path stays
// where it is, in collaboration/credential, for git and gigot tokens.
//
//	<dir>/vault.json                 header: vault id, KDF params, salt, canary
//	<dir>/secrets/<name>.secret      one AES-256-GCM record per secret
package vault

import (
	"encoding/json"
	"strings"
	"time"
)

// On-disk names, fixed by the format.
const (
	headerFileName = "vault.json"
	secretsDirName = "secrets"
	secretExt      = ".secret"
)

// Crypto sizes, fixed by the format.
const (
	saltLen  = 16
	nonceLen = 12
	tagLen   = 16
	keyLen   = 32
)

// formatVersion is the on-disk shape this package writes. Records carry their
// own version so a later algorithm change can lazy-migrate on next write.
const formatVersion = 1

const algorithmAESGCM = "aes-256-gcm"
const algorithmArgon2id = "argon2id"

// Params are the Argon2id work factors. They live in the header in the clear
// and are per-vault, so a vault created on weak hardware can be re-keyed to
// stronger parameters later without a format change.
type Params struct {
	MemoryKiB   uint32 `json:"memoryKiB"`
	Iterations  uint32 `json:"iterations"`
	Parallelism uint8  `json:"parallelism"`
}

// DefaultParams matches the SecretBlast defaults, so a vault created here opens
// there with identical cost.
var DefaultParams = Params{MemoryKiB: 65536, Iterations: 3, Parallelism: 1}

// header is vault.json. Salt and KDF parameters are plaintext on purpose: what
// protects the vault is the password and the work factor, not obscurity.
type header struct {
	Version    int       `json:"version"`
	VaultID    string    `json:"vaultId"`
	CreatedUTC utcTime   `json:"createdUtc"`
	KDF        kdfHeader `json:"kdf"`
	Canary     record    `json:"canary"`
}

type kdfHeader struct {
	Algorithm   string `json:"algorithm"`
	MemoryKiB   uint32 `json:"memoryKiB"`
	Iterations  uint32 `json:"iterations"`
	Parallelism uint8  `json:"parallelism"`
	Salt        string `json:"salt"`
}

// record is one AES-GCM payload, used both for a *.secret file and for the
// header canary. The canary carries no version or algorithm field, so both are
// omitempty and the two shapes share one type.
type record struct {
	Version    int     `json:"version,omitempty"`
	Algorithm  string  `json:"algorithm,omitempty"`
	Nonce      string  `json:"nonce"`
	Ciphertext string  `json:"ciphertext"`
	Tag        string  `json:"tag"`
	UpdatedUTC utcTime `json:"updatedUtc,omitzero"`
}

// utcTime reads the timestamps .NET writes as well as the ones Go writes. The
// .NET serializer emits a Kind-dependent suffix, so a stored value may or may
// not carry a zone; refusing the unzoned form would make a vault written by
// one runtime unreadable by the other.
type utcTime struct {
	time.Time
}

var timeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.9999999",
	"2006-01-02T15:04:05",
}

func (t *utcTime) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if strings.TrimSpace(s) == "" {
		return nil
	}
	for _, layout := range timeLayouts {
		if parsed, err := time.Parse(layout, s); err == nil {
			t.Time = parsed.UTC()
			return nil
		}
	}
	return &CorruptError{Detail: "unparseable timestamp " + s}
}

func (t utcTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.UTC().Format(time.RFC3339Nano))
}

// IsZero lets encoding/json omit an unset timestamp via omitzero.
func (t utcTime) IsZero() bool { return t.Time.IsZero() }

// Entry is one secret as the caller sees it, without its value.
type Entry struct {
	Name       string    `json:"name"`
	UpdatedUTC time.Time `json:"updated_utc"`
}
