package sysgit

import (
	"errors"
	"strings"
	"testing"
)

type fakeExec struct {
	binary  string
	lookErr error

	gotWork string
	gotName string
	gotArgs []string

	stdout string
	stderr string
	runErr error
}

func (f *fakeExec) LookPath(name string) (string, error) {
	if f.lookErr != nil {
		return "", f.lookErr
	}
	return f.binary, nil
}

func (f *fakeExec) Run(workdir, name string, args []string) (string, string, error) {
	f.gotWork = workdir
	f.gotName = name
	f.gotArgs = append([]string(nil), args...)
	return f.stdout, f.stderr, f.runErr
}

func TestRunner_NotAvailableWhenBinaryMissing(t *testing.T) {
	r := newRunner(&fakeExec{lookErr: errors.New("not found")}, nil)
	if r.Available() {
		t.Fatalf("Available() = true, want false when LookPath errored")
	}
	if r.Binary() != "" {
		t.Fatalf("Binary() = %q, want empty", r.Binary())
	}
}

func TestRunner_AvailableWhenBinaryFound(t *testing.T) {
	r := newRunner(&fakeExec{binary: "/usr/bin/git"}, nil)
	if !r.Available() {
		t.Fatalf("Available() = false, want true")
	}
	if r.Binary() != "/usr/bin/git" {
		t.Fatalf("Binary() = %q, want /usr/bin/git", r.Binary())
	}
}

func TestRunner_FetchReturnsErrNotAvailable(t *testing.T) {
	r := newRunner(&fakeExec{lookErr: errors.New("not found")}, nil)
	err := r.Fetch("/repo", "origin")
	if !errors.Is(err, ErrNotAvailable()) {
		t.Fatalf("got %v, want ErrNotAvailable", err)
	}
}

func TestRunner_FetchRequiresWorkdir(t *testing.T) {
	r := newRunner(&fakeExec{binary: "/usr/bin/git"}, nil)
	err := r.Fetch("   ", "origin")
	if err == nil || !strings.Contains(err.Error(), "path required") {
		t.Fatalf("got %v, want path-required error", err)
	}
}

func TestRunner_FetchDefaultsRemoteToOrigin(t *testing.T) {
	fe := &fakeExec{binary: "/usr/bin/git"}
	r := newRunner(fe, nil)
	if err := r.Fetch("/repo", ""); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got, want := fe.gotArgs, []string{"fetch", "origin"}; !equal(got, want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
	if fe.gotWork != "/repo" {
		t.Fatalf("workdir = %q, want /repo", fe.gotWork)
	}
	if fe.gotName != "/usr/bin/git" {
		t.Fatalf("name = %q, want /usr/bin/git", fe.gotName)
	}
}

func TestRunner_FetchPassesRemoteVerbatim(t *testing.T) {
	fe := &fakeExec{binary: "/usr/bin/git"}
	r := newRunner(fe, nil)
	if err := r.Fetch("/repo", "upstream"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got, want := fe.gotArgs, []string{"fetch", "upstream"}; !equal(got, want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
}

func TestRunner_FetchSurfacesStderr(t *testing.T) {
	fe := &fakeExec{
		binary: "/usr/bin/git",
		stderr: "fatal: repository 'https://example/' not found\n",
		runErr: errors.New("exit status 128"),
	}
	r := newRunner(fe, nil)
	err := r.Fetch("/repo", "origin")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "repository 'https://example/' not found") {
		t.Fatalf("err missing stderr content: %v", err)
	}
}

func TestRunner_FetchFallsBackToErrWhenStderrEmpty(t *testing.T) {
	fe := &fakeExec{
		binary: "/usr/bin/git",
		runErr: errors.New("exit status 1"),
	}
	r := newRunner(fe, nil)
	err := r.Fetch("/repo", "origin")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("err missing wrapped exit: %v", err)
	}
}

func TestRunner_PushReportsAlreadyUpToDate(t *testing.T) {
	fe := &fakeExec{
		binary: "/usr/bin/git",
		stderr: "Everything up-to-date\n",
	}
	r := newRunner(fe, nil)
	up, err := r.Push("/repo", "origin")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !up {
		t.Fatal("expected alreadyUpToDate=true")
	}
}

func TestRunner_PushAdvancingReportsFalse(t *testing.T) {
	fe := &fakeExec{
		binary: "/usr/bin/git",
		stderr: "To https://example/repo.git\n   abc..def  master -> master\n",
	}
	r := newRunner(fe, nil)
	up, err := r.Push("/repo", "origin")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if up {
		t.Fatal("expected alreadyUpToDate=false")
	}
}

func TestRunner_PushSurfacesAuthError(t *testing.T) {
	fe := &fakeExec{
		binary: "/usr/bin/git",
		stderr: "remote: Repository not found.\nfatal: Authentication failed\n",
		runErr: errors.New("exit status 128"),
	}
	r := newRunner(fe, nil)
	_, err := r.Push("/repo", "origin")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Authentication failed") {
		t.Fatalf("err missing stderr: %v", err)
	}
}

