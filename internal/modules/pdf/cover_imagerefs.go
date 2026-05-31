package pdf

import (
	"regexp"
	"strings"
)

// extractCoverImageRefs scans <img src> and CSS url() refs to bundle.
// URLs, absolute paths, and `{{...}}` placeholders are skipped: a
// placeholder is filled at render time from frontmatter, so it has no
// static asset to bundle. Returns first-seen order, de-duplicated.
var (
	imgSrcRE = regexp.MustCompile(`(?i)<img\b[^>]*?\bsrc\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	cssURLRE = regexp.MustCompile(`(?i)url\(\s*(?:"([^"]*)"|'([^']*)'|([^)\s'"]+))\s*\)`)
)

func extractCoverImageRefs(html string) []string {
	if html == "" {
		return nil
	}
	seen := make(map[string]struct{})
	out := []string{}

	add := func(ref string) {
		ref = strings.TrimSpace(ref)
		if !isBundlableImageRef(ref) {
			return
		}
		if _, ok := seen[ref]; ok {
			return
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}

	for _, m := range imgSrcRE.FindAllStringSubmatch(html, -1) {
		add(firstNonEmpty(m[1], m[2]))
	}
	for _, m := range cssURLRE.FindAllStringSubmatch(html, -1) {
		add(firstNonEmpty(m[1], m[2], m[3]))
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func isBundlableImageRef(ref string) bool {
	if ref == "" {
		return false
	}
	if strings.Contains(ref, "{{") {
		return false
	}
	switch {
	case strings.HasPrefix(ref, "http://"),
		strings.HasPrefix(ref, "https://"),
		strings.HasPrefix(ref, "file://"),
		strings.HasPrefix(ref, "data:"),
		strings.HasPrefix(ref, "//"):
		return false
	}
	if strings.HasPrefix(ref, "/") {
		return false
	}
	if len(ref) >= 2 && ref[1] == ':' {
		return false
	}
	return true
}
