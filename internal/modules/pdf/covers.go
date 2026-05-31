package pdf

import (
	"embed"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	picoloom "github.com/alnah/picoloom/v2"
)

// coversFS embeds the cover-page library, signature template, and
// default logo. It is only the boot-time seed for the on-disk library;
// the running system reads from disk at render time, never the embed.
// `all:covers` brings the whole subtree including images/.
//
//go:embed all:covers
var coversFS embed.FS

const coversDir = "covers"             // embedded seed directory
const signatureFile = "signature.html" // reserved name (bundled signature)
const coverImagesSubdir = "images"     // <coversDir>/images/ - logo seeds

// ErrCoverNotFound is returned when a named cover does not exist on disk.
var ErrCoverNotFound = fmt.Errorf("pdf: cover template not found")

// ErrCoverPathInvalid wraps a filesystem error loading a CoverFM.TemplatePath.
var ErrCoverPathInvalid = fmt.Errorf("pdf: cover template path invalid")

// ErrCoverInvalid wraps a ValidateCover failure.
var ErrCoverInvalid = fmt.Errorf("pdf: cover invalid")

// ErrSignatureMissing means the bundled signature seed is gone from
// disk; only fatal when a cover override is active, since picoloom's
// WithTemplateSet requires a signature alongside the cover.
var ErrSignatureMissing = fmt.Errorf("pdf: bundled signature missing on disk")

// CoverDescriptor is one entry in the cover-picker dropdown. OK is
// false when ValidateCover errored (the picker may still show it, flagged).
type CoverDescriptor struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	OK          bool   `json:"ok"`
}

// loadDiskCover reads <onDiskCoversDir>/<name>.html and validates it.
func loadDiskCover(fs storeFS, name string) (string, error) {
	if name == "" {
		return "", ErrCoverNotFound
	}
	if name == "signature" || strings.ContainsAny(name, "/\\") {
		return "", fmt.Errorf("%w: %q is reserved or contains path separators", ErrCoverNotFound, name)
	}
	if fs == nil {
		return "", fmt.Errorf("%w: no filesystem available", ErrCoverPathInvalid)
	}
	diskPath := path.Join(onDiskCoversDir, name+".html")
	content, err := fs.LoadFile(diskPath)
	if err != nil {
		return "", fmt.Errorf("%w: %q: %v", ErrCoverNotFound, name, err)
	}
	if v := ValidateCover(content); !v.OK {
		return "", fmt.Errorf("%w: %q: %s", ErrCoverInvalid, name, summarizeIssues(v.Issues))
	}
	return content, nil
}

// loadDiskCoverRaw reads the cover without ValidateCover, so the
// editor can load a broken cover for the user to repair.
func loadDiskCoverRaw(fs storeFS, name string) (string, error) {
	if name == "" {
		return "", ErrCoverNotFound
	}
	if name == "signature" || strings.ContainsAny(name, "/\\") {
		return "", fmt.Errorf("%w: %q is reserved or contains path separators", ErrCoverNotFound, name)
	}
	if fs == nil {
		return "", fmt.Errorf("%w: no filesystem available", ErrCoverPathInvalid)
	}
	diskPath := path.Join(onDiskCoversDir, name+".html")
	content, err := fs.LoadFile(diskPath)
	if err != nil {
		return "", fmt.Errorf("%w: %q: %v", ErrCoverNotFound, name, err)
	}
	return content, nil
}

// loadDiskSignature reads the bundled signature off disk. Skips
// ValidateCover (signature uses a different magic family); a malformed
// one surfaces as a picoloom render error at convert time.
func loadDiskSignature(fs storeFS) (string, error) {
	if fs == nil {
		return "", fmt.Errorf("%w: no filesystem available", ErrCoverPathInvalid)
	}
	content, err := fs.LoadFile(path.Join(onDiskCoversDir, signatureFile))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrSignatureMissing, err)
	}
	return content, nil
}