func TestRunner_PullReportsAlreadyUpToDate(t *testing.T) {
	fe := &fakeExec{
		binary: "/usr/bin/git",
		stdout: "Already up to date.\n",
	}
	r := newRunner(fe, nil)
	up, err := r.Pull("/repo", "origin")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !up {
		t.Fatal("expected alreadyUpToDate=true")
	}
}

func TestRunner_PullAdvancingReportsFalse(t *testing.T) {
	fe := &fakeExec{
		binary: "/usr/bin/git",
		stdout: "Updating abc..def\nFast-forward\n",
	}
	r := newRunner(fe, nil)
	up, err := r.Pull("/repo", "origin")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if up {
		t.Fatal("expected alreadyUpToDate=false")
	}
}

func TestRunner_PullSurfacesConflict(t *testing.T) {
	fe := &fakeExec{
		binary: "/usr/bin/git",
		stderr: "fatal: refusing to merge unrelated histories\n",
		runErr: errors.New("exit status 128"),
	}
	r := newRunner(fe, nil)
	_, err := r.Pull("/repo", "origin")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unrelated histories") {
		t.Fatalf("err missing stderr: %v", err)
	}
}

func TestRunner_PushRequiresWorkdir(t *testing.T) {
	r := newRunner(&fakeExec{binary: "/usr/bin/git"}, nil)
	if _, err := r.Push("   ", "origin"); err == nil || !strings.Contains(err.Error(), "path required") {
		t.Fatalf("got %v, want path-required error", err)
	}
}

func TestRunner_PullRequiresWorkdir(t *testing.T) {
	r := newRunner(&fakeExec{binary: "/usr/bin/git"}, nil)
	if _, err := r.Pull("", "origin"); err == nil || !strings.Contains(err.Error(), "path required") {
		t.Fatalf("got %v, want path-required error", err)
	}
}

func TestRunner_PushPullDefaultRemoteToOrigin(t *testing.T) {
	fe := &fakeExec{binary: "/usr/bin/git"}
	r := newRunner(fe, nil)
	if _, err := r.Push("/repo", ""); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if got, want := fe.gotArgs, []string{"push", "origin"}; !equal(got, want) {
		t.Errorf("Push args = %v, want %v", got, want)
	}
	if _, err := r.Pull("/repo", ""); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if got, want := fe.gotArgs, []string{"pull", "origin"}; !equal(got, want) {
		t.Errorf("Pull args = %v, want %v", got, want)
	}
}

func TestRunner_PushPullCustomRemote(t *testing.T) {
	fe := &fakeExec{binary: "/usr/bin/git"}
	r := newRunner(fe, nil)
	if _, err := r.Push("/repo", "upstream"); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if fe.gotArgs[1] != "upstream" {
		t.Errorf("Push: remote = %q, want upstream", fe.gotArgs[1])
	}
	if _, err := r.Pull("/repo", "fork"); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if fe.gotArgs[1] != "fork" {
		t.Errorf("Pull: remote = %q, want fork", fe.gotArgs[1])
	}
}

