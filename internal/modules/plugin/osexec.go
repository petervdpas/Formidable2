package plugin

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"

	"github.com/petervdpas/formidable2/internal/util/proc"
)

// OSExec is the default ExecRunner. Args pass as a slice (no shell), so plugin input can't become a shell metacharacter;
// the trade-off is no pipelines/redirection without an explicit `bash -c`. A non-zero process exit is a normal result,
// not a Go error: authors branch on res.exit. Only command-not-found, startup failure, or timeout return an error.
type OSExec struct{}

// Exec runs cmd with args; opts.Env is additive to the inherited environment (not a replacement) so PATH keeps working.
func (OSExec) Exec(cmd string, args []string, opts ExecOptions) (ExecResult, error) {
	ctx := context.Background()
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}
	c := exec.CommandContext(ctx, cmd, args...)
	if opts.Cwd != "" {
		c.Dir = opts.Cwd
	}
	if len(opts.Env) > 0 {
		env := os.Environ()
		for k, v := range opts.Env {
			env = append(env, k+"="+v)
		}
		c.Env = env
	}
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	proc.HideWindow(c) // no console flash on Windows

	err := c.Run()
	res := ExecResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if c.ProcessState != nil {
		res.Exit = c.ProcessState.ExitCode()
	}

	// A context timeout/cancel must win even when the killed process also reports a non-zero exit, so authors see "timed out" not "exited -1".
	if ctxErr := ctx.Err(); ctxErr != nil {
		return res, ctxErr
	}

	// *exec.ExitError is a normal non-zero exit, not a Go error; anything else (not found, perm denied) propagates.
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return res, err
	}
	return res, nil
}
