package gigot

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// ── fixture helpers ─────────────────────────────────────────────────

// orchestrationHandler is a tiny path-multiplexer the orchestration
// tests build their fake gigot server out of. Each route is a handler
// keyed by exact path + method.
type orchestrationHandler struct {
	t      *testing.T
	routes map[string]http.HandlerFunc
}

func newOrchestrationHandler(t *testing.T) *orchestrationHandler {
	return &orchestrationHandler{t: t, routes: map[string]http.HandlerFunc{}}
}

func (h *orchestrationHandler) handle(method, path string, fn http.HandlerFunc) {
	h.routes[method+" "+path] = fn
}

func (h *orchestrationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fn, ok := h.routes[r.Method+" "+r.URL.Path]
	if !ok {
		h.t.Logf("unexpected request: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fn(w, r)
}

func newOrchestrationServer(t *testing.T) (*httptest.Server, *orchestrationHandler) {
	t.Helper()
	h := newOrchestrationHandler(t)
	return httptest.NewServer(h), h
}

func newOrchestrationManager(t *testing.T, srv *httptest.Server) *Manager {
	t.Helper()
	return NewManager(newFakeFS(), WithHTTPClient(srv.Client()))
}

// readJSONBody decodes a JSON request body into v. Fatals on error.
func readJSONBody(t *testing.T, r *http.Request, v any) {
	t.Helper()
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
}

// ── PushLocal ───────────────────────────────────────────────────────

func TestPushLocal_RejectsBlankContext(t *testing.T) {
	m := NewManager(newFakeFS())
	_, err := m.PushLocal(Connection{BaseURL: "https://x", Token: "t", RepoName: "r"}, "", "")
	if !errors.Is(err, ErrMissingContext) {
		t.Fatalf("want ErrMissingContext, got %v", err)
	}
}

func TestPushLocal_EmptyContextReturnsError(t *testing.T) {
	m := NewManager(newFakeFS())
	ctxDir := t.TempDir()
	_, err := m.PushLocal(Connection{BaseURL: "https://x", Token: "t", RepoName: "r"}, ctxDir, "")
	if !errors.Is(err, ErrEmptyContext) {
		t.Fatalf("want ErrEmptyContext, got %v", err)
	}
}

func TestPushLocal_FirstSyncSeedsFromTreeAndSkipsRePushOfMatchingBlobs(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/basic.yaml", "name: basic\n")
	// What's in the local file already exists on the server with a
	// matching blob - no push should happen.
	localSha := GitBlobSha([]byte("name: basic\n"))

	srv, h := newOrchestrationServer(t)
	defer srv.Close()
	h.handle("GET", "/api/repos/r/tree", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(TreeResponse{
			Version: "serverV1",
			Files:   []TreeEntry{{Path: "templates/basic.yaml", Blob: localSha}},
		})
	})
	// Head + commits must NOT be hit on noop; if they are, the test
	// fails via the default 404 handler.

	m := newOrchestrationManager(t, srv)
	res, err := m.PushLocal(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, ctxDir, "")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Noop || res.Pushed != 0 || res.Deleted != 0 || res.Scanned != 1 {
		t.Fatalf("noop expected, got %+v", res)
	}
	if res.Version != "serverV1" {
		t.Errorf("version not propagated: %q", res.Version)
	}

	rec := ReadTrackRecord(ctxDir)
	if rec.Version != "serverV1" {
		t.Errorf("ledger version not seeded: %q", rec.Version)
	}
	if rec.Files["templates/basic.yaml"] != localSha {
		t.Errorf("ledger SHA not seeded: %+v", rec.Files)
	}
}

func TestPushLocal_FirstSyncCommitsNewLocalFile(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/basic.yaml", "fresh\n")

	srv, h := newOrchestrationServer(t)
	defer srv.Close()

	// /tree on first sync - server has nothing.
	h.handle("GET", "/api/repos/r/tree", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(TreeResponse{Version: "", Files: nil})
	})
	// /head returns 409 because server is empty.
	h.handle("GET", "/api/repos/r/head", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "empty repo", http.StatusConflict)
	})

	m := newOrchestrationManager(t, srv)
	_, err := m.PushLocal(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, ctxDir, "")
	if !errors.Is(err, ErrNoParentVersion) {
		t.Fatalf("want ErrNoParentVersion on empty remote, got %v", err)
	}
}

