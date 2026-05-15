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

// coversFS embeds the hand-authored cover-page library + a bundled
// signature template + a default logo image. Post Stage 6.1, the
// embed is the *seed* used at boot to scaffold the on-disk library
// at <AppRoot>/pdf/covers/. The running system never reads from the
// embed at render time — disk is the source of truth.
//
// `all:covers` brings the entire subtree, including the images/
// subdirectory (where formidable.svg lives as a default logo seed).
//
//go:embed all:covers
var coversFS embed.FS

const coversDir = "covers"             // embedded seed directory
const signatureFile = "signature.html" // reserved name (bundled signature)
const coverImagesSubdir = "images"     // <coversDir>/images/ — logo seeds

// ErrCoverNotFound is returned when CoverFM.Template names a cover
// that doesn't exist on disk under <AppRoot>/pdf/covers/. Surfaced
// via Export so the frontend can show a corrective error.
var ErrCoverNotFound = fmt.Errorf("pdf: cover template not found")

// ErrCoverPathInvalid wraps any filesystem error encountered while
// loading a user-supplied CoverFM.TemplatePath.
var ErrCoverPathInvalid = fmt.Errorf("pdf: cover template path invalid")

// ErrCoverInvalid wraps any failure from ValidateCover — used at both
// load time (loader refuses to inject a broken cover) and save time
// (SaveCover refuses to persist one).
var ErrCoverInvalid = fmt.Errorf("pdf: cover invalid")

// ErrSignatureMissing is returned when the bundled signature seed is
// gone from disk AND the scaffold hasn't replaced it. Only surfaces
// during Export when a cover override is also active — picoloom's
// WithTemplateSet requires a signature template alongside the cover.
var ErrSignatureMissing = fmt.Errorf("pdf: bundled signature missing on disk")

// CoverDescriptor is one entry in the cover-picker dropdown. Returned
// by ListCovers; consumed by the frontend.
//
//   - Name: filename stem (the value users put in `cover.template:`).
//   - Label: human-readable display name from the magic-line `name:`
//     field, or capitalised Name when absent.
//   - Description: from the magic-line `description:` field, or "".
//   - OK: false when ValidateCover errored — the picker may still
//     surface the entry, but flagged.
type CoverDescriptor struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	OK          bool   `json:"ok"`
}

// loadDiskCover reads `<onDiskCoversDir>/<name>.html` via fs and
// validates it. Used by both the renderer (Manager.Export) and the
// picker (Manager.ListCovers).
//
// Returns:
//   - (html, nil) for a valid cover
//   - ("", ErrCoverNotFound) for missing files
//   - ("", ErrCoverInvalid + details) for files that fail validation
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

// loadDiskSignature reads the bundled-default signature off disk.
// Surfaces ErrSignatureMissing if the file isn't there — caller
// decides whether that's fatal. Skips ValidateCover (signature uses
// a different magic family); a malformed signature will surface as
// a picoloom render error at convert time.
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

// listDiskCovers scans <AppRoot>/pdf/covers/ via the (extended)
// storeFS.ListDir method and produces one CoverDescriptor per .html
// entry. signature.html is filtered out (it's not user-pickable).
// Invalid files are returned with OK=false so the picker can flag
// them rather than silently dropping the user's existing files.
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

// saveDiskCover validates incoming HTML, then writes it atomically
// to <AppRoot>/pdf/covers/<name>.html. Used by Service.SaveCover.
//
// Name must be a safe filename stem: no path separators, no leading
// dot, not the reserved "signature". Invalid HTML → ErrCoverInvalid.
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

// summarizeIssues collapses error-severity issues into a single
// human-readable summary for wrapping into Err* values.
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

// ResolveCoverTemplateSet picks the cover HTML the converter should
// inject, given a merged Frontmatter cover block and the template's
// storage directory (used to resolve relative TemplatePath values).
//
// Returns nil + nil error to mean "no override — let picoloom use
// its bundled default". Errors are returned for genuine failures:
// unknown library name, missing user file, validation failures,
// missing bundled signature on disk.
//
// Priority (highest first):
//  1. CoverFM.TemplatePath — user-authored HTML file. Relative paths
//     resolve against sourceDir; absolute paths are used as-is.
//  2. CoverFM.Template — name from the on-disk library at
//     <AppRoot>/pdf/covers/<name>.html.
//  3. Neither set → nil (picoloom default).
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

// loadCoverFromPath reads a user-supplied cover HTML file. Relative
// paths resolve against sourceDir; absolute paths are used as-is. The
// fs argument is the same storeFS the pdf module already uses for
// state + PDF writes — passing it through keeps unit tests off the
// real filesystem.
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

// Convenience guard so the package compiles when nothing else uses
// errors.Is on ErrSignatureMissing (callers may add later).
var _ = errors.Is

// ResolveCoverLogo rewrites a cover-logo reference into an absolute
// filesystem path that picoloom can validate + embed. Picoloom's
// Cover.Validate checks the path exists via os.Stat, so we have to
// hand it a real path (or leave it alone for picoloom to surface a
// "file not found" error to the user).
//
// Resolution order:
//
//   - Empty input → empty output. The cover HTML's `{{if .Logo}}`
//     guard collapses the image zone gracefully.
//   - Absolute path → returned verbatim. Honors fully-qualified user
//     references like `/home/peter/team-logo.png`.
//   - Bare filename (no slashes) → first try
//     `<AppRoot>/pdf/covers/images/<name>`, then `<sourceDir>/<name>`.
//     This is what makes `cover.logo: formidable.svg` work: the
//     scaffolded default lives in the central images dir and is
//     gigot-synced for team consistency.
//   - Relative path with slashes → resolved against sourceDir first,
//     then against `<AppRoot>/pdf/covers/images/<basename>` as a
//     fallback for users who half-remember the search-path rule.
//
// If no candidate exists, returns the original input unchanged so
// picoloom's own existence check produces the canonical
// "cover logo file not found" error.
func ResolveCoverLogo(logo, sourceDir string, fs storeFS) string {
	if logo == "" || fs == nil {
		return logo
	}
	if filepath.IsAbs(logo) {
		return logo
	}
	imagesDir := path.Join(onDiskCoversDir, coverImagesSubdir)
	if !strings.ContainsAny(logo, "/\\") {
		// Bare filename — central images dir wins.
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
	// Relative path with slashes — sourceDir wins.
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

// tryResolve returns the absolute path of dir/name if the file
// exists, otherwise an empty string.
func tryResolve(fs storeFS, dir, name string) string {
	candidate := path.Join(dir, name)
	if !fs.FileExists(candidate) {
		return ""
	}
	return fs.ResolvePath(candidate)
}
