package auth

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// LoopbackOnly returns a middleware that 403s any request whose
// RemoteAddr isn't an IPv4 or IPv6 loopback address. Defense-in-depth:
// the wiki listener already binds to 127.0.0.1, but if that bind ever
// drifts (config, reverse proxy, tunnel) the handler layer keeps the
// API closed by default.
func LoopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			// Bracket-stripped IPv6 also sometimes lands here without a port.
			host = r.RemoteAddr
		}
		// Trim IPv6 brackets if present.
		host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			writeForbidden(w, "non-loopback")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireOrigin returns a middleware that gates write methods on the
// Origin (or, fallback, Referer) header matching one of the configured
// allowlist URLs. GET / HEAD / OPTIONS pass through untouched — CSRF
// only threatens state-changing methods, and forcing an Origin on safe
// reads breaks tooling that doesn't set one (curl, scripts).
//
// allowedOrigins should be the scheme+host[+port] strings that the
// wiki/API server itself serves under (e.g. "http://127.0.0.1:8080").
// Empty allowlist denies every write — fail-closed by default.
func RequireOrigin(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[normaliseOrigin(o)] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isSafeMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			origin := r.Header.Get("Origin")
			if origin == "" {
				// Referer fallback: many old HTTP clients omit Origin
				// but always send Referer.
				if ref := r.Header.Get("Referer"); ref != "" {
					if u, err := url.Parse(ref); err == nil {
						origin = u.Scheme + "://" + u.Host
					}
				}
			}
			if origin == "" {
				writeForbidden(w, "missing-origin")
				return
			}
			if _, ok := allowed[normaliseOrigin(origin)]; !ok {
				writeForbidden(w, "cross-origin")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ResolveIdentity runs the configured Resolver and stuffs the returned
// Identity onto the request context, so downstream handlers (and
// storage.SaveForm via ctx) attribute writes correctly.
//
// Failure modes:
//
//	ErrNotImplemented → 501 (subscription path not built yet)
//	other error       → 403 (resolver rejected the caller)
//	!Identity.Valid() → 403 (resolver buggy or token unmapped)
//
// Fail-closed so a misconfigured resolver can't quietly elevate an
// unauthenticated request.
func ResolveIdentity(r Resolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			id, err := r.Resolve(req)
			if err != nil {
				if errors.Is(err, ErrNotImplemented) {
					writeJSONError(w, http.StatusNotImplemented, "not-implemented")
					return
				}
				writeForbidden(w, "unresolved-identity")
				return
			}
			if !id.Valid() {
				writeForbidden(w, "invalid-identity")
				return
			}
			next.ServeHTTP(w, req.WithContext(WithIdentity(req.Context(), id)))
		})
	}
}

func isSafeMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

func normaliseOrigin(s string) string {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Host == "" {
		return strings.TrimSpace(s)
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

func writeForbidden(w http.ResponseWriter, code string) {
	writeJSONError(w, http.StatusForbidden, code)
}

func writeJSONError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