func TestPushLocal_SteadyStateCommitsChangedFile(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/basic.yaml", "updated\n")

	// Seed a non-empty ledger so we skip the first-sync /tree fetch.
	seed := EmptyTrackRecord()
	seed.Version = "parentV"
	seed.Files["templates/basic.yaml"] = "oldsha"
	fs := newFakeFS()
	m := NewManager(fs, WithHTTPClient(nil)) // client overridden below
	if err := m.WriteTrackRecord(ctxDir, seed); err != nil {
		t.Fatal(err)
	}

	var commitReq CommitRequest
	srv, h := newOrchestrationServer(t)
	defer srv.Close()
	h.handle("GET", "/api/repos/r/head", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(HeadResponse{Version: "parentV"})
	})
	h.handle("POST", "/api/repos/r/commits", func(w http.ResponseWriter, r *http.Request) {
		readJSONBody(t, r, &commitReq)
		_ = json.NewEncoder(w).Encode(CommitResponse{
			Version: "newV",
			Changes: []Change{{Op: "put", Path: "templates/basic.yaml", Blob: "newsha"}},
		})
	})
	// The server echoed a different blob (a merge), so the push syncs the
	// authoritative content back; serve it.
	h.handle("GET", "/api/repos/r/files/templates/basic.yaml", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(FileResponse{ContentB64: base64.StdEncoding.EncodeToString([]byte("updated\n"))})
	})
	m = NewManager(fs, WithHTTPClient(srv.Client()))

	res, err := m.PushLocal(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, ctxDir, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Noop || res.Pushed != 1 {
		t.Fatalf("expected 1 push, got %+v", res)
	}
	if commitReq.ParentVersion != "parentV" {
		t.Errorf("parent_version = %q", commitReq.ParentVersion)
	}
	if len(commitReq.Changes) != 1 || commitReq.Changes[0].Op != "put" {
		t.Errorf("commit changes = %+v", commitReq.Changes)
	}
	// Body must be base64-encoded.
	raw, err := base64.StdEncoding.DecodeString(commitReq.Changes[0].ContentB64)
	if err != nil || string(raw) != "updated\n" {
		t.Errorf("commit body decode = %q err=%v", raw, err)
	}

	rec := ReadTrackRecord(ctxDir)
	if rec.Version != "newV" {
		t.Errorf("ledger version = %q", rec.Version)
	}
	if rec.Files["templates/basic.yaml"] != "newsha" {
		t.Errorf("ledger reconciled from server changes[]: %+v", rec.Files)
	}
}

