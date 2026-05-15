package pdf

import (
	"embed"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	picoloom "github.com/alnah/picoloom/v2"
)

// coversFS embeds the hand-authored cover-page library + a bundled
// signature template. The signature is a verbatim copy of picoloom's
// default — kept here so that WithTemplateSet (which requires BOTH a
// cover AND a signature template) doesn't strip signature behavior
// when only the cover is being overridden.
//
//go:embed covers/*.html
var coversFS embed.FS

const coversDir = "covers"
const bundledSignaturePath = "covers/signature.html"

// ErrCoverNotFound is returned when CoverFM.Template names an
// embedded layout that doesn't exist. Surfaced via Export so the
// frontend can show a corrective error (typo in the user's
// frontmatter or template manifest).
var ErrCoverNotFound = fmt.Errorf("pdf: cover template not found")

// ErrCoverPathInvalid wraps any filesystem error encountered while
// loading a user-supplied TemplatePath.
var ErrCoverPathInvalid = fmt.Errorf("pdf: cover template path invalid")

// EmbeddedCoverNames returns the sorted list of cover layouts shipped
// in the binary (excluding `signature` — that's the bundled signature
// passthrough, not a user-pickable cover). Used by the frontend to
// populate the cover picker.
func EmbeddedCoverNames() []string {
	entries, err := coversFS.ReadDir(coversDir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		if name == "" || name == "signature" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// embeddedCover reads `covers/<name>.html` from the bundled FS. Empty
// name → ErrCoverNotFound (no implicit default; callers decide what
// "unset" means via ResolveCoverTemplateSet).
func embeddedCover(name string) (string, error) {
	if name == "" {
		return "", ErrCoverNotFound
	}
	b, err := coversFS.ReadFile(filepath.ToSlash(filepath.Join(coversDir, name+".html")))
	if err != nil {
		return "", fmt.Errorf("%w: %q: %v", ErrCoverNotFound, name, err)
	}
	return string(b), nil
}

// bundledSignature returns the verbatim copy of picoloom's default
// signature HTML. Always succeeds for a healthy binary; an error
// here is a "static asset went missing" failure that can only happen
// if go:embed regressed.
func bundledSignature() (string, error) {
	b, err := coversFS.ReadFile(bundledSignaturePath)
	if err != nil {
		return "", fmt.Errorf("pdf: bundled signature missing: %w", err)
	}
	return string(b), nil
}

// ResolveCoverTemplateSet picks the cover HTML the converter should
// inject, given a merged Frontmatter cover block and the template's
// storage directory (used to resolve relative TemplatePath values).
//
// Returns nil + nil error to mean "no override — let picoloom use
// its bundled default". Errors are returned for genuine failures:
// unknown library name, missing user file, read failures.
//
// Priority (highest first):
//  1. CoverFM.TemplatePath — user-authored HTML file. Relative paths
//     resolve against sourceDir; absolute paths are used as-is.
//  2. CoverFM.Template — name from the embedded library.
//  3. Neither set → nil (picoloom default).
func ResolveCoverTemplateSet(fm *CoverFM, sourceDir string, fs interface {
	LoadFile(path string) (string, error)
}) (*picoloom.TemplateSet, error) {
	if fm == nil {
		return nil, nil
	}
	if fm.TemplatePath == "" && fm.Template == "" {
		return nil, nil
	}

	sig, err := bundledSignature()
	if err != nil {
		return nil, err
	}

	if fm.TemplatePath != "" {
		coverHTML, err := loadCoverFromPath(fm.TemplatePath, sourceDir, fs)
		if err != nil {
			return nil, err
		}
		return picoloom.NewTemplateSet("custom-cover", coverHTML, sig), nil
	}

	coverHTML, err := embeddedCover(fm.Template)
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
func loadCoverFromPath(path, sourceDir string, fs interface {
	LoadFile(path string) (string, error)
}) (string, error) {
	if fs == nil {
		return "", fmt.Errorf("%w: no filesystem available", ErrCoverPathInvalid)
	}
	full := path
	if !filepath.IsAbs(full) {
		full = filepath.Join(sourceDir, path)
	}
	full = filepath.Clean(full)
	content, err := fs.LoadFile(full)
	if err != nil {
		return "", fmt.Errorf("%w: %q: %v", ErrCoverPathInvalid, path, err)
	}
	return content, nil
}
