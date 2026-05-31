package plugin

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// OSFS is the default FSAccess: direct os.* calls, no atomic-write machinery, for paths outside the storage tree.
// Inside the storage tree, plugins should prefer formidable.form.save, which goes through the atomic-write path.
type OSFS struct{}

func (OSFS) Read(p string) (string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (OSFS) Write(p, content string) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(content), 0o644)
}

func (OSFS) Mkdir(p string) error { return os.MkdirAll(p, 0o755) }

func (OSFS) List(p string) ([]string, error) {
	entries, err := os.ReadDir(p)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out, nil
}

func (OSFS) Exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// Copy streams from to to via io.Copy (large images needn't fit in a Lua string), creating parent dirs and overwriting the destination.
func (OSFS) Copy(from, to string) error {
	if from == "" || to == "" {
		return errors.New("fs.copy: empty path")
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return nil
}

// Remove deletes a file or empty dir; missing target is a no-op. Not os.RemoveAll: recursive remove is too easy to misfire, so plugins do it themselves.
func (OSFS) Remove(p string) error {
	if p == "" {
		return errors.New("fs.remove: empty path")
	}
	err := os.Remove(p)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
