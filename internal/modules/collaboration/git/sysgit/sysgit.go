// Package sysgit shells out to the system `git` binary for network
// operations (fetch/push/pull) when the user opts into "self-cloned"
// mode. Lets Formidable honour the user's existing credential helper
// (Windows Credential Manager, libsecret, GitHub CLI) instead of
// managing a separate PAT in its own keychain.
//
// Local ops stay on go-git inside the parent package — this layer is
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
type Executor interface {
	LookPath(name string) (string, error)
	Run(workdir, name string, args []string) (stderr string, err error)
}

type realExecutor struct{}

func (realExecutor) LookPath(name string) (string, error) { return exec.LookPath(name) }

func (realExecutor) Run(workdir, name string, args []string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = workdir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stderr.String(), err
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
// detection fails — callers must check before invoking an op.
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
// about (auth, unknown remote, network).
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
	r.log.Info("sysgit: fetch", "path", workdir, "remote", remote)
	stderr, err := r.exec.Run(workdir, r.binary, []string{"fetch", remote})
	if err != nil {
		return wrapErr("fetch", stderr, err)
	}
	return nil
}

func wrapErr(op, stderr string, err error) error {
	msg := strings.TrimSpace(stderr)
	if msg == "" {
		return fmt.Errorf("sysgit: %s: %w", op, err)
	}
	return fmt.Errorf("sysgit: %s: %s", op, msg)
}
