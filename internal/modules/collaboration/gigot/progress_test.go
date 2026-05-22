package gigot

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// ── helper recorders ────────────────────────────────────────────────

type progressRecorder struct {
	mu     sync.Mutex
	events []SyncProgress
}

func (p *progressRecorder) cb(ev SyncProgress) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, ev)
}

func (p *progressRecorder) snapshot() []SyncProgress {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]SyncProgress, len(p.events))
	copy(out, p.events)
	return out
}

func (p *progressRecorder) phases() []SyncPhase {
	out := []SyncPhase{}
	for _, e := range p.snapshot() {
		out = append(out, e.Phase)
	}
	return out
}

// ── PullLocalWithProgress ───────────────────────────────────────────

func TestPullLocalWithProgress_NilCallbackIsSafe(t *testing.T) {
	ctxDir := t.TempDir()
	srv, h := newOrchestrationServer(t)
	defer srv.Close()
	h.handle("GET", "/api/repos/r/tree", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(TreeResponse{Version: "v1"})
	})
	m := newOrchestrationManager(t, srv)
	if _, err := m.PullLocalWithProgress(
		Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, ctxDir, nil,
	); err != nil {
		t.Fatal(err)
	}
}

func TestPullLocalWithProgress_FiresStartTreeFetchDone(t *testing.T) {
	ctxDir := t.TempDir()
	body := []byte("hello\n")
	sha := GitBlobSha(body)

	srv, h := newOrchestrationServer(t)
	defer srv.Close()
	h.handle("GET", "/api/repos/r/tree", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(TreeResponse{
			Version: "v1",
			Files: []TreeEntry{
				{Path: "templates/a.yaml", Blob: sha},
				{Path: "templates/b.yaml", Blob: sha},
			},
		})
	})
	for _, p := range []string{"templates/a.yaml", "templates/b.yaml"} {
		path := p
		h.handle("GET", "/api/repos/r/files/"+path, func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(FileResponse{
				Path:       path,
				ContentB64: base64.StdEncoding.EncodeToString(body),
				Blob:       sha,
			})
		})
	}

	rec := &progressRecorder{}
	m := newOrchestrationManager(t, srv)
	if _, err := m.PullLocalWithProgress(
		Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, ctxDir, rec.cb,
	); err != nil {
		t.Fatal(err)
	}

	phases := rec.phases()
	if len(phases) == 0 || phases[0] != PhaseStart {
		t.Errorf("first phase = %v, want start", phases)
	}
	if phases[len(phases)-1] != PhaseDone {
		t.Errorf("last phase = %v, want done", phases)
	}
	var sawTree bool
	fetches := 0
	for _, e := range rec.snapshot() {
		if e.Phase == PhaseTree {
			sawTree = true
			if e.Total != 2 {
				t.Errorf("tree total = %d, want 2", e.Total)
			}
		}
		if e.Phase == PhaseFetch {
			fetches++
		}
	}
	if !sawTree {
		t.Error("no tree event")
	}
	if fetches != 2 {
		t.Errorf("fetch events = %d, want 2", fetches)
	}
}

func TestPullLocalWithProgress_FetchEventsCarryPath(t *testing.T) {
	ctxDir := t.TempDir()
	body := []byte("x")
	sha := GitBlobSha(body)
	srv, h := newOrchestrationServer(t)
	defer srv.Close()
	h.handle("GET", "/api/repos/r/tree", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(TreeResponse{
			Version: "v1",
			Files:   []TreeEntry{{Path: "templates/one.yaml", Blob: sha}},
		})
	})
	h.handle("GET", "/api/repos/r/files/templates/one.yaml", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(FileResponse{
			Path: "templates/one.yaml", ContentB64: base64.StdEncoding.EncodeToString(body), Blob: sha,
		})
	})

	rec := &progressRecorder{}
	m := newOrchestrationManager(t, srv)
	if _, err := m.PullLocalWithProgress(
		Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, ctxDir, rec.cb,
	); err != nil {
		t.Fatal(err)
	}
	for _, e := range rec.snapshot() {
		if e.Phase == PhaseFetch && e.Path != "templates/one.yaml" {
			t.Errorf("fetch event path = %q, want templates/one.yaml", e.Path)
		}
	}
}

func TestPullLocalWithProgress_SkippedShaMatchStillEmitsFetch(t *testing.T) {
	// A file whose local SHA matches the server doesn't trigger an
	// HTTP fetch - but it must still be counted in the progress
	// stream so the bar advances past it. Without this the user sees
	// a stuck progress bar on no-op syncs.
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/match.yaml", "stable\n")
	sha := GitBlobSha([]byte("stable\n"))

	srv, h := newOrchestrationServer(t)
	defer srv.Close()
	h.handle("GET", "/api/repos/r/tree", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(TreeResponse{
			Version: "v1",
			Files:   []TreeEntry{{Path: "templates/match.yaml", Blob: sha}},
		})
	})

	rec := &progressRecorder{}
	m := newOrchestrationManager(t, srv)
	if _, err := m.PullLocalWithProgress(
		Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, ctxDir, rec.cb,
	); err != nil {
		t.Fatal(err)
	}
	fetches := 0
	for _, e := range rec.snapshot() {
		if e.Phase == PhaseFetch {
			fetches++
			if e.Current != 1 || e.Total != 1 {
				t.Errorf("fetch event = %+v, want 1/1", e)
			}
		}
	}
	if fetches != 1 {
		t.Errorf("fetch events = %d, want 1 (sha-match still counts)", fetches)
	}
}

