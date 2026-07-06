package storage

import (
	"regexp"
	"strings"
)

// The datafile stem charset. A datafile name is the join key shared by the disk
// file, the index primary key (template, filename), the REST API path, and the
// virtual file system, so it must be a portable slug: [A-Za-z0-9._-].
var (
	dfWhitespace = regexp.MustCompile(`\s+`)
	dfDisallowed = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	dfDashRuns   = regexp.MustCompile(`-+`)
	dfDotRuns    = regexp.MustCompile(`\.{2,}`)
)

// SlugifyDatafileStem reduces a freely-typed entry name to a valid datafile stem
// (extension excluded). Whitespace becomes '-', disallowed characters drop,
// dash runs collapse, and separator characters are trimmed from the ends.
// Interior dots are kept (e.g. "v1.2"); case is preserved. Returns "" when
// nothing valid remains. This is the single source of truth for the rule; the
// frontend calls it rather than re-implementing the slug.
func SlugifyDatafileStem(raw string) string {
	s := strings.TrimSpace(raw)
	s = dfWhitespace.ReplaceAllString(s, "-")
	s = dfDisallowed.ReplaceAllString(s, "")
	s = dfDashRuns.ReplaceAllString(s, "-")
	// Collapse dot runs: the backend rejects any ".." substring (traversal
	// floor), so a slug must never emit one, even away from the ends.
	s = dfDotRuns.ReplaceAllString(s, ".")
	return strings.Trim(s, "-._")
}
