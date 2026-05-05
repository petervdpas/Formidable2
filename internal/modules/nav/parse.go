package nav

import "strings"

const scheme = "formidable://"

// ParseFormidableHref parses `formidable://<template>:<datafile>[#<fragment>]`.
//
// Mirrors the original `parseFormidableHref` (utils/fieldRenderers.js)
// with the same lastIndexOf-of-":" split — that split rule is
// deliberately permissive on the datafile side because filenames can
// in theory contain colons, but the template side never does. Callers
// of ParseFormidableHref then validate template/datafile against the
// real disk layout downstream.
//
// Returns nil for any URL that isn't well-formed: missing scheme,
// missing colon, empty halves, traversal sequences, or path segments
// that contain `/` or `\` (we only navigate to top-level template
// names + datafile names, never into nested paths).
func ParseFormidableHref(href string) *Target {
	if !strings.HasPrefix(href, scheme) {
		return nil
	}
	rest := href[len(scheme):]

	// Split off optional #fragment — pure suffix, no extra rules.
	var fragment string
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		fragment = rest[i+1:]
		rest = rest[:i]
	}

	// Template / datafile split: last ":" so weird datafiles with a
	// colon still parse the way the JS implementation did.
	idx := strings.LastIndexByte(rest, ':')
	if idx <= 0 || idx == len(rest)-1 {
		return nil
	}
	tpl := rest[:idx]
	df := rest[idx+1:]

	if !isSafeName(tpl) || !isSafeName(df) {
		return nil
	}
	return &Target{Template: tpl, Datafile: df, Fragment: fragment}
}

// isSafeName rejects path traversal, embedded directory separators,
// and empty strings. The Vue + render layers already pass user-typed
// values through, but this is the last barrier before we hand a name
// to template.LoadTemplate / storage.LoadForm.
func isSafeName(s string) bool {
	if s == "" {
		return false
	}
	if strings.Contains(s, "..") {
		return false
	}
	if strings.ContainsAny(s, `/\`) {
		return false
	}
	return true
}
