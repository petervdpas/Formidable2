package auth

import (
	"errors"
	"net/http"
	"testing"
)

func TestDesktopResolver_WithFullProvider(t *testing.T) {
	r := NewDesktopResolver(func() (string, string, string) {
		return "peter", "Peter van de Pas", "peter@example.com"
	})
	id, err := r.Resolve(httptestRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.Kind != KindDesktop {
		t.Errorf("Kind = %q, want %q", id.Kind, KindDesktop)
	}
	if id.Subject != "peter" {
		t.Errorf("Subject = %q, want %q", id.Subject, "peter")
	}
	if id.ProfileID != "peter" {
		t.Errorf("ProfileID = %q, want %q", id.ProfileID, "peter")
	}
	if id.Name != "Peter van de Pas" {
		t.Errorf("Name = %q", id.Name)
	}
	if id.Email != "peter@example.com" {
		t.Errorf("Email = %q", id.Email)
	}
	if !id.Valid() {
		t.Fatal("result must be Valid()")
	}
}

func TestDesktopResolver_EmptyProvider(t *testing.T) {
	r := NewDesktopResolver(func() (string, string, string) {
		return "", "", ""
	})
	id, err := r.Resolve(httptestRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.Subject != DesktopFallbackSubject {
		t.Errorf("Subject = %q, want fallback %q", id.Subject, DesktopFallbackSubject)
	}
	if id.Name != "Unknown" {
		t.Errorf("Name = %q, want %q", id.Name, "Unknown")
	}
	if id.Email != "unknown@example.com" {
		t.Errorf("Email = %q, want %q", id.Email, "unknown@example.com")
	}
	if !id.Valid() {
		t.Fatal("fallback Identity must still be Valid()")
	}
}

func TestDesktopResolver_NilProvider(t *testing.T) {
	r := NewDesktopResolver(nil)
	id, err := r.Resolve(httptestRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.Subject != DesktopFallbackSubject || id.Name != "Unknown" {
		t.Errorf("nil-provider fallback shape wrong: %+v", id)
	}
	if !id.Valid() {
		t.Fatal("nil-provider Identity must still be Valid()")
	}
}

func TestSubscriptionResolver_ReturnsNotImplemented(t *testing.T) {
	r := NewSubscriptionResolver(nil)
	id, err := r.Resolve(httptestRequest())
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("err = %v, want ErrNotImplemented", err)
	}
	if !id.IsZero() {
		t.Errorf("expected zero Identity, got %+v", id)
	}
}

func httptestRequest() *http.Request {
	r, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:0/", nil)
	return r
}