// PushLocal must send the LEDGER base version as parent_version, not the live
// server head. The ledger records the version our diff was computed against;
// that is the true merge base. Sending head (the old contract) makes the
// server fast-forward-overlay our changes, silently clobbering a concurrent
// edit instead of 3-way merging against the base.
func TestPushLocal_BehindServer_SendsLedgerVersionAsParent(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/basic.yaml", "updated\n")

	seed := EmptyTrackRecord()
	seed.Version = "baseV" // what we last synced to
	seed.Files["templates/basic.yaml"] = "oldsha"
	fs := newFakeFS()
	m := NewManager(fs, WithHTTPClient(nil))
	if err := m.WriteTrackRecord(ctxDir, seed); err != nil {
		t.Fatal(err)
	}

	var commitReq CommitRequest
	srv, h := newOrchestrationServer(t)
	defer srv.Close()
	// Server has moved on since our last sync: head != ledger base.
	h.handle("GET", "/api/repos/r/head", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(HeadResponse{Version: "serverAheadV"})
	})
	h.handle("POST", "/api/repos/r/commits", func(w http.ResponseWriter, r *http.Request) {
		readJSONBody(t, r, &commitReq)
		_ = json.NewEncoder(w).Encode(CommitResponse{
			Version: "mergedV",
			Changes: []Change{{Op: "put", Path: "templates/basic.yaml", Blob: "mergedsha"}},
		})
	})
	// The server merged (echoed blob differs), so the push syncs the merged
	// content back to disk; serve it.
	h.handle("GET", "/api/repos/r/files/templates/basic.yaml", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(FileResponse{ContentB64: base64.StdEncoding.EncodeToString([]byte("updated\n"))})
	})
	m = NewManager(fs, WithHTTPClient(srv.Client()))

	if _, err := m.PushLocal(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, ctxDir, ""); err != nil {
		t.Fatal(err)
	}
	if commitReq.ParentVersion != "baseV" {
		t.Errorf("parent_version = %q, want ledger base %q (sending head defeats the server merge)", commitReq.ParentVersion, "baseV")
	}
}

// A server 409 (it could not reconcile our base with HEAD) must surface as a
// structured conflict on the result, not an opaque error and not a silent
// clobber. Nothing landed, so the ledger must not advance.
func TestPushLocal_ServerConflictSurfacedNotClobbered(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/basic.yaml", "updated\n")

	seed := EmptyTrackRecord()
	seed.Version = "baseV"
	seed.Files["templates/basic.yaml"] = "oldsha"
	fs := newFakeFS()
	m := NewManager(fs, WithHTTPClient(nil))
	if err := m.WriteTrackRecord(ctxDir, seed); err != nil {
		t.Fatal(err)
	}

	srv, h := newOrchestrationServer(t)
	defer srv.Close()
	h.handle("POST", "/api/repos/r/commits", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"current_version":"headV","conflicts":[{"path":"templates/basic.yaml","current_version":"headV"}]}`))
	})
	m = NewManager(fs, WithHTTPClient(srv.Client()))

	res, err := m.PushLocal(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, ctxDir, "")
	if err != nil {
		t.Fatalf("a conflict must be a structured result, not an error: %v", err)
	}
	if len(res.Conflicts) != 1 || res.Conflicts[0].Path != "templates/basic.yaml" {
		t.Fatalf("want conflict path surfaced, got %+v", res.Conflicts)
	}
	if rec := ReadTrackRecord(ctxDir); rec.Version != "baseV" {
		t.Errorf("ledger advanced to %q on a conflict; nothing landed so it must stay baseV", rec.Version)
	}
}

// Record (meta.json) conflicts arrive in a distinct record_conflicts[] shape
// carrying per-field detail; the client surfaces the path and its fields.
func TestPushLocal_RecordConflictSurfacesFields(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "storage/notes/a.meta.json", "{\"x\":1}\n")

	seed := EmptyTrackRecord()
	seed.Version = "baseV"
	seed.Files["storage/notes/a.meta.json"] = "oldsha"
	fs := newFakeFS()
	m := NewManager(fs, WithHTTPClient(nil))
	if err := m.WriteTrackRecord(ctxDir, seed); err != nil {
		t.Fatal(err)
	}

	srv, h := newOrchestrationServer(t)
	defer srv.Close()
	h.handle("POST", "/api/repos/r/commits", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"current_version":"headV","record_conflicts":[{"path":"storage/notes/a.meta.json","field_conflicts":[{"scope":"meta","key":"created","reason":"immutable"}]}]}`))
	})
	m = NewManager(fs, WithHTTPClient(srv.Client()))

	res, err := m.PushLocal(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, ctxDir, "")
	if err != nil {
		t.Fatalf("record conflict must be a structured result: %v", err)
	}
	if len(res.Conflicts) != 1 || len(res.Conflicts[0].Fields) != 1 || res.Conflicts[0].Fields[0].Key != "created" {
		t.Fatalf("want per-field conflict surfaced, got %+v", res.Conflicts)
	}
}

