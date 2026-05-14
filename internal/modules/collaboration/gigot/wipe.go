package gigot

import (
	"os"
	"path/filepath"
)

// wipeManagedContent removes every file gigot considers "managed"
// under contextFolder: the templates/ and storage/ subtrees, the
// allowlisted root files (README.md, .gitignore), and the
// .formidable/sync.json ledger. The context folder itself, the
// .formidable/ directory, and any non-managed files survive. A
// missing context folder is treated as a no-op so the Reclone
// orchestrator can call this safely on the very first run.
//
// This is the explicit-destructive primitive behind Reclone. Use
// PullLocal for the merge-aware case where local-only edits should
// survive the round-trip.
func wipeManagedContent(contextFolder string) error {
	if contextFolder == "" {
		return ErrMissingContext
	}
	root := filepath.Clean(contextFolder)
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if err := os.RemoveAll(filepath.Join(root, "templates")); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(root, "storage")); err != nil {
		return err
	}
	for name := range rootAllowlist {
		if err := removeIfExists(filepath.Join(root, name)); err != nil {
			return err
		}
	}
	if err := removeIfExists(TrackRecordPath(root)); err != nil {
		return err
	}
	return nil
}

// removeIfExists deletes a single path and treats "missing" as
// success. Distinct from os.RemoveAll so the wipe pass doesn't have
// to special-case file vs directory.
func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
