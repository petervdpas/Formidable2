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

// CoverImageDescriptor is one entry returned by ListCoverImages.
// IsSeed flags an embedded seed; deleting one is allowed since the
// next boot's scaffold re-writes it from the embed.
type CoverImageDescriptor struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	IsSeed bool   `json:"isSeed"`
}

// ErrCoverImageInvalid wraps every image-surface validation failure.
var ErrCoverImageInvalid = errors.New("pdf: invalid cover image")

var coverImageNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

var coverImageExtensions = map[string]struct{}{
	".png":  {},
	".jpg":  {},
	".jpeg": {},
	".gif":  {},
	".svg":  {},
	".webp": {},
}

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

// ListCoverImages returns one descriptor per recognised image under
// <AppRoot>/pdf/covers/images/. Unknown extensions are skipped silently.
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

// SaveCoverImage atomically writes data to <AppRoot>/pdf/covers/images/<name>.
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

// LoadCoverImage reads the raw bytes for one image.
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
// optimistic UI may delete twice on a race). Seeds are restored by the
// next boot's scaffold.
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