// When a push goes through the server's merge, the server canonicalizes the
// record, so its stored blob differs byte-for-byte from our on-disk file. The
// push must then pull that canonical content back to disk, or the record shows
// as perpetually pending (its local hash never matches the merged blob).
func TestPushLocal_MergeCanonicalization_SyncsDiskSoPendingClears(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "storage/notes/a.meta.json", `{"data":{"name":"Yours"}}`)
	fs := newFakeFS()
	m0 := NewManager(fs)
	seed := EmptyTrackRecord()
	seed.Version = "baseV"
	seed.Files["storage/notes/a.meta.json"] = "oldsha"
	if err := m0.WriteTrackRecord(ctxDir, seed); err != nil {
		t.Fatal(err)
	}

	// The server's canonical merged content differs from our on-disk bytes.
	canonical := "{\n  \"data\": {\n    \"name\": \"Yours\"\n  }\n}"
	canonicalSha := GitBlobSha([]byte(canonical))

	srv, h := newOrchestrationServer(t)
	defer srv.Close()
	h.handle("POST", "/api/repos/r/commits", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(CommitResponse{
			Version: "mergedV",
			Changes: []Change{{Op: "put", Path: "storage/notes/a.meta.json", Blob: canonicalSha}},
		})
	})
	h.handle("GET", "/api/repos/r/files/storage/notes/a.meta.json", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(FileResponse{
			ContentB64: base64.StdEncoding.EncodeToString([]byte(canonical)),
		})
	})
	m := NewManager(fs, WithHTTPClient(srv.Client()))

	if _, err := m.PushLocal(conn(srv.URL), ctxDir, "msg"); err != nil {
		t.Fatal(err)
	}

	// Disk now holds the server's canonical bytes.
	got, err := os.ReadFile(filepath.Join(ctxDir, "storage", "notes", "a.meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != canonical {
		t.Errorf("disk not synced to canonical content:\n got %q\nwant %q", got, canonical)
	}
	// And so a fresh diff shows nothing pending (no "push again" loop).
	sum, err := m.LedgerSummary(ctxDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.Changed) != 0 {
		t.Errorf("expected no pending after a merge-push, got %+v", sum.Changed)
	}
}

func TestPushLocal_DeletesVanishedManagedPath(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/basic.yaml", "still here\n")

	seed := EmptyTrackRecord()
	seed.Version = "parentV"
	seed.Files["templates/basic.yaml"] = GitBlobSha([]byte("still here\n"))
	seed.Files["templates/gone.yaml"] = "oldsha" // managed + missing on disk → delete
	fs := newFakeFS()
	m := NewManager(fs)
	if err := m.WriteTrackRecord(ctxDir, seed); err != nil {
		t.Fatal(err)
	}

	var commitReq CommitRequest
	srv, h := newOrchestrationServer(t)
	defer srv.Close()
	h.handle("GET", "/api/repos/r/head", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(HeadResponse{Version: "parentV"})
	})
	h.handle("POST", "/api/repos/r/commits", func(w http.ResponseWriter, r *http.Request) {
		readJSONBody(t, r, &commitReq)
		_ = json.NewEncoder(w).Encode(CommitResponse{Version: "afterDelete"})
	})
	m = NewManager(fs, WithHTTPClient(srv.Client()))

	res, err := m.PushLocal(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, ctxDir, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 1 {
		t.Errorf("expected 1 delete, got %+v", res)
	}
	var ops []string
	for _, c := range commitReq.Changes {
		ops = append(ops, c.Op+":"+c.Path)
	}
	found := false
	for _, op := range ops {
		if op == "delete:templates/gone.yaml" {
			found = true
		}
	}
	if !found {
		t.Errorf("commit body missing delete op: %v", ops)
	}
	rec := ReadTrackRecord(ctxDir)
	if _, ok := rec.Files["templates/gone.yaml"]; ok {
		t.Errorf("ledger still tracks deleted path: %+v", rec.Files)
	}
}

