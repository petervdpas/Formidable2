package bundle

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
)

// Brand marks a file as a Formidable bundle. It is the viewer-coupling: the
// Viewer opens only files carrying this brand and manifest. It is not a secret
// and provides no protection; the password does that.
const Brand = "formidable-bundle"

const formatVersion = 1

// containerMagic prefixes every bundle. It differs from the sealed-payload magic
// so the outer container and the inner ciphertext never collide.
var containerMagic = [8]byte{'F', 'M', 'B', 'P', 'A', 'C', 'K', '1'}

// Manifest is the cleartext descriptor the Viewer reads without a password: what
// the pack is and whether it is locked. It carries no key material. Brand,
// Version and Encrypted are authoritative and set by Pack; the rest is metadata.
type Manifest struct {
	Brand       string `json:"brand"`
	Version     int    `json:"version"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Author      string `json:"author,omitempty"`
	Created     string `json:"created,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Encrypted   bool   `json:"encrypted"`
}

// Pack wraps a payload (a zip, or later a SQLite image) in a branded container
// with a cleartext manifest. A non-empty password seals the payload with
// Seal (Argon2id + AES-256-GCM); an empty password stores it plainly. Pack owns
// Brand, Version and Encrypted, so a caller cannot forge them.
func Pack(meta Manifest, payload []byte, password string) ([]byte, error) {
	m := meta
	m.Brand = Brand
	m.Version = formatVersion
	m.Encrypted = password != ""

	body := payload
	if password != "" {
		sealed, err := Seal(payload, password)
		if err != nil {
			return nil, err
		}
		body = sealed
	}
	return buildContainer(m, body)
}

// ReadManifest returns the cleartext manifest without needing the password, so
// the Viewer can show the pack's identity and prompt only when it is locked.
func ReadManifest(b []byte) (Manifest, error) {
	m, _, err := splitContainer(b)
	return m, err
}

// Unpack returns the payload. Whether to decrypt is decided by sniffing the
// payload itself (IsEncrypted), not by trusting the mutable manifest flag, so a
// tampered manifest cannot make Unpack hand back ciphertext as plaintext.
func Unpack(b []byte, password string) ([]byte, error) {
	_, body, err := splitContainer(b)
	if err != nil {
		return nil, err
	}
	if IsEncrypted(body) {
		return Open(body, password)
	}
	return body, nil
}

func buildContainer(m Manifest, body []byte) ([]byte, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("bundle: marshal manifest: %w", err)
	}
	buf := new(bytes.Buffer)
	buf.Write(containerMagic[:])
	buf.WriteByte(formatVersion)
	var ln [4]byte
	binary.BigEndian.PutUint32(ln[:], uint32(len(raw)))
	buf.Write(ln[:])
	buf.Write(raw)
	buf.Write(body)
	return buf.Bytes(), nil
}

func splitContainer(b []byte) (Manifest, []byte, error) {
	off := 0
	read := func(n int) ([]byte, bool) {
		if n < 0 || off+n > len(b) {
			return nil, false
		}
		s := b[off : off+n]
		off += n
		return s, true
	}

	m, ok := read(len(containerMagic))
	if !ok || !bytes.Equal(m, containerMagic[:]) {
		return Manifest{}, nil, ErrNotBundle
	}
	v, ok := read(1)
	if !ok {
		return Manifest{}, nil, ErrNotBundle
	}
	if v[0] != formatVersion {
		return Manifest{}, nil, ErrUnsupportedVersion
	}
	lnB, ok := read(4)
	if !ok {
		return Manifest{}, nil, ErrNotBundle
	}
	raw, ok := read(int(binary.BigEndian.Uint32(lnB)))
	if !ok {
		return Manifest{}, nil, ErrNotBundle
	}
	var man Manifest
	if err := json.Unmarshal(raw, &man); err != nil {
		return Manifest{}, nil, ErrNotBundle
	}
	return man, b[off:], nil
}
