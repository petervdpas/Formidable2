package auth

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initAuthScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			Output:   colorWriter(),
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fail()
	}
}

func colorWriter() io.Writer {
	if w, ok := any(os.Stdout).(io.Writer); ok {
		return w
	}
	return io.Discard
}

// authWorld is per-scenario state for the middleware feature. We build
// a tiny stack: LoopbackOnly → RequireOrigin → ResolveIdentity → echo,
// and inspect (a) the wire-level response and (b) the Identity the
// downstream handler observed.
type authWorld struct {
	resolver       Resolver
	allowedOrigin  string
	downstreamSeen Identity
	downstreamHit  bool
	resp           *httptest.ResponseRecorder
}

func (w *authWorld) reset() { *w = authWorld{} }

func (w *authWorld) buildStack() http.Handler {
	echo := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		w.downstreamHit = true
		w.downstreamSeen, _ = IdentityFromContext(r.Context())
		rw.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(rw, "echo")
	})
	// Order matches the production wiring planned in app.go:
	// outer LoopbackOnly → RequireOrigin → ResolveIdentity → handler.
	return LoopbackOnly(
		RequireOrigin([]string{w.allowedOrigin})(
			ResolveIdentity(w.resolver)(echo),
		),
	)
}

func initAuthScenario(ctx *godog.ScenarioContext) {
	w := &authWorld{}

	ctx.Step(`^an auth handler stack mounted on a downstream echo handler$`, func() error {
		w.reset()
		return nil
	})

	ctx.Step(`^the allowed origin is "([^"]*)"$`, func(o string) error {
		w.allowedOrigin = o
		return nil
	})

	ctx.Step(`^a desktop resolver returning profile "([^"]*)" / "([^"]*)" / "([^"]*)"$`,
		func(pid, name, email string) error {
			w.resolver = NewDesktopResolver(func() (string, string, string) {
				return pid, name, email
			})
			return nil
		})

	ctx.Step(`^the subscription resolver is mounted$`, func() error {
		w.resolver = NewSubscriptionResolver(nil)
		return nil
	})

	ctx.Step(`^a resolver returning an invalid identity$`, func() error {
		w.resolver = &constResolver{id: Identity{}}
		return nil
	})

	ctx.Step(`^a request arrives from "([^"]*)" with method "([^"]*)" and origin "([^"]*)"$`,
		func(remoteAddr, method, origin string) error {
			if w.resolver == nil {
				return fmt.Errorf("resolver not configured")
			}
			req := httptest.NewRequest(method, "http://127.0.0.1/api/x", nil)
			req.RemoteAddr = remoteAddr
			if origin != "" {
				req.Header.Set("Origin", origin)
			}
			w.resp = httptest.NewRecorder()
			w.buildStack().ServeHTTP(w.resp, req)
			return nil
		})

	ctx.Step(`^the response status is (\d+)$`, func(want int) error {
		if w.resp.Code != want {
			return fmt.Errorf("status: got %d, want %d (body=%s)", w.resp.Code, want, w.resp.Body.String())
		}
		return nil
	})

	ctx.Step(`^the downstream handler observed identity "([^"]*)"$`, func(subject string) error {
		if !w.downstreamHit {
			return fmt.Errorf("downstream handler was not invoked")
		}
		if w.downstreamSeen.Subject != subject {
			return fmt.Errorf("subject: got %q, want %q", w.downstreamSeen.Subject, subject)
		}
		return nil
	})

	ctx.Step(`^the downstream handler was not invoked$`, func() error {
		if w.downstreamHit {
			return fmt.Errorf("downstream handler should not have run")
		}
		return nil
	})

	ctx.Step(`^the response body contains "([^"]*)"$`, func(needle string) error {
		body := w.resp.Body.String()
		if !strings.Contains(body, needle) {
			return fmt.Errorf("body %q does not contain %q", body, needle)
		}
		return nil
	})
}

// constResolver returns the same Identity every call. Used to verify
// middleware behavior in the face of malformed resolver output.
type constResolver struct {
	id  Identity
	err error
}

func (c *constResolver) Resolve(_ *http.Request) (Identity, error) { return c.id, c.err }
