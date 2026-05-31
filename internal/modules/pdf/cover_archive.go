package pdf

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

var (
	ErrCoverArchiveInvalid   = errors.New("pdf: cover archive invalid")
	ErrCoverArchiveTraversal = errors.New("pdf: cover archive path traversal blocked")
	ErrCoverArchiveExists    = errors.New("pdf: cover already exists (set overwrite=true to replace)")
	ErrCoverArchiveNotFound  = errors.New("pdf: cover archive not found")
)

// ExportCoverArchiveResult describes what was bundled into the zip.
// MissingImages collects refs the .html mentions but that are absent
// on disk, surfaced so the user can chase them before sharing.
type ExportCoverArchiveResult struct {
	Name          string   `json:"name"`
	ZipPath       string   `json:"zip_path"`
	Images        []string `json:"images"`
	MissingImages []string `json:"missing_images,omitempty"`
}

// ImportCoverArchiveResult describes what was materialised on import.
type ImportCoverArchiveResult struct {
	Name        string   `json:"name"`
	Overwritten bool     `json:"overwritten"`
	Images      []string `json:"images,omitempty"`
}

// exportCoverArchive zips the cover .html plus its image refs.
// Missing refs do NOT abort; they are reported in the result.
func exportCoverArchive(fs storeFS, coverName, zipPath string) (ExportCoverArchiveResult, error) {
	var zero ExportCoverArchiveResult
	if fs == nil {
		return zero, fmt.Errorf("%w: no filesystem available", ErrCoverPathInvalid)
	}
	if coverName == "" {
		return zero, fmt.Errorf("%w: cover name required", ErrCoverArchiveInvalid)
	}
	if zipPath == "" {
		return zero, fmt.Errorf("%w: zip path required", ErrCoverArchiveInvalid)
	}
	if !validCoverNameStem(coverName) {
		return zero, fmt.Errorf("%w: invalid cover name %q", ErrCoverArchiveInvalid, coverName)
	}

	htmlRel := path.Join(onDiskCoversDir, coverName+".html")
	if !fs.FileExists(htmlRel) {
		return zero, fmt.Errorf("%w: %s", ErrCoverArchiveNotFound, coverName)
	}
	html, err := fs.LoadFile(htmlRel)
	if err != nil {
		return zero, fmt.Errorf("read cover: %w", err)
	}

	refs := extractCoverImageRefs(html)
	var (
		bundled []string
		missing []string
		assets  = map[string]string{}
	)
	for _, ref := range refs {
		hit, body, found := resolveExportImage(fs, ref)
		if !found {
			missing = append(missing, ref)
			continue
		}
		basename := path.Base(hit)
		if _, ok := assets[basename]; ok {
			continue
		}
		assets[basename] = body
		bundled = append(bundled, basename)
	}

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	if err := writeZipEntry(zw, coverName+".html", html); err != nil {
		return zero, err
	}
	for name, body := range assets {
		if err := writeZipEntry(zw, "images/"+name, body); err != nil {
			return zero, err
		}
	}
	if err := zw.Close(); err != nil {
		return zero, fmt.Errorf("finalize zip: %w", err)
	}

	if err := fs.SaveFile(zipPath, buf.String()); err != nil {
		return zero, fmt.Errorf("write zip: %w", err)
	}

	return ExportCoverArchiveResult{
		Name:          coverName,
		ZipPath:       fs.ResolvePath(zipPath),
		Images:        bundled,
		MissingImages: missing,
	}, nil
}

