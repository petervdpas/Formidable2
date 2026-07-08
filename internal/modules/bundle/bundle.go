// Package bundle seals and opens Formidable information packs. A sealed pack is
// a self-describing container: magic + version + KDF parameters + salt + nonce +
// AEAD ciphertext. The payload (a zip, or later a SQLite image) is encrypted as
// one authenticated blob, so the Viewer can decrypt it in memory and never write
// plaintext to disk. Without the password the content is unreadable; the Viewer
// being the reader is UX, not DRM.
package bundle

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
)

// magic identifies a Formidable bundle container (8 bytes).
var magic = [8]byte{'F', 'M', 'B', 'U', 'N', 'D', 'L', 'E'}

const (
	version1    = 1
	kdfArgon2id = 1
	aeadAESGCM  = 1

	keyLen = 32 // AES-256

	// saltFieldOffset is where the salt bytes begin in the header:
	// magic(8) + version(1) + kdf(1) + aead(1) + time(4) + memory(4) + threads(1) + saltLen(1).
	saltFieldOffset = 8 + 1 + 1 + 1 + 4 + 4 + 1 + 1
)

var (
	ErrNotBundle          = errors.New("bundle: not a Formidable bundle")
	ErrUnsupportedVersion = errors.New("bundle: unsupported bundle version")
	ErrEmptyPassword      = errors.New("bundle: password must not be empty")
	ErrDecrypt            = errors.New("bundle: wrong password or corrupted bundle")
)

// params holds the Argon2id cost parameters. They are written into every
// container so Open is self-describing and future tuning stays backward
// compatible.
type params struct {
	time    uint32
	memory  uint32 // KiB
	threads uint8
}

var defaultParams = params{time: 1, memory: 64 * 1024, threads: 4}

// Seal encrypts plaintext with a password-derived key and returns a
// self-describing container. The password must not be empty.
func Seal(plaintext []byte, password string) ([]byte, error) {
	return seal(plaintext, password, defaultParams)
}

func seal(plaintext []byte, password string, p params) ([]byte, error) {
	if password == "" {
		return nil, ErrEmptyPassword
	}
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("bundle: read salt: %w", err)
	}
	gcm, err := newGCM(password, salt, p)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("bundle: read nonce: %w", err)
	}
	header := buildHeader(p, salt, nonce)
	// The header is authenticated as additional data, so any tampering with the
	// declared parameters fails the open even before the key mismatch would.
	ciphertext := gcm.Seal(nil, nonce, plaintext, header)
	out := make([]byte, 0, len(header)+len(ciphertext))
	out = append(out, header...)
	out = append(out, ciphertext...)
	return out, nil
}

// Open decrypts a container produced by Seal. A wrong password or any tampering
// returns ErrDecrypt; malformed input returns ErrNotBundle or
// ErrUnsupportedVersion. The password must not be empty.
func Open(sealed []byte, password string) ([]byte, error) {
	if password == "" {
		return nil, ErrEmptyPassword
	}
	h, ciphertext, err := parse(sealed)
	if err != nil {
		return nil, err
	}
	gcm, err := newGCM(password, h.salt, h.p)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, h.nonce, ciphertext, h.header)
	if err != nil {
		return nil, ErrDecrypt
	}
	return plaintext, nil
}

// IsEncrypted reports whether b carries the Formidable bundle magic. It is a
// cheap sniff for the Viewer to tell a locked pack from a plain payload; it does
// not validate the rest of the container.
func IsEncrypted(b []byte) bool {
	return len(b) >= len(magic) && bytes.Equal(b[:len(magic)], magic[:])
}

func newGCM(password string, salt []byte, p params) (cipher.AEAD, error) {
	key := argon2.IDKey([]byte(password), salt, p.time, p.memory, p.threads, keyLen)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("bundle: cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("bundle: gcm: %w", err)
	}
	return gcm, nil
}

func buildHeader(p params, salt, nonce []byte) []byte {
	buf := new(bytes.Buffer)
	buf.Write(magic[:])
	buf.WriteByte(version1)
	buf.WriteByte(kdfArgon2id)
	buf.WriteByte(aeadAESGCM)
	_ = binary.Write(buf, binary.BigEndian, p.time)
	_ = binary.Write(buf, binary.BigEndian, p.memory)
	buf.WriteByte(p.threads)
	buf.WriteByte(byte(len(salt)))
	buf.Write(salt)
	buf.WriteByte(byte(len(nonce)))
	buf.Write(nonce)
	return buf.Bytes()
}

type parsed struct {
	header []byte
	salt   []byte
	nonce  []byte
	p      params
}

// parse validates the container header and returns it alongside the trailing
// ciphertext. Any short read yields ErrNotBundle rather than a panic.
func parse(b []byte) (parsed, []byte, error) {
	off := 0
	read := func(n int) ([]byte, bool) {
		if n < 0 || off+n > len(b) {
			return nil, false
		}
		s := b[off : off+n]
		off += n
		return s, true
	}

	m, ok := read(len(magic))
	if !ok || !bytes.Equal(m, magic[:]) {
		return parsed{}, nil, ErrNotBundle
	}
	ids, ok := read(3) // version, kdf, aead
	if !ok {
		return parsed{}, nil, ErrNotBundle
	}
	if ids[0] != version1 || ids[1] != kdfArgon2id || ids[2] != aeadAESGCM {
		return parsed{}, nil, ErrUnsupportedVersion
	}
	timeB, ok := read(4)
	if !ok {
		return parsed{}, nil, ErrNotBundle
	}
	memB, ok := read(4)
	if !ok {
		return parsed{}, nil, ErrNotBundle
	}
	threadsB, ok := read(1)
	if !ok {
		return parsed{}, nil, ErrNotBundle
	}
	saltLenB, ok := read(1)
	if !ok {
		return parsed{}, nil, ErrNotBundle
	}
	salt, ok := read(int(saltLenB[0]))
	if !ok {
		return parsed{}, nil, ErrNotBundle
	}
	nonceLenB, ok := read(1)
	if !ok {
		return parsed{}, nil, ErrNotBundle
	}
	nonce, ok := read(int(nonceLenB[0]))
	if !ok {
		return parsed{}, nil, ErrNotBundle
	}

	return parsed{
		header: b[:off],
		salt:   salt,
		nonce:  nonce,
		p: params{
			time:    binary.BigEndian.Uint32(timeB),
			memory:  binary.BigEndian.Uint32(memB),
			threads: threadsB[0],
		},
	}, b[off:], nil
}
