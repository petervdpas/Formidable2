package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"

	"golang.org/x/crypto/argon2"
	"golang.org/x/text/unicode/norm"
)

// AAD construction. Every ciphertext is bound to its slot by
// "SecretBlast" || vaultId || slot, so a record file lifted from another vault,
// or renamed into a different slot in this one, fails authentication rather
// than silently decrypting into the wrong place.
const (
	domainSeparator = "SecretBlast"
	canarySlot      = "canary"
)

// canaryPlaintext is what the header canary encrypts. The bytes carry no
// meaning; only that decrypting them succeeds proves the derived key.
var canaryPlaintext = []byte("canary-v1")

// deriveKey runs Argon2id over the password. The password is normalised to NFC
// before encoding, so the same characters typed on one platform and pasted from
// another derive the same key. ASCII passwords are unaffected.
func deriveKey(password string, salt []byte, p Params) []byte {
	pw := []byte(norm.NFC.String(password))
	parallelism := p.Parallelism
	if parallelism == 0 {
		parallelism = 1
	}
	key := argon2.IDKey(pw, salt, p.Iterations, p.MemoryKiB, parallelism, keyLen)
	zero(pw)
	return key
}

func buildAAD(vaultID, slot string) []byte {
	aad := make([]byte, 0, len(domainSeparator)+len(vaultID)+len(slot))
	aad = append(aad, domainSeparator...)
	aad = append(aad, vaultID...)
	aad = append(aad, slot...)
	return aad
}

// seal encrypts plaintext into the on-disk record shape. Go's GCM appends the
// tag to the ciphertext; the format keeps them in separate fields, so they are
// split back apart here.
func seal(key []byte, vaultID, slot string, plaintext []byte) (record, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return record{}, err
	}
	gcm, err := cipher.NewGCMWithTagSize(block, tagLen)
	if err != nil {
		return record{}, err
	}
	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return record{}, err
	}

	out := gcm.Seal(nil, nonce, plaintext, buildAAD(vaultID, slot))
	split := len(out) - tagLen
	return record{
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(out[:split]),
		Tag:        base64.StdEncoding.EncodeToString(out[split:]),
	}, nil
}

// open reverses seal. A false ok means the tag did not authenticate, which is
// either a wrong key or a tampered record; err is non-nil only when the record
// is structurally malformed, which is a different problem for the caller.
func open(key []byte, vaultID, slot string, r record, path string) (plaintext []byte, ok bool, err error) {
	nonce, err := decodeField(r.Nonce, nonceLen, "nonce", path)
	if err != nil {
		return nil, false, err
	}
	tag, err := decodeField(r.Tag, tagLen, "tag", path)
	if err != nil {
		return nil, false, err
	}
	ct, err := base64.StdEncoding.DecodeString(r.Ciphertext)
	if err != nil {
		return nil, false, corrupt(path, "ciphertext is not valid base64", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, false, err
	}
	gcm, err := cipher.NewGCMWithTagSize(block, tagLen)
	if err != nil {
		return nil, false, err
	}

	sealed := make([]byte, 0, len(ct)+len(tag))
	sealed = append(sealed, ct...)
	sealed = append(sealed, tag...)

	out, err := gcm.Open(nil, nonce, sealed, buildAAD(vaultID, slot))
	if err != nil {
		return nil, false, nil
	}
	return out, true, nil
}

// decodeField decodes a fixed-width base64 field, so a truncated nonce or tag
// is reported as corruption rather than surfacing as a wrong password.
func decodeField(value string, want int, field, path string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, corrupt(path, field+" is not valid base64", err)
	}
	if len(b) != want {
		return nil, corrupt(path, field+" has the wrong length", nil)
	}
	return b, nil
}

// zero overwrites a buffer holding key material. Best effort: the garbage
// collector and the pager can both have left copies elsewhere. It costs
// nothing and is the right thing to do.
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// decodeBase64 / encodeBase64 keep the standard-encoding choice in one place;
// the format uses standard base64 with padding throughout.
func decodeBase64(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }

func encodeBase64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
