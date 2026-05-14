package gigot

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cucumber/godog"

	"github.com/petervdpas/formidable2/internal/modules/journal"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initGigotScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}

// gigotWorld holds per-scenario state. Reset in Before; everything in
// here is owned by exactly one scenario at a time.
type gigotWorld struct {
	tmp string
	srv *httptest.Server
	mux *gigotMux
	m   *Manager

	// Service + injectables for service-layer scenarios. Nil unless
	// the scenario builds them.
	svc  *Service
	jrnl *fakeJournal

	// Latest operation result.
	summary  *LedgerSummary
	push     *PushResult
	pull     *PullResult
	syncRes  *SyncResult
	health   *HealthResponse
	progress []SyncProgress
	lastErr  error

	// Server-side captures (mux populates these).
	capturedCommits []CommitRequest
	capturedAuth    string
	fetchedPaths    map[string]int
}

// gigotMux is a tiny path-multiplexer the scenarios build their fake
// gigot server out of. Steps register routes; the mux dispatches by
// method+path and records auth + per-path fetch counts so assertions
// can verify "no commit reached the server" / "no fetch was issued".
type gigotMux struct {
	mu     sync.Mutex
	routes map[string]http.HandlerFunc
	world  *gigotWorld
}

func newGigotMux(w *gigotWorld) *gigotMux {
	return &gigotMux{routes: map[string]http.HandlerFunc{}, world: w}
}

func (m *gigotMux) handle(method, path string, fn http.HandlerFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routes[method+" "+path] = fn
}

func (m *gigotMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.world.capturedAuth = r.Header.Get("Authorization")
	if strings.HasPrefix(r.URL.Path, "/api/repos/r/files/") {
		path := strings.TrimPrefix(r.URL.Path, "/api/repos/r/files/")
		if m.world.fetchedPaths == nil {
			m.world.fetchedPaths = map[string]int{}
		}
		m.world.fetchedPaths[path]++
	}
	fn, ok := m.routes[r.Method+" "+r.URL.Path]
	m.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fn(w, r)
}

