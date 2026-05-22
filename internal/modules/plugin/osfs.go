package plugin

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// OSFS is the default FSAccess: direct os.* calls, no atomic-write
// machinery. Plugins use this to read/write paths outside the
// Formidable storage tree (the "Azure DevOps wiki" use case writes
// to <home>/wikis/<repo>/...).
//
// Atomicity isn't free here, but plugins authoring outside the
// storage tree are already crossing the trust boundary - if they
// need durability guarantees they can call OS-level fsync via
// formidable.exec. Inside the storage tree, plugin authors should
// prefer formidable.form.save, which goes through the storage
// manager's atomic-write path.
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

// Copy streams from→to, creating the destination's parent dirs. Uses
// io.Copy so large image transfers don't have to fit in a Lua string.
// Existing destination is overwritten - the wiki-export use case
// re-renders images on every run, so refusing to overwrite would mean
// every run leaves stale files behind.
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

// Remove deletes a file or empty directory. Missing target is a no-op
// - saves plugin authors a stat-then-rm dance. Use os.RemoveAll
// semantics? No: too easy to nuke a directory by accident. Plugins
// that want recursive remove can list+remove themselves.
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
