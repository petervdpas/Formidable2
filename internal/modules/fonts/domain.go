// Package fonts is a small on-disk resource: user-supplied font files live under
// <AppRoot>/fonts/ and become selectable in the slide text Font picker. It mirrors
// the pdf cover-images resource (list / save / delete over a filesystem surface,
// with name validation and a traversal guard) and adds FontFaceCSS, which inlines
// every font as a data: URI so the deck renders identically on every surface,
// including the self-contained PDF baker.
package fonts

import (
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	gofs "io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
)

// onDiskFontsDir is AppRoot-relative; FS.ResolvePath joins it against AppRoot.
const onDiskFontsDir = "fonts"

// factoryDir is the embedded seed root. Font files dropped in ./factory ship in
// the binary and are scaffolded to <AppRoot>/fonts/ on boot. It ships empty by
// design (Formidable bundles no specific typeface); a README keeps the embed
// valid until real fonts are added.
const factoryDir = "factory"

//go:embed all:factory
var factoryFS embed.FS

// FS is the filesystem surface the fonts module needs; system.Manager satisfies it.
type FS interface {
	FileExists(path string) bool
	LoadFile(path string) (string, error)
	SaveFile(path, content string) error
	DeleteFile(path string) error
	ListDir(path string) ([]string, error)
}

// ErrInvalidFont wraps every validation failure on the font surface.
var ErrInvalidFont = errors.New("fonts: invalid font")

