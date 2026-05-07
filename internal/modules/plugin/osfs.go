package plugin

import (
	"os"
	"path/filepath"
)

// OSFS is the default FSAccess: direct os.* calls, no atomic-write
// machinery. Plugins use this to read/write paths outside the
// Formidable storage tree (the "Azure DevOps wiki" use case writes
// to <home>/wikis/<repo>/...).
//
// Atomicity isn't free here, but plugins authoring outside the
// storage tree are already crossing the trust boundary — if they
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
