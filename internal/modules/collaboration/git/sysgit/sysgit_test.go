package sysgit

import (
	"errors"
	"strings"
	"testing"
)

type fakeExec struct {
	binary string
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
