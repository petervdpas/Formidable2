package auth

import "context"

type identityKey struct{}

// WithIdentity returns ctx augmented with the given Identity. Middleware
// calls this after the configured Resolver runs so storage.SaveForm can
// attribute the write without consulting a global provider.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, identityKey{}, id)
}

// IdentityFromContext returns the Identity previously installed by
// WithIdentity. The second return reports presence so callers can fail
// closed when no identity was resolved. nil ctx is tolerated for
// defensive callers — equivalent to a bare context.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	if ctx == nil {
		return Identity{}, false
	}
	v, ok := ctx.Value(identityKey{}).(Identity)
	if !ok {
		return Identity{}, false
	}
	return v, true
}