func initGigotScenario(ctx *godog.ScenarioContext) {
	w := &gigotWorld{}

	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		dir, err := os.MkdirTemp("", "gigot-godog-")
		if err != nil {
			return ctx, err
		}
		w.tmp = dir
		w.srv = nil
		w.mux = nil
		w.m = nil
		w.svc = nil
		w.jrnl = nil
		w.summary = nil
		w.push = nil
		w.pull = nil
		w.syncRes = nil
		w.health = nil
		w.progress = nil
		w.lastErr = nil
		w.capturedCommits = nil
		w.capturedAuth = ""
		w.fetchedPaths = nil
		return ctx, nil
	})

	ctx.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w.srv != nil {
			w.srv.Close()
		}
		if w.tmp != "" {
			_ = os.RemoveAll(w.tmp)
		}
		return ctx, nil
	})

	// ── Background ────────────────────────────────────────────────────

	ctx.Step(`^a fresh context folder$`, func() error {
		if w.tmp == "" {
			return fmt.Errorf("context folder not initialised")
		}
		return nil
	})

	ctx.Step(`^a fake gigot server$`, func() error {
		w.mux = newGigotMux(w)
		w.srv = httptest.NewServer(w.mux)
		return nil
	})

	ctx.Step(`^a gigot manager bound to that server$`, func() error {
		w.m = NewManager(newFakeFS(), WithHTTPClient(w.srv.Client()))
		return nil
	})

	// ── Filesystem fixtures ───────────────────────────────────────────

	ctx.Step(`^the file "([^"]*)" exists with content "([^"]*)"$`, func(rel, content string) error {
		return writeWorldFile(w, rel, content)
	})

	ctx.Step(`^the ledger records "([^"]*)" with the current blob sha at version "([^"]*)"$`, func(rel, version string) error {
		buf, err := os.ReadFile(filepath.Join(w.tmp, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		return mergeLedger(w, rel, GitBlobSha(buf), version)
	})

	ctx.Step(`^the ledger records "([^"]*)" with sha "([^"]*)" at version "([^"]*)"$`, func(rel, sha, version string) error {
		return mergeLedger(w, rel, sha, version)
	})

	// ── Server-side fixtures ──────────────────────────────────────────

	ctx.Step(`^the server's tree at version "([^"]*)" is empty$`, func(version string) error {
		w.mux.handle("GET", "/api/repos/r/tree", func(wr http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(wr).Encode(TreeResponse{Version: version})
		})
		return nil
	})

	ctx.Step(`^the server's tree at version "([^"]*)" lists "([^"]*)" with content "([^"]*)"$`, func(version, repoPath, content string) error {
		body := []byte(content)
		sha := GitBlobSha(body)
		w.mux.handle("GET", "/api/repos/r/tree", func(wr http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(wr).Encode(TreeResponse{
				Version: version,
				Files:   []TreeEntry{{Path: repoPath, Blob: sha}},
			})
		})
		w.mux.handle("GET", "/api/repos/r/files/"+repoPath, func(wr http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(wr).Encode(FileResponse{
				Path:       repoPath,
				ContentB64: base64.StdEncoding.EncodeToString(body),
				Blob:       sha,
			})
		})
		return nil
	})

	ctx.Step(`^the server's tree at version "([^"]*)" lists "([^"]*)" with the current local blob sha$`, func(version, repoPath string) error {
		buf, err := os.ReadFile(filepath.Join(w.tmp, filepath.FromSlash(repoPath)))
		if err != nil {
			return err
		}
		sha := GitBlobSha(buf)
		w.mux.handle("GET", "/api/repos/r/tree", func(wr http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(wr).Encode(TreeResponse{
				Version: version,
				Files:   []TreeEntry{{Path: repoPath, Blob: sha}},
			})
		})
		return nil
	})

	ctx.Step(`^the server head is at version "([^"]*)"$`, func(version string) error {
		w.mux.handle("GET", "/api/repos/r/head", func(wr http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(wr).Encode(HeadResponse{Version: version})
		})
		return nil
	})

	ctx.Step(`^the server accepts the next commit as version "([^"]*)"$`, func(version string) error {
		w.mux.handle("POST", "/api/repos/r/commits", func(wr http.ResponseWriter, r *http.Request) {
			var req CommitRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(wr, err.Error(), 500)
				return
			}
			w.mux.mu.Lock()
			w.capturedCommits = append(w.capturedCommits, req)
			w.mux.mu.Unlock()
			_ = json.NewEncoder(wr).Encode(CommitResponse{Version: version})
		})
		return nil
	})

	// ── Manager-level operations ──────────────────────────────────────

	ctx.Step(`^I call LedgerSummary on the context folder$`, func() error {
		w.summary, w.lastErr = w.m.LedgerSummary(w.tmp)
		return nil
	})

	ctx.Step(`^I call LedgerSummary on the context folder (\d+) times$`, func(n int) error {
		for range n {
			w.summary, w.lastErr = w.m.LedgerSummary(w.tmp)
			if w.lastErr != nil {
				return w.lastErr
			}
		}
		return nil
	})

	ctx.Step(`^I push local with message "([^"]*)"$`, func(msg string) error {
		w.push, w.lastErr = w.m.PushLocal(testConn(w.srv.URL), w.tmp, decodeEscapes(msg))
		return nil
	})

	ctx.Step(`^I pull local$`, func() error {
		w.pull, w.lastErr = w.m.PullLocal(testConn(w.srv.URL), w.tmp)
		return nil
	})

	ctx.Step(`^I pull local with progress recording$`, func() error {
		w.progress = nil
		w.pull, w.lastErr = w.m.PullLocalWithProgress(testConn(w.srv.URL), w.tmp, func(p SyncProgress) {
			w.progress = append(w.progress, p)
		})
		return nil
	})

	ctx.Step(`^I reclone$`, func() error {
		w.pull, w.lastErr = w.m.Reclone(testConn(w.srv.URL), w.tmp)
		return nil
	})

	ctx.Step(`^I reclone with progress recording$`, func() error {
		w.progress = nil
		w.pull, w.lastErr = w.m.RecloneWithProgress(testConn(w.srv.URL), w.tmp, func(p SyncProgress) {
			w.progress = append(w.progress, p)
		})
		return nil
	})

	ctx.Step(`^I sync with message "([^"]*)"$`, func(msg string) error {
		w.syncRes, w.lastErr = w.m.Sync(testConn(w.srv.URL), w.tmp, decodeEscapes(msg))
		return nil
	})

	// ── Manager-level assertions ──────────────────────────────────────

	ctx.Step(`^the summary scanned count is (\d+)$`, func(n int) error {
		return mustErr(w.lastErr).orElse(func() error {
			if w.summary.Scanned != n {
				return fmt.Errorf("scanned = %d, want %d", w.summary.Scanned, n)
			}
			return nil
		})
	})

	ctx.Step(`^the summary changed list is empty$`, func() error {
		if len(w.summary.Changed) != 0 {
			return fmt.Errorf("changed = %v, want empty", w.summary.Changed)
		}
		return nil
	})

	ctx.Step(`^the summary deleted list is empty$`, func() error {
		if len(w.summary.Deleted) != 0 {
			return fmt.Errorf("deleted = %v, want empty", w.summary.Deleted)
		}
		return nil
	})

	ctx.Step(`^the summary has (\d+) changed entries$`, func(n int) error {
		if len(w.summary.Changed) != n {
			return fmt.Errorf("changed = %v, want %d entries", w.summary.Changed, n)
		}
		return nil
	})

	ctx.Step(`^the summary has (\d+) changed entry containing "([^"]*)"$`, func(n int, want string) error {
		if len(w.summary.Changed) != n {
			return fmt.Errorf("changed = %v, want %d entries", w.summary.Changed, n)
		}
		for _, p := range w.summary.Changed {
			if p == want {
				return nil
			}
		}
		return fmt.Errorf("changed = %v, missing %q", w.summary.Changed, want)
	})

	ctx.Step(`^the summary has (\d+) deleted entry containing "([^"]*)"$`, func(n int, want string) error {
		if len(w.summary.Deleted) != n {
			return fmt.Errorf("deleted = %v, want %d entries", w.summary.Deleted, n)
		}
		for _, p := range w.summary.Deleted {
			if p == want {
				return nil
			}
		}
		return fmt.Errorf("deleted = %v, missing %q", w.summary.Deleted, want)
	})

	ctx.Step(`^the summary version is "([^"]*)"$`, func(want string) error {
		if w.summary.Version != want {
			return fmt.Errorf("summary version = %q, want %q", w.summary.Version, want)
		}
		return nil
	})

	ctx.Step(`^the ledger version is still "([^"]*)"$`, func(want string) error {
		rec := ReadTrackRecord(w.tmp)
		if rec.Version != want {
			return fmt.Errorf("ledger version mutated: %q != %q", rec.Version, want)
		}
		return nil
	})

	ctx.Step(`^the ledger version is "([^"]*)"$`, func(want string) error {
		rec := ReadTrackRecord(w.tmp)
		if rec.Version != want {
			return fmt.Errorf("ledger version = %q, want %q", rec.Version, want)
		}
		return nil
	})

	ctx.Step(`^the push result is a noop$`, func() error {
		if w.push == nil || !w.push.Noop {
			return fmt.Errorf("expected noop push, got %+v", w.push)
		}
		return nil
	})

	ctx.Step(`^the push result has pushed=(\d+) deleted=(\d+)$`, func(p, d int) error {
		if w.push == nil {
			return fmt.Errorf("no push result")
		}
		if w.push.Pushed != p || w.push.Deleted != d {
			return fmt.Errorf("push result = %+v, want pushed=%d deleted=%d", w.push, p, d)
		}
		return nil
	})

	ctx.Step(`^the pull result has files=(\d+) deleted=(\d+)$`, func(f, d int) error {
		if w.pull == nil {
			return fmt.Errorf("no pull result")
		}
		if w.pull.Files != f || w.pull.Deleted != d {
			return fmt.Errorf("pull result = %+v, want files=%d deleted=%d", w.pull, f, d)
		}
		return nil
	})

	ctx.Step(`^the sync result is a noop$`, func() error {
		if w.syncRes == nil || !w.syncRes.Noop {
			return fmt.Errorf("expected noop sync, got %+v", w.syncRes)
		}
		return nil
	})

	ctx.Step(`^the local file "([^"]*)" contains "([^"]*)"$`, func(rel, want string) error {
		buf, err := os.ReadFile(filepath.Join(w.tmp, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		if string(buf) != want {
			return fmt.Errorf("%q content = %q, want %q", rel, string(buf), want)
		}
		return nil
	})

	ctx.Step(`^the local file "([^"]*)" does not exist$`, func(rel string) error {
		_, err := os.Stat(filepath.Join(w.tmp, filepath.FromSlash(rel)))
		if !os.IsNotExist(err) {
			return fmt.Errorf("%q still present (err=%v)", rel, err)
		}
		return nil
	})

	ctx.Step(`^the operation returned ErrEmptyContext$`, func() error {
		if w.lastErr == nil || w.lastErr.Error() != ErrEmptyContext.Error() {
			return fmt.Errorf("expected ErrEmptyContext, got %v", w.lastErr)
		}
		return nil
	})

	ctx.Step(`^the operation returned ErrMissingToken$`, func() error {
		if w.lastErr == nil || w.lastErr.Error() != ErrMissingToken.Error() {
			return fmt.Errorf("expected ErrMissingToken, got %v", w.lastErr)
		}
		return nil
	})

	ctx.Step(`^the server did not receive any commits$`, func() error {
		if len(w.capturedCommits) != 0 {
			return fmt.Errorf("expected no commits, got %d", len(w.capturedCommits))
		}
		return nil
	})

	ctx.Step(`^the server did not receive a file fetch for "([^"]*)"$`, func(rel string) error {
		if c := w.fetchedPaths[rel]; c != 0 {
			return fmt.Errorf("expected no fetch for %q, saw %d", rel, c)
		}
		return nil
	})

	ctx.Step(`^the captured commit message equals "([^"]*)"$`, func(want string) error {
		if len(w.capturedCommits) == 0 {
			return fmt.Errorf("no commit captured")
		}
		got := w.capturedCommits[len(w.capturedCommits)-1].Message
		if got != want {
			return fmt.Errorf("commit message = %q, want %q", got, want)
		}
		return nil
	})

	ctx.Step(`^the captured commit message contains "([^"]*)"$`, func(needle string) error {
		if len(w.capturedCommits) == 0 {
			return fmt.Errorf("no commit captured")
		}
		got := w.capturedCommits[len(w.capturedCommits)-1].Message
		if !strings.Contains(got, needle) {
			return fmt.Errorf("commit message = %q, missing %q", got, needle)
		}
		return nil
	})

	// ── Progress assertions ───────────────────────────────────────────

	ctx.Step(`^the first emitted phase is Start$`, func() error {
		if len(w.progress) == 0 || w.progress[0].Phase != PhaseStart {
			return fmt.Errorf("first phase = %v, want Start", phasesOf(w.progress))
		}
		return nil
	})

	ctx.Step(`^the last emitted phase is Done$`, func() error {
		if len(w.progress) == 0 || w.progress[len(w.progress)-1].Phase != PhaseDone {
			return fmt.Errorf("last phase = %v, want Done", phasesOf(w.progress))
		}
		return nil
	})

	ctx.Step(`^one Tree phase was emitted with total (\d+)$`, func(total int) error {
		for _, p := range w.progress {
			if p.Phase == PhaseTree {
				if p.Total != total {
					return fmt.Errorf("tree total = %d, want %d", p.Total, total)
				}
				return nil
			}
		}
		return fmt.Errorf("no Tree phase emitted")
	})

	ctx.Step(`^one Fetch phase was emitted$`, func() error {
		for _, p := range w.progress {
			if p.Phase == PhaseFetch {
				return nil
			}
		}
		return fmt.Errorf("no Fetch phase emitted")
	})

	ctx.Step(`^one Delete phase was emitted for "([^"]*)"$`, func(wantPath string) error {
		for _, p := range w.progress {
			if p.Phase == PhaseDelete && p.Path == wantPath {
				return nil
			}
		}
		return fmt.Errorf("no Delete phase emitted for %q (saw %v)", wantPath, phasesOf(w.progress))
	})

	ctx.Step(`^a Wipe phase was emitted before any Tree, Fetch, or Delete phase$`, func() error {
		wipeIdx := -1
		for i, p := range w.progress {
			if p.Phase == PhaseWipe {
				wipeIdx = i
				break
			}
		}
		if wipeIdx == -1 {
			return fmt.Errorf("no Wipe phase emitted (saw %v)", phasesOf(w.progress))
		}
		for i := 0; i < wipeIdx; i++ {
			ph := w.progress[i].Phase
			if ph == PhaseTree || ph == PhaseFetch || ph == PhaseDelete {
				return fmt.Errorf("phase %v at %d came before Wipe", ph, i)
			}
		}
		return nil
	})

	// ── Service-layer fixtures ────────────────────────────────────────

	ctx.Step(`^a gigot service wired with a keychain entry "([^"]*)"$`, func(secret string) error {
		creds := &fakeCreds{store: map[string]string{"default.json:gigot:r": secret}}
		w.svc = NewService(w.m,
			creds,
			&fakeProfile{name: "default.json"},
			&fakeConfig{baseURL: w.srv.URL, repoName: "r", context: w.tmp},
			nil,
		)
		return nil
	})

	ctx.Step(`^a gigot service wired with no keychain entry$`, func() error {
		creds := &fakeCreds{store: map[string]string{}}
		w.svc = NewService(w.m,
			creds,
			&fakeProfile{name: "default.json"},
			&fakeConfig{baseURL: w.srv.URL, repoName: "r", context: w.tmp},
			nil,
		)
		return nil
	})

	ctx.Step(`^a gigot service with a journal recorder$`, func() error {
		w.jrnl = &fakeJournal{}
		creds := &fakeCreds{store: map[string]string{"default.json:gigot:r": "tok"}}
		w.svc = NewService(w.m,
			creds,
			&fakeProfile{name: "default.json"},
			&fakeConfig{baseURL: w.srv.URL, repoName: "r", context: w.tmp},
			w.jrnl,
		)
		return nil
	})

	// Health route is always available — used by the auth-header test.
	ctx.Step(`^the service issues Ping$`, func() error {
		w.mux.handle("GET", "/api/health", func(wr http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(wr, `{"ok":true}`)
		})
		w.health, w.lastErr = w.svc.Ping()
		return nil
	})

	ctx.Step(`^the captured Authorization header equals "([^"]*)"$`, func(want string) error {
		if w.capturedAuth != want {
			return fmt.Errorf("auth header = %q, want %q", w.capturedAuth, want)
		}
		return nil
	})

	ctx.Step(`^the service pushes local with message "([^"]*)"$`, func(msg string) error {
		w.push, w.lastErr = w.svc.PushLocal(decodeEscapes(msg))
		return nil
	})

	ctx.Step(`^the service pulls local$`, func() error {
		w.pull, w.lastErr = w.svc.PullLocal()
		return nil
	})

	ctx.Step(`^the service syncs with message "([^"]*)"$`, func(msg string) error {
		w.syncRes, w.lastErr = w.svc.Sync(decodeEscapes(msg))
		return nil
	})

	ctx.Step(`^the journal recorded one gigot sync at version "([^"]*)"$`, func(version string) error {
		syncs, _ := w.jrnl.snapshot()
		if len(syncs) != 1 {
			return fmt.Errorf("expected 1 sync entry, got %d", len(syncs))
		}
		if syncs[0].backend != journal.BackendGigot || syncs[0].version != version {
			return fmt.Errorf("sync entry = %+v, want gigot/%s", syncs[0], version)
		}
		return nil
	})

	ctx.Step(`^the journal recorded one gigot remote-seen at version "([^"]*)"$`, func(version string) error {
		_, seens := w.jrnl.snapshot()
		if len(seens) != 1 {
			return fmt.Errorf("expected 1 remote-seen entry, got %d", len(seens))
		}
		if seens[0].backend != journal.BackendGigot || seens[0].version != version {
			return fmt.Errorf("seen entry = %+v, want gigot/%s", seens[0], version)
		}
		return nil
	})

	ctx.Step(`^the journal recorded exactly one gigot remote-seen entry$`, func() error {
		_, seens := w.jrnl.snapshot()
		if len(seens) != 1 {
			return fmt.Errorf("expected 1 remote-seen entry, got %d", len(seens))
		}
		return nil
	})

	ctx.Step(`^the journal did not record a sync entry$`, func() error {
		syncs, _ := w.jrnl.snapshot()
		if len(syncs) != 0 {
			return fmt.Errorf("expected no sync entries, got %d", len(syncs))
		}
		return nil
	})

	ctx.Step(`^the journal did not record a remote-seen entry$`, func() error {
		_, seens := w.jrnl.snapshot()
		if len(seens) != 0 {
			return fmt.Errorf("expected no remote-seen entries, got %d", len(seens))
		}
		return nil
	})
}

// ── helpers ─────────────────────────────────────────────────────────

func testConn(baseURL string) Connection {
	return Connection{BaseURL: baseURL, Token: "tok", RepoName: "r"}
}

func writeWorldFile(w *gigotWorld, rel, content string) error {
	abs := filepath.Join(w.tmp, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, []byte(content), 0o644)
}

// mergeLedger reads the on-disk ledger, sets a (path, sha) entry, and
// writes it back atomically. Lets each step build up the ledger one
// path at a time without overwriting earlier entries.
func mergeLedger(w *gigotWorld, repoPath, sha, version string) error {
	rec := ReadTrackRecord(w.tmp)
	rec.Version = version
	if rec.Files == nil {
		rec.Files = map[string]string{}
	}
	rec.Files[repoPath] = sha
	if err := os.MkdirAll(filepath.Join(w.tmp, ".formidable"), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(w.tmp, ".formidable", "sync.json"), raw, 0o644)
}

// decodeEscapes turns Gherkin-style literal "\n" / "\t" into runtime
// whitespace so a scenario can pass "   \n\t  " and have it land as
// the actual whitespace-only string the whitespace-fallback test
// needs.
func decodeEscapes(s string) string {
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\t`, "\t")
	s = strings.ReplaceAll(s, `\r`, "\r")
	return s
}

func phasesOf(events []SyncProgress) []SyncPhase {
	out := make([]SyncPhase, len(events))
	for i, e := range events {
		out[i] = e.Phase
	}
	return out
}

// mustErr is a tiny helper that fast-fails an assertion when a prior
// step's lastErr is non-nil — keeps the assertion bodies clean from
// the (very common) "did the call even succeed first?" boilerplate.
type maybeErr struct{ err error }

func mustErr(e error) maybeErr { return maybeErr{err: e} }

func (m maybeErr) orElse(fn func() error) error {
	if m.err != nil {
		return m.err
	}
	return fn()
}