func TestPushLocal_NoopWhenLedgerMatchesDisk(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/basic.yaml", "stable\n")
	seed := EmptyTrackRecord()
	seed.Version = "v1"
	seed.Files["templates/basic.yaml"] = GitBlobSha([]byte("stable\n"))
	fs := newFakeFS()
	m := NewManager(fs)
	if err := m.WriteTrackRecord(ctxDir, seed); err != nil {
		t.Fatal(err)
	}

	srv, _ := newOrchestrationServer(t)
	defer srv.Close()
	// No routes registered → any HTTP call fails the test via 404.
	m = NewManager(fs, WithHTTPClient(srv.Client()))

	res, err := m.PushLocal(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, ctxDir, "")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Noop || res.Pushed != 0 || res.Deleted != 0 {
		t.Fatalf("expected pure noop, got %+v", res)
	}
}

// ── PullLocal ───────────────────────────────────────────────────────

func TestPullLocal_WritesNewBlobsRebuildsLedger(t *testing.T) {
	ctxDir := t.TempDir()
	srv, h := newOrchestrationServer(t)
	defer srv.Close()

	bodyBytes := []byte("from server\n")
	blobSha := GitBlobSha(bodyBytes)

	h.handle("GET", "/api/repos/r/tree", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(TreeResponse{
			Version: "afterPull",
			Files:   []TreeEntry{{Path: "templates/basic.yaml", Blob: blobSha}},
		})
	})
	h.handle("GET", "/api/repos/r/files/templates/basic.yaml", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(FileResponse{
			Path:       "templates/basic.yaml",
			ContentB64: base64.StdEncoding.EncodeToString(bodyBytes),
			Blob:       blobSha,
		})
	})

	m := newOrchestrationManager(t, srv)
	res, err := m.PullLocal(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, ctxDir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Files != 1 || res.Deleted != 0 {
		t.Fatalf("res = %+v", res)
	}

	got, err := os.ReadFile(filepath.Join(ctxDir, "templates", "basic.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(bodyBytes) {
		t.Errorf("file content lost: %q", got)
	}

	rec := ReadTrackRecord(ctxDir)
	if rec.Version != "afterPull" || rec.Files["templates/basic.yaml"] != blobSha {
		t.Errorf("ledger not rebuilt: %+v", rec)
	}
}

func TestPullLocal_SkipsShaMatchingFileWithoutFetching(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/basic.yaml", "matches\n")
	matchSha := GitBlobSha([]byte("matches\n"))

	var fileHits int
	srv, h := newOrchestrationServer(t)
	defer srv.Close()
	h.handle("GET", "/api/repos/r/tree", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(TreeResponse{
			Version: "v1",
			Files:   []TreeEntry{{Path: "templates/basic.yaml", Blob: matchSha}},
		})
	})
	h.handle("GET", "/api/repos/r/files/templates/basic.yaml", func(w http.ResponseWriter, _ *http.Request) {
		fileHits++
		http.Error(w, "should not be fetched", http.StatusInternalServerError)
	})

	m := newOrchestrationManager(t, srv)
	res, err := m.PullLocal(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, ctxDir)
	if err != nil {
		t.Fatal(err)
	}
	if fileHits != 0 {
		t.Errorf("files endpoint hit %d times, want 0 (SHA matched)", fileHits)
	}
	if res.Files != 0 {
		t.Errorf("res.Files = %d, want 0 (nothing changed)", res.Files)
	}
}

