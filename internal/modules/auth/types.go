// Package auth scaffolds the request-identity + capability surface that
// future versions of Formidable will rely on for distinguishing the
// trusted in-process desktop profile from external HTTP callers.
//
// Today the desktop app is the only consumer: the wiki/api HTTP server
// binds to 127.0.0.1 and inherits the active profile's identity for
// every write. This package's job is to make that boundary explicit in
// code rather than convention, so the upcoming subscription model (per-
// profile API capability grants) and CLI server mode have a concrete
// seam to slot into. The Subscription, SubscriptionResolver, and Server
// mode pieces are intentionally minimal - they exist to document the
// direction, not to ship features.
package auth

import (
	"errors"
	"slices"
	"strings"
)

// IdentityKind classifies who is making a request. The zero value ("")
// means "no identity resolved" - handlers should reject writes from a
// zero-kind identity once the middleware is wired.
type IdentityKind string

const (
	KindDesktop      IdentityKind = "desktop"
	KindSubscription IdentityKind = "subscription"
	KindSystem       IdentityKind = "system"
)

// Identity is the resolved caller of a request. Carried through ctx so
// storage.SaveForm can attribute writes without a global provider.
//
// Subject is the stable id (profile name for desktop, subscription id
// for subscription, fixed sentinel for system). Name/Email feed the
// audit block; they default from the owning profile but may differ for
// system actors (e.g. migration jobs).
type Identity struct {
	Kind      IdentityKind
	Subject   string
	Name      string
	Email     string
	ProfileID string
}

func (i Identity) IsZero() bool { return i == Identity{} }

// Valid returns true when Kind is recognised and Subject is non-empty.
// The middleware uses this to reject malformed Identities before they
// reach storage.
func (i Identity) Valid() bool {
	switch i.Kind {
	case KindDesktop, KindSubscription, KindSystem:
		return i.Subject != ""
	}
	return false
}

// Mode is the deployment posture. Desktop is the only mode that ships
// today; Server is the directive constant for the planned CLI daemon.
// app.go pins this to Desktop and gates server-only paths off it.
type Mode int

const (
	ModeDesktop Mode = iota
	ModeServer
)

func (m Mode) String() string {
	switch m {
	case ModeDesktop:
		return "desktop"
	case ModeServer:
		return "server"
	}
	return "unknown"
}

// Subscription is the minimal future capability grant: a profile-bound
// allowlist of templates × methods, authenticated by a bearer-token
// hash. Intentionally light - no rate limits, scoping rules, or
// revocation flow yet; those land with the CLI daemon.
type Subscription struct {
	ID               string
	ProfileID        string
	AllowedTemplates []string
	AllowedMethods   []string
	TokenHash        string
}

// Allows returns true when the subscription's allowlists cover the
// given template stem and HTTP method. Method comparison is case-
// insensitive so callers don't have to normalise.
func (s Subscription) Allows(template, method string) bool {
	if !slices.Contains(s.AllowedTemplates, template) {
		return false
	}
	upper := strings.ToUpper(method)
	for _, m := range s.AllowedMethods {
		if strings.ToUpper(m) == upper {
			return true
		}
	}
	return false
}

var (
	// ErrNotImplemented marks future-path stubs (SubscriptionResolver,
	// server-mode features). Callers should surface 501 to the wire.
	ErrNotImplemented = errors.New("auth: not implemented")

	// ErrNoIdentity is returned by IdentityFromContext when no identity
	// has been resolved. Middleware ordering should make this
	// unreachable in practice - kept so handlers can fail closed.
	ErrNoIdentity = errors.New("auth: no identity in context")
)
