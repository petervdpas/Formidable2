package plugin

import (
	"embed"
	gofs "io/fs"
	"log/slog"
	"path/filepath"
	"strings"
)

// pluginsFS embeds the seed plugin library shipped with the binary.
// On first boot ScaffoldPlugins materializes the tree onto disk at
// <PluginsDir>/<id>/{plugin.json,main.lua,form.json}; the running
// system never reads from this embed at refresh time - disk is the
// source of truth so user edits, imported plugins, and team-shared
// archives all live side-by-side without an "is this embedded?"
// branch in the manager.
//
// `all:plugins` brings the whole subtree so future side files
// (icons, fixture data) under a seeded plugin folder come along.
//
//go:embed all:plugins
var pluginsFS embed.FS

// seedDir is the directory inside pluginsFS that holds the embedded
// seed. Anything under <seedDir>/<id>/* gets translated to
// <PluginsDir>/<id>/* on disk.
const seedDir = "plugins"

// ScaffoldPlugins writes each embedded seed file under pluginsFS to
// its counterpart under pluginsDir, but only when the on-disk file is
// missing. Idempotent - safe to run on every boot. User edits are
// sacrosanct: once a file exists at the target path, the seed is left
// alone. Delete-to-reset works for free: removing a file before boot
// re-scaffolds the bundled copy.
//
// Errors writing one seed don't abort the whole pass - the function
// logs and moves on, mirroring the pdf cover scaffold semantics, so a
// permission glitch on one file can't block the rest of the library
// from materializing.
//
// fs is the same editorFS the Plugin manager uses for Create/Save/
// Delete. nil fs is a no-op (matches the manager's "editor not
// configured" stance).
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
