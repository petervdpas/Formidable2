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
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
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

	// Whatever the most recent operation produced.
	status   *Status
	branches *Branches
	log      []Commit
	clone    *CloneResult
	repoRoot string
	boolRes  bool
	lastErr  error

	// Source repo for clone scenarios (created via "a source repo
	// with a commit").
	srcDir string

	// HTTP test server bits — used by the wire-level auth scenarios.
	authServer    *httptest.Server
	capturedAuthM sync.Mutex
	capturedAuth  string
}

func initGitScenario(ctx *godog.ScenarioContext) {
	w := &gitWorld{}

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		dir, err := os.MkdirTemp("", "git-godog-")
		if err != nil {
			return ctx, err
		}
		// Reset all fields — Background runs after Before, so
		// fresh state is the only invariant we need.
		w.tmp = dir
		w.m = nil
		w.status = nil
		w.branches = nil
		w.log = nil
		w.clone = nil
		w.repoRoot = ""
		w.boolRes = false
		w.lastErr = nil
		w.srcDir = ""
		w.authServer = nil
		w.capturedAuth = ""
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
		return ctx, nil
	})

	// ── Background ────────────────────────────────────────────────────

	ctx.Step(`^a fresh temp directory$`, func() error {
		// Already created in Before — nothing to do.
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

	ctx.Step(`^no Authorization header was captured$`, func() error {
		w.capturedAuthM.Lock()
		got := w.capturedAuth
		w.capturedAuthM.Unlock()
		if got != "" {
			return fmt.Errorf("Authorization = %q, want empty", got)
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
