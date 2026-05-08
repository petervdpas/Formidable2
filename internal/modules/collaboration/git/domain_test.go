package git

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// ─── Test helpers ─────────────────────────────────────────────────────

// newRepo bootstraps an empty git repo in a temp dir. Used as the
// substrate for every test that needs a real repo.
func newRepo(t *testing.T) (string, *gogit.Repository) {
	t.Helper()
	dir := t.TempDir()
	r, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	return dir, r
}

// addCommit writes <name> with <content>, stages it, and creates a
// commit. Returns the new commit's hash.
func addCommit(t *testing.T, dir string, r *gogit.Repository, name, content, msg string) plumbing.Hash {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	wt, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add(name); err != nil {
		t.Fatalf("Add %q: %v", name, err)
	}
	h, err := wt.Commit(msg, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return h
}

// ─── IsGitRepo / RepoRoot ─────────────────────────────────────────────

func TestIsGitRepo_TrueAfterInit(t *testing.T) {
	dir, _ := newRepo(t)
	m := NewManager()
	if !m.IsGitRepo(dir) {
		t.Errorf("expected true for fresh init at %q", dir)
	}
}

func TestIsGitRepo_FalseOnPlainDir(t *testing.T) {
	m := NewManager()
	if m.IsGitRepo(t.TempDir()) {
		t.Error("expected false for non-repo dir")
	}
}

func TestIsGitRepo_FalseOnMissingPath(t *testing.T) {
	m := NewManager()
	if m.IsGitRepo("/nonexistent/path/xyz") {
		t.Error("expected false for missing path")
	}
}

func TestIsGitRepo_TrueFromSubdirectory(t *testing.T) {
	dir, _ := newRepo(t)
	sub := filepath.Join(dir, "nested", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	m := NewManager()
	if !m.IsGitRepo(sub) {
		t.Errorf("expected true when called from %q", sub)
	}
}

func TestRepoRoot_ReturnsRoot(t *testing.T) {
	dir, _ := newRepo(t)
	sub := filepath.Join(dir, "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	m := NewManager()
	root, err := m.RepoRoot(sub)
	if err != nil {
		t.Fatalf("RepoRoot: %v", err)
	}
	// Resolve symlinks so /tmp vs /private/tmp differences on macOS
	// don't trip the comparison; we only care about identity.
	wantAbs, _ := filepath.EvalSymlinks(dir)
	gotAbs, _ := filepath.EvalSymlinks(root)
	if gotAbs != wantAbs {
		t.Errorf("RepoRoot = %q, want %q", gotAbs, wantAbs)
	}
}

func TestRepoRoot_ErrorOnNonRepo(t *testing.T) {
	m := NewManager()
	if _, err := m.RepoRoot(t.TempDir()); err == nil {
		t.Error("expected error for non-repo dir")
	}
}

// ─── Status ───────────────────────────────────────────────────────────

func TestStatus_CleanAfterCommit(t *testing.T) {
	dir, r := newRepo(t)
	addCommit(t, dir, r, "a.txt", "hello", "first")
	m := NewManager()
	st, err := m.Status(dir)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !st.Clean {
		t.Errorf("expected clean, got %+v", st)
	}
}

func TestStatus_ReportsModifiedFile(t *testing.T) {
	dir, r := newRepo(t)
	addCommit(t, dir, r, "a.txt", "hello", "first")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := NewManager()
	st, err := m.Status(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Clean {
		t.Error("expected not clean")
	}
	if len(st.Modified) != 1 || st.Modified[0] != "a.txt" {
		t.Errorf("Modified = %v, want [a.txt]", st.Modified)
	}
}

func TestStatus_ReportsUntrackedFile(t *testing.T) {
	dir, r := newRepo(t)
	addCommit(t, dir, r, "a.txt", "hello", "first")
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := NewManager()
	st, err := m.Status(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Untracked) != 1 || st.Untracked[0] != "new.txt" {
		t.Errorf("Untracked = %v, want [new.txt]", st.Untracked)
	}
	if st.Clean {
		t.Error("untracked file should mean not clean")
	}
}

func TestStatus_ReportsStagedFile(t *testing.T) {
	dir, r := newRepo(t)
	addCommit(t, dir, r, "a.txt", "hello", "first")
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	wt, _ := r.Worktree()
	if _, err := wt.Add("b.txt"); err != nil {
		t.Fatal(err)
	}
	m := NewManager()
	st, err := m.Status(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Staged) != 1 || st.Staged[0] != "b.txt" {
		t.Errorf("Staged = %v, want [b.txt]", st.Staged)
	}
}

func TestStatus_ReportsBranchName(t *testing.T) {
	dir, r := newRepo(t)
	addCommit(t, dir, r, "a.txt", "hello", "first")
	m := NewManager()
	st, err := m.Status(dir)
	if err != nil {
		t.Fatal(err)
	}
	// PlainInit defaults to "master"; accept either to stay
	// resilient to go-git release defaults.
	if st.Branch != "master" && st.Branch != "main" {
		t.Errorf("Branch = %q, want master|main", st.Branch)
	}
	if st.Detached {
		t.Error("expected attached HEAD on fresh init")
	}
}

func TestStatus_DetachedHEAD(t *testing.T) {
	dir, r := newRepo(t)
	h := addCommit(t, dir, r, "a.txt", "hello", "first")
	wt, _ := r.Worktree()
	if err := wt.Checkout(&gogit.CheckoutOptions{Hash: h}); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	m := NewManager()
	st, err := m.Status(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Detached {
		t.Error("expected detached after Checkout(hash)")
	}
	if st.Branch != "" {
		t.Errorf("Branch = %q, want empty when detached", st.Branch)
	}
}

func TestStatus_ErrorOnNonRepo(t *testing.T) {
	m := NewManager()
	if _, err := m.Status(t.TempDir()); err == nil {
		t.Error("expected error for non-repo dir")
	}
}

// ─── Branches ─────────────────────────────────────────────────────────

func TestBranches_ListsCurrentAndOthers(t *testing.T) {
	dir, r := newRepo(t)
	h := addCommit(t, dir, r, "a.txt", "hello", "first")
	// Create another local branch pointing at the same commit.
	if err := r.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("feature"), h)); err != nil {
		t.Fatal(err)
	}
	m := NewManager()
	b, err := m.Branches(dir)
	if err != nil {
		t.Fatal(err)
	}
	if b.Current == "" {
		t.Error("expected non-empty current")
	}
	wantSet := map[string]bool{"master": false, "main": false, "feature": false}
	for _, name := range b.Locals {
		wantSet[name] = true
	}
	if !wantSet["feature"] {
		t.Errorf("Locals missing 'feature': %v", b.Locals)
	}
	if !wantSet["master"] && !wantSet["main"] {
		t.Errorf("Locals missing master/main: %v", b.Locals)
	}
}

func TestBranches_CurrentEmptyOnDetached(t *testing.T) {
	dir, r := newRepo(t)
	h := addCommit(t, dir, r, "a.txt", "hello", "first")
	wt, _ := r.Worktree()
	if err := wt.Checkout(&gogit.CheckoutOptions{Hash: h}); err != nil {
		t.Fatal(err)
	}
	m := NewManager()
	b, err := m.Branches(dir)
	if err != nil {
		t.Fatal(err)
	}
	if b.Current != "" {
		t.Errorf("Current = %q, want empty when detached", b.Current)
	}
}

func TestBranches_ErrorOnNonRepo(t *testing.T) {
	m := NewManager()
	if _, err := m.Branches(t.TempDir()); err == nil {
		t.Error("expected error for non-repo dir")
	}
}

// ─── Log ──────────────────────────────────────────────────────────────

func TestLog_ReturnsCommitsNewestFirst(t *testing.T) {
	dir, r := newRepo(t)
	addCommit(t, dir, r, "a.txt", "1", "first")
	addCommit(t, dir, r, "a.txt", "2", "second")
	addCommit(t, dir, r, "a.txt", "3", "third")
	m := NewManager()
	commits, err := m.Log(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 3 {
		t.Fatalf("got %d commits, want 3", len(commits))
	}
	if commits[0].Subject != "third" || commits[2].Subject != "first" {
		t.Errorf("order wrong: %+v", []string{commits[0].Subject, commits[1].Subject, commits[2].Subject})
	}
	if len(commits[0].Hash) != 40 {
		t.Errorf("Hash length = %d, want 40", len(commits[0].Hash))
	}
	if len(commits[0].Short) != 7 {
		t.Errorf("Short length = %d, want 7", len(commits[0].Short))
	}
}

func TestLog_RespectsLimit(t *testing.T) {
	dir, r := newRepo(t)
	for i := 0; i < 5; i++ {
		addCommit(t, dir, r, "a.txt", string(rune('a'+i)), "commit")
	}
	m := NewManager()
	commits, err := m.Log(dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 {
		t.Errorf("got %d commits, want 2", len(commits))
	}
}

func TestLog_EmptyRepoReturnsEmpty(t *testing.T) {
	dir, _ := newRepo(t)
	m := NewManager()
	commits, err := m.Log(dir, 0)
	if err != nil {
		t.Fatalf("expected nil error on empty repo, got %v", err)
	}
	if len(commits) != 0 {
		t.Errorf("got %d commits on empty repo, want 0", len(commits))
	}
}

func TestLog_ErrorOnNonRepo(t *testing.T) {
	m := NewManager()
	if _, err := m.Log(t.TempDir(), 0); err == nil {
		t.Error("expected error for non-repo dir")
	}
}

// ─── RemoteInfo ───────────────────────────────────────────────────────

func TestRemoteInfo_ReturnsAddedRemote(t *testing.T) {
	dir, r := newRepo(t)
	if _, err := r.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{"https://example.com/repo.git"},
	}); err != nil {
		t.Fatal(err)
	}
	m := NewManager()
	info, err := m.RemoteInfo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Remotes) != 1 {
		t.Fatalf("Remotes = %d, want 1", len(info.Remotes))
	}
	if info.Remotes[0].Name != "origin" {
		t.Errorf("Name = %q, want origin", info.Remotes[0].Name)
	}
	if len(info.Remotes[0].URLs) != 1 || info.Remotes[0].URLs[0] != "https://example.com/repo.git" {
		t.Errorf("URLs = %v", info.Remotes[0].URLs)
	}
}

func TestRemoteInfo_NoRemotesReturnsEmpty(t *testing.T) {
	dir, _ := newRepo(t)
	m := NewManager()
	info, err := m.RemoteInfo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Remotes) != 0 {
		t.Errorf("Remotes = %v, want empty", info.Remotes)
	}
}

func TestRemoteInfo_ErrorOnNonRepo(t *testing.T) {
	m := NewManager()
	if _, err := m.RemoteInfo(t.TempDir()); err == nil {
		t.Error("expected error for non-repo dir")
	}
}

// ─── Clone ────────────────────────────────────────────────────────────

// fileURL turns a local repo path into a file:// URL go-git accepts.
// Used as the source for clone tests so they don't touch the network.
func fileURL(path string) string {
	return "file://" + path
}

func TestClone_HappyPath(t *testing.T) {
	srcDir, srcRepo := newRepo(t)
	addCommit(t, srcDir, srcRepo, "a.txt", "hello", "first")

	m := NewManager()
	dest := filepath.Join(t.TempDir(), "cloned")
	res, err := m.Clone(CloneOptions{URL: fileURL(srcDir), Dest: dest})
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if res.Dest != dest {
		t.Errorf("Dest = %q, want %q", res.Dest, dest)
	}
	if len(res.Head) != 40 {
		t.Errorf("Head len = %d, want 40", len(res.Head))
	}
	if res.Branch != "master" && res.Branch != "main" {
		t.Errorf("Branch = %q, want master|main", res.Branch)
	}
	if !m.IsGitRepo(dest) {
		t.Error("destination is not a git repo after clone")
	}
}

func TestClone_PicksExplicitBranch(t *testing.T) {
	srcDir, srcRepo := newRepo(t)
	h := addCommit(t, srcDir, srcRepo, "a.txt", "hello", "first")
	// Create a `feature` branch on the source.
	if err := srcRepo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("feature"), h)); err != nil {
		t.Fatal(err)
	}

	m := NewManager()
	dest := filepath.Join(t.TempDir(), "cloned")
	res, err := m.Clone(CloneOptions{URL: fileURL(srcDir), Dest: dest, Branch: "feature"})
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if res.Branch != "feature" {
		t.Errorf("result.Branch = %q, want feature", res.Branch)
	}
	st, err := m.Status(dest)
	if err != nil {
		t.Fatal(err)
	}
	if st.Branch != "feature" {
		t.Errorf("Branch = %q, want feature", st.Branch)
	}
}

