package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

// ─────────────────────────────────────────────────────────────────────
// LoopbackOnly - handler-layer defense-in-depth (regardless of listener)
// ─────────────────────────────────────────────────────────────────────

func TestLoopbackOnly_AllowsIPv4Loopback(t *testing.T) {
	h := LoopbackOnly(okHandler(t))
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/x", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("loopback should pass, got %d (%s)", rr.Code, rr.Body.String())
	}
}

func TestLoopbackOnly_AllowsIPv6Loopback(t *testing.T) {
	h := LoopbackOnly(okHandler(t))
	req := httptest.NewRequest(http.MethodGet, "http://[::1]/api/x", nil)
	req.RemoteAddr = "[::1]:54321"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("ipv6 loopback should pass, got %d", rr.Code)
	}
}

func TestLoopbackOnly_RejectsLAN(t *testing.T) {
	h := LoopbackOnly(okHandler(t))
	for _, addr := range []string{"192.168.1.5:54321", "10.0.0.1:443", "[2001:db8::1]:80"} {
		t.Run(addr, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://x/api/x", nil)
			req.RemoteAddr = addr
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusForbidden {
				t.Errorf("expected 403 for %q, got %d", addr, rr.Code)
			}
		})
	}
}

func TestLoopbackOnly_RejectsMalformedRemoteAddr(t *testing.T) {
	h := LoopbackOnly(okHandler(t))
	req := httptest.NewRequest(http.MethodGet, "http://x/api/x", nil)
	req.RemoteAddr = "garbage"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("malformed addr should be rejected, got %d", rr.Code)
	}
}

func TestLoopbackOnly_RejectsEmptyRemoteAddr_AssetMiddlewareCase(t *testing.T) {
	// Regression: Wails' AssetServer middleware delivers requests with
	// an empty r.RemoteAddr (no real TCP peer - the webview is fetching
	// from itself). LoopbackOnly is a network-only defense and must
	// reject empty addrs, which is precisely why the asset-middleware
	// path uses ResolveIdentity-only, NOT the full network chain.
	// (See app.go: apiHandlerInProcess vs apiHandlerNetwork.)
	h := LoopbackOnly(okHandler(t))
	req := httptest.NewRequest(http.MethodGet, "http://x/api/images/foo/bar.png", nil)
	req.RemoteAddr = ""
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("empty RemoteAddr must be 403 - would otherwise hide network exposure: got %d", rr.Code)
	}
}

func TestResolveIdentityOnly_AcceptsEmptyRemoteAddr(t *testing.T) {
	// Companion to the LoopbackOnly empty-addr test. The in-process
	// chain (ResolveIdentity only, no network defenses) MUST accept the
	// asset-middleware request shape so <img src="/api/images/…"> from
	// the slideout iframe resolves. Without this, the slideout shows
	// broken-image placeholders.
	h := ResolveIdentity(NewDesktopResolver(func() (string, string, string) {
		return "peter", "Peter", "peter@example.com"
	}))(okHandler(t))
	req := httptest.NewRequest(http.MethodGet, "http://x/api/images/foo/bar.png", nil)
	req.RemoteAddr = ""
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("in-process chain must accept empty RemoteAddr, got %d (%s)",
			rr.Code, rr.Body.String())
	}
}

// ─────────────────────────────────────────────────────────────────────
// RequireOrigin - CSRF defense for write methods
// ─────────────────────────────────────────────────────────────────────

func TestRequireOrigin_AllowsSafeMethodsWithoutHeader(t *testing.T) {
	h := RequireOrigin([]string{"http://127.0.0.1:8080"})(okHandler(t))
	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		t.Run(m, func(t *testing.T) {
			req := httptest.NewRequest(m, "http://127.0.0.1/api/x", nil)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Errorf("%s without Origin should pass, got %d", m, rr.Code)
			}
		})
	}
}

func TestRequireOrigin_AllowsAllowlistedOriginOnWrite(t *testing.T) {
	h := RequireOrigin([]string{"http://127.0.0.1:8080"})(okHandler(t))
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/x", nil)
	req.Header.Set("Origin", "http://127.0.0.1:8080")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("allowlisted origin should pass, got %d (%s)", rr.Code, rr.Body.String())
	}
}

func TestRequireOrigin_RejectsForeignOriginOnWrite(t *testing.T) {
	h := RequireOrigin([]string{"http://127.0.0.1:8080"})(okHandler(t))
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/x", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("foreign origin should be 403, got %d", rr.Code)
	}
}

func TestRequireOrigin_RejectsWriteWithoutOriginOrReferer(t *testing.T) {
	// Header-less POST/PUT/PATCH is the classic CSRF posture (a stray
	// form submit). Reject conservatively.
	h := RequireOrigin([]string{"http://127.0.0.1:8080"})(okHandler(t))
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(m, func(t *testing.T) {
			req := httptest.NewRequest(m, "http://127.0.0.1/api/x", nil)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusForbidden {
				t.Errorf("%s without Origin/Referer should 403, got %d", m, rr.Code)
			}
		})
	}
}

func TestRequireOrigin_AllowsRefererFallback(t *testing.T) {
	h := RequireOrigin([]string{"http://127.0.0.1:8080"})(okHandler(t))
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/x", nil)
	req.Header.Set("Referer", "http://127.0.0.1:8080/api/docs/")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("matching Referer should pass, got %d", rr.Code)
	}
}

// ─────────────────────────────────────────────────────────────────────
// ResolveIdentity - runs the Resolver, stuffs Identity into ctx
// ─────────────────────────────────────────────────────────────────────

type fakeResolver struct {
	id  Identity
	err error
}

func (f *fakeResolver) Resolve(_ *http.Request) (Identity, error) { return f.id, f.err }

func TestResolveIdentity_PopulatesContext(t *testing.T) {
	expected := Identity{Kind: KindDesktop, Subject: "peter", Name: "Peter", Email: "peter@x"}
	var seen Identity
	var seenOk bool
	h := ResolveIdentity(&fakeResolver{id: expected})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, seenOk = IdentityFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/x", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !seenOk || seen != expected {
		t.Fatalf("handler saw %+v ok=%v, want %+v", seen, seenOk, expected)
	}
}

func TestResolveIdentity_NotImplementedReturns501(t *testing.T) {
	h := ResolveIdentity(&fakeResolver{err: ErrNotImplemented})(okHandler(t))
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/x", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("ErrNotImplemented should surface as 501, got %d", rr.Code)
	}
}

func TestResolveIdentity_GenericErrorReturns403(t *testing.T) {
	h := ResolveIdentity(&fakeResolver{err: errors.New("bad token")})(okHandler(t))
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/x", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("generic resolve error should 403, got %d", rr.Code)
	}
}

func TestResolveIdentity_InvalidIdentityRejected(t *testing.T) {
	// A resolver returning (Identity{}, nil) - malformed but no error.
	// The middleware must catch this rather than letting an empty
	// Identity propagate to storage.
	h := ResolveIdentity(&fakeResolver{id: Identity{}})(okHandler(t))
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/x", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("invalid Identity should 403, got %d", rr.Code)
	}
}
