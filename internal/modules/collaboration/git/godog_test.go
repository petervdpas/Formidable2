package git

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cucumber/godog"
	gogit "github.com/go-git/go-git/v5"
	gogitcfg "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/petervdpas/formidable2/internal/modules/journal"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initGitScenario,
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

// gitWorld holds per-scenario state. Reset in Before; everything in
// here is owned by exactly one scenario at a time.
type gitWorld struct {
	tmp string
	m   *Manager

	// Service + fakeJournal for the wiring scenarios. Nil unless a
	// scenario opens with "a journal-recording git service".
	svc     *Service
	jrnl    *fakeJournal
	gitRoot *fakeRoot

	// Whatever the most recent operation produced.
	status   *Status
	branches *Branches
	log      []Commit
	clone    *CloneResult
	commit   *CommitResult
	push     *PushResult
	pull     *PullResult
	pullStash *StashedPullResult
	fetch    *FetchResult
	repoRoot string
	boolRes  bool
	lastErr  error

	// Bare repo path used by push/fetch scenarios.
	bareDir string

	// Source repo for clone scenarios (created via "a source repo
	// with a commit").
	srcDir string

	// HTTP test server bits - used by the wire-level auth scenarios.
	authServer    *httptest.Server
	capturedAuthM sync.Mutex
	capturedAuth  string

	// Sysgit dispatch wiring - populated by the "fake sysgit
	// recorder" steps so scenarios can prove the Service routed
	// through the shell-out surface (or didn't).
	fakeSys   *fakeSysgit
	fakeFlags *fakeFlags
}

// wireFakeSysgit attaches a fake Sysgit + FlagReader to the Service
// in the gitWorld. The Service must already exist (declare "a
// journal-recording git service" before the fake-sysgit step).
// flag default is OFF so individual scenarios can flip it with the
// "the self-cloned toggle is on" step - keeps each scenario's
// preconditions explicit on the line that matters.
func wireFakeSysgit(w *gitWorld, available, upToDate bool, runErr error) error {
	if w.svc == nil {
		return fmt.Errorf("declare a journal-recording git service before wiring sysgit")
	}
	w.fakeSys = &fakeSysgit{available: available, upToDate: upToDate, err: runErr}
	w.fakeFlags = &fakeFlags{selfCloned: false}
	AttachSysgit(w.svc, w.fakeFlags, w.fakeSys)
	return nil
}

