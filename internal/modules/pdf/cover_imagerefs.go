package pdf

import (
	"regexp"
	"strings"
)

// extractCoverImageRefs scans a cover .html body for image references
// that must be bundled when the cover is exported for team sharing.
// Two source patterns are recognised:
//
//   - <img src="…"> attributes
//   - CSS url(…) inside <style> blocks (single, double, or unquoted)
//
// References are skipped when they are URLs (http/https/file/data/
// protocol-relative), absolute filesystem paths, or contain a `{{`
// template placeholder - the latter is filled at render time by
// picoloom from the user's frontmatter (e.g. `cover.logo` →
// `{{.Logo}}`), so it has no static asset to bundle.
//
// Returns refs in first-seen order, de-duplicated.
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
