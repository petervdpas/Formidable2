package gigot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// User-supplied commit messages flow Service.PushLocal(message) →
// Manager.PushLocal(conn, ctx, message) → CommitRequest.Message
// untouched. An empty string falls back to the auto-generated audit
// string ("<who>: sync N file(s)\n- path") so existing call sites
// keep their previous behaviour.

func newCommitCapturingServer(t *testing.T, body *CommitRequest, version string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/repos/r/tree", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(TreeResponse{Version: "v0"})
	})
	mux.HandleFunc("/api/repos/r/head", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(HeadResponse{Version: "v0"})
	})
	mux.HandleFunc("/api/repos/r/commits", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(body)
		_ = json.NewEncoder(w).Encode(CommitResponse{Version: version})
	})
	return httptest.NewServer(mux)
}

// ── Manager-level ───────────────────────────────────────────────────

func TestManager_PushLocal_UserMessageOverridesAuto(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/basic.yaml", "fresh\n")

	var captured CommitRequest
	srv := newCommitCapturingServer(t, &captured, "v1")
	defer srv.Close()

	m := NewManager(newFakeFS(), WithHTTPClient(srv.Client()))
	_, err := m.PushLocal(
		Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"},
		ctxDir,
		"Refactor field labels",
	)
	if err != nil {
		t.Fatal(err)
	}
	if captured.Message != "Refactor field labels" {
		t.Errorf("commit message = %q, want %q", captured.Message, "Refactor field labels")
	}
}

func TestManager_PushLocal_EmptyMessageFallsBackToAuto(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/basic.yaml", "fresh\n")

	var captured CommitRequest
	srv := newCommitCapturingServer(t, &captured, "v1")
	defer srv.Close()

	m := NewManager(newFakeFS(), WithHTTPClient(srv.Client()))
	_, err := m.PushLocal(
		Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"},
		ctxDir,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if captured.Message == "" {
		t.Fatal("blank caller message must fall back to auto, got empty")
	}
	if !strings.Contains(captured.Message, "sync") {
		t.Errorf("auto-message should contain 'sync', got %q", captured.Message)
	}
	if !strings.Contains(captured.Message, "templates/basic.yaml") {
		t.Errorf("auto-message should list pushed paths, got %q", captured.Message)
	}
}

func TestManager_PushLocal_WhitespaceOnlyMessageFallsBackToAuto(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/basic.yaml", "fresh\n")

	var captured CommitRequest
	srv := newCommitCapturingServer(t, &captured, "v1")
	defer srv.Close()

	m := NewManager(newFakeFS(), WithHTTPClient(srv.Client()))
	_, err := m.PushLocal(
		Connection{BaseURL: srv.URL, Token: "t", RepoName: "r"},
		ctxDir,
		"   \n\t  ",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(captured.Message, "sync") {
		t.Errorf("whitespace-only must fall back to auto, got %q", captured.Message)
	}
}

// ── Service-level ───────────────────────────────────────────────────

func TestService_PushLocal_PassesUserMessage(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/basic.yaml", "fresh\n")

	var captured CommitRequest
	srv := newCommitCapturingServer(t, &captured, "v1")
	defer srv.Close()

	m := NewManager(newFakeFS(), WithHTTPClient(srv.Client()))
	cfg := &fakeConfig{baseURL: srv.URL, repoName: "r", context: ctxDir}
	creds := &fakeCreds{store: map[string]string{"default.json:gigot:r": "tok"}}
	profile := &fakeProfile{name: "default.json"}
	s := NewService(m, creds, profile, cfg, nil)

	if _, err := s.PushLocal("Fix label typo"); err != nil {
		t.Fatal(err)
	}
	if captured.Message != "Fix label typo" {
		t.Errorf("service did not forward message: got %q", captured.Message)
	}
}

func TestService_PushLocal_EmptyMessageFallsBackToAuto(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/basic.yaml", "fresh\n")

	var captured CommitRequest
	srv := newCommitCapturingServer(t, &captured, "v1")
	defer srv.Close()

	m := NewManager(newFakeFS(), WithHTTPClient(srv.Client()))
	cfg := &fakeConfig{baseURL: srv.URL, repoName: "r", context: ctxDir}
	creds := &fakeCreds{store: map[string]string{"default.json:gigot:r": "tok"}}
	profile := &fakeProfile{name: "default.json"}
	s := NewService(m, creds, profile, cfg, nil)

	if _, err := s.PushLocal(""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(captured.Message, "sync") {
		t.Errorf("service should fall back to auto on blank, got %q", captured.Message)
	}
}

func TestService_Sync_PassesUserMessageToPushHalf(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, "templates/basic.yaml", "fresh\n")

	var captured CommitRequest
	srv := newCommitCapturingServer(t, &captured, "vAfter")
	defer srv.Close()

	m := NewManager(newFakeFS(), WithHTTPClient(srv.Client()))
	cfg := &fakeConfig{baseURL: srv.URL, repoName: "r", context: ctxDir}
	creds := &fakeCreds{store: map[string]string{"default.json:gigot:r": "tok"}}
	profile := &fakeProfile{name: "default.json"}
	s := NewService(m, creds, profile, cfg, nil)

	if _, err := s.Sync("Daily catch-up"); err != nil {
		t.Fatal(err)
	}
	if captured.Message != "Daily catch-up" {
		t.Errorf("Sync did not forward message to push, got %q", captured.Message)
	}
}