func TestRunner_PushFallsBackToErrWhenStderrEmpty(t *testing.T) {
	fe := &fakeExec{
		binary: "/usr/bin/git",
		runErr: errors.New("exit status 128"),
	}
	r := newRunner(fe, nil)
	_, err := r.Push("/repo", "origin")
	if err == nil || !strings.Contains(err.Error(), "exit status 128") {
		t.Fatalf("got %v, want wrapped exit", err)
	}
}

func TestRunner_PullFallsBackToErrWhenStderrEmpty(t *testing.T) {
	fe := &fakeExec{
		binary: "/usr/bin/git",
		runErr: errors.New("exit status 1"),
	}
	r := newRunner(fe, nil)
	_, err := r.Pull("/repo", "origin")
	if err == nil || !strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("got %v, want wrapped exit", err)
	}
}

func TestRunner_PushUpToDateNotMistakenForFailure(t *testing.T) {
	// "Everything up-to-date" must only flip the bool on exit==0. If
	// git exits non-zero for an unrelated reason but stderr happens to
	// mention "Everything up-to-date" in noise, that's still an error.
	fe := &fakeExec{
		binary: "/usr/bin/git",
		stderr: "Everything up-to-date\nfatal: something else broke\n",
		runErr: errors.New("exit status 1"),
	}
	r := newRunner(fe, nil)
	up, err := r.Push("/repo", "origin")
	if err == nil {
		t.Fatal("expected error on non-zero exit")
	}
	if up {
		t.Fatal("alreadyUpToDate must not flip on error path")
	}
}

func TestRunner_PushPullRequireBinary(t *testing.T) {
	r := newRunner(&fakeExec{lookErr: errors.New("not found")}, nil)
	if _, err := r.Push("/repo", "origin"); !errors.Is(err, ErrNotAvailable()) {
		t.Errorf("Push: got %v, want ErrNotAvailable", err)
	}
	if _, err := r.Pull("/repo", "origin"); !errors.Is(err, ErrNotAvailable()) {
		t.Errorf("Pull: got %v, want ErrNotAvailable", err)
	}
}

// multiExec records every Run call so Restore (which shells out twice) can be
// asserted on both invocations, and lets a chosen call return an error.
type multiExec struct {
	binary  string
	calls   [][]string
	failIdx int // 1-based call index to fail; 0 = never
	failErr error
	stderr  string
}

func (m *multiExec) LookPath(string) (string, error) { return m.binary, nil }
func (m *multiExec) Run(_, _ string, args []string) (string, string, error) {
	m.calls = append(m.calls, append([]string(nil), args...))
	if m.failIdx == len(m.calls) {
		return "", m.stderr, m.failErr
	}
	return "", "", nil
}

func TestRunner_RestoreRunsCheckoutThenClean(t *testing.T) {
	me := &multiExec{binary: "/usr/bin/git"}
	r := newRunner(me, nil)
	if err := r.Restore("/repo", "storage/x.meta.json"); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(me.calls) != 2 {
		t.Fatalf("expected 2 git calls, got %d: %v", len(me.calls), me.calls)
	}
	if got, want := me.calls[0], []string{"checkout", "--", "storage/x.meta.json"}; !equal(got, want) {
		t.Errorf("call 1 = %v, want %v", got, want)
	}
	if got, want := me.calls[1], []string{"clean", "-fd", "--", "storage/x.meta.json"}; !equal(got, want) {
		t.Errorf("call 2 = %v, want %v", got, want)
	}
}

