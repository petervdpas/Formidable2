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

// expect records the path + method the server saw on the most recent
// request — used by route tests to assert "client hit the right URL"
// without per-test scaffolding.
type capture struct {
	Method string
	Path   string
	Query  string
	Body   []byte
}

// stubServer is the shared route-test fixture: returns a fresh
// httptest.Server whose handler captures the request shape into the
// given capture pointer and replies with the supplied JSON body.
func stubServer(t *testing.T, cap *capture, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		*cap = capture{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Body:   raw,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
}

// ── Ping ────────────────────────────────────────────────────────────

func TestPing_HitsHealthEndpoint(t *testing.T) {
	var cap capture
	srv := stubServer(t, &cap, http.StatusOK, `{"ok":true,"version":"1.0.0"}`)
	defer srv.Close()

	m := newTestManager(srv)
	got, err := m.Ping(Connection{BaseURL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if cap.Method != http.MethodGet || cap.Path != "/api/health" {
		t.Errorf("hit %s %s, want GET /api/health", cap.Method, cap.Path)
	}
	if got == nil || !got.OK || got.Version != "1.0.0" {
		t.Errorf("decoded = %+v", got)
	}
}

func TestPing_PropagatesHTTPError(t *testing.T) {
	var cap capture
	srv := stubServer(t, &cap, http.StatusUnauthorized, `unauthorized`)
	defer srv.Close()

	m := newTestManager(srv)
	_, err := m.Ping(Connection{BaseURL: srv.URL, Token: "t"})
	var he *HTTPError
	if !errors.As(err, &he) || he.Status != http.StatusUnauthorized {
		t.Fatalf("want 401 HTTPError, got %v", err)
	}
}

// ── Me ──────────────────────────────────────────────────────────────

func TestMe_HitsMeEndpoint(t *testing.T) {
	var cap capture
	srv := stubServer(t, &cap, http.StatusOK, `{
		"user":{"username":"alice","provider":"github"},
		"subscription":{"repo":"addresses","abilities":["read","write"]}
	}`)
	defer srv.Close()

	m := newTestManager(srv)
	got, err := m.Me(Connection{BaseURL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if cap.Method != http.MethodGet || cap.Path != "/api/me" {
		t.Errorf("hit %s %s", cap.Method, cap.Path)
	}
	if got.User.Username != "alice" || got.Subscription.Repo != "addresses" {
		t.Errorf("decoded = %+v", got)
	}
	if len(got.Subscription.Abilities) != 2 {
		t.Errorf("abilities = %+v", got.Subscription.Abilities)
	}
}

// ── Context ─────────────────────────────────────────────────────────

func TestContext_HitsRepoContextEndpoint(t *testing.T) {
	var cap capture
	srv := stubServer(t, &cap, http.StatusOK, `{
		"user":{"username":"alice"},
		"subscription":{"repo":"addresses"},
		"repo":{"head":"sha","default_branch":"main","empty":false,"is_formidable":true,"destination_count":2}
	}`)
	defer srv.Close()

	m := newTestManager(srv)
	got, err := m.Context(Connection{BaseURL: srv.URL, Token: "t", RepoName: "addresses"})
	if err != nil {
		t.Fatal(err)
	}
	if cap.Path != "/api/repos/addresses/context" {
		t.Errorf("path = %q", cap.Path)
	}
	if !got.Repo.IsFormidable || got.Repo.DestinationCount != 2 || got.Repo.Head != "sha" {
		t.Errorf("decoded = %+v", got)
	}
}

func TestContext_RequiresRepoName(t *testing.T) {
	m := NewManager(newFakeFS())
	_, err := m.Context(Connection{BaseURL: "https://x", Token: "t"})
	if !errors.Is(err, ErrMissingRepo) {
		t.Fatalf("want ErrMissingRepo, got %v", err)
	}
}

// ── Formidable ──────────────────────────────────────────────────────

func TestFormidable_HitsFormidableEndpoint(t *testing.T) {
	var cap capture
	srv := stubServer(t, &cap, http.StatusOK, `{
		"marker_present":true,
		"marker":{"version":1,"scaffolded_by":"alice","scaffolded_at":"2026-01-01T00:00:00Z"},
		"templates":[{"name":"basic","path":"templates/basic.yaml"}],
		"storage":[{"template":"addresses","files":3}]
	}`)
	defer srv.Close()

	m := newTestManager(srv)
	got, err := m.Formidable(Connection{BaseURL: srv.URL, Token: "t", RepoName: "addresses"})
	if err != nil {
		t.Fatal(err)
	}
	if cap.Path != "/api/repos/addresses/formidable" {
		t.Errorf("path = %q", cap.Path)
	}
	if !got.MarkerPresent || got.Marker == nil || got.Marker.Version != 1 {
		t.Errorf("marker decode = %+v", got.Marker)
	}
	if len(got.Templates) != 1 || got.Templates[0].Name != "basic" {
		t.Errorf("templates = %+v", got.Templates)
	}
	if len(got.Storage) != 1 || got.Storage[0].Files != 3 {
		t.Errorf("storage = %+v", got.Storage)
	}
}

// ── Head ────────────────────────────────────────────────────────────

func TestHead_HitsHeadEndpoint(t *testing.T) {
	var cap capture
	srv := stubServer(t, &cap, http.StatusOK, `{"version":"abc123","default_branch":"main"}`)
	defer srv.Close()

	m := newTestManager(srv)
	got, err := m.Head(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"})
	if err != nil {
		t.Fatal(err)
	}
	if cap.Path != "/api/repos/r/head" {
		t.Errorf("path = %q", cap.Path)
	}
	if got.Version != "abc123" || got.DefaultBranch != "main" {
		t.Errorf("decoded = %+v", got)
	}
}

func TestHead_409MapsToHTTPError(t *testing.T) {
	var cap capture
	srv := stubServer(t, &cap, http.StatusConflict, `repo empty`)
	defer srv.Close()

	m := newTestManager(srv)
	_, err := m.Head(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"})
	var he *HTTPError
	if !errors.As(err, &he) || he.Status != http.StatusConflict {
		t.Fatalf("want 409 HTTPError, got %v", err)
	}
}

// ── Tree ────────────────────────────────────────────────────────────

func TestTree_HitsTreeEndpoint(t *testing.T) {
	var cap capture
	srv := stubServer(t, &cap, http.StatusOK, `{
		"version":"v1",
		"files":[
			{"path":"templates/basic.yaml","blob":"aaa","size":42},
			{"path":"storage/x/one.meta.json","blob":"bbb"}
		]
	}`)
	defer srv.Close()

	m := newTestManager(srv)
	got, err := m.Tree(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"})
	if err != nil {
		t.Fatal(err)
	}
	if cap.Path != "/api/repos/r/tree" {
		t.Errorf("path = %q", cap.Path)
	}
	if got.Version != "v1" || len(got.Files) != 2 || got.Files[0].Blob != "aaa" {
		t.Errorf("decoded = %+v", got)
	}
}

// ── GetFile ─────────────────────────────────────────────────────────

func TestGetFile_HitsFilesEndpointWithEncodedPath(t *testing.T) {
	var cap capture
	srv := stubServer(t, &cap, http.StatusOK,
		`{"path":"storage/my notes/x.meta.json","content_b64":"YWJj","blob":"sha","size":3}`)
	defer srv.Close()

	m := newTestManager(srv)
	got, err := m.GetFile(
		Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"},
		"storage/my notes/x.meta.json",
	)
	if err != nil {
		t.Fatal(err)
	}
	// Server-side path is URL-decoded by net/http, so cap.Path is the
	// raw user-facing path. What matters is that slashes between
	// segments survived (so it really hit the files route) and spaces
	// were not literally injected (the round-trip path equals the
	// requested repo-relative path).
	if cap.Path != "/api/repos/r/files/storage/my notes/x.meta.json" {
		t.Errorf("server received path %q", cap.Path)
	}
	if got.ContentB64 != "YWJj" || got.Blob != "sha" {
		t.Errorf("decoded = %+v", got)
	}
}

// ── Log ─────────────────────────────────────────────────────────────

func TestLog_DecodesWrappedResponse(t *testing.T) {
	var cap capture
	srv := stubServer(t, &cap, http.StatusOK, `{
		"name":"r",
		"entries":[
			{"hash":"a","parents":["p1"],"refs":["HEAD","main"],"author":"alice","email":"a@example.com","date":"2026-01-01T00:00:00Z","message":"hi"}
		],
		"count":1
	}`)
	defer srv.Close()

	m := newTestManager(srv)
	got, err := m.Log(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, 25, false)
	if err != nil {
		t.Fatal(err)
	}
	if cap.Path != "/api/repos/r/log" {
		t.Errorf("path = %q", cap.Path)
	}
	if cap.Query != "limit=25" {
		t.Errorf("query = %q", cap.Query)
	}
	if got.Name != "r" || got.Count != 1 || len(got.Entries) != 1 {
		t.Fatalf("envelope decode = %+v", got)
	}
	e := got.Entries[0]
	if e.Hash != "a" || e.Email != "a@example.com" {
		t.Errorf("entry hash/email lost: %+v", e)
	}
	if len(e.Parents) != 1 || e.Parents[0] != "p1" {
		t.Errorf("parents lost: %+v", e.Parents)
	}
	if len(e.Refs) != 2 || e.Refs[0] != "HEAD" {
		t.Errorf("refs lost: %+v", e.Refs)
	}
	if e.Changes != nil {
		t.Errorf("changes should be omitted without with_changes, got %+v", e.Changes)
	}
}

func TestLog_OmitsLimitQueryWhenNonPositive(t *testing.T) {
	var cap capture
	srv := stubServer(t, &cap, http.StatusOK, `{"name":"r","entries":[],"count":0}`)
	defer srv.Close()

	m := newTestManager(srv)
	if _, err := m.Log(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, 0, false); err != nil {
		t.Fatal(err)
	}
	if cap.Query != "" {
		t.Errorf("non-positive limit should not produce query, got %q", cap.Query)
	}
}

func TestLog_WithChangesPassesQueryAndDecodesFilePaths(t *testing.T) {
	var cap capture
	srv := stubServer(t, &cap, http.StatusOK, `{
		"name":"r",
		"entries":[{
			"hash":"a","author":"alice","date":"2026-01-01T00:00:00Z","message":"audit",
			"changes":[
				{"path":"templates/basic.yaml","status":"M"},
				{"path":"storage/x/one.meta.json","status":"A"}
			]
		}],
		"count":1
	}`)
	defer srv.Close()

	m := newTestManager(srv)
	got, err := m.Log(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, 5, true)
	if err != nil {
		t.Fatal(err)
	}
	if cap.Query == "" || !strings.Contains(cap.Query, "with_changes=1") {
		t.Fatalf("query should include with_changes=1, got %q", cap.Query)
	}
	if !strings.Contains(cap.Query, "limit=5") {
		t.Fatalf("query should still include limit=5, got %q", cap.Query)
	}
	if len(got.Entries) != 1 || len(got.Entries[0].Changes) != 2 {
		t.Fatalf("changes not decoded: %+v", got)
	}
	cs := got.Entries[0].Changes
	if cs[0].Path != "templates/basic.yaml" || cs[0].Status != "M" {
		t.Errorf("changes[0] = %+v", cs[0])
	}
	if cs[1].Path != "storage/x/one.meta.json" || cs[1].Status != "A" {
		t.Errorf("changes[1] = %+v", cs[1])
	}
}

// ── Destinations / DestinationSync ──────────────────────────────────

func TestDestinations_HitsDestinationsEndpoint(t *testing.T) {
	var cap capture
	srv := stubServer(t, &cap, http.StatusOK,
		`[{"id":"d1","name":"mirror","url":"git@x","auto":true,"last_sync_status":"ok"}]`)
	defer srv.Close()

	m := newTestManager(srv)
	got, err := m.Destinations(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"})
	if err != nil {
		t.Fatal(err)
	}
	if cap.Path != "/api/repos/r/destinations" {
		t.Errorf("path = %q", cap.Path)
	}
	if len(got) != 1 || got[0].ID != "d1" || !got[0].Auto {
		t.Errorf("decoded = %+v", got)
	}
}

func TestDestinationSync_PostsToDestinationSyncEndpoint(t *testing.T) {
	var cap capture
	srv := stubServer(t, &cap, http.StatusOK,
		`{"id":"d1","name":"mirror","url":"git@x","last_sync_status":"ok","last_sync_at":"2026-01-01T00:00:00Z"}`)
	defer srv.Close()

	m := newTestManager(srv)
	got, err := m.DestinationSync(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, "d1")
	if err != nil {
		t.Fatal(err)
	}
	if cap.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", cap.Method)
	}
	if cap.Path != "/api/repos/r/destinations/d1/sync" {
		t.Errorf("path = %q", cap.Path)
	}
	if got.LastSyncStatus != "ok" {
		t.Errorf("decoded = %+v", got)
	}
}

func TestDestinationSync_RequiresDestinationID(t *testing.T) {
	m := NewManager(newFakeFS())
	_, err := m.DestinationSync(Connection{BaseURL: "https://x", Token: "t", RepoName: "r"}, "")
	if err == nil {
		t.Fatal("blank destination id must error")
	}
}

// ── Commit ──────────────────────────────────────────────────────────

func TestCommit_PostsBodyToCommitsEndpoint(t *testing.T) {
	var cap capture
	srv := stubServer(t, &cap, http.StatusOK,
		`{"version":"newv","changes":[{"op":"put","path":"templates/basic.yaml","blob":"sha"}]}`)
	defer srv.Close()

	m := newTestManager(srv)
	req := CommitRequest{
		ParentVersion: "oldv",
		Message:       "sync",
		Changes:       []Change{{Op: "put", Path: "templates/basic.yaml", ContentB64: "YWJj"}},
		Author:        &Author{Name: "alice", Email: "a@example.com"},
	}
	got, err := m.Commit(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, req)
	if err != nil {
		t.Fatal(err)
	}
	if cap.Method != http.MethodPost {
		t.Errorf("method = %q", cap.Method)
	}
	if cap.Path != "/api/repos/r/commits" {
		t.Errorf("path = %q", cap.Path)
	}

	var sent CommitRequest
	if err := json.Unmarshal(cap.Body, &sent); err != nil {
		t.Fatalf("server-received body not JSON: %v", err)
	}
	if sent.ParentVersion != "oldv" || sent.Message != "sync" || len(sent.Changes) != 1 {
		t.Errorf("body lost: %+v", sent)
	}
	if sent.Author == nil || sent.Author.Name != "alice" {
		t.Errorf("author lost: %+v", sent.Author)
	}

	if got.Version != "newv" || len(got.Changes) != 1 || got.Changes[0].Blob != "sha" {
		t.Errorf("decoded = %+v", got)
	}
}

func TestCommit_409MapsToHTTPError(t *testing.T) {
	var cap capture
	srv := stubServer(t, &cap, http.StatusConflict, `parent_version stale`)
	defer srv.Close()

	m := newTestManager(srv)
	req := CommitRequest{ParentVersion: "old", Changes: []Change{{Op: "put", Path: "p"}}}
	_, err := m.Commit(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, req)
	var he *HTTPError
	if !errors.As(err, &he) || he.Status != http.StatusConflict {
		t.Fatalf("want 409, got %v", err)
	}
}
