package auth

import "net/http"

// DesktopFallbackSubject stands in as Identity.Subject when the active
// profile is not (yet) known — e.g. the config has not loaded or the
// user has not set a profile name. Keeps Identity.Valid() true so the
// write isn't rejected; the audit block still gets "Unknown" / the
// unknown email so attribution is visibly unset.
const DesktopFallbackSubject = "<desktop>"

// Resolver maps an incoming HTTP request to a resolved caller Identity.
// Implementations should never panic; failures must surface as errors
// so middleware can pick the right response code.
type Resolver interface {
	Resolve(r *http.Request) (Identity, error)
}

// ProfileProvider returns the active profile's identity fields. Wired
// in app.go to read from the config manager so a mid-session profile
// switch is picked up live (the closure is re-invoked per resolve).
type ProfileProvider func() (profileID, name, email string)

// DesktopResolver attributes every request to the active desktop
// profile. Behaviour-equivalent to today's AuthorProvider closure, just
// promoted into an explicit type so the trust boundary is visible.
type DesktopResolver struct {
	profile ProfileProvider
}

func NewDesktopResolver(p ProfileProvider) *DesktopResolver {
	return &DesktopResolver{profile: p}
}

func (d *DesktopResolver) Resolve(_ *http.Request) (Identity, error) {
	pid, name, email := "", "", ""
	if d.profile != nil {
		pid, name, email = d.profile()
	}
	if pid == "" {
		pid = DesktopFallbackSubject
	}
	if name == "" {
		name = "Unknown"
	}
	if email == "" {
		email = "unknown@example.com"
	}
	return Identity{
		Kind:      KindDesktop,
		Subject:   pid,
		Name:      name,
		Email:     email,
		ProfileID: pid,
	}, nil
}

// SubscriptionStore is the read surface a SubscriptionResolver consults
// when matching incoming bearer tokens. Today no backend implements it;
// the future config-rooted store will live alongside profile state.
type SubscriptionStore interface {
	FindByToken(tokenHash string) (Subscription, error)
}

// SubscriptionResolver is the directive stub for the planned API
// subscription model. Resolve returns ErrNotImplemented unconditionally
// — we deliberately don't even sniff the Authorization header, so the
// "not built yet" posture is unmistakable in traces.
type SubscriptionResolver struct {
	store SubscriptionStore
}

func NewSubscriptionResolver(s SubscriptionStore) *SubscriptionResolver {
	return &SubscriptionResolver{store: s}
}

func (s *SubscriptionResolver) Resolve(_ *http.Request) (Identity, error) {
	return Identity{}, ErrNotImplemented
}
