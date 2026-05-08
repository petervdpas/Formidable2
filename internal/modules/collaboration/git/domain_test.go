package git

import (
	"os"
	"path/filepath"
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