func initGitScenario(ctx *godog.ScenarioContext) {
	w := &gitWorld{}

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		dir, err := os.MkdirTemp("", "git-godog-")
		if err != nil {
			return ctx, err
		}
		// Reset all fields - Background runs after Before, so
		// fresh state is the only invariant we need.
		w.tmp = dir
		w.m = nil
		w.svc = nil
		w.jrnl = nil
		w.status = nil
		w.branches = nil
		w.log = nil
		w.clone = nil
		w.commit = nil
		w.push = nil
		w.pull = nil
		w.pullStash = nil
		w.fetch = nil
		w.repoRoot = ""
		w.boolRes = false
		w.lastErr = nil
		w.bareDir = ""
		w.srcDir = ""
		w.authServer = nil
		w.capturedAuth = ""
		w.fakeSys = nil
		w.fakeFlags = nil
		return ctx, nil
	})

	ctx.After(func(ctx context.Context, sc *godog.Scenario, _ error) (context.Context, error) {
		if w.authServer != nil {
			w.authServer.Close()
		}
		if w.tmp != "" {
			_ = os.RemoveAll(w.tmp)
		}
		if w.srcDir != "" {
			_ = os.RemoveAll(w.srcDir)
		}
		if w.bareDir != "" {
			_ = os.RemoveAll(w.bareDir)
		}
		return ctx, nil
	})

	// ── Background ────────────────────────────────────────────────────

	ctx.Step(`^a fresh temp directory$`, func() error {
		// Already created in Before - nothing to do.
		if w.tmp == "" {
			return fmt.Errorf("temp dir not initialized")
		}
		return nil
	})

	ctx.Step(`^a git manager$`, func() error {
		w.m = NewManager()
		return nil
	})

	// ── Repo setup ────────────────────────────────────────────────────

	ctx.Step(`^the temp dir is a git repo$`, func() error {
		_, err := gogit.PlainInit(w.tmp, false)
		return err
	})

	ctx.Step(`^a subdirectory "([^"]*)" exists$`, func(rel string) error {
		return os.MkdirAll(filepath.Join(w.tmp, rel), 0o755)
	})

	ctx.Step(`^the temp dir has a commit on "([^"]*)" with content "([^"]*)"$`, func(name, content string) error {
		return commitInRepo(w.tmp, name, content, "test commit")
	})

	ctx.Step(`^the temp dir has a commit on "([^"]*)" with content "([^"]*)" and message "([^"]*)"$`, func(name, content, msg string) error {
		return commitInRepo(w.tmp, name, content, msg)
	})

	ctx.Step(`^"([^"]*)" is rewritten to "([^"]*)"$`, func(name, content string) error {
		return os.WriteFile(filepath.Join(w.tmp, name), []byte(content), 0o644)
	})

	ctx.Step(`^the file "([^"]*)" exists with content "([^"]*)"$`, func(name, content string) error {
		return os.WriteFile(filepath.Join(w.tmp, name), []byte(content), 0o644)
	})

	ctx.Step(`^"([^"]*)" is removed from the worktree$`, func(name string) error {
		return os.Remove(filepath.Join(w.tmp, name))
	})

	ctx.Step(`^a local branch "([^"]*)" pointing at HEAD$`, func(name string) error {
		repo, err := gogit.PlainOpen(w.tmp)
		if err != nil {
			return err
		}
		head, err := repo.Head()
		if err != nil {
			return err
		}
		return repo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName(name), head.Hash()))
	})

	// ── Clone setup ───────────────────────────────────────────────────

	ctx.Step(`^a source repo with a commit$`, func() error {
		dir, err := os.MkdirTemp("", "git-godog-src-")
		if err != nil {
			return err
		}
		w.srcDir = dir
		if err := commitInRepo(dir, "a.txt", "hello", "first"); err != nil {
			return err
		}
		return nil
	})

	ctx.Step(`^the destination "([^"]*)" inside temp contains a leftover file$`, func(rel string) error {
		dest := filepath.Join(w.tmp, rel)
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dest, "leftover"), []byte("x"), 0o644)
	})

	ctx.Step(`^an HTTP test server that returns 401$`, func() error {
		w.authServer = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			w.capturedAuthM.Lock()
			if w.capturedAuth == "" {
				w.capturedAuth = r.Header.Get("Authorization")
			}
			w.capturedAuthM.Unlock()
			rw.Header().Set("WWW-Authenticate", `Basic realm="Git"`)
			rw.WriteHeader(http.StatusUnauthorized)
		}))
		return nil
	})

	// ── Whens ─────────────────────────────────────────────────────────

	ctx.Step(`^I check IsGitRepo on the temp dir$`, func() error {
		w.boolRes = w.m.IsGitRepo(w.tmp)
		return nil
	})

	ctx.Step(`^I check IsGitRepo on "([^"]*)"$`, func(rel string) error {
		w.boolRes = w.m.IsGitRepo(filepath.Join(w.tmp, rel))
		return nil
	})

	ctx.Step(`^I get the repo root$`, func() error {
		w.repoRoot, w.lastErr = w.m.RepoRoot(w.tmp)
		return nil
	})

	ctx.Step(`^I check the status$`, func() error {
		w.status, w.lastErr = w.m.Status(w.tmp)
		return nil
	})

	ctx.Step(`^I list branches$`, func() error {
		w.branches, w.lastErr = w.m.Branches(w.tmp)
		return nil
	})

	ctx.Step(`^I read the log with limit (\d+)$`, func(limit int) error {
		w.log, w.lastErr = w.m.Log(w.tmp, limit)
		return nil
	})

	ctx.Step(`^I clone the source into "([^"]*)" inside temp$`, func(rel string) error {
		dest := filepath.Join(w.tmp, rel)
		w.clone, w.lastErr = w.m.Clone(CloneOptions{URL: "file://" + w.srcDir, Dest: dest})
		return nil
	})

	ctx.Step(`^I clone with an empty URL$`, func() error {
		_, w.lastErr = w.m.Clone(CloneOptions{URL: "", Dest: filepath.Join(w.tmp, "x")})
		return nil
	})

	ctx.Step(`^I attempt to clone the test server with PAT "([^"]*)"$`, func(pat string) error {
		dest := filepath.Join(w.tmp, "auth-clone")
		_, w.lastErr = w.m.Clone(CloneOptions{
			URL:  w.authServer.URL + "/repo.git",
			Dest: dest,
			PAT:  pat,
		})
		return nil
	})

	ctx.Step(`^I attempt to clone the test server with no PAT$`, func() error {
		dest := filepath.Join(w.tmp, "anon-clone")
		_, w.lastErr = w.m.Clone(CloneOptions{
			URL:  w.authServer.URL + "/repo.git",
			Dest: dest,
		})
		return nil
	})

	ctx.Step(`^I commit with message "([^"]*)"$`, func(msg string) error {
		w.commit, w.lastErr = w.m.Commit(CommitOptions{
			Path:    w.tmp,
			Message: msg,
			Author:  "Test",
			Email:   "test@example.com",
		})
		return nil
	})

	ctx.Step(`^I commit with message "([^"]*)" and empty author$`, func(msg string) error {
		w.commit, w.lastErr = w.m.Commit(CommitOptions{
			Path:    w.tmp,
			Message: msg,
			Author:  "",
			Email:   "",
		})
		return nil
	})

	ctx.Step(`^I discard "([^"]*)"$`, func(file string) error {
		w.lastErr = w.m.Discard(DiscardOptions{Path: w.tmp, File: file})
		return nil
	})

	// ── Push / Fetch setup ───────────────────────────────────────────

	ctx.Step(`^a bare repo seeded with one commit$`, func() error {
		bare, err := os.MkdirTemp("", "git-godog-bare-")
		if err != nil {
			return err
		}
		w.bareDir = bare
		if _, err := gogit.PlainInit(bare, true); err != nil {
			return err
		}
		// Seed via a working clone-and-push.
		work, err := os.MkdirTemp("", "git-godog-seed-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(work)
		wr, err := gogit.PlainInit(work, false)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(work, "seed.txt"), []byte("seed"), 0o644); err != nil {
			return err
		}
		wt, err := wr.Worktree()
		if err != nil {
			return err
		}
		if _, err := wt.Add("seed.txt"); err != nil {
			return err
		}
		if _, err := wt.Commit("seed", &gogit.CommitOptions{
			Author: &object.Signature{Name: "Test", Email: "t@example.com", When: time.Now()},
		}); err != nil {
			return err
		}
		if _, err := wr.CreateRemote(&gogitcfg.RemoteConfig{
			Name: "origin",
			URLs: []string{bare},
		}); err != nil {
			return err
		}
		return wr.Push(&gogit.PushOptions{RemoteName: "origin"})
	})

	ctx.Step(`^a clone of the bare repo at "([^"]*)" inside temp$`, func(rel string) error {
		dest := filepath.Join(w.tmp, rel)
		_, err := gogit.PlainClone(dest, false, &gogit.CloneOptions{URL: w.bareDir})
		return err
	})

	ctx.Step(`^a new commit "([^"]*)" with content "([^"]*)" in "([^"]*)"$`, func(name, content, rel string) error {
		dir := filepath.Join(w.tmp, rel)
		return commitInRepo(dir, name, content, "godog: "+name)
	})

	ctx.Step(`^"([^"]*)" is rewritten to "([^"]*)" inside "([^"]*)"$`, func(name, content, rel string) error {
		return os.WriteFile(filepath.Join(w.tmp, rel, name), []byte(content), 0o644)
	})

	ctx.Step(`^the bare repo rewrites "([^"]*)" to "([^"]*)"$`, func(name, content string) error {
		// Side clone, modify the named path, push. Distinct from
		// "the bare repo gains another commit" (which adds remote.txt)
		// - this lets a scenario set up a same-path divergence between
		// the bare and the local clone.
		work, err := os.MkdirTemp("", "git-godog-rewrite-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(work)
		wr, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: w.bareDir})
		if err != nil {
			return err
		}
		full := filepath.Join(work, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return err
		}
		wt, err := wr.Worktree()
		if err != nil {
			return err
		}
		if _, err := wt.Add(name); err != nil {
			return err
		}
		if _, err := wt.Commit("rewrite "+name, &gogit.CommitOptions{
			Author: &object.Signature{Name: "T", Email: "t@example.com", When: time.Now()},
		}); err != nil {
			return err
		}
		return wr.Push(&gogit.PushOptions{RemoteName: "origin"})
	})

	ctx.Step(`^the file "([^"]*)" exists with content "([^"]*)" inside "([^"]*)"$`, func(name, content, rel string) error {
		full := filepath.Join(w.tmp, rel, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		return os.WriteFile(full, []byte(content), 0o644)
	})

	ctx.Step(`^the bare repo gains another commit$`, func() error {
		// Push a new commit into the bare from a fresh ephemeral clone.
		work, err := os.MkdirTemp("", "git-godog-advance-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(work)
		wr, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: w.bareDir})
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(work, "remote.txt"), []byte("remote"), 0o644); err != nil {
			return err
		}
		wt, err := wr.Worktree()
		if err != nil {
			return err
		}
		if _, err := wt.Add("remote.txt"); err != nil {
			return err
		}
		if _, err := wt.Commit("remote-side commit", &gogit.CommitOptions{
			Author: &object.Signature{Name: "T", Email: "t@example.com", When: time.Now()},
		}); err != nil {
			return err
		}
		return wr.Push(&gogit.PushOptions{RemoteName: "origin"})
	})

	ctx.Step(`^I push from "([^"]*)"$`, func(rel string) error {
		dir := filepath.Join(w.tmp, rel)
		w.push, w.lastErr = w.m.Push(PushOptions{Path: dir})
		return nil
	})

	ctx.Step(`^I push with an empty path$`, func() error {
		w.push, w.lastErr = w.m.Push(PushOptions{Path: ""})
		return nil
	})

	ctx.Step(`^I fetch from "([^"]*)"$`, func(rel string) error {
		dir := filepath.Join(w.tmp, rel)
		w.fetch, w.lastErr = w.m.Fetch(FetchOptions{Path: dir})
		return nil
	})

	ctx.Step(`^I fetch with an empty path$`, func() error {
		w.fetch, w.lastErr = w.m.Fetch(FetchOptions{Path: ""})
		return nil
	})

	ctx.Step(`^I pull from "([^"]*)"$`, func(rel string) error {
		dir := filepath.Join(w.tmp, rel)
		w.pull, w.lastErr = w.m.Pull(PullOptions{Path: dir})
		return nil
	})

	ctx.Step(`^I pull with an empty path$`, func() error {
		w.pull, w.lastErr = w.m.Pull(PullOptions{Path: ""})
		return nil
	})

	ctx.Step(`^I attempt to clone path "([^"]*)" with PAT "([^"]*)"$`, func(path, pat string) error {
		dest := filepath.Join(w.tmp, "ado-clone")
		_, w.lastErr = w.m.Clone(CloneOptions{
			URL:  w.authServer.URL + path,
			Dest: dest,
			PAT:  pat,
		})
		return nil
	})

	// ── Thens ─────────────────────────────────────────────────────────

	ctx.Step(`^the result is true$`, func() error {
		if !w.boolRes {
			return fmt.Errorf("expected true, got false")
		}
		return nil
	})

	ctx.Step(`^the result is false$`, func() error {
		if w.boolRes {
			return fmt.Errorf("expected false, got true")
		}
		return nil
	})

	ctx.Step(`^the operation returned an error$`, func() error {
		if w.lastErr == nil {
			return fmt.Errorf("expected an error, got nil")
		}
		return nil
	})

	ctx.Step(`^the operation succeeded$`, func() error {
		if w.lastErr != nil {
			return fmt.Errorf("expected success, got %v", w.lastErr)
		}
		return nil
	})

	ctx.Step(`^status is behind by (\d+)$`, func(n int) error {
		if w.status == nil {
			return fmt.Errorf("no status captured")
		}
		if w.status.Behind != n {
			return fmt.Errorf("behind = %d, want %d", w.status.Behind, n)
		}
		return nil
	})

	ctx.Step(`^status reports clean$`, func() error {
		if w.status == nil || !w.status.Clean {
			return fmt.Errorf("expected clean, got %+v", w.status)
		}
		return nil
	})

	ctx.Step(`^status is not clean$`, func() error {
		if w.status == nil || w.status.Clean {
			return fmt.Errorf("expected not clean, got %+v", w.status)
		}
		return nil
	})

	ctx.Step(`^the status branch is one of "([^"]*)"$`, func(csv string) error {
		want := strings.Split(csv, ",")
		for _, b := range want {
			if w.status.Branch == strings.TrimSpace(b) {
				return nil
			}
		}
		return fmt.Errorf("status branch %q not in %v", w.status.Branch, want)
	})

	ctx.Step(`^status reports modified "([^"]*)"$`, func(name string) error {
		if !contains(w.status.Modified, name) {
			return fmt.Errorf("Modified = %v, want to contain %q", w.status.Modified, name)
		}
		return nil
	})

	ctx.Step(`^status reports untracked "([^"]*)"$`, func(name string) error {
		if !contains(w.status.Untracked, name) {
			return fmt.Errorf("Untracked = %v, want to contain %q", w.status.Untracked, name)
		}
		return nil
	})

	ctx.Step(`^the branches list contains "([^"]*)"$`, func(name string) error {
		if w.branches == nil || !contains(w.branches.Locals, name) {
			return fmt.Errorf("Locals does not contain %q: %+v", name, w.branches)
		}
		return nil
	})

	ctx.Step(`^the log has (\d+) commits$`, func(n int) error {
		if len(w.log) != n {
			return fmt.Errorf("log length = %d, want %d", len(w.log), n)
		}
		return nil
	})

	ctx.Step(`^log entry (\d+) has subject "([^"]*)"$`, func(idx int, subject string) error {
		if idx >= len(w.log) {
			return fmt.Errorf("index %d out of range (log length %d)", idx, len(w.log))
		}
		if w.log[idx].Subject != subject {
			return fmt.Errorf("log[%d].Subject = %q, want %q", idx, w.log[idx].Subject, subject)
		}
		return nil
	})

	ctx.Step(`^the destination is a git repo$`, func() error {
		if w.clone == nil {
			return fmt.Errorf("clone result is nil")
		}
		if !w.m.IsGitRepo(w.clone.Dest) {
			return fmt.Errorf("destination %q is not a git repo", w.clone.Dest)
		}
		return nil
	})

	ctx.Step(`^the clone result head has 40 characters$`, func() error {
		if w.clone == nil || len(w.clone.Head) != 40 {
			return fmt.Errorf("Head = %q (len %d), want 40 chars", safeHead(w.clone), len(safeHead(w.clone)))
		}
		return nil
	})

	ctx.Step(`^the clone result branch is one of "([^"]*)"$`, func(csv string) error {
		if w.clone == nil {
			return fmt.Errorf("clone result is nil")
		}
		want := strings.Split(csv, ",")
		for _, b := range want {
			if w.clone.Branch == strings.TrimSpace(b) {
				return nil
			}
		}
		return fmt.Errorf("clone branch %q not in %v", w.clone.Branch, want)
	})

	ctx.Step(`^the captured Authorization header is BasicAuth for username "([^"]*)" and password "([^"]*)"$`, func(user, pass string) error {
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
		w.capturedAuthM.Lock()
		got := w.capturedAuth
		w.capturedAuthM.Unlock()
		if got != want {
			return fmt.Errorf("Authorization = %q, want %q", got, want)
		}
		return nil
	})

	ctx.Step(`^the commit succeeded$`, func() error {
		if w.lastErr != nil {
			return fmt.Errorf("commit failed: %v", w.lastErr)
		}
		if w.commit == nil || w.commit.Hash == "" {
			return fmt.Errorf("commit result is nil or has empty hash")
		}
		return nil
	})

	ctx.Step(`^file "([^"]*)" exists with content "([^"]*)"$`, func(name, want string) error {
		got, err := os.ReadFile(filepath.Join(w.tmp, name))
		if err != nil {
			return fmt.Errorf("read %q: %w", name, err)
		}
		if string(got) != want {
			return fmt.Errorf("file %q content = %q, want %q", name, string(got), want)
		}
		return nil
	})

	ctx.Step(`^file "([^"]*)" does not exist$`, func(name string) error {
		_, err := os.Stat(filepath.Join(w.tmp, name))
		if err == nil {
			return fmt.Errorf("expected %q to be absent, but it exists", name)
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("stat %q: %w", name, err)
		}
		return nil
	})

	ctx.Step(`^the push succeeded$`, func() error {
		if w.lastErr != nil {
			return fmt.Errorf("push failed: %v", w.lastErr)
		}
		if w.push == nil {
			return fmt.Errorf("push result is nil")
		}
		return nil
	})

	ctx.Step(`^push is already-up-to-date$`, func() error {
		if w.push == nil || !w.push.AlreadyUpToDate {
			return fmt.Errorf("expected AlreadyUpToDate=true, got %+v", w.push)
		}
		return nil
	})

	ctx.Step(`^push is not already-up-to-date$`, func() error {
		if w.push == nil || w.push.AlreadyUpToDate {
			return fmt.Errorf("expected AlreadyUpToDate=false, got %+v", w.push)
		}
		return nil
	})

	ctx.Step(`^the pull succeeded$`, func() error {
		if w.lastErr != nil {
			return fmt.Errorf("pull failed: %v", w.lastErr)
		}
		if w.pull == nil {
			return fmt.Errorf("pull result is nil")
		}
		return nil
	})

	ctx.Step(`^pull is already-up-to-date$`, func() error {
		if w.pull == nil || !w.pull.AlreadyUpToDate {
			return fmt.Errorf("expected AlreadyUpToDate=true, got %+v", w.pull)
		}
		return nil
	})

	ctx.Step(`^pull is not already-up-to-date$`, func() error {
		if w.pull == nil || w.pull.AlreadyUpToDate {
			return fmt.Errorf("expected AlreadyUpToDate=false, got %+v", w.pull)
		}
		return nil
	})

	ctx.Step(`^the fetch succeeded$`, func() error {
		if w.lastErr != nil {
			return fmt.Errorf("fetch failed: %v", w.lastErr)
		}
		if w.fetch == nil {
			return fmt.Errorf("fetch result is nil")
		}
		return nil
	})

	ctx.Step(`^fetch is already-up-to-date$`, func() error {
		if w.fetch == nil || !w.fetch.AlreadyUpToDate {
			return fmt.Errorf("expected AlreadyUpToDate=true, got %+v", w.fetch)
		}
		return nil
	})

	ctx.Step(`^fetch is not already-up-to-date$`, func() error {
		if w.fetch == nil || w.fetch.AlreadyUpToDate {
			return fmt.Errorf("expected AlreadyUpToDate=false, got %+v", w.fetch)
		}
		return nil
	})

	ctx.Step(`^after commit status reports clean$`, func() error {
		// Refetch status to confirm the post-commit worktree is clean.
		s, err := w.m.Status(w.tmp)
		if err != nil {
			return fmt.Errorf("post-commit status: %w", err)
		}
		if !s.Clean {
			return fmt.Errorf("expected clean post-commit, got %+v", s)
		}
		return nil
	})

	ctx.Step(`^no Authorization header was captured$`, func() error {
		w.capturedAuthM.Lock()
		got := w.capturedAuth
		w.capturedAuthM.Unlock()
		if got != "" {
			return fmt.Errorf("Authorization = %q, want empty", got)
		}
		return nil
	})

	// ── Service + fakeJournal scenarios ───────────────────────────────

	ctx.Step(`^a journal-recording git service$`, func() error {
		if w.m == nil {
			w.m = NewManager()
		}
		w.jrnl = &fakeJournal{}
		w.svc = NewService(w.m, nil, nil, w.jrnl)
		w.gitRoot = &fakeRoot{}
		AttachRoot(w.svc, w.gitRoot)
		return nil
	})

	ctx.Step(`^a git service with no journal recorder$`, func() error {
		if w.m == nil {
			w.m = NewManager()
		}
		w.svc = NewService(w.m, nil, nil, nil)
		w.gitRoot = &fakeRoot{}
		AttachRoot(w.svc, w.gitRoot)
		return nil
	})

	ctx.Step(`^I push from "([^"]*)" via the service$`, func(rel string) error {
		w.gitRoot.path = filepath.Join(w.tmp, rel)
		w.push, w.lastErr = w.svc.Push(PushOptions{})
		return nil
	})

	ctx.Step(`^I push with an empty path via the service$`, func() error {
		w.gitRoot.path = ""
		w.push, w.lastErr = w.svc.Push(PushOptions{})
		return nil
	})

	ctx.Step(`^I pull from "([^"]*)" via the service$`, func(rel string) error {
		w.gitRoot.path = filepath.Join(w.tmp, rel)
		w.pull, w.lastErr = w.svc.Pull(PullOptions{})
		return nil
	})

	ctx.Step(`^I pull with an empty path via the service$`, func() error {
		w.gitRoot.path = ""
		w.pull, w.lastErr = w.svc.Pull(PullOptions{})
		return nil
	})

	ctx.Step(`^I fetch from "([^"]*)" via the service$`, func(rel string) error {
		w.gitRoot.path = filepath.Join(w.tmp, rel)
		w.fetch, w.lastErr = w.svc.Fetch(FetchOptions{Remote: "origin"})
		return nil
	})

	ctx.Step(`^I fetch status from "([^"]*)" via the service$`, func(rel string) error {
		w.gitRoot.path = ""
		if rel != "" {
			w.gitRoot.path = filepath.Join(w.tmp, rel)
		}
		w.status, w.lastErr = w.svc.FetchStatus(FetchOptions{Remote: "origin"})
		return nil
	})

	// ── Sysgit dispatch wiring ────────────────────────────────────

	ctx.Step(`^a fake sysgit recorder marked available$`, func() error {
		return wireFakeSysgit(w, true, false, nil)
	})

	ctx.Step(`^a fake sysgit recorder marked unavailable$`, func() error {
		return wireFakeSysgit(w, false, false, nil)
	})

	ctx.Step(`^a fake sysgit recorder marked available and reporting up-to-date$`, func() error {
		return wireFakeSysgit(w, true, true, nil)
	})

	ctx.Step(`^a fake sysgit recorder marked available with error "([^"]*)"$`, func(msg string) error {
		return wireFakeSysgit(w, true, false, fakeErr(msg))
	})

	ctx.Step(`^the self-cloned toggle is (on|off)$`, func(state string) error {
		if w.fakeFlags == nil {
			return fmt.Errorf("no fake flags wired; declare a fake sysgit recorder first")
		}
		w.fakeFlags.selfCloned = state == "on"
		return nil
	})

	ctx.Step(`^the fake sysgit recorded (\d+) calls?$`, func(n int) error {
		if w.fakeSys == nil {
			return fmt.Errorf("no fake sysgit wired in this scenario")
		}
		if w.fakeSys.calls != n {
			return fmt.Errorf("fake sysgit calls = %d, want %d", w.fakeSys.calls, n)
		}
		return nil
	})

	ctx.Step(`^the fake sysgit was asked for remote "([^"]*)"$`, func(remote string) error {
		if w.fakeSys == nil {
			return fmt.Errorf("no fake sysgit wired in this scenario")
		}
		if w.fakeSys.gotRemote != remote {
			return fmt.Errorf("fake sysgit remote = %q, want %q", w.fakeSys.gotRemote, remote)
		}
		return nil
	})

	ctx.Step(`^the push NewHead is empty$`, func() error {
		if w.push == nil {
			return fmt.Errorf("no push result captured")
		}
		if w.push.NewHead != "" {
			return fmt.Errorf("push NewHead = %q, want empty", w.push.NewHead)
		}
		return nil
	})

	ctx.Step(`^the journal recorded (\d+) syncs?$`, func(n int) error {
		if w.jrnl == nil {
			return fmt.Errorf("no journal recorder wired in this scenario")
		}
		syncs, _ := w.jrnl.snapshot()
		if len(syncs) != n {
			return fmt.Errorf("sync count = %d, want %d (%+v)", len(syncs), n, syncs)
		}
		return nil
	})

	ctx.Step(`^the journal recorded (\d+) sync for backend "([^"]*)"$`, func(n int, backend string) error {
		if w.jrnl == nil {
			return fmt.Errorf("no journal recorder wired in this scenario")
		}
		syncs, _ := w.jrnl.snapshot()
		if len(syncs) != n {
			return fmt.Errorf("sync count = %d, want %d (%+v)", len(syncs), n, syncs)
		}
		for _, s := range syncs {
			if s.backend != backend {
				return fmt.Errorf("sync backend = %q, want %q", s.backend, backend)
			}
		}
		return nil
	})

	ctx.Step(`^the journal recorded (\d+) remote-seens?$`, func(n int) error {
		if w.jrnl == nil {
			return fmt.Errorf("no journal recorder wired in this scenario")
		}
		_, seens := w.jrnl.snapshot()
		if len(seens) != n {
			return fmt.Errorf("remote-seen count = %d, want %d (%+v)", len(seens), n, seens)
		}
		return nil
	})

	ctx.Step(`^the journal recorded (\d+) remote-seen for backend "([^"]*)"$`, func(n int, backend string) error {
		if w.jrnl == nil {
			return fmt.Errorf("no journal recorder wired in this scenario")
		}
		_, seens := w.jrnl.snapshot()
		if len(seens) != n {
			return fmt.Errorf("remote-seen count = %d, want %d (%+v)", len(seens), n, seens)
		}
		for _, s := range seens {
			if s.backend != backend {
				return fmt.Errorf("remote-seen backend = %q, want %q", s.backend, backend)
			}
		}
		return nil
	})

	ctx.Step(`^the recorded sync version equals the push NewHead$`, func() error {
		if w.jrnl == nil || w.push == nil {
			return fmt.Errorf("missing journal or push state")
		}
		syncs, _ := w.jrnl.snapshot()
		if len(syncs) == 0 {
			return fmt.Errorf("no sync recorded to compare against")
		}
		if syncs[0].version != w.push.NewHead {
			return fmt.Errorf("sync.version = %q, want push.NewHead = %q", syncs[0].version, w.push.NewHead)
		}
		return nil
	})

	ctx.Step(`^the recorded remote-seen version equals the pull NewHead$`, func() error {
		if w.jrnl == nil || w.pull == nil {
			return fmt.Errorf("missing journal or pull state")
		}
		_, seens := w.jrnl.snapshot()
		if len(seens) == 0 {
			return fmt.Errorf("no remote-seen recorded to compare against")
		}
		if seens[0].version != w.pull.NewHead {
			return fmt.Errorf("seen.version = %q, want pull.NewHead = %q", seens[0].version, w.pull.NewHead)
		}
		return nil
	})

	// ── PullWithStash scenarios ───────────────────────────────────────
	// Scenarios use the journal-recording service plus a configurable
	// pending list. They focus on user-visible outcomes: stash content
	// round-trips, conflicts surface a recovery directory, unrelated
	// dirt isn't clobbered.

	ctx.Step(`^the journal pending for "([^"]*)" includes "([^"]*)" with op "([^"]*)"$`, func(backend, path, op string) error {
		if w.jrnl == nil {
			return fmt.Errorf("no journal-recording service in scope")
		}
		// Append, not replace - multiple Given lines can build up the
		// pending list.
		current := w.jrnl.Pending(backend).Paths
		current = append(current, journal.PendingChange{Path: path, Op: op})
		w.jrnl.setPending(backend, current)
		return nil
	})

	ctx.Step(`^I pull-with-stash from "([^"]*)" via the service$`, func(rel string) error {
		dir := filepath.Join(w.tmp, rel)
		w.pullStash, w.lastErr = w.svc.PullWithStash(PullOptions{Path: dir})
		// Mirror the simpler Pull's bookkeeping so existing assertions
		// ("pull is not already-up-to-date" / "pull succeeded") work
		// against PullWithStash too.
		if w.pullStash != nil {
			w.pull = w.pullStash.Pull
		}
		return nil
	})

	ctx.Step(`^the stash result restored "([^"]*)"$`, func(path string) error {
		if w.pullStash == nil {
			return fmt.Errorf("no stash result")
		}
		for _, p := range w.pullStash.Restored {
			if p == path {
				return nil
			}
		}
		return fmt.Errorf("Restored = %v, want to contain %q", w.pullStash.Restored, path)
	})

	ctx.Step(`^the stash result has (\d+) overrides?$`, func(n int) error {
		if w.pullStash == nil {
			return fmt.Errorf("no stash result")
		}
		if len(w.pullStash.Overridden) != n {
			return fmt.Errorf("Overridden = %v, want %d", w.pullStash.Overridden, n)
		}
		return nil
	})

	ctx.Step(`^the stash result has "([^"]*)" in overrides$`, func(path string) error {
		if w.pullStash == nil {
			return fmt.Errorf("no stash result")
		}
		for _, p := range w.pullStash.Overridden {
			if p.Path == path {
				return nil
			}
		}
		return fmt.Errorf("Overridden = %v, want to contain %q", w.pullStash.Overridden, path)
	})

	ctx.Step(`^the override for "([^"]*)" names an author$`, func(path string) error {
		if w.pullStash == nil {
			return fmt.Errorf("no stash result")
		}
		for _, p := range w.pullStash.Overridden {
			if p.Path == path {
				if p.Author == "" {
					return fmt.Errorf("override for %q has empty Author", path)
				}
				return nil
			}
		}
		return fmt.Errorf("no override entry for %q", path)
	})

	ctx.Step(`^the stash result has (\d+) auto-merges?$`, func(n int) error {
		if w.pullStash == nil {
			return fmt.Errorf("no stash result")
		}
		if len(w.pullStash.AutoMerged) != n {
			return fmt.Errorf("AutoMerged = %v, want %d", w.pullStash.AutoMerged, n)
		}
		return nil
	})

	ctx.Step(`^the stash result has "([^"]*)" auto-merged$`, func(path string) error {
		if w.pullStash == nil {
			return fmt.Errorf("no stash result")
		}
		for _, p := range w.pullStash.AutoMerged {
			if p == path {
				return nil
			}
		}
		return fmt.Errorf("AutoMerged = %v, want to contain %q", w.pullStash.AutoMerged, path)
	})

	ctx.Step(`^file "([^"]*)" inside "([^"]*)" has content "([^"]*)"$`, func(name, rel, want string) error {
		got, err := os.ReadFile(filepath.Join(w.tmp, rel, name))
		if err != nil {
			return fmt.Errorf("read %q: %w", filepath.Join(rel, name), err)
		}
		if string(got) != want {
			return fmt.Errorf("file content = %q, want %q", string(got), want)
		}
		return nil
	})

	ctx.Step(`^a stashed copy of "([^"]*)" exists under "([^"]*)"$`, func(name, rel string) error {
		stashFile := filepath.Join(w.tmp, rel, stashSubdir, name)
		if _, err := os.Stat(stashFile); err != nil {
			return fmt.Errorf("stash file missing: %w", err)
		}
		return nil
	})

	ctx.Step(`^no stash directory exists under "([^"]*)"$`, func(rel string) error {
		stashDir := filepath.Join(w.tmp, rel, stashSubdir)
		if _, err := os.Stat(stashDir); !os.IsNotExist(err) {
			return fmt.Errorf("stash dir present: %v", err)
		}
		return nil
	})
}

// ── tiny test helpers (not exported beyond the test binary) ─────────

func commitInRepo(dir, name, content, msg string) error {
	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		// Initialize on first commit.
		repo, err = gogit.PlainInit(dir, false)
		if err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	if _, err := wt.Add(name); err != nil {
		return err
	}
	_, err = wt.Commit(msg, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	return err
}

func contains(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}

func safeHead(c *CloneResult) string {
	if c == nil {
		return ""
	}
	return c.Head
}
