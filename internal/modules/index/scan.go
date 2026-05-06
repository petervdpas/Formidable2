package index

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FileEntry is the unit of comparison between disk and the index.
// Filename is the basename only (e.g. "basic.yaml" or "test.meta.json")
// — paths get rebuilt in scan/reconcile, never carried in the entry.
type FileEntry struct {
	Filename string
	Mtime    int64 // unix nanoseconds
	Size     int64
}

// ScanResult is what a single disk walk produces. Forms and Images are
// keyed by template stem (the template filename minus ".yaml") so the
// reconciler can match disk against the index without further parsing.
type ScanResult struct {
	Templates []FileEntry
	Forms     map[string][]FileEntry // template-stem → *.meta.json files
	Images    map[string][]FileEntry // template-stem → image files
}

// scanDisk walks <root>/templates and <root>/storage and returns the
// canonical disk view: every .yaml under templates/, every .meta.json
// under storage/<stem>/, every file (any extension) under
// storage/<stem>/images/. Missing directories are not errors —
// fresh contexts may have neither yet.
//
// Hidden files (".formidable", ".DS_Store", etc.) are skipped at every
// level so platform crud doesn't sneak into the index.
func scanDisk(root string) (*ScanResult, error) {
	res := &ScanResult{
		Forms:  map[string][]FileEntry{},
		Images: map[string][]FileEntry{},
	}

	templatesDir := filepath.Join(root, "templates")
	storageDir := filepath.Join(root, "storage")

	templates, err := listFilesByExt(templatesDir, ".yaml")
	if err != nil {
		return nil, err
	}
	res.Templates = templates

	stems, err := listSubdirs(storageDir)
	if err != nil {
		return nil, err
	}
	for _, stem := range stems {
		stemDir := filepath.Join(storageDir, stem)

		forms, err := listFilesBySuffix(stemDir, ".meta.json")
		if err != nil {
			return nil, err
		}
		if len(forms) > 0 {
			res.Forms[stem] = forms
		}

		images, err := listAllFiles(filepath.Join(stemDir, "images"))
		if err != nil {
			return nil, err
		}
		if len(images) > 0 {
			res.Images[stem] = images
		}
	}
	return res, nil
}

// diffEntries computes the (added, changed, removed) sets between disk
// and the index for a single bucket of files (templates, one
// template's forms, or one template's images). Equality is defined as
// (mtime, size) — equal mtime + different size catches sub-resolution
// rewrites that some filesystems silently allow.
type Diff struct {
	Added   []FileEntry
	Changed []FileEntry
	Removed []string
}

func diffEntries(disk, idx []FileEntry) Diff {
	idxByName := make(map[string]FileEntry, len(idx))
	for _, e := range idx {
		idxByName[e.Filename] = e
	}

	out := Diff{}
	seen := make(map[string]struct{}, len(disk))
	for _, d := range disk {
		seen[d.Filename] = struct{}{}
		prev, ok := idxByName[d.Filename]
		switch {
		case !ok:
			out.Added = append(out.Added, d)
		case prev.Mtime != d.Mtime || prev.Size != d.Size:
			out.Changed = append(out.Changed, d)
		}
	}
	for _, e := range idx {
		if _, ok := seen[e.Filename]; !ok {
			out.Removed = append(out.Removed, e.Filename)
		}
	}
	return out
}

// listFilesByExt returns plain files in dir whose name ends in ext.
// Missing dir → empty slice, no error.
func listFilesByExt(dir, ext string) ([]FileEntry, error) {
	return readDirFiltered(dir, func(name string) bool {
		return strings.HasSuffix(name, ext)
	})
}

// listFilesBySuffix is identical to listFilesByExt but its name reads
// better when the suffix is multi-dotted (".meta.json").
func listFilesBySuffix(dir, suffix string) ([]FileEntry, error) {
	return readDirFiltered(dir, func(name string) bool {
		return strings.HasSuffix(name, suffix)
	})
}

// listAllFiles returns every plain file in dir (no extension filter).
// Used for images/ where the user is free to pick png/jpg/svg/whatever.
func listAllFiles(dir string) ([]FileEntry, error) {
	return readDirFiltered(dir, func(string) bool { return true })
}

// listSubdirs returns immediate subdirectory names of dir. Missing
// dir → empty slice. Hidden dirs (leading ".") are skipped.
func listSubdirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		out = append(out, e.Name())
	}
	return out, nil
}

// readDirFiltered reads dir's plain files, keeping those whose names
// pass the predicate. Hidden files are dropped. Missing dir → nil, nil.
func readDirFiltered(dir string, keep func(name string) bool) ([]FileEntry, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || !keep(name) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		out = append(out, FileEntry{
			Filename: name,
			Mtime:    info.ModTime().UnixNano(),
			Size:     info.Size(),
		})
	}
	return out, nil
}
