package gigot

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/journal"
)

// ── fake injectables ────────────────────────────────────────────────

type syncCall struct {
	backend, version string
	pushed, pulled   int
}

type seenCall struct {
	backend, version string
}

type fakeJournal struct {
	mu    sync.Mutex
	syncs []syncCall
	seens []seenCall
}

func (f *fakeJournal) RecordSync(backend, version string, pushed, pulled int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.syncs = append(f.syncs, syncCall{backend, version, pushed, pulled})
}

func (f *fakeJournal) RecordRemoteSeen(backend, version string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seens = append(f.seens, seenCall{backend, version})
}

func (f *fakeJournal) Pending(backend string) journal.PendingResult {
	return journal.PendingResult{}
}

func (f *fakeJournal) snapshot() ([]syncCall, []seenCall) {
	f.mu.Lock()
	defer f.mu.Unlock()
	syncs := make([]syncCall, len(f.syncs))
	copy(syncs, f.syncs)
	seens := make([]seenCall, len(f.seens))
	copy(seens, f.seens)
	return syncs, seens
}

type fakeCreds struct {
	store map[string]string
	err   error
}

func (f *fakeCreds) Get(account string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.store[account], nil
}

type fakeProfile struct{ name string }

func (f *fakeProfile) CurrentProfileFilename() string { return f.name }

type fakeConfig struct {
	baseURL  string
	repoName string
	author   string
	email    string
	context  string
}

func (f *fakeConfig) GigotBaseURL() string  { return f.baseURL }
func (f *fakeConfig) GigotRepoName() string { return f.repoName }
func (f *fakeConfig) AuthorName() string    { return f.author }
func (f *fakeConfig) AuthorEmail() string   { return f.email }
func (f *fakeConfig) ContextFolder() string { return f.context }

// ── token resolution ────────────────────────────────────────────────

func TestService_TokenResolvedFromKeychain(t *testing.T) {
	var seenAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	m := NewManager(newFakeFS(), WithHTTPClient(srv.Client()))
	cfg := &fakeConfig{baseURL: srv.URL, repoName: "addresses"}
	creds := &fakeCreds{store: map[string]string{
		"default.json:gigot:addresses": "secret-bearer",
	}}
	profile := &fakeProfile{name: "default.json"}

	s := NewService(m, creds, profile, cfg, nil)
	if _, err := s.Ping(); err != nil {
		t.Fatal(err)
	}
	if seenAuth != "Bearer secret-bearer" {
		t.Fatalf("Authorization header = %q", seenAuth)
	}
}

func TestService_MissingKeychainEntryFailsWithMissingToken(t *testing.T) {
	m := NewManager(newFakeFS())
	cfg := &fakeConfig{baseURL: "https://gigot.example", repoName: "addresses"}
	creds := &fakeCreds{store: map[string]string{}} // empty
	profile := &fakeProfile{name: "default.json"}

	s := NewService(m, creds, profile, cfg, nil)
	_, err := s.Ping()
	if !errors.Is(err, ErrMissingToken) {
		t.Fatalf("want ErrMissingToken, got %v", err)
	}
}

func TestService_MissingBaseURLFailsBeforeKeychain(t *testing.T) {
	m := NewManager(newFakeFS())
	cfg := &fakeConfig{baseURL: "", repoName: "addresses"}
	// Keychain has a token, but ConfigReader yields no base URL — the
	// Service must surface that first, not try to use the (irrelevant)
	// keychain entry.
	creds := &fakeCreds{store: map[string]string{"default.json:gigot:addresses": "t"}}
	profile := &fakeProfile{name: "default.json"}

	s := NewService(m, creds, profile, cfg, nil)
	_, err := s.Ping()
	if !errors.Is(err, ErrMissingBaseURL) {
		t.Fatalf("want ErrMissingBaseURL, got %v", err)
	}
}

