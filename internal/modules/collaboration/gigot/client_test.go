package gigot

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── buildURL ────────────────────────────────────────────────────────

func TestBuildURL_MissingBaseURLErrors(t *testing.T) {
	if _, err := buildURL("", "/api/health", nil); !errors.Is(err, ErrMissingBaseURL) {
		t.Fatalf("want ErrMissingBaseURL, got %v", err)
	}
}

func TestBuildURL_RelPathMustStartWithSlash(t *testing.T) {
	if _, err := buildURL("https://x", "api/health", nil); err == nil {
		t.Fatal("path without leading / should error")
	}
}

func TestBuildURL_HappyPathNoQuery(t *testing.T) {
	got, err := buildURL("https://gigot.example", "/api/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://gigot.example/api/health" {
		t.Fatalf("got %q", got)
	}
}

func TestBuildURL_TrimsTrailingSlashOnBase(t *testing.T) {
	got, err := buildURL("https://gigot.example/", "/api/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://gigot.example/api/health" {
		t.Fatalf("got %q — base trailing slash not trimmed", got)
	}
}

func TestBuildURL_QueryEncoded(t *testing.T) {
	got, err := buildURL("https://x", "/api/repos/r/log", map[string]string{"limit": "20"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "?limit=20") {
		t.Fatalf("expected ?limit=20 suffix, got %q", got)
	}
}

func TestBuildURL_EmptyQueryValueDropped(t *testing.T) {
	got, err := buildURL("https://x", "/api/x", map[string]string{"limit": ""})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "?") {
		t.Fatalf("empty query value should not produce ?: %q", got)
	}
}

func TestBuildURL_QueryValuesEscaped(t *testing.T) {
	got, err := buildURL("https://x", "/api/x", map[string]string{"q": "hello world"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "q=hello+world") && !strings.Contains(got, "q=hello%20world") {
		t.Fatalf("query value should be URL-escaped, got %q", got)
	}
}

// ── encodeSegment / encodeSegments ──────────────────────────────────

func TestEncodeSegment_EscapesSpacesAndSlashes(t *testing.T) {
	if got := encodeSegment("hello world"); got != "hello%20world" {
		t.Fatalf("got %q", got)
	}
	// PathEscape escapes slash inside a single segment so it can't
	// accidentally split into two segments on the server side.
	if got := encodeSegment("a/b"); !strings.Contains(got, "%2F") {
		t.Fatalf("slash inside segment should be %%2F-encoded, got %q", got)
	}
}

func TestEncodeSegments_PreservesPathSlashes(t *testing.T) {
	got := encodeSegments("templates/basic.yaml")
	if got != "templates/basic.yaml" {
		t.Fatalf("plain path round-trip lost: %q", got)
	}
}

func TestEncodeSegments_EscapesSegmentSpaces(t *testing.T) {
	got := encodeSegments("storage/my notes/x.meta.json")
	if !strings.Contains(got, "my%20notes") {
		t.Fatalf("space in middle segment not escaped: %q", got)
	}
	if !strings.Contains(got, "/x.meta.json") {
		t.Fatalf("trailing segment lost: %q", got)
	}
}

// ── do — transport mechanics via httptest.Server ────────────────────

// newTestManager wires a Manager whose http.Client points at the given
// test server. Avoids the default 30s timeout in unit tests.
func newTestManager(srv *httptest.Server) *Manager {
	return NewManager(newFakeFS(), WithHTTPClient(srv.Client()))
}

// connFor builds a Connection that talks to a test server with a token
// + repo. Both fields populated so the route methods that require them
// pass validation.
func connFor(srv *httptest.Server) Connection {
	return Connection{
		BaseURL:  srv.URL,
		Token:    "test-bearer",
		RepoName: "addresses",
	}
}

func TestDo_SendsBearerHeader(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	m := newTestManager(srv)
	if err := m.do(http.MethodGet, connFor(srv), "/api/health", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if seen != "Bearer test-bearer" {
		t.Fatalf("Authorization header = %q", seen)
	}
}

func TestDo_OmitsBearerWhenTokenBlank(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	m := newTestManager(srv)
	conn := Connection{BaseURL: srv.URL}
	if err := m.do(http.MethodGet, conn, "/api/health", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if seen != "" {
		t.Fatalf("Authorization header should be absent on blank token, got %q", seen)
	}
}

func TestDo_DecodesJSONBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true,"version":"1.2.3"}`)
	}))
	defer srv.Close()

	m := newTestManager(srv)
	var got HealthResponse
	if err := m.do(http.MethodGet, connFor(srv), "/api/health", nil, nil, &got); err != nil {
		t.Fatal(err)
	}
	if !got.OK || got.Version != "1.2.3" {
		t.Fatalf("decoded = %+v", got)
	}
}

func TestDo_EmptyResponseBodyOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := newTestManager(srv)
	var got HealthResponse
	if err := m.do(http.MethodGet, connFor(srv), "/api/health", nil, nil, &got); err != nil {
		t.Fatalf("empty 2xx body should not error: %v", err)
	}
}

func TestDo_DiscardsBodyWhenOutNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"ignored":true}`)
	}))
	defer srv.Close()

	m := newTestManager(srv)
	if err := m.do(http.MethodGet, connFor(srv), "/api/health", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestDo_EncodesJSONRequestBody(t *testing.T) {
	var (
		seenCT   string
		seenBody []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenCT = r.Header.Get("Content-Type")
		seenBody, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	m := newTestManager(srv)
	req := CommitRequest{ParentVersion: "abc", Message: "hi", Changes: []Change{{Op: "put", Path: "p"}}}
	if err := m.do(http.MethodPost, connFor(srv), "/api/repos/r/commits", nil, req, nil); err != nil {
		t.Fatal(err)
	}
	if seenCT != "application/json" {
		t.Errorf("Content-Type = %q", seenCT)
	}
	var back CommitRequest
	if err := json.Unmarshal(seenBody, &back); err != nil {
		t.Fatalf("server-received body not valid JSON: %v (%s)", err, seenBody)
	}
	if back.ParentVersion != "abc" || back.Message != "hi" || len(back.Changes) != 1 {
		t.Fatalf("body round-trip = %+v", back)
	}
}

func TestDo_PassesQueryString(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.RawQuery
		_, _ = io.WriteString(w, `[]`)
	}))
	defer srv.Close()

	m := newTestManager(srv)
	var out []LogEntry
	if err := m.do(http.MethodGet, connFor(srv), "/api/repos/r/log",
		map[string]string{"limit": "42"}, nil, &out); err != nil {
		t.Fatal(err)
	}
	if seen != "limit=42" {
		t.Fatalf("query = %q", seen)
	}
}

func TestDo_NonSuccessReturnsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()

	m := newTestManager(srv)
	err := m.do(http.MethodGet, connFor(srv), "/api/health", nil, nil, nil)
	if err == nil {
		t.Fatal("expected error on 401")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("want *HTTPError, got %T (%v)", err, err)
	}
	if httpErr.Status != http.StatusUnauthorized {
		t.Errorf("status = %d", httpErr.Status)
	}
	if !strings.Contains(httpErr.Body, "nope") {
		t.Errorf("body lost: %q", httpErr.Body)
	}
	if httpErr.Method != http.MethodGet || httpErr.Path != "/api/health" {
		t.Errorf("method/path lost: %+v", httpErr)
	}
}

func TestDo_NetworkErrorPropagates(t *testing.T) {
	// Point at a closed server to provoke a transport error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.Close()

	m := newTestManager(srv)
	err := m.do(http.MethodGet, Connection{BaseURL: srv.URL, Token: "t"}, "/api/health", nil, nil, nil)
	if err == nil {
		t.Fatal("expected transport error")
	}
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		t.Fatalf("transport error should NOT be HTTPError, got %+v", httpErr)
	}
}
