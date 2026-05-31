package gigot

import (
	"os"
	"path/filepath"
)

// wipeManagedContent removes managed content (templates/, storage/, allowlisted root files, the ledger); the .formidable/ dir and non-managed files survive.
// A missing context folder is a no-op so Reclone can call it on the first run. Destructive primitive behind Reclone; use PullLocal for the merge-aware case.
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

// removeIfExists deletes a single path, treating "missing" as success.
func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