func TestService_MissingRepoFailsForScopedOps(t *testing.T) {
	m := NewManager(newFakeFS())
	cfg := &fakeConfig{baseURL: "https://x", repoName: ""}
	creds := &fakeCreds{}
	profile := &fakeProfile{name: "default.json"}

	s := NewService(m, creds, profile, cfg, nil)
	_, err := s.Context()
	if !errors.Is(err, ErrMissingRepo) {
		t.Fatalf("want ErrMissingRepo, got %v", err)
	}
}

func TestService_PingToleratesMissingRepo(t *testing.T) {
	// /health is repo-agnostic; the Service should not require a
	// RepoName on the active profile. Keychain lookup needs both
	// profile name AND repo name to compose an account, so missing
	// repo collapses token resolve to empty — and Ping's pre-flight
	// then surfaces ErrMissingToken (the validation layer the user
	// sees), not ErrMissingRepo (the scoped-op marker). This pinpoints
	// the failure to "no bearer configured" rather than "no repo on
	// profile" which would mis-message the user.
	m := NewManager(newFakeFS())
	cfg := &fakeConfig{baseURL: "https://x", repoName: ""}
	creds := &fakeCreds{}
	profile := &fakeProfile{name: "default.json"}

	s := NewService(m, creds, profile, cfg, nil)
	_, err := s.Ping()
	if !errors.Is(err, ErrMissingToken) {
		t.Fatalf("want ErrMissingToken on missing-repo Ping, got %v", err)
	}
}

func TestService_MissingDepsCollapseToConfigErrors(t *testing.T) {
	// All injectables nil — Service should still return a typed error
	// rather than panic. Without cfg there is no BaseURL; the
	// validation layer surfaces that.
	m := NewManager(newFakeFS())
	s := NewService(m, nil, nil, nil, nil)
	_, err := s.Ping()
	if !errors.Is(err, ErrMissingBaseURL) {
		t.Fatalf("want ErrMissingBaseURL with nil deps, got %v", err)
	}
}

// ── author propagation ──────────────────────────────────────────────

func TestService_AuthorFlowsIntoCommitConnection(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/basic.yaml", "fresh\n")

	var commit CommitRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/repos/r/tree":
			_ = json.NewEncoder(w).Encode(TreeResponse{Version: "v0"})
		case "/api/repos/r/head":
			_ = json.NewEncoder(w).Encode(HeadResponse{Version: "v0"})
		case "/api/repos/r/commits":
			_ = json.NewDecoder(r.Body).Decode(&commit)
			_ = json.NewEncoder(w).Encode(CommitResponse{Version: "v1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	m := NewManager(newFakeFS(), WithHTTPClient(srv.Client()))
	cfg := &fakeConfig{
		baseURL: srv.URL, repoName: "r", context: ctxDir,
		author: "alice", email: "alice@example.com",
	}
	creds := &fakeCreds{store: map[string]string{"default.json:gigot:r": "tok"}}
	profile := &fakeProfile{name: "default.json"}

	s := NewService(m, creds, profile, cfg, nil)
	if _, err := s.PushLocal(); err != nil {
		t.Fatal(err)
	}
	if commit.Author == nil {
		t.Fatal("commit Author dropped")
	}
	if commit.Author.Name != "alice" || commit.Author.Email != "alice@example.com" {
		t.Errorf("commit Author = %+v", commit.Author)
	}
}

func TestService_NoAuthorWhenConfigBlank(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/basic.yaml", "fresh\n")

	var commit CommitRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/repos/r/tree":
			_ = json.NewEncoder(w).Encode(TreeResponse{Version: "v0"})
		case "/api/repos/r/head":
			_ = json.NewEncoder(w).Encode(HeadResponse{Version: "v0"})
		case "/api/repos/r/commits":
			_ = json.NewDecoder(r.Body).Decode(&commit)
			_ = json.NewEncoder(w).Encode(CommitResponse{Version: "v1"})
		}
	}))
	defer srv.Close()

	m := NewManager(newFakeFS(), WithHTTPClient(srv.Client()))
	cfg := &fakeConfig{baseURL: srv.URL, repoName: "r", context: ctxDir}
	creds := &fakeCreds{store: map[string]string{"default.json:gigot:r": "tok"}}
	profile := &fakeProfile{name: "default.json"}

	s := NewService(m, creds, profile, cfg, nil)
	if _, err := s.PushLocal(); err != nil {
		t.Fatal(err)
	}
	if commit.Author != nil {
		t.Fatalf("blank profile author should not propagate, got %+v", commit.Author)
	}
}

