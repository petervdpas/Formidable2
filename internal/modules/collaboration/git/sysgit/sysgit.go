// Package sysgit shells out to the system `git` binary for network
// operations (fetch/push/pull) when the user opts into "self-cloned"
// mode. Lets Formidable honour the user's existing credential helper
// (Windows Credential Manager, libsecret, GitHub CLI) instead of
// managing a separate PAT in its own keychain.
//
// Local ops stay on go-git inside the parent package - this layer is
// only for the auth-bearing transport calls.
package sysgit

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

// Executor is the seam tests stub. Real wiring uses os/exec; tests
// inject a fake so the unit pass doesn't spawn `git`.
//
// Both stdout and stderr are captured because git splits its
// up-to-date messages between them: `git push` reports "Everything
// up-to-date" on stderr, `git pull` reports "Already up to date." on
// stdout. The dispatcher needs both to translate into a result the
// journal can consume without recording phantom sync markers.
type Executor interface {
	LookPath(name string) (string, error)
	Run(workdir, name string, args []string) (stdout, stderr string, err error)
}

type realExecutor struct{}

func (realExecutor) LookPath(name string) (string, error) { return exec.LookPath(name) }

func (realExecutor) Run(workdir, name string, args []string) (string, string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = workdir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// Runner shells out to system git. Construct once at startup;
// Available() reflects the detection result and stays stable for the
// process lifetime.
type Runner struct {
	exec   Executor
	binary string
	log    *slog.Logger
}

// NewRunner detects the git binary on PATH. Available() is false when
// detection fails - callers must check before invoking an op.
func NewRunner(log *slog.Logger) *Runner { return newRunner(realExecutor{}, log) }

func newRunner(ex Executor, log *slog.Logger) *Runner {
	if log == nil {
		log = slog.Default()
	}
	bin, _ := ex.LookPath("git")
	return &Runner{exec: ex, binary: bin, log: log}
}

func (r *Runner) Available() bool { return r.binary != "" }

// Binary returns the resolved git path. "" when not available.
func (r *Runner) Binary() string { return r.binary }

// errNotAvailable is the sentinel for "system git not installed".
// Service-layer dispatchers translate this into a user-facing error
// when the toggle is on but the binary is missing.
var errNotAvailable = errors.New("sysgit: git binary not found on PATH")

// ErrNotAvailable reports the not-installed case so callers can
// translate it into a localized message without string matching.
func ErrNotAvailable() error { return errNotAvailable }

// Fetch runs `git fetch <remote>` inside workdir. Stderr is folded
// into the error message so the UI toast shows what git complained
// about (auth, unknown remote, network) - and the same stderr is
// emitted as a warn-level log so users without devtools (Windows
// builds) can find it in Information → Logging.
func (r *Runner) Fetch(workdir, remote string) error {
	if !r.Available() {
		return errNotAvailable
	}
	if strings.TrimSpace(workdir) == "" {
		return errors.New("sysgit: fetch: path required")
	}
	if remote == "" {
		remote = "origin"
	}
	r.log.Info("sysgit: fetch", "path", workdir, "remote", remote, "binary", r.binary)
	_, stderr, err := r.exec.Run(workdir, r.binary, []string{"fetch", remote})
	if err != nil {
		r.log.Warn("sysgit: fetch failed", "path", workdir, "remote", remote, "err", err.Error(), "stderr", strings.TrimSpace(stderr))
		return wrapErr("fetch", stderr, err)
	}
	return nil
}

// Push runs `git push <remote>`. alreadyUpToDate flips true when git
// reports "Everything up-to-date" on stderr - matches go-git's
// NoErrAlreadyUpToDate semantics so the journal records remote-seen
// instead of a phantom sync marker.
func (r *Runner) Push(workdir, remote string) (alreadyUpToDate bool, err error) {
	if !r.Available() {
		return false, errNotAvailable
	}
	if strings.TrimSpace(workdir) == "" {
		return false, errors.New("sysgit: push: path required")
	}
	if remote == "" {
		remote = "origin"
	}
	r.log.Info("sysgit: push", "path", workdir, "remote", remote, "binary", r.binary)
	_, stderr, runErr := r.exec.Run(workdir, r.binary, []string{"push", remote})
	if runErr != nil {
		r.log.Warn("sysgit: push failed", "path", workdir, "remote", remote, "err", runErr.Error(), "stderr", strings.TrimSpace(stderr))
		return false, wrapErr("push", stderr, runErr)
	}
	if strings.Contains(stderr, "Everything up-to-date") {
		r.log.Info("sysgit: push up-to-date", "path", workdir, "remote", remote)
		return true, nil
	}
	r.log.Info("sysgit: push advanced", "path", workdir, "remote", remote)
	return false, nil
}

// Pull runs `git pull <remote>`. alreadyUpToDate flips true when git
// reports "Already up to date." on stdout. Pull goes through the
// system git binary even though it touches the worktree - that's the
// whole point of self-cloned mode (credential helper resolves auth).
func (r *Runner) Pull(workdir, remote string) (alreadyUpToDate bool, err error) {
	if !r.Available() {
		return false, errNotAvailable
	}
	if strings.TrimSpace(workdir) == "" {
		return false, errors.New("sysgit: pull: path required")
	}
	if remote == "" {
		remote = "origin"
	}
	r.log.Info("sysgit: pull", "path", workdir, "remote", remote, "binary", r.binary)
	stdout, stderr, runErr := r.exec.Run(workdir, r.binary, []string{"pull", remote})
	if runErr != nil {
		r.log.Warn("sysgit: pull failed", "path", workdir, "remote", remote, "err", runErr.Error(), "stderr", strings.TrimSpace(stderr))
		return false, wrapErr("pull", stderr, runErr)
	}
	if strings.Contains(stdout, "Already up to date") {
		r.log.Info("sysgit: pull up-to-date", "path", workdir, "remote", remote)
		return true, nil
	}
	r.log.Info("sysgit: pull advanced", "path", workdir, "remote", remote)
	return false, nil
}

func wrapErr(op, stderr string, err error) error {
	msg := strings.TrimSpace(stderr)
	if msg == "" {
		return fmt.Errorf("sysgit: %s: %w", op, err)
	}
	return fmt.Errorf("sysgit: %s: %s", op, msg)
}