// importCoverArchive materialises a cover-archive zip under
// <onDiskCoversDir>/. The zip must hold exactly one root *.html (the
// cover) plus assets under images/, with no traversal after Clean.
// Refuses to clobber an existing cover unless overwrite=true; image
// assets always overwrite (bundle-bound resources, not user state).
func importCoverArchive(fs storeFS, zipPath string, overwrite bool) (ImportCoverArchiveResult, error) {
	var zero ImportCoverArchiveResult
	if fs == nil {
		return zero, fmt.Errorf("%w: no filesystem available", ErrCoverPathInvalid)
	}
	if zipPath == "" {
		return zero, fmt.Errorf("%w: zip path required", ErrCoverArchiveInvalid)
	}
	if !fs.FileExists(zipPath) {
		return zero, fmt.Errorf("%w: %s", ErrCoverArchiveNotFound, zipPath)
	}
	raw, err := fs.LoadFile(zipPath)
	if err != nil {
		return zero, fmt.Errorf("read zip: %w", err)
	}
	zr, err := zip.NewReader(bytes.NewReader([]byte(raw)), int64(len(raw)))
	if err != nil {
		return zero, fmt.Errorf("%w: %v", ErrCoverArchiveInvalid, err)
	}

	var (
		htmlName     string
		htmlBody     string
		htmlSeen     int
		imageEntries = map[string]string{}
	)
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		cleaned := path.Clean(f.Name)
		if strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") || strings.HasPrefix(cleaned, "/") {
			return zero, fmt.Errorf("%w: %q", ErrCoverArchiveTraversal, f.Name)
		}
		switch {
		case strings.HasSuffix(cleaned, ".html") && !strings.Contains(cleaned, "/"):
			body, err := readZipFile(f)
			if err != nil {
				return zero, fmt.Errorf("%w: read %q: %v", ErrCoverArchiveInvalid, f.Name, err)
			}
			htmlName = strings.TrimSuffix(cleaned, ".html")
			htmlBody = body
			htmlSeen++
		case strings.HasPrefix(cleaned, "images/") && strings.Count(cleaned, "/") == 1:
			body, err := readZipFile(f)
			if err != nil {
				return zero, fmt.Errorf("%w: read %q: %v", ErrCoverArchiveInvalid, f.Name, err)
			}
			imageEntries[strings.TrimPrefix(cleaned, "images/")] = body
		default:
			return zero, fmt.Errorf("%w: unexpected entry %q (expected <name>.html or images/<file>)", ErrCoverArchiveInvalid, f.Name)
		}
	}
	if htmlSeen == 0 {
		return zero, fmt.Errorf("%w: zip contains no cover html at root", ErrCoverArchiveInvalid)
	}
	if htmlSeen > 1 {
		return zero, fmt.Errorf("%w: zip contains multiple cover html files at root", ErrCoverArchiveInvalid)
	}
	if !validCoverNameStem(htmlName) {
		return zero, fmt.Errorf("%w: invalid cover name %q in zip", ErrCoverArchiveInvalid, htmlName)
	}

	v := ValidateCover(htmlBody)
	if !v.OK {
		return zero, fmt.Errorf("%w: %s", ErrCoverInvalid, summarizeIssues(v.Issues))
	}

	htmlTarget := path.Join(onDiskCoversDir, htmlName+".html")
	existed := fs.FileExists(htmlTarget)
	if existed && !overwrite {
		return zero, fmt.Errorf("%w: %s", ErrCoverArchiveExists, htmlName)
	}

	if err := fs.SaveFile(htmlTarget, htmlBody); err != nil {
		return zero, fmt.Errorf("write cover html: %w", err)
	}
	var imagesOut []string
	for name, body := range imageEntries {
		dst := path.Join(onDiskCoversDir, coverImagesSubdir, name)
		if err := fs.SaveFile(dst, body); err != nil {
			return zero, fmt.Errorf("write image %q: %w", name, err)
		}
		imagesOut = append(imagesOut, name)
	}

	return ImportCoverArchiveResult{
		Name:        htmlName,
		Overwritten: existed,
		Images:      imagesOut,
	}, nil
}

// resolveExportImage looks up an image ref, trying images/<ref> then
// <ref>. Mirrors ResolveCoverLogo's priority so the bundle decision
// matches runtime resolution.
func resolveExportImage(fs storeFS, ref string) (string, string, bool) {
	candidates := []string{
		path.Join(onDiskCoversDir, coverImagesSubdir, ref),
		path.Join(onDiskCoversDir, ref),
	}
	for _, c := range candidates {
		if !fs.FileExists(c) {
			continue
		}
		body, err := fs.LoadFile(c)
		if err != nil {
			continue
		}
		return c, body, true
	}
	return "", "", false
}

func writeZipEntry(zw *zip.Writer, name, body string) error {
	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("zip create %q: %w", name, err)
	}
	if _, err := io.WriteString(w, body); err != nil {
		return fmt.Errorf("zip write %q: %w", name, err)
	}
	return nil
}

func readZipFile(f *zip.File) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
