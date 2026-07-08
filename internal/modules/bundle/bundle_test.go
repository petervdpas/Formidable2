package bundle

import (
	"bytes"
	"errors"
	"testing"
)

const pw = "correct horse battery staple"

func TestSealOpenRoundTrip(t *testing.T) {
	plain := []byte("the quick brown fox jumps over the lazy dog")
	sealed, err := Seal(plain, pw)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if bytes.Contains(sealed, plain) {
		t.Fatal("sealed bytes contain the plaintext")
	}
	got, err := Open(sealed, pw)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("round trip mismatch: got %q", got)
	}
}

func TestSealEmptyPlaintextRoundTrips(t *testing.T) {
	sealed, err := Seal(nil, pw)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	got, err := Open(sealed, pw)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty payload, got %d bytes", len(got))
	}
}

func TestSealOpenLargeRoundTrip(t *testing.T) {
	plain := bytes.Repeat([]byte("formidable-"), 100_000) // ~1.1 MB
	sealed, err := Seal(plain, pw)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	got, err := Open(sealed, pw)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatal("large round trip mismatch")
	}
}

func TestOpenWrongPassword(t *testing.T) {
	sealed, err := Seal([]byte("secret"), pw)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, err := Open(sealed, "not the password"); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("want ErrDecrypt, got %v", err)
	}
}

func TestOpenTamperedCiphertext(t *testing.T) {
	sealed, err := Seal([]byte("secret payload that is long enough"), pw)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	sealed[len(sealed)-1] ^= 0xFF
	if _, err := Open(sealed, pw); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("want ErrDecrypt, got %v", err)
	}
}

func TestOpenTamperedSalt(t *testing.T) {
	sealed, err := Seal([]byte("secret"), pw)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	sealed[saltFieldOffset] ^= 0xFF // flip a byte inside the salt: derives a different key
	if _, err := Open(sealed, pw); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("want ErrDecrypt, got %v", err)
	}
}

func TestOpenTamperedVersion(t *testing.T) {
	sealed, err := Seal([]byte("secret"), pw)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	sealed[len(magic)] ^= 0xFF
	if _, err := Open(sealed, pw); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("want ErrUnsupportedVersion, got %v", err)
	}
}

func TestOpenNotABundle(t *testing.T) {
	if _, err := Open([]byte("PK\x03\x04 an ordinary zip"), pw); !errors.Is(err, ErrNotBundle) {
		t.Fatalf("want ErrNotBundle, got %v", err)
	}
}

func TestOpenEmptyInput(t *testing.T) {
	if _, err := Open(nil, pw); !errors.Is(err, ErrNotBundle) {
		t.Fatalf("want ErrNotBundle, got %v", err)
	}
}

func TestOpenTruncatedHeader(t *testing.T) {
	sealed, err := Seal([]byte("secret"), pw)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, err := Open(sealed[:15], pw); !errors.Is(err, ErrNotBundle) {
		t.Fatalf("want ErrNotBundle, got %v", err)
	}
}

func TestSealEmptyPasswordRejected(t *testing.T) {
	if _, err := Seal([]byte("x"), ""); !errors.Is(err, ErrEmptyPassword) {
		t.Fatalf("want ErrEmptyPassword, got %v", err)
	}
}

func TestOpenEmptyPasswordRejected(t *testing.T) {
	sealed, err := Seal([]byte("x"), pw)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, err := Open(sealed, ""); !errors.Is(err, ErrEmptyPassword) {
		t.Fatalf("want ErrEmptyPassword, got %v", err)
	}
}

func TestSaltAndNonceAreRandom(t *testing.T) {
	a, err := Seal([]byte("x"), pw)
	if err != nil {
		t.Fatalf("seal a: %v", err)
	}
	b, err := Seal([]byte("x"), pw)
	if err != nil {
		t.Fatalf("seal b: %v", err)
	}
	if bytes.Equal(a, b) {
		t.Fatal("two seals of the same input are byte-identical")
	}
	pa, _, err := parse(a)
	if err != nil {
		t.Fatalf("parse a: %v", err)
	}
	pb, _, err := parse(b)
	if err != nil {
		t.Fatalf("parse b: %v", err)
	}
	if bytes.Equal(pa.salt, pb.salt) {
		t.Fatal("salt reused across seals")
	}
	if bytes.Equal(pa.nonce, pb.nonce) {
		t.Fatal("nonce reused across seals")
	}
}

func TestIsEncrypted(t *testing.T) {
	sealed, err := Seal([]byte("x"), pw)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if !IsEncrypted(sealed) {
		t.Fatal("sealed bundle not recognized as encrypted")
	}
	if IsEncrypted([]byte("PK\x03\x04")) {
		t.Fatal("plain zip flagged as encrypted")
	}
	if IsEncrypted(nil) {
		t.Fatal("nil flagged as encrypted")
	}
}