func TestRunner_RestoreToleratesCheckoutErrorButSurfacesCleanError(t *testing.T) {
	// Checkout fails (untracked path) - non-fatal. Clean fails - fatal.
	me := &multiExec{binary: "/usr/bin/git", failIdx: 1, failErr: errors.New("exit 1"), stderr: "pathspec did not match"}
	r := newRunner(me, nil)
	if err := r.Restore("/repo", "new.txt"); err != nil {
		t.Fatalf("checkout failure must be tolerated, got %v", err)
	}

	me2 := &multiExec{binary: "/usr/bin/git", failIdx: 2, failErr: errors.New("exit 1"), stderr: "permission denied"}
	r2 := newRunner(me2, nil)
	err := r2.Restore("/repo", "x.txt")
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("clean failure must surface, got %v", err)
	}
}

func TestRunner_RestoreRequiresBinaryWorkdirAndSafePath(t *testing.T) {
	if err := newRunner(&fakeExec{lookErr: errors.New("x")}, nil).Restore("/repo", "a"); !errors.Is(err, ErrNotAvailable()) {
		t.Errorf("missing binary: got %v, want ErrNotAvailable", err)
	}
	r := newRunner(&fakeExec{binary: "/usr/bin/git"}, nil)
	if err := r.Restore("  ", "a"); err == nil || !strings.Contains(err.Error(), "path required") {
		t.Errorf("empty workdir: got %v", err)
	}
	for _, bad := range []string{"", "..", "../escape", "/abs/path"} {
		if err := r.Restore("/repo", bad); err == nil || !strings.Contains(err.Error(), "invalid file path") {
			t.Errorf("Restore(%q): got %v, want invalid-path error", bad, err)
		}
	}
}

func TestRunner_StatusPorcelainPassesArgsAndReturnsStdout(t *testing.T) {
	fe := &fakeExec{binary: "/usr/bin/git", stdout: "## master...origin/master\n M a.txt\n"}
	r := newRunner(fe, nil)
	out, err := r.StatusPorcelain("/repo")
	if err != nil {
		t.Fatalf("StatusPorcelain: %v", err)
	}
	if !strings.Contains(out, "## master") {
		t.Errorf("stdout not returned verbatim: %q", out)
	}
	if got, want := fe.gotArgs, []string{"status", "--porcelain=v1", "--branch"}; !equal(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}
}

func TestRunner_StatusPorcelainSurfacesError(t *testing.T) {
	fe := &fakeExec{binary: "/usr/bin/git", stderr: "fatal: not a git repository\n", runErr: errors.New("exit 128")}
	r := newRunner(fe, nil)
	if _, err := r.StatusPorcelain("/repo"); err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("got %v, want stderr surfaced", err)
	}
}

func TestRunner_StatusPorcelainRequiresBinaryAndWorkdir(t *testing.T) {
	if _, err := newRunner(&fakeExec{lookErr: errors.New("x")}, nil).StatusPorcelain("/repo"); !errors.Is(err, ErrNotAvailable()) {
		t.Errorf("missing binary: got %v", err)
	}
	if _, err := newRunner(&fakeExec{binary: "/usr/bin/git"}, nil).StatusPorcelain(""); err == nil || !strings.Contains(err.Error(), "path required") {
		t.Errorf("empty workdir: got %v", err)
	}
}

func TestRunner_HeadHashTrimsOutput(t *testing.T) {
	fe := &fakeExec{binary: "/usr/bin/git", stdout: "deadbeef1234\n"}
	r := newRunner(fe, nil)
	got, err := r.HeadHash("/repo")
	if err != nil {
		t.Fatalf("HeadHash: %v", err)
	}
	if got != "deadbeef1234" {
		t.Errorf("HeadHash = %q, want trimmed hash", got)
	}
	if want := []string{"rev-parse", "HEAD"}; !equal(fe.gotArgs, want) {
		t.Errorf("args = %v, want %v", fe.gotArgs, want)
	}
}

func TestRunner_HeadHashSurfacesErrorOnNonRepo(t *testing.T) {
	fe := &fakeExec{binary: "/usr/bin/git", stderr: "fatal: not a git repository\n", runErr: errors.New("exit 128")}
	r := newRunner(fe, nil)
	if _, err := r.HeadHash("/repo"); err == nil {
		t.Fatal("expected error on non-repo rev-parse")
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
