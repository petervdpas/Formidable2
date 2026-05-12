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

	stderr string
	runErr error
}

func (f *fakeExec) LookPath(name string) (string, error) {
	if f.lookErr != nil {
		return "", f.lookErr
	}
	return f.binary, nil
}

func (f *fakeExec) Run(workdir, name string, args []string) (string, error) {
	f.gotWork = workdir
	f.gotName = name
	f.gotArgs = append([]string(nil), args...)
	return f.stderr, f.runErr
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