// ── journal hops ────────────────────────────────────────────────────

func TestService_PushLocalRecordsSyncOnSuccess(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/basic.yaml", "fresh\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/repos/r/tree":
			_ = json.NewEncoder(w).Encode(TreeResponse{Version: "v0"})
		case "/api/repos/r/head":
			_ = json.NewEncoder(w).Encode(HeadResponse{Version: "v0"})
		case "/api/repos/r/commits":
			_ = json.NewEncoder(w).Encode(CommitResponse{Version: "afterPush"})
		}
	}))
	defer srv.Close()

	m := NewManager(newFakeFS(), WithHTTPClient(srv.Client()))
	cfg := &fakeConfig{baseURL: srv.URL, repoName: "r", context: ctxDir}
	creds := &fakeCreds{store: map[string]string{"default.json:gigot:r": "tok"}}
	profile := &fakeProfile{name: "default.json"}
	jrnl := &fakeJournal{}

	s := NewService(m, creds, profile, cfg, jrnl)
	if _, err := s.PushLocal(); err != nil {
		t.Fatal(err)
	}
	syncs, seens := jrnl.snapshot()
	if len(syncs) != 1 || syncs[0].backend != journal.BackendGigot {
		t.Fatalf("expected 1 gigot sync, got %+v", syncs)
	}
	if syncs[0].version != "afterPush" || syncs[0].pushed != 1 {
		t.Errorf("sync entry = %+v", syncs[0])
	}
	if len(seens) != 0 {
		t.Errorf("push should not emit remote-seen, got %+v", seens)
	}
}

func TestService_PushLocalDoesNotRecordOnNoop(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/basic.yaml", "stable\n")
	sha := GitBlobSha([]byte("stable\n"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/repos/r/tree":
			_ = json.NewEncoder(w).Encode(TreeResponse{
				Version: "v0",
				Files:   []TreeEntry{{Path: "templates/basic.yaml", Blob: sha}},
			})
		}
	}))
	defer srv.Close()

	m := NewManager(newFakeFS(), WithHTTPClient(srv.Client()))
	cfg := &fakeConfig{baseURL: srv.URL, repoName: "r", context: ctxDir}
	creds := &fakeCreds{store: map[string]string{"default.json:gigot:r": "tok"}}
	profile := &fakeProfile{name: "default.json"}
	jrnl := &fakeJournal{}

	s := NewService(m, creds, profile, cfg, jrnl)
	if _, err := s.PushLocal(); err != nil {
		t.Fatal(err)
	}
	syncs, _ := jrnl.snapshot()
	if len(syncs) != 0 {
		t.Errorf("noop push must not emit sync marker, got %+v", syncs)
	}
}

func TestService_PullLocalRecordsRemoteSeen(t *testing.T) {
	ctxDir := t.TempDir()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/repos/r/tree" {
			_ = json.NewEncoder(w).Encode(TreeResponse{Version: "afterPull"})
		}
	}))
	defer srv.Close()

	m := NewManager(newFakeFS(), WithHTTPClient(srv.Client()))
	cfg := &fakeConfig{baseURL: srv.URL, repoName: "r", context: ctxDir}
	creds := &fakeCreds{store: map[string]string{"default.json:gigot:r": "tok"}}
	profile := &fakeProfile{name: "default.json"}
	jrnl := &fakeJournal{}

	s := NewService(m, creds, profile, cfg, jrnl)
	if _, err := s.PullLocal(); err != nil {
		t.Fatal(err)
	}
	_, seens := jrnl.snapshot()
	if len(seens) != 1 {
		t.Fatalf("expected 1 remote-seen, got %+v", seens)
	}
	if seens[0].backend != journal.BackendGigot || seens[0].version != "afterPull" {
		t.Errorf("seen entry = %+v", seens[0])
	}
}

