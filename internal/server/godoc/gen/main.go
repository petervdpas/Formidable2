// Command gen renders the exported API of every internal package via `go doc`
// and writes it to static/godoc.txt for the godoc package to embed. Run by
// `go generate ./internal/server/godoc` before a build so the shipped binary
// carries current docs without needing source or the go toolchain at runtime.
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	root, err := moduleRoot()
	if err != nil {
		fail(err)
	}

	pkgs, err := run(root, "go", "list", "./internal/...")
	if err != nil {
		fail(err)
	}

	var buf bytes.Buffer
	for p := range strings.FieldsSeq(pkgs) {
		doc, err := run(root, "go", "doc", "-all", p)
		if err != nil || strings.TrimSpace(doc) == "" {
			continue
		}
		buf.WriteString(doc)
		buf.WriteString("\n\n")
	}

	out := filepath.Join(root, "internal", "server", "godoc", "static")
	if err := os.MkdirAll(out, 0o755); err != nil {
		fail(err)
	}
	if err := os.WriteFile(filepath.Join(out, "godoc.txt"), buf.Bytes(), 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("godoc: wrote %d bytes\n", buf.Len())
}

func run(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

// moduleRoot walks up from the working directory to the dir holding go.mod.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "godoc gen:", err)
	os.Exit(1)
}
