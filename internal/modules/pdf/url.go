package pdf

import (
	"net/url"
	"path/filepath"
	"strings"
)

// ImageFileURL builds the `file://` URL that the PDF target's image-
// helper closure should emit for one image. The helper output gets
// injected verbatim into the markdown as an image destination, so it
// must satisfy goldmark/CommonMark's "link destinations may not
// contain unescaped spaces" rule — otherwise the `![alt](url)` syntax
// falls through as literal text in the rendered PDF.
//
// Linux/macOS: file:///abs/path/foo%20bar.png
// Windows:     file:///C:/abs/path/foo%20bar.png  (path prefixed with /)
//
// Backslashes are normalised to forward slashes so Windows paths work
// in URLs. The leading-slash prefix when the path doesn't start with
// one preserves the conventional triple-slash form for Windows drive
// letters (file:///C:/...).
func ImageFileURL(storageDir, name string) string {
	p := filepath.ToSlash(filepath.Join(storageDir, "images", name))
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	u := &url.URL{Scheme: "file", Path: p}
	return u.String()
}