func TestService_SyncDoesNotDoubleRecord(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/basic.yaml", "stable\n")
	sha := GitBlobSha([]byte("stable\n"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/repos/r/tree" {
			_ = json.NewEncoder(w).Encode(TreeResponse{
				Version: "v0",
				Files:   []TreeEntry{{Path: "templates/basic.yaml", Blob: sha}},
			})
		}
	}))
	defer srv.Close()

	m := NewManager(newFakeFS(), WithHTTPClient(srv.Client()))
	cfg := &fakeConfig{baseURL: srv.URL, repoName: "r", context: ctxDir}
	creds := &fakeCreds{store: map[string]string{"default.json:gigot:r": "tok"}}
	profile := &fakeProfile{name: "default.json"}
	jrnl := &fakeJournal{}

	s := NewService(m, creds, profile, cfg, jrnl)
	if _, err := s.Sync(); err != nil {
		t.Fatal(err)
	}
	syncs, seens := jrnl.snapshot()
	// Noop push → no sync marker. Pull emits a single remote-seen.
	if len(syncs) != 0 {
		t.Errorf("syncs unexpected: %+v", syncs)
	}
	if len(seens) != 1 {
		t.Errorf("expected exactly 1 remote-seen, got %+v", seens)
	}
}

// ── AttachProgress ──────────────────────────────────────────────────

func TestService_AttachProgress_RoutesEventsThroughEmitter(t *testing.T) {
	ctxDir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/repos/r/tree" {
			_ = json.NewEncoder(w).Encode(TreeResponse{Version: "v0"})
		}
	}))
	defer srv.Close()

	m := NewManager(newFakeFS(), WithHTTPClient(srv.Client()))
	cfg := &fakeConfig{baseURL: srv.URL, repoName: "r", context: ctxDir}
	creds := &fakeCreds{store: map[string]string{"default.json:gigot:r": "tok"}}
	profile := &fakeProfile{name: "default.json"}
	s := NewService(m, creds, profile, cfg, nil)

	type capturedEvent struct {
		name string
		data any
	}
	var captured []capturedEvent
	AttachProgress(s, func(name string, data any) {
		captured = append(captured, capturedEvent{name, data})
	})

	if _, err := s.PullLocal(); err != nil {
		t.Fatal(err)
	}
	if len(captured) == 0 {
		t.Fatal("AttachProgress did not route any events")
	}
	for _, c := range captured {
		if c.name != EventSyncProgress {
			t.Errorf("event name = %q, want %q", c.name, EventSyncProgress)
		}
		if _, ok := c.data.(SyncProgress); !ok {
			t.Errorf("event payload type = %T, want SyncProgress", c.data)
		}
	}
}

func TestService_AttachProgress_NilEmitIsSafe(t *testing.T) {
	m := NewManager(newFakeFS())
	s := NewService(m, nil, nil, nil, nil)
	AttachProgress(s, nil)
	AttachProgress(nil, nil)
}

func TestService_JournalNilDoesNotPanic(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/basic.yaml", "stable\n")
	sha := GitBlobSha([]byte("stable\n"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/repos/r/tree" {
			_ = json.NewEncoder(w).Encode(TreeResponse{
				Version: "v0",
				Files:   []TreeEntry{{Path: "templates/basic.yaml", Blob: sha}},
			})
		}
	}))
	defer srv.Close()

	m := NewManager(newFakeFS(), WithHTTPClient(srv.Client()))
	cfg := &fakeConfig{baseURL: srv.URL, repoName: "r", context: ctxDir}
	creds := &fakeCreds{store: map[string]string{"default.json:gigot:r": "tok"}}
	profile := &fakeProfile{name: "default.json"}

	s := NewService(m, creds, profile, cfg, nil) // nil journal
	if _, err := s.Sync(); err != nil {
		t.Fatal(err)
	}
}
