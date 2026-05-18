package plugin

import (
	"embed"
	"encoding/json"
	gofs "io/fs"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
)

// pluginsFS embeds the seed plugin library shipped with the binary.
// On first boot ScaffoldPlugins materializes the tree onto disk at
// <PluginsDir>/<id>/{plugin.json,main.lua,form.json}; the running
// system never reads from this embed at refresh time — disk is the
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

// seedAction is the per-plugin decision ScaffoldPlugins makes
// before walking individual files: write only files that are
// missing on disk, or rewrite every seed file. The "rewrite"
// branch is gated on a successful semver comparison so user-edited
// manifests without a version field (and therefore no way to
// compare) stay untouched.
type seedAction int

const (
	seedWriteIfMissing seedAction = iota
	seedWriteAll
)

// ScaffoldPlugins writes each embedded seed file under pluginsFS to
// its counterpart under pluginsDir. Per-plugin behavior:
//
//   - On-disk plugin folder is missing → write every seed file.
//   - Bundled manifest.version > on-disk manifest.version (both
//     parseable as dotted numerics) → rewrite every seed file. This
//     auto-upgrades stale seeds when the host binary ships a newer
//     bundled plugin. KV state at <PluginsDir>/.kv/<id>.json lives
//     outside the plugin folder and is untouched.
//   - Otherwise (same version, older bundled, or any version field
//     unparseable) → only write files that are missing on disk.
//     User edits stay sacrosanct on dev installs that haven't bumped
//     the manifest version.
//
// Errors on a single file don't abort the pass — the function logs
// and continues so a permission glitch can't block the rest of the
// library from materializing.
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

	decisions := pluginDecisions(fs, pluginsDir, log)

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
		pluginID := pluginIDFromRel(rel)
		action, known := decisions[pluginID]
		if !known {
			// Shouldn't happen — pluginDecisions enumerates every
			// top-level dir under seedDir. Skip safely.
			return nil
		}
		if action == seedWriteIfMissing && fs.FileExists(diskPath) {
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
		if action == seedWriteAll {
			log.Info("plugin: scaffolded plugin seed (overwrite)", "path", diskPath)
		} else {
			log.Info("plugin: scaffolded plugin seed", "path", diskPath)
		}
		return nil
	})
}

// pluginDecisions enumerates every plugin folder in the embedded
// seed and decides, per plugin, whether to rewrite all seed files
// or only fill in missing ones. The decision is cached so each
// file in the WalkDir pass costs one map lookup.
func pluginDecisions(fs editorFS, pluginsDir string, log *slog.Logger) map[string]seedAction {
	out := map[string]seedAction{}
	entries, err := pluginsFS.ReadDir(seedDir)
	if err != nil {
		log.Warn("plugin: scaffold read seed dir failed", "err", err)
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		seedManifest := seedDir + "/" + id + "/plugin.json"
		diskManifest := filepath.Join(pluginsDir, id, "plugin.json")

		if !fs.FileExists(diskManifest) {
			out[id] = seedWriteAll
			continue
		}
		seedVer := manifestVersionFromSeed(seedManifest)
		diskVer := manifestVersionFromDisk(fs, diskManifest)
		if seedVer != "" && diskVer != "" && compareDottedVersions(seedVer, diskVer) > 0 {
			out[id] = seedWriteAll
			log.Info("plugin: bundled seed newer than on-disk, re-seeding",
				"id", id, "on_disk", diskVer, "bundled", seedVer)
			continue
		}
		out[id] = seedWriteIfMissing
	}
	return out
}

func pluginIDFromRel(rel string) string {
	// rel is "<id>/<file>" or deeper. The plugin id is the first
	// path segment. Tolerate both forward and back slashes since the
	// embed.FS keys use forward; on Windows filepath.Join produces
	// backslashes but rel is pre-Join.
	rel = strings.ReplaceAll(rel, "\\", "/")
	head, _, _ := strings.Cut(rel, "/")
	return head
}

func manifestVersionFromSeed(seedPath string) string {
	raw, err := pluginsFS.ReadFile(seedPath)
	if err != nil {
		return ""
	}
	return manifestVersionFromBytes(raw)
}

func manifestVersionFromDisk(fs editorFS, diskPath string) string {
	raw, err := fs.LoadFile(diskPath)
	if err != nil {
		return ""
	}
	return manifestVersionFromBytes([]byte(raw))
}

func manifestVersionFromBytes(raw []byte) string {
	var m struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	return strings.TrimSpace(m.Version)
}

// compareDottedVersions returns -1, 0, +1 for a < b, a == b, a > b
// in a tolerant dotted-numeric semver-ish style:
//
//   - Each dot-separated segment is parsed as an integer; missing
//     segments compare as 0 so "1.2" == "1.2.0" == "1.2.0.0".
//   - When both sides' segment is numeric, numeric compare.
//   - When at least one side is non-numeric (e.g. "v0" prefix or
//     "0-rc1" suffix), fall back to lexical compare for that
//     segment only.
//
// This is NOT full semver — pre-release tags ("1.0.0-rc1") aren't
// recognized as ordering-less-than release ("1.0.0"). That's fine
// for the plugin-reseed use case where plugin authors ship plain
// X.Y.Z versions; if richer semantics are ever needed, swap in
// golang.org/x/mod/semver.
func compareDottedVersions(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := max(len(as), len(bs))
	for i := range n {
		aMissing := i >= len(as)
		bMissing := i >= len(bs)
		var sa, sb string
		if !aMissing {
			sa = as[i]
		}
		if !bMissing {
			sb = bs[i]
		}
		na, errA := strconv.Atoi(sa)
		nb, errB := strconv.Atoi(sb)
		if aMissing {
			na, errA = 0, nil
		}
		if bMissing {
			nb, errB = 0, nil
		}
		if errA == nil && errB == nil {
			switch {
			case na < nb:
				return -1
			case na > nb:
				return 1
			}
			continue
		}
		switch {
		case sa < sb:
			return -1
		case sa > sb:
			return 1
		}
	}
	return 0
}
