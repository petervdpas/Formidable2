package auth

import (
	"errors"
	"testing"
)

func TestIdentityKind_String(t *testing.T) {
	cases := []struct {
		k    IdentityKind
		want string
	}{
		{KindDesktop, "desktop"},
		{KindSubscription, "subscription"},
		{KindSystem, "system"},
		{"", ""},
	}
	for _, c := range cases {
		if got := string(c.k); got != c.want {
			t.Errorf("string(%v) = %q, want %q", c.k, got, c.want)
		}
	}
}

func TestIdentity_IsZero(t *testing.T) {
	if !(Identity{}).IsZero() {
		t.Fatal("zero-value Identity must be IsZero()")
	}
	id := Identity{Kind: KindDesktop, Subject: "peter"}
	if id.IsZero() {
		t.Fatal("populated Identity must NOT be IsZero()")
	}
}

func TestIdentity_Valid(t *testing.T) {
	t.Run("zero is invalid", func(t *testing.T) {
		if (Identity{}).Valid() {
			t.Fatal("zero identity should be invalid")
		}
	})
	t.Run("desktop requires subject", func(t *testing.T) {
		if (Identity{Kind: KindDesktop}).Valid() {
			t.Fatal("desktop without subject should be invalid")
		}
		if !(Identity{Kind: KindDesktop, Subject: "peter"}).Valid() {
			t.Fatal("desktop with subject should be valid")
		}
	})
	t.Run("subscription requires subject", func(t *testing.T) {
		if (Identity{Kind: KindSubscription}).Valid() {
			t.Fatal("subscription without subject should be invalid")
		}
		if !(Identity{Kind: KindSubscription, Subject: "sub-123"}).Valid() {
			t.Fatal("subscription with subject should be valid")
		}
	})
	t.Run("unknown kind is invalid", func(t *testing.T) {
		if (Identity{Kind: "alien", Subject: "x"}).Valid() {
			t.Fatal("unknown kind should be invalid")
		}
	})
}

func TestMode_String(t *testing.T) {
	if ModeDesktop.String() != "desktop" {
		t.Errorf("ModeDesktop.String() = %q, want %q", ModeDesktop.String(), "desktop")
	}
	if ModeServer.String() != "server" {
		t.Errorf("ModeServer.String() = %q, want %q", ModeServer.String(), "server")
	}
	if Mode(99).String() != "unknown" {
		t.Errorf("Mode(99).String() = %q, want %q", Mode(99).String(), "unknown")
	}
}

func TestSubscription_Allows(t *testing.T) {
	s := Subscription{
		ID:               "sub-1",
		ProfileID:        "peter",
		AllowedTemplates: []string{"bread", "recipe"},
		AllowedMethods:   []string{"GET", "POST"},
	}

	t.Run("allowed template + method", func(t *testing.T) {
		if !s.Allows("bread", "GET") {
			t.Fatal("expected allow")
		}
		if !s.Allows("recipe", "POST") {
			t.Fatal("expected allow")
		}
	})
	t.Run("disallowed template", func(t *testing.T) {
		if s.Allows("secret", "GET") {
			t.Fatal("expected deny")
		}
	})
	t.Run("disallowed method", func(t *testing.T) {
		if s.Allows("bread", "DELETE") {
			t.Fatal("expected deny")
		}
	})
	t.Run("method match is case-insensitive", func(t *testing.T) {
		if !s.Allows("bread", "get") {
			t.Fatal("lowercase method should match")
		}
	})
	t.Run("empty allowlists deny everything", func(t *testing.T) {
		var empty Subscription
		if empty.Allows("bread", "GET") {
			t.Fatal("empty subscription should deny all")
		}
	})
}

func TestSentinelErrors(t *testing.T) {
	if !errors.Is(ErrNotImplemented, ErrNotImplemented) {
		t.Fatal("ErrNotImplemented must be its own sentinel")
	}
	if !errors.Is(ErrNoIdentity, ErrNoIdentity) {
		t.Fatal("ErrNoIdentity must be its own sentinel")
	}
	if errors.Is(ErrNoIdentity, ErrNotImplemented) {
		t.Fatal("ErrNoIdentity must not match ErrNotImplemented")
	}
}
