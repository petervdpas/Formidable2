package pdf

import (
	gofs "io/fs"
	"log/slog"
	"path"
	"strings"
)

// onDiskCoversDir is where covers live at runtime, relative to the
// storeFS root. User-editable + gigot-synced; the embed is only a
// first-run seed source.
const onDiskCoversDir = "pdf/covers"

// scaffoldCovers writes each embedded seed to disk only when the
// on-disk file is missing. Idempotent; user edits are sacrosanct (an
// existing file is left alone, so delete-to-reset works for free). A
// write error on one seed logs and continues rather than aborting the
// pass.
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
