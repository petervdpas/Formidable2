package bundle

import (
	"bytes"
	"errors"
	"testing"
)

func sampleMeta() Manifest {
	return Manifest{
		Title:       "Audit Controls",
		Description: "How the SURF audit controls hang together.",
		Author:      "Peter",
		Created:     "2026-07-08",
		Kind:        "pack",
	}
}

func TestPackUnpackEncryptedRoundTrip(t *testing.T) {
	payload := []byte("PK\x03\x04 pretend this is a zip of html and a db")
	packed, err := Pack(sampleMeta(), payload, pw)
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	if bytes.Contains(packed, payload) {
		t.Fatal("encrypted container leaks the payload")
	}

	m, err := ReadManifest(packed)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !m.Encrypted {
		t.Fatal("manifest should report encrypted")
	}
	if m.Title != "Audit Controls" || m.Author != "Peter" {
		t.Fatalf("metadata lost: %+v", m)
	}
	if m.Description != "How the SURF audit controls hang together." {
		t.Fatalf("description lost: %+v", m)
	}
	if m.Brand != Brand {
		t.Fatalf("brand = %q", m.Brand)
	}
	if m.Version != formatVersion {
		t.Fatalf("version = %d", m.Version)
	}

	got, err := Unpack(packed, pw)
	if err != nil {
		t.Fatalf("unpack: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("encrypted round trip mismatch")
	}
}

func TestPackUnpackPlainRoundTrip(t *testing.T) {
	payload := []byte("PK\x03\x04 plain, no password")
	packed, err := Pack(sampleMeta(), payload, "")
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	m, err := ReadManifest(packed)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if m.Encrypted {
		t.Fatal("plain bundle should not report encrypted")
	}
	got, err := Unpack(packed, "")
	if err != nil {
		t.Fatalf("unpack: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("plain round trip mismatch")
	}
}

func TestReadManifestNeedsNoPassword(t *testing.T) {
	packed, err := Pack(sampleMeta(), []byte("secret content"), pw)
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	m, err := ReadManifest(packed) // no password given
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if m.Title != "Audit Controls" || !m.Encrypted {
		t.Fatalf("manifest not readable without password: %+v", m)
	}
}

func TestPackOverridesForgedManifestFields(t *testing.T) {
	forged := sampleMeta()
	forged.Brand = "evil"
	forged.Encrypted = false // lie: claim plain
	forged.Version = 999
	packed, err := Pack(forged, []byte("x"), pw)
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	m, _ := ReadManifest(packed)
	if m.Brand != Brand {
		t.Fatalf("brand not enforced: %q", m.Brand)
	}
	if m.Version != formatVersion {
		t.Fatalf("version not enforced: %d", m.Version)
	}
	if !m.Encrypted {
		t.Fatal("encrypted flag not enforced from the password")
	}
}

func TestUnpackEncryptedWrongPassword(t *testing.T) {
	packed, _ := Pack(sampleMeta(), []byte("secret"), pw)
	if _, err := Unpack(packed, "nope"); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("want ErrDecrypt, got %v", err)
	}
}

func TestUnpackEncryptedEmptyPassword(t *testing.T) {
	packed, _ := Pack(sampleMeta(), []byte("secret"), pw)
	if _, err := Unpack(packed, ""); !errors.Is(err, ErrEmptyPassword) {
		t.Fatalf("want ErrEmptyPassword, got %v", err)
	}
}

func TestUnpackNotABundle(t *testing.T) {
	if _, err := Unpack([]byte("PK\x03\x04 raw zip, no container"), pw); !errors.Is(err, ErrNotBundle) {
		t.Fatalf("want ErrNotBundle, got %v", err)
	}
}

func TestReadManifestNotABundle(t *testing.T) {
	if _, err := ReadManifest([]byte("garbage")); !errors.Is(err, ErrNotBundle) {
		t.Fatalf("want ErrNotBundle, got %v", err)
	}
}

func TestReadManifestTruncated(t *testing.T) {
	packed, _ := Pack(sampleMeta(), []byte("secret"), pw)
	if _, err := ReadManifest(packed[:10]); !errors.Is(err, ErrNotBundle) {
		t.Fatalf("want ErrNotBundle, got %v", err)
	}
}

// Correctness must not depend on the cleartext manifest flag: Unpack decides
// whether to decrypt by sniffing the payload, not by trusting a mutable field.
func TestUnpackIgnoresLyingManifestFlag(t *testing.T) {
	sealed, err := Seal([]byte("real secret"), pw)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	lying := Manifest{Brand: Brand, Version: formatVersion, Encrypted: false} // claims plain
	packed, err := buildContainer(lying, sealed)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	got, err := Unpack(packed, pw)
	if err != nil {
		t.Fatalf("unpack: %v", err)
	}
	if !bytes.Equal(got, []byte("real secret")) {
		t.Fatal("did not decrypt despite a lying manifest flag")
	}
}