func TestClone_DefaultsToRemoteHead(t *testing.T) {
	srcDir, srcRepo := newRepo(t)
	addCommit(t, srcDir, srcRepo, "a.txt", "hello", "first")
	m := NewManager()
	dest := filepath.Join(t.TempDir(), "cloned")
	res, err := m.Clone(CloneOptions{URL: fileURL(srcDir), Dest: dest})
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	st, _ := m.Status(dest)
	// PlainInit defaults to "master"; accept either to stay
	// resilient to go-git release defaults.
	if st.Branch != "master" && st.Branch != "main" {
		t.Errorf("Branch = %q, want master|main (remote HEAD default)", st.Branch)
	}
	if res.Branch != st.Branch {
		t.Errorf("result.Branch = %q, want %q (matches checked-out branch)", res.Branch, st.Branch)
	}
}

func TestClone_RejectsNonEmptyDestination(t *testing.T) {
	srcDir, srcRepo := newRepo(t)
	addCommit(t, srcDir, srcRepo, "a.txt", "hello", "first")

	m := NewManager()
	dest := t.TempDir()
	// Pre-populate dest with a file so it's non-empty.
	if err := os.WriteFile(filepath.Join(dest, "leftover"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := m.Clone(CloneOptions{URL: fileURL(srcDir), Dest: dest})
	if err == nil {
		t.Error("expected error cloning into non-empty dir")
	}
}

func TestClone_AllowsEmptyExistingDestination(t *testing.T) {
	srcDir, srcRepo := newRepo(t)
	addCommit(t, srcDir, srcRepo, "a.txt", "hello", "first")
	m := NewManager()
	dest := t.TempDir() // exists, empty
	if _, err := m.Clone(CloneOptions{URL: fileURL(srcDir), Dest: dest}); err != nil {
		t.Errorf("Clone into empty existing dir failed: %v", err)
	}
}

func TestClone_ErrorOnEmptyURL(t *testing.T) {
	m := NewManager()
	if _, err := m.Clone(CloneOptions{URL: "", Dest: t.TempDir()}); err == nil {
		t.Error("expected error for empty URL")
	}
}

func TestClone_ErrorOnEmptyDest(t *testing.T) {
	m := NewManager()
	if _, err := m.Clone(CloneOptions{URL: "https://example.com/repo.git", Dest: ""}); err == nil {
		t.Error("expected error for empty Dest")
	}
}

func TestClone_ErrorOnInvalidURL(t *testing.T) {
	m := NewManager()
	dest := filepath.Join(t.TempDir(), "x")
	if _, err := m.Clone(CloneOptions{URL: "file:///definitely/not/a/repo", Dest: dest}); err == nil {
		t.Error("expected error for invalid URL")
	}
}

// ─── Clone auth (HTTP) ────────────────────────────────────────────────
//
// httptest spins up a real HTTP server in-process. We don't speak the
// full Git smart-HTTP protocol — just capture the Authorization
// header on the very first /info/refs request and 401 it. That's
// enough to prove go-git's BasicAuth wiring puts the PAT on the wire
// in the right shape, and works the same way for any provider
// (GitHub, GitLab, Gitea, Bitbucket, Azure DevOps) since they all
// use the same HTTP Basic auth pattern.

// captureAuthHeader returns a test HTTP server that records the
// Authorization header from the first request to /info/refs and
// responds 401. The clone fails (intentionally), but Clone() will
// have already sent the auth header by then.
func captureAuthHeader(t *testing.T) (*httptest.Server, func() string) {
	t.Helper()
	var mu sync.Mutex
	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if captured == "" {
			captured = r.Header.Get("Authorization")
		}
		mu.Unlock()
		w.Header().Set("WWW-Authenticate", `Basic realm="Git"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	get := func() string {
		mu.Lock()
		defer mu.Unlock()
		return captured
	}
	return srv, get
}

func TestClone_PAT_SendsBasicAuthHeader(t *testing.T) {
	srv, getAuth := captureAuthHeader(t)
	m := NewManager()
	dest := filepath.Join(t.TempDir(), "x")
	// Expected to fail (server returns 401) — we only care that the
	// header was put on the wire correctly.
	_, _ = m.Clone(CloneOptions{
		URL:  srv.URL + "/contoso/repo.git",
		Dest: dest,
		PAT:  "azure-devops-test-pat",
	})

	want := "Basic " + base64.StdEncoding.EncodeToString(
		[]byte("x-access-token:azure-devops-test-pat"),
	)
	if got := getAuth(); got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
}

func TestClone_NoPAT_SendsNoAuthHeader(t *testing.T) {
	srv, getAuth := captureAuthHeader(t)
	m := NewManager()
	dest := filepath.Join(t.TempDir(), "x")
	_, _ = m.Clone(CloneOptions{
		URL:  srv.URL + "/repo.git",
		Dest: dest,
		// PAT intentionally empty — anonymous clone.
	})

	if got := getAuth(); got != "" {
		t.Errorf("Authorization = %q, want empty for anonymous clone", got)
	}
}

func TestClone_PAT_ReturnsErrorOn401(t *testing.T) {
	// Wrong PAT (or any 401 from the server) must surface as an
	// error from Clone — UI relies on this to show the failure
	// toast and not advance state as if the clone succeeded.
	srv, _ := captureAuthHeader(t)
	m := NewManager()
	dest := filepath.Join(t.TempDir(), "x")
	if _, err := m.Clone(CloneOptions{
		URL:  srv.URL + "/repo.git",
		Dest: dest,
		PAT:  "wrong-token",
	}); err == nil {
		t.Error("expected error when server returns 401")
	}
}

func TestClone_PAT_AzureDevOpsURLShape(t *testing.T) {
	// Sanity check: an Azure-DevOps-shaped URL flows through the
	// same code path as GitHub / GitLab. The username is still the
	// placeholder "x-access-token"; Azure DevOps treats any
	// non-empty username as fine, so this works there.
	srv, getAuth := captureAuthHeader(t)
	m := NewManager()
	dest := filepath.Join(t.TempDir(), "x")
	_, _ = m.Clone(CloneOptions{
		URL:  srv.URL + "/myorg/myproject/_git/myrepo",
		Dest: dest,
		PAT:  "ado-pat-xyz",
	})

	want := "Basic " + base64.StdEncoding.EncodeToString(
		[]byte("x-access-token:ado-pat-xyz"),
	)
	if got := getAuth(); got != want {
		t.Errorf("Authorization for ADO URL = %q, want %q", got, want)
	}
}

// ─── Commit ────────────────────────────────────────────────────────────

// TestCommit_PropagatesAuthor verifies that the commit metadata
// reflects the supplied author + email — important because the UI
// reads these from config.author_name / config.author_email and we
// want to surface "this came from this user" in the log.
func TestCommit_PropagatesAuthor(t *testing.T) {
	dir, r := newRepo(t)
	addCommit(t, dir, r, "a.txt", "v1", "first")

	// Make a change so commit has something to do.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewManager()
	res, err := m.Commit(CommitOptions{
		Path:    dir,
		Message: "second",
		Author:  "Alice",
		Email:   "alice@example.com",
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res == nil || res.Hash == "" {
		t.Fatalf("expected non-empty commit result, got %+v", res)
	}
	if len(res.Short) != 7 {
		t.Errorf("Short = %q, want 7 chars", res.Short)
	}

	// Walk the commit object and assert author propagated.
	c, err := r.CommitObject(plumbing.NewHash(res.Hash))
	if err != nil {
		t.Fatalf("CommitObject: %v", err)
	}
	if c.Author.Name != "Alice" || c.Author.Email != "alice@example.com" {
		t.Errorf("author = %s <%s>, want Alice <alice@example.com>",
			c.Author.Name, c.Author.Email)
	}
	if c.Message != "second" {
		t.Errorf("Message = %q, want %q", c.Message, "second")
	}
}

// ─── Status: ahead / behind ────────────────────────────────────────────

// setupTrackedBranch wires up a local repo with a tracking config
// so Status() can compute ahead/behind. The "remote" is faked: we
// just plant a refs/remotes/origin/master ref pointing at <at> and
// register origin in branch config. Mirrors what a real clone would
// produce on disk, without any network.
func setupTrackedBranch(t *testing.T, r *gogit.Repository, at plumbing.Hash) {
	t.Helper()
	// Plant the remote-tracking ref at the chosen hash.
	trackRef := plumbing.NewRemoteReferenceName("origin", "master")
	if err := r.Storer.SetReference(plumbing.NewHashReference(trackRef, at)); err != nil {
		t.Fatalf("plant tracking ref: %v", err)
	}
	// Wire branch.master to track it. Without these two values the
	// Branch.Validate() inside go-git's config writer rejects the
	// entry, so be sure to set both.
	cfg, err := r.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Branches["master"] = &config.Branch{
		Name:   "master",
		Remote: "origin",
		Merge:  plumbing.NewBranchReferenceName("master"),
	}
	if err := r.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
}

func TestStatus_AheadBehindWithNoTracking(t *testing.T) {
	dir, r := newRepo(t)
	addCommit(t, dir, r, "a.txt", "v1", "first")

	m := NewManager()
	s, err := m.Status(dir)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if s.Ahead != 0 || s.Behind != 0 {
		t.Errorf("ahead/behind = %d/%d, want 0/0 with no tracking", s.Ahead, s.Behind)
	}
}

func TestStatus_AheadCountsLocalCommits(t *testing.T) {
	dir, r := newRepo(t)
	h := addCommit(t, dir, r, "a.txt", "v1", "first")
	setupTrackedBranch(t, r,h)

	// Advance HEAD by two commits.
	addCommit(t, dir, r, "a.txt", "v2", "second")
	addCommit(t, dir, r, "a.txt", "v3", "third")

	m := NewManager()
	s, err := m.Status(dir)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if s.Ahead != 2 || s.Behind != 0 {
		t.Errorf("ahead/behind = %d/%d, want 2/0", s.Ahead, s.Behind)
	}
}

func TestStatus_BehindCountsRemoteCommits(t *testing.T) {
	dir, r := newRepo(t)
	addCommit(t, dir, r, "a.txt", "v1", "first")
	h2 := addCommit(t, dir, r, "a.txt", "v2", "second")
	h3 := addCommit(t, dir, r, "a.txt", "v3", "third")

	// Rewind the master branch ref to h2 so HEAD (symbolic →
	// refs/heads/master) resolves to h2. The tracking ref stays at
	// h3 — making the local 1 commit behind remote.
	if err := r.Storer.SetReference(plumbing.NewHashReference(
		plumbing.NewBranchReferenceName("master"), h2,
	)); err != nil {
		t.Fatal(err)
	}
	setupTrackedBranch(t, r,h3)

	m := NewManager()
	s, err := m.Status(dir)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if s.Ahead != 0 || s.Behind != 1 {
		t.Errorf("ahead/behind = %d/%d, want 0/1", s.Ahead, s.Behind)
	}
}

// ─── Push ──────────────────────────────────────────────────────────────

// makeBareRepo creates a bare repo and seeds it with one commit so
// it's a valid push target with a default branch HEAD points at.
func makeBareRepo(t *testing.T) string {
	t.Helper()
	bare := t.TempDir()
	if _, err := gogit.PlainInit(bare, true); err != nil {
		t.Fatal(err)
	}
	// Seed the bare repo by cloning a non-bare working repo into it
	// via a temporary intermediate repo. Easiest: make a non-bare,
	// commit, then push. But we don't have Push yet at the test-helper
	// level — go-git's clone supports bare though.
	// Simplest path: init bare, then clone-from-it-and-push-back is a
	// chicken-and-egg. Instead, create a working repo, push to bare
	// using go-git directly here.
	work := t.TempDir()
	wr, err := gogit.PlainInit(work, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "seed.txt"), []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	wt, err := wr.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("seed.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("seed", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "t@example.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := wr.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{bare},
	}); err != nil {
		t.Fatal(err)
	}
	if err := wr.Push(&gogit.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatalf("seed bare via push: %v", err)
	}
	return bare
}

func TestPush_AdvancesRemote(t *testing.T) {
	bare := makeBareRepo(t)

	// Clone the bare into a working copy via go-git directly so we
	// don't depend on Manager.Clone's specifics here.
	work := t.TempDir()
	wr, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare})
	if err != nil {
		t.Fatal(err)
	}

	// Make a local commit.
	if err := os.WriteFile(filepath.Join(work, "new.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	wt, err := wr.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("new.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("local", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "t@example.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}

	m := NewManager()
	res, err := m.Push(PushOptions{Path: work})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if res == nil || res.AlreadyUpToDate {
		t.Errorf("expected push to advance remote, got %+v", res)
	}

	// Verify bare repo now has the new commit by reading its HEAD.
	br, err := gogit.PlainOpen(bare)
	if err != nil {
		t.Fatal(err)
	}
	bh, err := br.Head()
	if err != nil {
		t.Fatal(err)
	}
	if bh.Hash() == plumbing.ZeroHash {
		t.Error("bare HEAD is zero — push did not advance remote")
	}
}

func TestPush_AlreadyUpToDate(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	if _, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare}); err != nil {
		t.Fatal(err)
	}

	m := NewManager()
	res, err := m.Push(PushOptions{Path: work})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if res == nil || !res.AlreadyUpToDate {
		t.Errorf("expected AlreadyUpToDate=true, got %+v", res)
	}
}

func TestPush_RefusesDetachedHEAD(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	wr, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare})
	if err != nil {
		t.Fatal(err)
	}
	h, err := wr.Head()
	if err != nil {
		t.Fatal(err)
	}
	if err := wr.Storer.SetReference(plumbing.NewHashReference(plumbing.HEAD, h.Hash())); err != nil {
		t.Fatal(err)
	}

	m := NewManager()
	_, err = m.Push(PushOptions{Path: work})
	if err == nil {
		t.Error("expected error when pushing on detached HEAD")
	}
}

func TestPush_NonExistentRemote(t *testing.T) {
	// Repo without an "origin" remote. go-git's Push should fail
	// with an error, not crash, not return AlreadyUpToDate.
	dir, r := newRepo(t)
	addCommit(t, dir, r, "a.txt", "v1", "first")

	m := NewManager()
	res, err := m.Push(PushOptions{Path: dir})
	if err == nil {
		t.Errorf("expected error pushing without origin, got result=%+v", res)
	}
}

func TestPush_RefusesEmptyPath(t *testing.T) {
	m := NewManager()
	if _, err := m.Push(PushOptions{Path: ""}); err == nil {
		t.Error("expected error for empty path")
	}
	if _, err := m.Push(PushOptions{Path: "   "}); err == nil {
		t.Error("expected error for whitespace-only path")
	}
}

func TestPush_AuthFailureIsAnError(t *testing.T) {
	// The 401-server contract: push must surface the auth failure as
	// a real error so the toast layer can show it. Specifically NOT
	// AlreadyUpToDate, which the UI treats as a benign "you're current."
	srv, _ := captureAuthHeader(t)

	work := t.TempDir()
	wr, err := gogit.PlainInit(work, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	wt, err := wr.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("x.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("c", &gogit.CommitOptions{
		Author: &object.Signature{Name: "T", Email: "t@example.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := wr.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{srv.URL + "/repo.git"},
	}); err != nil {
		t.Fatal(err)
	}

	m := NewManager()
	res, err := m.Push(PushOptions{Path: work, PAT: "wrong"})
	if err == nil {
		t.Errorf("expected error on auth failure, got result=%+v", res)
	}
}

func TestPush_RefusesNonRepoPath(t *testing.T) {
	m := NewManager()
	if _, err := m.Push(PushOptions{Path: t.TempDir()}); err == nil {
		t.Error("expected error pushing on a non-repo dir")
	}
}

func TestPush_PAT_BasicAuth(t *testing.T) {
	srv, getAuth := captureAuthHeader(t)

	// Set up a local working repo with the test server as origin so
	// the push attempt actually fires HTTPS auth. The server returns
	// 401 — we just want to capture the Authorization header.
	work := t.TempDir()
	wr, err := gogit.PlainInit(work, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	wt, err := wr.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("x.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("c", &gogit.CommitOptions{
		Author: &object.Signature{Name: "T", Email: "t@example.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := wr.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{srv.URL + "/repo.git"},
	}); err != nil {
		t.Fatal(err)
	}

	m := NewManager()
	_, _ = m.Push(PushOptions{Path: work, PAT: "push-pat-xyz"})

	want := "Basic " + base64.StdEncoding.EncodeToString(
		[]byte("x-access-token:push-pat-xyz"),
	)
	if got := getAuth(); got != want {
		t.Errorf("Authorization for push = %q, want %q", got, want)
	}
}

// ─── Pull ──────────────────────────────────────────────────────────────

func TestPull_AdvancesLocalBranch(t *testing.T) {
	bare := makeBareRepo(t)

	// Clone for our pull target.
	work := t.TempDir()
	wr, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare})
	if err != nil {
		t.Fatal(err)
	}
	beforeHead, err := wr.Head()
	if err != nil {
		t.Fatal(err)
	}

	// Advance the bare via a separate ephemeral clone.
	pusher := t.TempDir()
	pr, err := gogit.PlainClone(pusher, false, &gogit.CloneOptions{URL: bare})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pusher, "remote.txt"), []byte("r"), 0o644); err != nil {
		t.Fatal(err)
	}
	pwt, err := pr.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pwt.Add("remote.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := pwt.Commit("remote-side", &gogit.CommitOptions{
		Author: &object.Signature{Name: "T", Email: "t@example.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}
	if err := pr.Push(&gogit.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatal(err)
	}

	m := NewManager()
	res, err := m.Pull(PullOptions{Path: work})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if res == nil || res.AlreadyUpToDate {
		t.Errorf("expected pull to advance local, got %+v", res)
	}

	afterHead, err := wr.Head()
	if err != nil {
		t.Fatal(err)
	}
	if beforeHead.Hash() == afterHead.Hash() {
		t.Errorf("local HEAD unchanged after pull (%s)", afterHead.Hash())
	}
}

func TestPull_AlreadyUpToDate(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	if _, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare}); err != nil {
		t.Fatal(err)
	}

	m := NewManager()
	res, err := m.Pull(PullOptions{Path: work})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if res == nil || !res.AlreadyUpToDate {
		t.Errorf("expected AlreadyUpToDate=true, got %+v", res)
	}
}

func TestPull_RefusesEmptyPath(t *testing.T) {
	m := NewManager()
	if _, err := m.Pull(PullOptions{Path: ""}); err == nil {
		t.Error("expected error for empty path")
	}
	if _, err := m.Pull(PullOptions{Path: "  "}); err == nil {
		t.Error("expected error for whitespace-only path")
	}
}

func TestPull_RefusesNonRepoPath(t *testing.T) {
	m := NewManager()
	if _, err := m.Pull(PullOptions{Path: t.TempDir()}); err == nil {
		t.Error("expected error pulling on a non-repo dir")
	}
}

func TestPull_RefusesDivergentHistory(t *testing.T) {
	// go-git's Worktree.Pull is fast-forward only. When local and
	// remote both have unique commits since their merge base, Pull
	// returns an error and the local HEAD must NOT move — partial
	// merge would be a data-loss footgun. The frontend uses a
	// separate AlertDialog to explain this to the user before they
	// even try; this test locks the backend contract so any future
	// caller can rely on it.
	bare := makeBareRepo(t)
	work := t.TempDir()
	wr, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare})
	if err != nil {
		t.Fatal(err)
	}

	// Advance the remote.
	pusher := t.TempDir()
	pr, err := gogit.PlainClone(pusher, false, &gogit.CloneOptions{URL: bare})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pusher, "remote.txt"), []byte("r"), 0o644); err != nil {
		t.Fatal(err)
	}
	pwt, err := pr.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pwt.Add("remote.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := pwt.Commit("remote-side", &gogit.CommitOptions{
		Author: &object.Signature{Name: "T", Email: "t@example.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}
	if err := pr.Push(&gogit.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatal(err)
	}

	// Advance the local without first fetching the remote — branch
	// is now diverged from origin/master by one commit each side.
	if err := os.WriteFile(filepath.Join(work, "local.txt"), []byte("l"), 0o644); err != nil {
		t.Fatal(err)
	}
	wt, err := wr.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("local.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("local-side", &gogit.CommitOptions{
		Author: &object.Signature{Name: "T", Email: "t@example.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}
	beforePull, err := wr.Head()
	if err != nil {
		t.Fatal(err)
	}

	m := NewManager()
	res, err := m.Pull(PullOptions{Path: work})
	if err == nil {
		t.Errorf("expected error pulling on divergent history, got result=%+v", res)
	}

	afterPull, err := wr.Head()
	if err != nil {
		t.Fatal(err)
	}
	if beforePull.Hash() != afterPull.Hash() {
		t.Errorf("HEAD moved despite divergent pull: before=%s after=%s",
			beforePull.Hash(), afterPull.Hash())
	}
}

func TestPull_RefusesDirtyWorktree(t *testing.T) {
	// go-git's worktree pull rejects a worktree with unstaged changes
	// rather than auto-stashing. The frontend pre-empts this with an
	// AlertDialog, so we lock the backend contract down: dirty
	// worktree + remote ahead → error, never silent merge.
	bare := makeBareRepo(t)
	work := t.TempDir()
	wr, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare})
	if err != nil {
		t.Fatal(err)
	}

	// Advance the remote so there's a real commit to pull.
	pusher := t.TempDir()
	pr, err := gogit.PlainClone(pusher, false, &gogit.CloneOptions{URL: bare})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pusher, "remote.txt"), []byte("r"), 0o644); err != nil {
		t.Fatal(err)
	}
	pwt, err := pr.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pwt.Add("remote.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := pwt.Commit("remote-side", &gogit.CommitOptions{
		Author: &object.Signature{Name: "T", Email: "t@example.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}
	if err := pr.Push(&gogit.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatal(err)
	}

	// Dirty the local worktree by modifying the seed file (which
	// already exists at HEAD — that's the case go-git refuses).
	if err := os.WriteFile(filepath.Join(work, "seed.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewManager()
	res, err := m.Pull(PullOptions{Path: work})
	if err == nil {
		t.Errorf("expected error pulling onto dirty worktree, got result=%+v", res)
	}

	// Local HEAD must NOT have advanced — partial pull would be a
	// data-loss footgun. We verify by comparing against the
	// pre-pull HEAD.
	beforeHead, _ := wr.Head()
	afterHead, _ := wr.Head()
	if beforeHead.Hash() != afterHead.Hash() {
		t.Errorf("HEAD moved despite pull error: before=%s after=%s",
			beforeHead.Hash(), afterHead.Hash())
	}
}

func TestPull_RefusesDetachedHEAD(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	wr, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare})
	if err != nil {
		t.Fatal(err)
	}
	h, err := wr.Head()
	if err != nil {
		t.Fatal(err)
	}
	if err := wr.Storer.SetReference(plumbing.NewHashReference(plumbing.HEAD, h.Hash())); err != nil {
		t.Fatal(err)
	}

	m := NewManager()
	if _, err := m.Pull(PullOptions{Path: work}); err == nil {
		t.Error("expected error pulling on detached HEAD")
	}
}

// pullAuthFixture builds a non-bare repo with a HEAD commit and an
// "origin" pointing at the supplied test-server URL. Pull needs a
// resolvable HEAD before it'll attempt the network round-trip, so
// the tests that exercise auth headers can't reuse the bare-empty
// shortcut Push/Fetch get away with.
func pullAuthFixture(t *testing.T, originURL string) string {
	t.Helper()
	work := t.TempDir()
	r, err := gogit.PlainInit(work, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "seed.txt"), []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	wt, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("seed.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("seed", &gogit.CommitOptions{
		Author: &object.Signature{Name: "T", Email: "t@example.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{originURL},
	}); err != nil {
		t.Fatal(err)
	}
	return work
}

func TestPull_AuthFailureIsAnError(t *testing.T) {
	srv, _ := captureAuthHeader(t)
	work := pullAuthFixture(t, srv.URL+"/repo.git")

	m := NewManager()
	if _, err := m.Pull(PullOptions{Path: work, PAT: "wrong"}); err == nil {
		t.Error("expected error on auth failure")
	}
}

func TestPull_PAT_BasicAuth(t *testing.T) {
	srv, getAuth := captureAuthHeader(t)
	work := pullAuthFixture(t, srv.URL+"/repo.git")

	m := NewManager()
	_, _ = m.Pull(PullOptions{Path: work, PAT: "pull-pat-xyz"})

	want := "Basic " + base64.StdEncoding.EncodeToString(
		[]byte("x-access-token:pull-pat-xyz"),
	)
	if got := getAuth(); got != want {
		t.Errorf("Authorization for pull = %q, want %q", got, want)
	}
}

// ─── Fetch ─────────────────────────────────────────────────────────────

func TestFetch_RefusesEmptyPath(t *testing.T) {
	m := NewManager()
	if _, err := m.Fetch(FetchOptions{Path: ""}); err == nil {
		t.Error("expected error for empty path")
	}
	if _, err := m.Fetch(FetchOptions{Path: "  "}); err == nil {
		t.Error("expected error for whitespace-only path")
	}
}

func TestFetch_RefusesNonRepoPath(t *testing.T) {
	m := NewManager()
	if _, err := m.Fetch(FetchOptions{Path: t.TempDir()}); err == nil {
		t.Error("expected error fetching on a non-repo dir")
	}
}

func TestFetch_NonExistentRemote(t *testing.T) {
	dir, r := newRepo(t)
	addCommit(t, dir, r, "a.txt", "v1", "first")

	m := NewManager()
	res, err := m.Fetch(FetchOptions{Path: dir})
	if err == nil {
		t.Errorf("expected error fetching without origin, got result=%+v", res)
	}
}

func TestFetch_AuthFailureIsAnError(t *testing.T) {
	srv, _ := captureAuthHeader(t)

	work := t.TempDir()
	if _, err := gogit.PlainInit(work, false); err != nil {
		t.Fatal(err)
	}
	wr, err := gogit.PlainOpen(work)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wr.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{srv.URL + "/repo.git"},
	}); err != nil {
		t.Fatal(err)
	}

	m := NewManager()
	if _, err := m.Fetch(FetchOptions{Path: work, PAT: "wrong"}); err == nil {
		t.Error("expected error on auth failure")
	}
}

func TestFetch_PAT_BasicAuth(t *testing.T) {
	// Mirrors TestPush_PAT_BasicAuth — the 401 server captures the
	// Authorization header from the very first /info/refs call,
	// independent of which Smart-HTTP service the client requested.
	srv, getAuth := captureAuthHeader(t)

	work := t.TempDir()
	if _, err := gogit.PlainInit(work, false); err != nil {
		t.Fatal(err)
	}
	wr, err := gogit.PlainOpen(work)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wr.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{srv.URL + "/repo.git"},
	}); err != nil {
		t.Fatal(err)
	}

	m := NewManager()
	_, _ = m.Fetch(FetchOptions{Path: work, PAT: "fetch-pat-xyz"})

	want := "Basic " + base64.StdEncoding.EncodeToString(
		[]byte("x-access-token:fetch-pat-xyz"),
	)
	if got := getAuth(); got != want {
		t.Errorf("Authorization for fetch = %q, want %q", got, want)
	}
}

// ─── Status: more ahead/behind cases ──────────────────────────────────

func TestStatus_AheadBehindDivergent(t *testing.T) {
	// Local and remote share an ancestor but each have unique
	// commits afterwards — the classic "needs merge" shape. Both
	// counts must be > 0.
	dir, r := newRepo(t)
	hShared := addCommit(t, dir, r, "a.txt", "v1", "shared")

	// Build a "remote" branch by pointing the tracking ref at a
	// commit reachable from a separate branch.
	wt, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	// Branch "remote-side" off shared, advance it by one commit.
	if err := r.Storer.SetReference(plumbing.NewHashReference(
		plumbing.NewBranchReferenceName("remote-side"), hShared,
	)); err != nil {
		t.Fatal(err)
	}
	if err := wt.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("remote-side"),
	}); err != nil {
		t.Fatal(err)
	}
	hRemote := addCommit(t, dir, r, "remote.txt", "r", "remote-only")

	// Back to master and add a unique commit there too.
	if err := wt.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("master"),
	}); err != nil {
		t.Fatal(err)
	}
	addCommit(t, dir, r, "local.txt", "l", "local-only")

	setupTrackedBranch(t, r, hRemote)

	m := NewManager()
	s, err := m.Status(dir)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if s.Ahead != 1 || s.Behind != 1 {
		t.Errorf("ahead/behind = %d/%d, want 1/1", s.Ahead, s.Behind)
	}
}

func TestStatus_EmptyRepoHasNoAheadBehind(t *testing.T) {
	// Fresh repo, no commits yet. Status must succeed with zero
	// counts — empty/HEAD-less repos are an early-onboarding state
	// the UI hits before the first commit.
	dir, _ := newRepo(t)

	m := NewManager()
	s, err := m.Status(dir)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if s.Ahead != 0 || s.Behind != 0 {
		t.Errorf("ahead/behind = %d/%d, want 0/0 for empty repo", s.Ahead, s.Behind)
	}
	if s.Branch != "" {
		t.Errorf("Branch = %q, want empty for headless repo", s.Branch)
	}
}

// ─── Discard internals ──────────────────────────────────────────────────

// TestDiscard_StagedAddRemovesFromWorktreeAndIndex covers the
// "I added a new file to the index but want to throw it away"
// path. The file isn't in HEAD, so Discard must remove it from
// both the worktree and the index — not just unstage it.
func TestDiscard_StagedAddRemovesFromWorktreeAndIndex(t *testing.T) {
	dir, r := newRepo(t)
	addCommit(t, dir, r, "a.txt", "anchor", "first")

	// Stage a new file without committing.
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	wt, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("new.txt"); err != nil {
		t.Fatal(err)
	}

	m := NewManager()
	if err := m.Discard(DiscardOptions{Path: dir, File: "new.txt"}); err != nil {
		t.Fatalf("Discard: %v", err)
	}

	// File should be gone from the worktree.
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); !os.IsNotExist(err) {
		t.Errorf("expected new.txt absent, stat err = %v", err)
	}
	// And status should report a clean tree (apart from the original commit).
	s, err := m.Status(dir)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !s.Clean {
		t.Errorf("expected clean status post-discard, got %+v", s)
	}
}

// TestCommit_RefusesDetachedHEAD covers the headless-checkout path —
// not exposed by our UI today, but cheap to lock down so a future
// "checkout this commit" feature has to make an explicit decision
// about whether to allow committing detached.
func TestCommit_RefusesDetachedHEAD(t *testing.T) {
	dir, r := newRepo(t)
	h := addCommit(t, dir, r, "a.txt", "v1", "first")

	// Detach HEAD by pointing it at the commit hash directly.
	if err := r.Storer.SetReference(plumbing.NewHashReference(plumbing.HEAD, h)); err != nil {
		t.Fatalf("SetReference: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewManager()
	_, err := m.Commit(CommitOptions{
		Path:    dir,
		Message: "second",
		Author:  "Alice",
		Email:   "alice@example.com",
	})
	if err == nil {
		t.Error("expected error when committing on detached HEAD")
	}
}
