package plugin

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestOSExec_EchoStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX echo")
	}
	res, err := OSExec{}.Exec("echo", []string{"hello"}, ExecOptions{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(res.Stdout, "hello") || res.Exit != 0 {
		t.Fatalf("got %+v", res)
	}
}

func TestOSExec_NonzeroExitIsNotErr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX false")
	}
	res, err := OSExec{}.Exec("false", nil, ExecOptions{})
	if err != nil {
		t.Fatalf("err should be nil (exit-status is data): %v", err)
	}
	if res.Exit == 0 {
		t.Fatalf("expected nonzero exit, got %+v", res)
	}
}

func TestOSExec_CommandNotFoundIsErr(t *testing.T) {
	_, err := OSExec{}.Exec("/no/such/command/lalalalalala", nil, ExecOptions{})
	if err == nil {
		t.Fatal("want error")
	}
}

func TestOSExec_TimeoutKills(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sleep")
	}
	start := time.Now()
	_, err := OSExec{}.Exec("sleep", []string{"5"}, ExecOptions{Timeout: 100 * time.Millisecond})
	if time.Since(start) > 2*time.Second {
		t.Fatalf("timeout didn't fire (took %v)", time.Since(start))
	}
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestOSExec_CwdHonored(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX pwd")
	}
	tmp := t.TempDir()
	res, err := OSExec{}.Exec("pwd", nil, ExecOptions{Cwd: tmp})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(res.Stdout, tmp) {
		t.Fatalf("expected %s in stdout, got %q", tmp, res.Stdout)
	}
}
