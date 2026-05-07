package plugin

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
)

// OSExec is the default ExecRunner: a thin os/exec wrapper that
// applies the (cwd, env, timeout) options. Args are passed as a
// slice (no shell), so plugin-supplied input cannot become a
// shell metacharacter — the trade-off is `formidable.exec` is
// not a shell; pipelines and redirection don't work without an
// explicit `bash -c "..."`.
//
// Non-zero process exits are NOT errors: they're normal results.
// Plugin authors branch on res.exit. Only "command not found",
// startup failure, or timeout return a Go error.
type OSExec struct{}

// Exec runs cmd with args. opts.Cwd / opts.Env / opts.Timeout map
// to the corresponding os/exec.Cmd fields. opts.Env is *additive*
// to the inherited environment, not a replacement — most plugin
// scripts want PATH to keep working.
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

	err := c.Run()
	res := ExecResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if c.ProcessState != nil {
		res.Exit = c.ProcessState.ExitCode()
	}

	// Timeout/cancel from the context propagates even if the process
	// also reported a non-zero exit (because it was killed by the
	// signal we delivered). Plugin authors need to know it timed out
	// rather than seeing the process "exited normally" with -1.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return res, ctxErr
	}

	// `*exec.ExitError` is "process ran and exited non-zero" — that
	// IS the result, not a Go error. Anything else (binary not
	// found, perm denied, …) propagates.
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return res, err
	}
	return res, nil
}
