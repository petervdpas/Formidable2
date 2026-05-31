package plugin

import (
	"embed"
	gofs "io/fs"
	"log/slog"
	"path/filepath"
	"strings"
)

// pluginsFS embeds the seed plugin library. ScaffoldPlugins materializes it to disk on boot; refresh-time reads only
// touch disk (disk is the source of truth), so user edits, imports, and shared archives coexist without an "is this embedded?" branch.
//
//go:embed all:plugins
var pluginsFS embed.FS

const seedDir = "plugins"

// ScaffoldPlugins writes each embedded seed to disk only when the target file is missing.
// Idempotent and edit-safe (existing files are left alone); delete-to-reset re-scaffolds the bundled copy on next boot.
// A write error logs and continues so one bad file can't block the rest. nil fs is a no-op.
func ScaffoldPlugins(fs editorFS, pluginsDir string, log *slog.Logger) error {
	if fs == nil {
		return nil
	}
	if log == nil {
		log = slog.Default()
	}
	return gofs.WalkDir(pluginsFS, seedDir, func(seedPath string, d gofs.DirEntry, err error) error {
		if err != nil {
			log.Warn("plugin: scaffold walk error", "path", seedPath, "err", err)
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(seedPath, seedDir+"/")
		diskPath := filepath.Join(pluginsDir, rel)

		if fs.FileExists(diskPath) {
			return nil
		}
		seedBytes, readErr := pluginsFS.ReadFile(seedPath)
		if readErr != nil {
			log.Warn("plugin: scaffold seed read failed", "seed", seedPath, "err", readErr)
			return nil
		}
		parent := filepath.Dir(diskPath)
		if dirErr := fs.EnsureDirectory(parent); dirErr != nil {
			log.Warn("plugin: scaffold ensure dir failed", "path", parent, "err", dirErr)
			return nil
		}
		if saveErr := fs.SaveFile(diskPath, string(seedBytes)); saveErr != nil {
			log.Warn("plugin: scaffold write failed", "path", diskPath, "err", saveErr)
			return nil
		}
		log.Info("plugin: scaffolded plugin seed", "path", diskPath)
		return nil
	})
}
