package pdf

import (
	"fmt"
	"log/slog"
	"path"
)

// onDiskCoversDir is where covers live at runtime: <AppRoot>/pdf/covers/.
// Relative to the storeFS root (system.Manager.AppRoot). User-editable
// + gigot-synced; the embedded library inside the binary serves only
// as a first-run seed source.
const onDiskCoversDir = "pdf/covers"

// scaffoldCovers writes each embedded seed under coversFS to its
// counterpart on disk if (and only if) the on-disk file is missing.
// Idempotent — safe to run on every boot. User edits are sacrosanct:
// once a file exists at the target path, the seed is left alone.
//
// Delete-to-reset works for free: removing pdf/covers/foo.html before
// boot re-scaffolds the bundled copy.
//
// Errors writing one seed don't abort the whole pass — the function
// logs and moves on, so a permission glitch on one file can't block
// the rest of the library from materializing.
func scaffoldCovers(fs storeFS, log *slog.Logger) error {
	if fs == nil {
		return nil
	}
	entries, err := coversFS.ReadDir(coversDir)
	if err != nil {
		return fmt.Errorf("pdf: read embedded covers: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		seedPath := path.Join(coversDir, e.Name())
		diskPath := path.Join(onDiskCoversDir, e.Name())

		if fs.FileExists(diskPath) {
			continue
		}
		seedBytes, err := coversFS.ReadFile(seedPath)
		if err != nil {
			if log != nil {
				log.Warn("pdf: scaffold seed read failed", "seed", seedPath, "err", err)
			}
			continue
		}
		if err := fs.SaveFile(diskPath, string(seedBytes)); err != nil {
			if log != nil {
				log.Warn("pdf: scaffold write failed", "path", diskPath, "err", err)
			}
			continue
		}
		if log != nil {
			log.Info("pdf: scaffolded cover seed", "path", diskPath)
		}
	}
	return nil
}
