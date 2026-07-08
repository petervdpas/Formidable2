package viewer

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/bundle"
	"github.com/petervdpas/formidable2/internal/modules/datadb"
)

func dataImage(t *testing.T) []byte {
	t.Helper()
	img, err := datadb.Build([]datadb.Record{
		{Template: "t.yaml", GUID: "g1", Title: "One", Payload: map[string]any{"a": 1}, Text: "one alpha"},
	})
	if err != nil {
		t.Fatalf("datadb.Build: %v", err)
	}
	return img
}

// zipWithData is an export zip carrying a data image alongside the home page.
func zipWithData(t *testing.T) []byte {
	t.Helper()
	return makeZip(t, map[string]string{
		"index.html": "<h1>HOME</h1>",
		dataEntry:    string(dataImage(t)),
	})
}

// keyed issues a GET carrying the API token as X-API-Key.
func keyed(t *testing.T, h http.Handler, path, key string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if key != "" {
		req.Header.Set("X-API-Key", key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

func TestBundleServesAPIWhenEnabled(t *testing.T) {
	b, err := BundleFromBytes(zipWithData(t), "d.bundle")
	if err != nil {
		t.Fatalf("BundleFromBytes: %v", err)
	}
	if !b.HasData() {
		t.Fatal("bundle with a data image should report HasData")
	}
	s := NewServer()
	s.SetBundle(b)
	s.SetAPIEnabled(true)
	s.SetAPIToken("tok")

	if res, body := get(t, s, "/api/templates"); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("without the key = %d %q, want 401", res.StatusCode, body)
	}
	if res := keyed(t, s, "/api/templates", "tok"); res.StatusCode != http.StatusOK {
		t.Fatalf("with the key = %d, want 200", res.StatusCode)
	}
	// HTML is still served for non-/api paths, no key needed.
	if res, body := get(t, s, "/"); res.StatusCode != http.StatusOK || !strings.Contains(body, "HOME") {
		t.Fatalf("GET / = %d %q", res.StatusCode, body)
	}
}

func TestAPITokenGate(t *testing.T) {
	b, _ := BundleFromBytes(zipWithData(t), "d.bundle")
	s := NewServer()
	s.SetBundle(b)
	s.SetAPIEnabled(true)
	s.SetAPIToken("s3cret")

	// Wrong / missing key -> 401 on data endpoints.
	if res := keyed(t, s, "/api/templates", "wrong"); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong key = %d, want 401", res.StatusCode)
	}
	// Query-param and Bearer forms are accepted.
	if res := get2(t, s, "/api/templates?key=s3cret"); res.StatusCode != http.StatusOK {
		t.Fatalf("?key= = %d, want 200", res.StatusCode)
	}
	if res := bearer(t, s, "/api/search?q=alpha", "s3cret"); res.StatusCode != http.StatusOK {
		t.Fatalf("Bearer = %d, want 200", res.StatusCode)
	}
	// Discovery stays open without a key.
	if res := get2(t, s, "/api/openapi.json"); res.StatusCode != http.StatusOK {
		t.Fatalf("openapi.json without key = %d, want 200", res.StatusCode)
	}
	if res := get2(t, s, "/api/docs/"); res.StatusCode != http.StatusOK {
		t.Fatalf("docs without key = %d, want 200", res.StatusCode)
	}
}

func TestAPIEmptyTokenFailsClosed(t *testing.T) {
	b, _ := BundleFromBytes(zipWithData(t), "d.bundle")
	s := NewServer()
	s.SetBundle(b)
	s.SetAPIEnabled(true) // no token set
	if res := get2(t, s, "/api/templates"); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("empty token should fail closed on data, got %d", res.StatusCode)
	}
}

func get2(t *testing.T, h http.Handler, path string) *http.Response {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Result()
}

func bearer(t *testing.T, h http.Handler, path, key string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+key)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

func TestServerAPIGatedOffIs404(t *testing.T) {
	b, err := BundleFromBytes(zipWithData(t), "d.bundle")
	if err != nil {
		t.Fatalf("BundleFromBytes: %v", err)
	}
	s := NewServer()
	s.SetBundle(b) // ServeAPI not enabled
	if res, _ := get(t, s, "/api/templates"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("API off should 404, got %d", res.StatusCode)
	}
}

func TestServerAPINoDataImageIs404(t *testing.T) {
	b, err := BundleFromBytes(makeZip(t, map[string]string{"index.html": "x"}), "d.bundle")
	if err != nil {
		t.Fatalf("BundleFromBytes: %v", err)
	}
	if b.HasData() {
		t.Fatal("bundle without a data image should not report HasData")
	}
	s := NewServer()
	s.SetBundle(b)
	s.SetAPIEnabled(true)
	if res, _ := get(t, s, "/api/templates"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("no data image should 404, got %d", res.StatusCode)
	}
}

func TestServiceAPIStatus(t *testing.T) {
	s := newService(t)
	packed, err := bundle.Pack(bundle.Manifest{Title: "P"}, zipWithData(t), "")
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	if _, err := s.OpenBytes("p.bundle", base64.StdEncoding.EncodeToString(packed), ""); err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}

	// Data is present, but the API is opt-in and off by default (no key shown).
	if st := s.APIStatus(); st.Enabled || !st.Available || len(st.URLs) != 0 || st.Token != "" {
		t.Fatalf("default status = %+v, want disabled but available with no urls/token", st)
	}

	cfg := s.GetConfig()
	cfg.ServeAPI = true
	if _, err := s.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	st := s.APIStatus()
	if !st.Enabled || !st.Available || len(st.URLs) == 0 {
		t.Fatalf("enabled status = %+v, want enabled + available + urls", st)
	}
	if st.Token == "" {
		t.Fatal("enabling the API should mint a token")
	}

	// Regenerate mints a different token.
	prev := st.Token
	st2, err := s.RegenerateAPIToken()
	if err != nil {
		t.Fatalf("RegenerateAPIToken: %v", err)
	}
	if st2.Token == "" || st2.Token == prev {
		t.Fatalf("regenerate should change the token: %q -> %q", prev, st2.Token)
	}
}
