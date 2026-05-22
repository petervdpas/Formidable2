package pdf

import (
	"errors"
	"fmt"
	gofs "io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
)

// CoverImageDescriptor is one entry returned by ListCoverImages. Mirrors
// the cover-picker descriptor shape so the frontend can render the
// image library with the same chrome it uses for covers.
//
// IsSeed is true when the filename matches an embedded seed (currently
// just formidable.svg). The frontend uses this to offer "Reset to
// default" - deleting a seed image is allowed; the next boot's
// scaffold pass re-writes it from the embed.
type CoverImageDescriptor struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	IsSeed bool   `json:"isSeed"`
}

// ErrCoverImageInvalid wraps every validation failure on the image
// surface (bad name, unknown extension, empty body, traversal).
var ErrCoverImageInvalid = errors.New("pdf: invalid cover image")

// coverImageNamePattern accepts simple basenames like `logo.png` or
// `team-banner_v2.svg`. No leading dot, no path separators, no
// `..` traversal.
var coverImageNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// coverImageExtensions enumerates the file extensions accepted by
// SaveCoverImage. Lowercased on entry so `.PNG` works for callers
// that don't normalise.
var coverImageExtensions = map[string]struct{}{
	".png":  {},
	".jpg":  {},
	".jpeg": {},
	".gif":  {},
	".svg":  {},
	".webp": {},
}

// seedCoverImageNames returns the basenames of images embedded under
// covers/images/ in coversFS. Used to flag IsSeed in ListCoverImages
// and to keep the frontend's reset affordance honest.
func seedCoverImageNames() []string {
	out := []string{}
	embeddedDir := path.Join(coversDir, coverImagesSubdir)
	_ = gofs.WalkDir(coversFS, embeddedDir, func(p string, d gofs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		out = append(out, path.Base(p))
		return nil
	})
	sort.Strings(out)
	return out
}

// ListCoverImages scans <AppRoot>/pdf/covers/images/ and returns one
// descriptor per recognised image. Files with unknown extensions are
// skipped silently so a stray README.txt doesn't break the picker.
func (m *Manager) ListCoverImages() ([]CoverImageDescriptor, error) {
	if m == nil || m.store == nil || m.store.fs == nil {
		return nil, nil
	}
	dir := path.Join(onDiskCoversDir, coverImagesSubdir)
	entries, err := m.store.fs.ListDir(dir)
	if err != nil {
		return nil, fmt.Errorf("pdf: list cover images: %w", err)
	}
	seeds := map[string]struct{}{}
	for _, s := range seedCoverImageNames() {
		seeds[s] = struct{}{}
	}
	out := make([]CoverImageDescriptor, 0, len(entries))
	for _, name := range entries {
		ext := strings.ToLower(path.Ext(name))
		if _, ok := coverImageExtensions[ext]; !ok {
			continue
		}
		size := int64(0)
		if content, err := m.store.fs.LoadFile(path.Join(dir, name)); err == nil {
			size = int64(len(content))
		}
		_, isSeed := seeds[name]
		out = append(out, CoverImageDescriptor{
			Name:   name,
			Size:   size,
			IsSeed: isSeed,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// SaveCoverImage writes data to <AppRoot>/pdf/covers/images/<name>
// atomically. Validates the filename and extension before touching
// disk so a bad input can't pollute the directory.
func (m *Manager) SaveCoverImage(name string, data []byte) error {
	if m == nil || m.store == nil || m.store.fs == nil {
		return fmt.Errorf("%w: filesystem unavailable", ErrCoverImageInvalid)
	}
	if err := validateCoverImageName(name); err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("%w: empty body", ErrCoverImageInvalid)
	}
	dst := path.Join(onDiskCoversDir, coverImagesSubdir, strings.ToLower(name))
	if err := m.store.fs.SaveFile(dst, string(data)); err != nil {
		return fmt.Errorf("pdf: save cover image %q: %w", name, err)
	}
	return nil
}

// LoadCoverImage reads the raw bytes for one image. Returns
// ErrCoverImageInvalid for a malformed name; bubbles fs.ErrNotExist
// (wrapped) when the file is missing.
func (m *Manager) LoadCoverImage(name string) ([]byte, error) {
	if m == nil || m.store == nil || m.store.fs == nil {
		return nil, fmt.Errorf("%w: filesystem unavailable", ErrCoverImageInvalid)
	}
	if err := validateCoverImageName(name); err != nil {
		return nil, err
	}
	src := path.Join(onDiskCoversDir, coverImagesSubdir, strings.ToLower(name))
	content, err := m.store.fs.LoadFile(src)
	if err != nil {
		return nil, fmt.Errorf("pdf: load cover image %q: %w", name, err)
	}
	return []byte(content), nil
}

// DeleteCoverImage removes one image. Missing files are a no-op (the
// frontend's optimistic UI may call delete twice on a race). Seed
// images are deletable: the next boot's scaffold restores them.
func (m *Manager) DeleteCoverImage(name string) error {
	if m == nil || m.store == nil || m.store.fs == nil {
		return fmt.Errorf("%w: filesystem unavailable", ErrCoverImageInvalid)
	}
	if err := validateCoverImageName(name); err != nil {
		return err
	}
	dst := path.Join(onDiskCoversDir, coverImagesSubdir, strings.ToLower(name))
	if !m.store.fs.FileExists(dst) {
		return nil
	}
	if err := m.store.fs.DeleteFile(dst); err != nil {
		return fmt.Errorf("pdf: delete cover image %q: %w", name, err)
	}
	return nil
}

// validateCoverImageName rejects empty strings, traversal, path
// separators, leading-dot, and unknown extensions in one shot. Case
// is normalised on the consumer side; this just enforces the shape.
func validateCoverImageName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: empty name", ErrCoverImageInvalid)
	}
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return fmt.Errorf("%w: %q contains path separators or traversal", ErrCoverImageInvalid, name)
	}
	lower := strings.ToLower(name)
	if !coverImageNamePattern.MatchString(lower) {
		return fmt.Errorf("%w: %q not a valid basename", ErrCoverImageInvalid, name)
	}
	ext := path.Ext(lower)
	if _, ok := coverImageExtensions[ext]; !ok {
		return fmt.Errorf("%w: %q has unsupported extension", ErrCoverImageInvalid, name)
	}
	return nil
}