// listDiskCovers produces one CoverDescriptor per .html under the
// covers dir. signature.html is filtered out (not user-pickable);
// invalid files get OK=false rather than being dropped.
func listDiskCovers(fs storeFS) ([]CoverDescriptor, error) {
	if fs == nil {
		return nil, nil
	}
	entries, err := fs.ListDir(onDiskCoversDir)
	if err != nil {
		return nil, fmt.Errorf("pdf: list cover dir: %w", err)
	}
	out := make([]CoverDescriptor, 0, len(entries))
	for _, fname := range entries {
		if !strings.HasSuffix(fname, ".html") {
			continue
		}
		stem := strings.TrimSuffix(fname, ".html")
		if stem == "signature" {
			continue
		}
		content, err := fs.LoadFile(path.Join(onDiskCoversDir, fname))
		desc := CoverDescriptor{Name: stem, Label: titlecase(stem)}
		if err != nil {
			desc.OK = false
			out = append(out, desc)
			continue
		}
		v := ValidateCover(content)
		desc.OK = v.OK
		if v.Token != nil {
			if v.Token.Name != "" {
				desc.Label = v.Token.Name
			}
			desc.Description = v.Token.Description
		}
		out = append(out, desc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// saveDiskCover validates then atomically writes <name>.html.
func saveDiskCover(fs storeFS, name, html string) error {
	if fs == nil {
		return fmt.Errorf("%w: no filesystem available", ErrCoverPathInvalid)
	}
	if !validCoverNameStem(name) {
		return fmt.Errorf("%w: invalid name %q", ErrCoverNotFound, name)
	}
	v := ValidateCover(html)
	if !v.OK {
		return fmt.Errorf("%w: %s", ErrCoverInvalid, summarizeIssues(v.Issues))
	}
	return fs.SaveFile(path.Join(onDiskCoversDir, name+".html"), html)
}

// deleteDiskCover removes <name>.html. Missing files are not an error
// so "delete twice" is safe.
func deleteDiskCover(fs storeFS, name string) error {
	if fs == nil {
		return fmt.Errorf("%w: no filesystem available", ErrCoverPathInvalid)
	}
	if !validCoverNameStem(name) {
		return fmt.Errorf("%w: invalid name %q", ErrCoverNotFound, name)
	}
	return fs.DeleteFile(path.Join(onDiskCoversDir, name+".html"))
}

func validCoverNameStem(name string) bool {
	if name == "" || name == "signature" {
		return false
	}
	if strings.ContainsAny(name, "/\\") {
		return false
	}
	if strings.HasPrefix(name, ".") {
		return false
	}
	return true
}

func summarizeIssues(issues []CoverIssue) string {
	parts := make([]string, 0, len(issues))
	for _, i := range issues {
		if i.Severity == CoverIssueError {
			parts = append(parts, i.Code+": "+i.Message)
		}
	}
	return strings.Join(parts, "; ")
}

func titlecase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// ResolveCoverTemplateSet picks the cover HTML to inject. Returns nil,
// nil to mean "no override; use picoloom's default". Priority: (1)
// CoverFM.TemplatePath, relative to sourceDir; (2) CoverFM.Template
// from the on-disk library; (3) neither -> nil.
func ResolveCoverTemplateSet(fm *CoverFM, sourceDir string, fs storeFS) (*picoloom.TemplateSet, error) {
	if fm == nil {
		return nil, nil
	}
	if fm.TemplatePath == "" && fm.Template == "" {
		return nil, nil
	}

	sig, err := loadDiskSignature(fs)
	if err != nil {
		return nil, err
	}

	if fm.TemplatePath != "" {
		coverHTML, err := loadCoverFromPath(fm.TemplatePath, sourceDir, fs)
		if err != nil {
			return nil, err
		}
		if v := ValidateCover(coverHTML); !v.OK {
			return nil, fmt.Errorf("%w: %q: %s", ErrCoverInvalid, fm.TemplatePath, summarizeIssues(v.Issues))
		}
		return picoloom.NewTemplateSet("custom-cover", coverHTML, sig), nil
	}

	coverHTML, err := loadDiskCover(fs, fm.Template)
	if err != nil {
		return nil, err
	}
	return picoloom.NewTemplateSet(fm.Template, coverHTML, sig), nil
}

// loadCoverFromPath reads a user-supplied cover HTML file; relative
// paths resolve against sourceDir.
func loadCoverFromPath(p, sourceDir string, fs storeFS) (string, error) {
	if fs == nil {
		return "", fmt.Errorf("%w: no filesystem available", ErrCoverPathInvalid)
	}
	full := p
	if !filepath.IsAbs(full) {
		full = filepath.Join(sourceDir, p)
	}
	full = filepath.Clean(full)
	content, err := fs.LoadFile(full)
	if err != nil {
		return "", fmt.Errorf("%w: %q: %v", ErrCoverPathInvalid, p, err)
	}
	return content, nil
}

// Keeps the errors import live for callers that add errors.Is later.
var _ = errors.Is

// ResolveCoverLogo rewrites a cover-logo reference into an absolute
// path picoloom can embed. Picoloom's Cover.Validate checks existence
// via os.Stat, so it needs a real path. Resolution order: absolute
// returned verbatim; bare filename tries images/ then sourceDir;
// relative-with-slashes tries sourceDir then images/<basename>. No
// hit returns the input unchanged so picoloom raises its own
// "logo not found".
func ResolveCoverLogo(logo, sourceDir string, fs storeFS) string {
	if logo == "" || fs == nil {
		return logo
	}
	if filepath.IsAbs(logo) {
		return logo
	}
	imagesDir := path.Join(onDiskCoversDir, coverImagesSubdir)
	if !strings.ContainsAny(logo, "/\\") {
		if hit := tryResolve(fs, imagesDir, logo); hit != "" {
			return hit
		}
		if sourceDir != "" {
			if hit := tryResolve(fs, sourceDir, logo); hit != "" {
				return hit
			}
		}
		return logo
	}
	if sourceDir != "" {
		if hit := tryResolve(fs, sourceDir, logo); hit != "" {
			return hit
		}
	}
	if hit := tryResolve(fs, imagesDir, filepath.Base(logo)); hit != "" {
		return hit
	}
	return logo
}

// BuildCoverLogoSrc returns the Cover.Logo string picoloom + Chrome can
// load cross-platform, mirroring ResolveCoverLogo's search order. For a
// central-library hit it emits the asset-server http:// URL when one is
// live, else a forward-slashed absolute path. For a sourceDir hit it
// returns the original relative ref so picoloom's RewriteRelativePaths
// makes the file:// URL. No hit returns the input verbatim.
//
// The asset server is load-bearing on Windows: pathrewrite refuses
// paths outside sourceDir, so without the loopback URL Chrome inside a
// file:// document cannot load central-library logos.
func BuildCoverLogoSrc(logo, sourceDir string, fs storeFS, as *AssetServer) string {
	if logo == "" || fs == nil {
		return logo
	}
	if isAbsoluteAny(logo) {
		return strings.ReplaceAll(logo, `\`, "/")
	}
	imagesDir := path.Join(onDiskCoversDir, coverImagesSubdir)
	bareName := !strings.ContainsAny(logo, "/\\")
	if bareName {
		if fs.FileExists(path.Join(imagesDir, logo)) {
			if u := as.URLFor(logo); u != "" {
				return u
			}
			return strings.ReplaceAll(fs.ResolvePath(path.Join(imagesDir, logo)), `\`, "/")
		}
		if sourceDir != "" && fs.FileExists(path.Join(sourceDir, logo)) {
			return logo
		}
		return logo
	}
	// Relative-with-slashes: sourceDir wins; "./subdir/foo.png" is
	// unambiguous about intent.
	if sourceDir != "" && fs.FileExists(path.Join(sourceDir, logo)) {
		return logo
	}
	base := filepath.Base(logo)
	if fs.FileExists(path.Join(imagesDir, base)) {
		if u := as.URLFor(base); u != "" {
			return u
		}
		return strings.ReplaceAll(fs.ResolvePath(path.Join(imagesDir, base)), `\`, "/")
	}
	return logo
}

// isAbsoluteAny reports whether p looks absolute on Unix OR Windows
// (incl. UNC and drive-letter forms). filepath.IsAbs is OS-aware and
// misses a Windows `C:\...` path on Linux, which BuildCoverLogoSrc must
// catch to normalise backslashes regardless of build host.
func isAbsoluteAny(p string) bool {
	if filepath.IsAbs(p) {
		return true
	}
	if strings.HasPrefix(p, `\\`) {
		return true
	}
	if len(p) >= 3 {
		c := p[0]
		isLetter := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
		if isLetter && p[1] == ':' && (p[2] == '/' || p[2] == '\\') {
			return true
		}
	}
	return false
}

// tryResolve returns the absolute path of dir/name if it exists, else "".
//
// The path is forward-slashed unconditionally: the logo lands in an
// <img src> rendered by Chrome under a file:// document, and backslashes
// trip Chrome's Windows URL parser whereas `C:/...` is WHATWG-valid
// (os.Stat accepts forward slashes too). strings.ReplaceAll, not
// filepath.ToSlash, because ToSlash is OS-aware (no-op on Linux) and
// would regress when a Windows-style path arrives on a non-Windows build.
func tryResolve(fs storeFS, dir, name string) string {
	candidate := path.Join(dir, name)
	if !fs.FileExists(candidate) {
		return ""
	}
	return strings.ReplaceAll(fs.ResolvePath(candidate), `\`, "/")
}