func TestPullLocalWithProgress_LocalDeletesFireDeletePhase(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/gone.yaml", "x")

	if err := os.MkdirAll(filepath.Join(ctxDir, ".formidable"), 0o755); err != nil {
		t.Fatal(err)
	}
	fs := newFakeFS()
	m := NewManager(fs)
	if err := m.WriteTrackRecord(ctxDir, TrackRecord{
		Version: "old",
		Files:   map[string]string{"templates/gone.yaml": "stale-sha"},
	}); err != nil {
		t.Fatal(err)
	}

	srv, h := newOrchestrationServer(t)
	defer srv.Close()
	h.handle("GET", "/api/repos/r/tree", func(w http.ResponseWriter, _ *http.Request) {
		// New tree omits gone.yaml - must trigger a local delete.
		_ = json.NewEncoder(w).Encode(TreeResponse{Version: "new"})
	})

	rec := &progressRecorder{}
	m = NewManager(fs, WithHTTPClient(srv.Client()))
	if _, err := m.PullLocalWithProgress(
		Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, ctxDir, rec.cb,
	); err != nil {
		t.Fatal(err)
	}

	deletes := 0
	for _, e := range rec.snapshot() {
		if e.Phase == PhaseDelete {
			deletes++
			if e.Path != "templates/gone.yaml" {
				t.Errorf("delete path = %q", e.Path)
			}
		}
	}
	if deletes != 1 {
		t.Errorf("delete events = %d, want 1", deletes)
	}
}

func TestPullLocalWithProgress_TotalCountsDeletesAndFetches(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/gone.yaml", "x")
	fs := newFakeFS()
	m := NewManager(fs)
	if err := m.WriteTrackRecord(ctxDir, TrackRecord{
		Version: "old",
		Files:   map[string]string{"templates/gone.yaml": "stale"},
	}); err != nil {
		t.Fatal(err)
	}

	body := []byte("x")
	sha := GitBlobSha(body)
	srv, h := newOrchestrationServer(t)
	defer srv.Close()
	h.handle("GET", "/api/repos/r/tree", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(TreeResponse{
			Version: "new",
			Files:   []TreeEntry{{Path: "templates/fresh.yaml", Blob: sha}},
		})
	})
	h.handle("GET", "/api/repos/r/files/templates/fresh.yaml", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(FileResponse{
			Path: "templates/fresh.yaml", ContentB64: base64.StdEncoding.EncodeToString(body), Blob: sha,
		})
	})

	rec := &progressRecorder{}
	m = NewManager(fs, WithHTTPClient(srv.Client()))
	if _, err := m.PullLocalWithProgress(
		Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, ctxDir, rec.cb,
	); err != nil {
		t.Fatal(err)
	}

	// One delete + one fetch → Total = 2 on the tree event, monotonic
	// Current from 1 (first delete) to 2 (second fetch).
	var treeTotal int
	for _, e := range rec.snapshot() {
		if e.Phase == PhaseTree {
			treeTotal = e.Total
		}
	}
	if treeTotal != 2 {
		t.Errorf("tree total = %d, want 2 (1 delete + 1 fetch)", treeTotal)
	}

	var lastCount int
	for _, e := range rec.snapshot() {
		if e.Phase != PhaseDelete && e.Phase != PhaseFetch {
			continue
		}
		if e.Current <= lastCount {
			t.Errorf("non-monotonic current: %d → %d", lastCount, e.Current)
		}
		lastCount = e.Current
	}
	if lastCount != 2 {
		t.Errorf("final count = %d, want 2", lastCount)
	}
}

// ── ReclonWithProgress ──────────────────────────────────────────────

func TestRecloneWithProgress_FiresWipePhaseFirst(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/old.yaml", "x")

	srv, h := newOrchestrationServer(t)
	defer srv.Close()
	h.handle("GET", "/api/repos/r/tree", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(TreeResponse{Version: "new"})
	})

	rec := &progressRecorder{}
	m := newOrchestrationManager(t, srv)
	if _, err := m.RecloneWithProgress(
		Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, ctxDir, rec.cb,
	); err != nil {
		t.Fatal(err)
	}

	phases := rec.phases()
	if len(phases) < 3 {
		t.Fatalf("phases too short: %+v", phases)
	}
	// Wipe must precede any pull-side phase (Tree/Fetch/Delete/Done).
	wipeIdx := -1
	for i, p := range phases {
		if p == PhaseWipe {
			wipeIdx = i
			break
		}
	}
	if wipeIdx == -1 {
		t.Fatalf("no wipe phase in %+v", phases)
	}
	for i := 0; i < wipeIdx; i++ {
		if phases[i] == PhaseTree || phases[i] == PhaseFetch || phases[i] == PhaseDelete {
			t.Errorf("phase %v at %d came before wipe", phases[i], i)
		}
	}
}

func TestRecloneWithProgress_NilCallbackIsSafe(t *testing.T) {
	ctxDir := t.TempDir()
	srv, h := newOrchestrationServer(t)
	defer srv.Close()
	h.handle("GET", "/api/repos/r/tree", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(TreeResponse{Version: "v1"})
	})
	m := newOrchestrationManager(t, srv)
	if _, err := m.RecloneWithProgress(
		Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, ctxDir, nil,
	); err != nil {
		t.Fatal(err)
	}
}
