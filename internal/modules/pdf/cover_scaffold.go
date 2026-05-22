package pdf

import (
	gofs "io/fs"
	"log/slog"
	"path"
	"strings"
)

// onDiskCoversDir is where covers live at runtime: <AppRoot>/pdf/covers/.
// Relative to the storeFS root (system.Manager.AppRoot). User-editable
// + gigot-synced; the embedded library inside the binary serves only
// as a first-run seed source.
const onDiskCoversDir = "pdf/covers"

// scaffoldCovers writes each embedded seed under coversFS to its
// counterpart on disk if (and only if) the on-disk file is missing.
// Walks the full embedded subtree so subdirectories like images/
// (where the default formidable.svg logo lives) get scaffolded too.
// Idempotent - safe to run on every boot. User edits are sacrosanct:
// once a file exists at the target path, the seed is left alone.
//
// Delete-to-reset works for free: removing a file before boot
// re-scaffolds the bundled copy.
//
// Errors writing one seed don't abort the whole pass - the function
// logs and moves on, so a permission glitch on one file can't block
// the rest of the library from materializing.
func scaffoldCovers(fs storeFS, log *slog.Logger) error {
	if fs == nil {
		return nil
	}
	return gofs.WalkDir(coversFS, coversDir, func(seedPath string, d gofs.DirEntry, err error) error {
		if err != nil {
			if log != nil {
				log.Warn("pdf: scaffold walk error", "path", seedPath, "err", err)
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		// Translate `covers/foo.html` (embedded) → `pdf/covers/foo.html` (disk).
		// `covers/images/formidable.svg` → `pdf/covers/images/formidable.svg`.
		rel := strings.TrimPrefix(seedPath, coversDir+"/")
		diskPath := path.Join(onDiskCoversDir, rel)

		if fs.FileExists(diskPath) {
			return nil
		}
		seedBytes, readErr := coversFS.ReadFile(seedPath)
		if readErr != nil {
			if log != nil {
				log.Warn("pdf: scaffold seed read failed", "seed", seedPath, "err", readErr)
			}
			return nil
		}
		if saveErr := fs.SaveFile(diskPath, string(seedBytes)); saveErr != nil {
			if log != nil {
				log.Warn("pdf: scaffold write failed", "path", diskPath, "err", saveErr)
			}
			return nil
		}
		if log != nil {
			log.Info("pdf: scaffolded cover seed", "path", diskPath)
		}
		return nil
	})
}