// A font basename: starts alphanumeric, then letters/digits/space/._- (spaces
// allowed so "Open Sans.woff2" keeps a readable family name).
var fontNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]*$`)

// fontExtensions maps an accepted extension to its @font-face format() token.
var fontExtensions = map[string]string{
	".woff2": "woff2",
	".woff":  "woff",
	".ttf":   "truetype",
	".otf":   "opentype",
}

var fontMIME = map[string]string{
	".woff2": "font/woff2",
	".woff":  "font/woff",
	".ttf":   "font/ttf",
	".otf":   "font/otf",
}

// FontInfo is one font. Family is the filename without its extension, used both
// as the @font-face family and the Font dropdown label. IsSeed marks a factory
// font: deleting one is allowed since Scaffold (Restore default fonts) rewrites
// it from the embed.
type FontInfo struct {
	Filename string `json:"filename"`
	Family   string `json:"family"`
	Size     int64  `json:"size"`
	IsSeed   bool   `json:"isSeed"`
}

// Manager owns the <AppRoot>/fonts/ directory. seedFS/seedDir hold the factory
// fonts (the embedded FS by default; injectable so tests can supply fakes).
type Manager struct {
	fs      FS
	seedFS  gofs.FS
	seedDir string
}

// NewManager wires the manager to a filesystem surface (the system module).
func NewManager(fs FS) *Manager {
	return &Manager{fs: fs, seedFS: factoryFS, seedDir: factoryDir}
}

func familyOf(filename string) string {
	return strings.TrimSuffix(filename, path.Ext(filename))
}

// seedNames returns the set of embedded factory font filenames (extension-filtered).
func (m *Manager) seedNames() map[string]struct{} {
	out := map[string]struct{}{}
	if m == nil || m.seedFS == nil {
		return out
	}
	_ = gofs.WalkDir(m.seedFS, m.seedDir, func(p string, d gofs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if _, ok := fontExtensions[strings.ToLower(path.Ext(p))]; ok {
			out[path.Base(p)] = struct{}{}
		}
		return nil
	})
	return out
}

// List returns one descriptor per recognised font file, sorted by family.
// Unknown extensions are skipped; a missing directory yields an empty list.
func (m *Manager) List() ([]FontInfo, error) {
	if m == nil || m.fs == nil {
		return nil, nil
	}
	entries, err := m.fs.ListDir(onDiskFontsDir)
	if err != nil {
		return nil, fmt.Errorf("fonts: list: %w", err)
	}
	seeds := m.seedNames()
	out := make([]FontInfo, 0, len(entries))
	for _, name := range entries {
		ext := strings.ToLower(path.Ext(name))
		if _, ok := fontExtensions[ext]; !ok {
			continue
		}
		size := int64(0)
		if content, err := m.fs.LoadFile(path.Join(onDiskFontsDir, name)); err == nil {
			size = int64(len(content))
		}
		_, isSeed := seeds[name]
		out = append(out, FontInfo{Filename: name, Family: familyOf(name), Size: size, IsSeed: isSeed})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Family < out[j].Family })
	return out, nil
}

// Scaffold writes any factory font missing from <AppRoot>/fonts/ back to disk.
// It runs at boot and backs the "Restore default fonts" action, so a deleted
// seed reappears. Existing files (incl. user edits) are never overwritten.
func (m *Manager) Scaffold() error {
	if m == nil || m.fs == nil {
		return nil
	}
	for name := range m.seedNames() {
		dst := path.Join(onDiskFontsDir, name)
		if m.fs.FileExists(dst) {
			continue
		}
		raw, err := gofs.ReadFile(m.seedFS, path.Join(m.seedDir, name))
		if err != nil {
			continue
		}
		if err := m.fs.SaveFile(dst, string(raw)); err != nil {
			return fmt.Errorf("fonts: scaffold %q: %w", name, err)
		}
	}
	return nil
}

// Save atomically writes a font's bytes to <AppRoot>/fonts/<name>.
func (m *Manager) Save(name string, data []byte) error {
	if m == nil || m.fs == nil {
		return fmt.Errorf("%w: filesystem unavailable", ErrInvalidFont)
	}
	if err := validateFontName(name); err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("%w: empty body", ErrInvalidFont)
	}
	if err := m.fs.SaveFile(path.Join(onDiskFontsDir, name), string(data)); err != nil {
		return fmt.Errorf("fonts: save %q: %w", name, err)
	}
	return nil
}

// Load reads one font's raw bytes.
func (m *Manager) Load(name string) ([]byte, error) {
	if m == nil || m.fs == nil {
		return nil, fmt.Errorf("%w: filesystem unavailable", ErrInvalidFont)
	}
	if err := validateFontName(name); err != nil {
		return nil, err
	}
	content, err := m.fs.LoadFile(path.Join(onDiskFontsDir, name))
	if err != nil {
		return nil, fmt.Errorf("fonts: load %q: %w", name, err)
	}
	return []byte(content), nil
}

// Delete removes one font. A missing file is a no-op (the optimistic UI may
// delete twice on a race).
func (m *Manager) Delete(name string) error {
	if m == nil || m.fs == nil {
		return fmt.Errorf("%w: filesystem unavailable", ErrInvalidFont)
	}
	if err := validateFontName(name); err != nil {
		return err
	}
	dst := path.Join(onDiskFontsDir, name)
	if !m.fs.FileExists(dst) {
		return nil
	}
	if err := m.fs.DeleteFile(dst); err != nil {
		return fmt.Errorf("fonts: delete %q: %w", name, err)
	}
	return nil
}

// FontFaceCSS returns @font-face rules for every uploaded font, each with the
// font inlined as a data: URI so the stylesheet is self-contained (works in the
// WebView and the headless-Chrome PDF baker with no separate serving). A font
// that fails to read is skipped rather than failing the whole sheet.
func (m *Manager) FontFaceCSS() (string, error) {
	infos, err := m.List()
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, fi := range infos {
		raw, err := m.Load(fi.Filename)
		if err != nil {
			continue
		}
		ext := strings.ToLower(path.Ext(fi.Filename))
		fmt.Fprintf(&sb,
			`@font-face{font-family:"%s";src:url(data:%s;base64,%s) format("%s");font-display:swap}`,
			fi.Family, fontMIME[ext], base64.StdEncoding.EncodeToString(raw), fontExtensions[ext])
	}
	return sb.String(), nil
}

func validateFontName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: empty name", ErrInvalidFont)
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf("%w: %q contains path separators or traversal", ErrInvalidFont, name)
	}
	if !fontNamePattern.MatchString(name) {
		return fmt.Errorf("%w: %q not a valid basename", ErrInvalidFont, name)
	}
	if _, ok := fontExtensions[strings.ToLower(path.Ext(name))]; !ok {
		return fmt.Errorf("%w: %q has unsupported extension", ErrInvalidFont, name)
	}
	return nil
}