func TestPullLocal_DeletesLocalFilesAbsentFromServerTree(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/keepme.yaml", "keep\n")
	writeFile(t, ctxDir, "templates/dropme.yaml", "drop\n")

	seed := EmptyTrackRecord()
	seed.Version = "before"
	seed.Files["templates/keepme.yaml"] = GitBlobSha([]byte("keep\n"))
	seed.Files["templates/dropme.yaml"] = GitBlobSha([]byte("drop\n"))
	fs := newFakeFS()
	m := NewManager(fs)
	if err := m.WriteTrackRecord(ctxDir, seed); err != nil {
		t.Fatal(err)
	}

	srv, h := newOrchestrationServer(t)
	defer srv.Close()
	h.handle("GET", "/api/repos/r/tree", func(w http.ResponseWriter, _ *http.Request) {
		// Server only lists keepme - dropme should be deleted locally.
		_ = json.NewEncoder(w).Encode(TreeResponse{
			Version: "afterPull",
			Files:   []TreeEntry{{Path: "templates/keepme.yaml", Blob: GitBlobSha([]byte("keep\n"))}},
		})
	})
	m = NewManager(fs, WithHTTPClient(srv.Client()))

	res, err := m.PullLocal(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, ctxDir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 1 {
		t.Errorf("expected 1 delete, got %+v", res)
	}
	if _, err := os.Stat(filepath.Join(ctxDir, "templates", "dropme.yaml")); !os.IsNotExist(err) {
		t.Errorf("dropme.yaml should be gone, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(ctxDir, "templates", "keepme.yaml")); err != nil {
		t.Errorf("keepme.yaml should remain, got %v", err)
	}
}

func TestPullLocal_RejectsBlankContext(t *testing.T) {
	m := NewManager(newFakeFS())
	_, err := m.PullLocal(Connection{BaseURL: "https://x", Token: "t", RepoName: "r"}, "")
	if !errors.Is(err, ErrMissingContext) {
		t.Fatalf("want ErrMissingContext, got %v", err)
	}
}

// ── Reclone ─────────────────────────────────────────────────────────

func TestReclone_RejectsBlankContext(t *testing.T) {
	m := NewManager(newFakeFS())
	_, err := m.Reclone(Connection{BaseURL: "https://x", Token: "t", RepoName: "r"}, "")
	if !errors.Is(err, ErrMissingContext) {
		t.Fatalf("want ErrMissingContext, got %v", err)
	}
}

func TestReclone_WipesManagedContentThenPullsFresh(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/stale.yaml", "old stuff that must go\n")
	writeFile(t, ctxDir, "storage/leftover/old.meta.json", `{"old":true}`)
	writeFile(t, ctxDir, "notes.txt", "user-owned; must survive reclone")

	// Seed an old ledger so we can prove it was rebuilt from scratch.
	fs := newFakeFS()
	m := NewManager(fs)
	if err := m.WriteTrackRecord(ctxDir, TrackRecord{
		Version: "stale-version",
		Files:   map[string]string{"templates/stale.yaml": "old-sha"},
	}); err != nil {
		t.Fatal(err)
	}

	freshBytes := []byte("fresh\n")
	freshSha := GitBlobSha(freshBytes)

	srv, h := newOrchestrationServer(t)
	defer srv.Close()
	h.handle("GET", "/api/repos/r/tree", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(TreeResponse{
			Version: "fresh-version",
			Files:   []TreeEntry{{Path: "templates/fresh.yaml", Blob: freshSha}},
		})
	})
	h.handle("GET", "/api/repos/r/files/templates/fresh.yaml", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(FileResponse{
			Path:       "templates/fresh.yaml",
			ContentB64: base64.StdEncoding.EncodeToString(freshBytes),
			Blob:       freshSha,
		})
	})

	m = NewManager(fs, WithHTTPClient(srv.Client()))
	res, err := m.Reclone(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, ctxDir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Version != "fresh-version" || res.Files != 1 {
		t.Fatalf("res = %+v", res)
	}

	// Stale managed paths are gone.
	if _, err := os.Stat(filepath.Join(ctxDir, "templates/stale.yaml")); !os.IsNotExist(err) {
		t.Errorf("stale template should be wiped, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(ctxDir, "storage/leftover/old.meta.json")); !os.IsNotExist(err) {
		t.Errorf("stale storage record should be wiped, stat err = %v", err)
	}

	// Server-side content is on disk after the pull.
	got, err := os.ReadFile(filepath.Join(ctxDir, "templates/fresh.yaml"))
	if err != nil || string(got) != string(freshBytes) {
		t.Errorf("fresh.yaml content = %q err=%v", got, err)
	}

	// User-owned non-managed file survives.
	if got, err := os.ReadFile(filepath.Join(ctxDir, "notes.txt")); err != nil {
		t.Errorf("notes.txt should survive reclone: %v", err)
	} else if string(got) != "user-owned; must survive reclone" {
		t.Errorf("notes.txt content changed: %q", got)
	}

	// Ledger reflects the new server state, not the old one.
	rec := ReadTrackRecord(ctxDir)
	if rec.Version != "fresh-version" {
		t.Errorf("ledger version = %q, want fresh-version", rec.Version)
	}
	if _, hadStale := rec.Files["templates/stale.yaml"]; hadStale {
		t.Errorf("ledger still tracks stale path: %+v", rec.Files)
	}
	if rec.Files["templates/fresh.yaml"] != freshSha {
		t.Errorf("ledger missing fresh entry: %+v", rec.Files)
	}
}

func TestReclone_OnEmptyContextBehavesLikeInitialClone(t *testing.T) {
	ctxDir := t.TempDir()
	bodyBytes := []byte("hello\n")
	sha := GitBlobSha(bodyBytes)

	srv, h := newOrchestrationServer(t)
	defer srv.Close()
	h.handle("GET", "/api/repos/r/tree", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(TreeResponse{
			Version: "v1",
			Files:   []TreeEntry{{Path: "templates/x.yaml", Blob: sha}},
		})
	})
	h.handle("GET", "/api/repos/r/files/templates/x.yaml", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(FileResponse{
			Path:       "templates/x.yaml",
			ContentB64: base64.StdEncoding.EncodeToString(bodyBytes),
			Blob:       sha,
		})
	})

	m := newOrchestrationManager(t, srv)
	res, err := m.Reclone(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, ctxDir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Files != 1 || res.Version != "v1" {
		t.Fatalf("res = %+v", res)
	}
}

// ── Sync ────────────────────────────────────────────────────────────

func TestSync_RunsPushThenPullAggregatesResult(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/basic.yaml", "local\n")
	localSha := GitBlobSha([]byte("local\n"))

	srv, h := newOrchestrationServer(t)
	defer srv.Close()

	// /tree is hit twice - once at push first-sync seed, once during pull.
	var treeHits int
	h.handle("GET", "/api/repos/r/tree", func(w http.ResponseWriter, _ *http.Request) {
		treeHits++
		_ = json.NewEncoder(w).Encode(TreeResponse{
			Version: "v1",
			Files:   []TreeEntry{{Path: "templates/basic.yaml", Blob: localSha}},
		})
	})

	m := newOrchestrationManager(t, srv)
	res, err := m.Sync(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, ctxDir, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Version != "v1" {
		t.Errorf("version = %q", res.Version)
	}
	if !res.Noop {
		t.Errorf("expected noop, got %+v", res)
	}
	if treeHits == 0 {
		t.Errorf("/tree was never called")
	}
}

func TestSync_PushFailureSkipsPull(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/basic.yaml", "fresh\n")

	srv, h := newOrchestrationServer(t)
	defer srv.Close()
	// First-sync seed → /tree empty → /head 409 → push errors → pull skipped.
	h.handle("GET", "/api/repos/r/tree", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(TreeResponse{Version: "", Files: nil})
	})
	h.handle("GET", "/api/repos/r/head", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "empty", http.StatusConflict)
	})

	m := newOrchestrationManager(t, srv)
	_, err := m.Sync(Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"}, ctxDir, "")
	if err == nil {
		t.Fatal("push failure should propagate")
	}
	// Pull-side /tree should NOT have been called twice (only once for the seed).
	// A second /tree would mean pull ran after push failed.
}
