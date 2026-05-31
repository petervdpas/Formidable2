package nav

import "strings"

const scheme = "formidable://"

// ParseFormidableHref parses `formidable://<template>:<datafile>[#<fragment>]`.
// The template/datafile split is on the last ":" so a datafile that
// contains a colon still parses; the template side never does. Returns
// nil for any malformed URL (missing scheme/colon, empty halves,
// traversal sequences, or `/`/`\` in a segment).
func ParseFormidableHref(href string) *Target {
	if !strings.HasPrefix(href, scheme) {
		return nil
	}
	rest := href[len(scheme):]

	var fragment string
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		fragment = rest[i+1:]
		rest = rest[:i]
	}

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

// MakeHref builds the canonical `formidable://<template>:<datafile>`
// URL from a Target. The fragment is dropped because history doesn't
// track scroll positions yet.
func MakeHref(t *Target) string {
	if t == nil {
		return ""
	}
	return scheme + t.Template + ":" + t.Datafile
}

// isSafeName is the last barrier before a name reaches
// template.LoadTemplate / storage.LoadForm: rejects traversal, embedded
// directory separators, and empty strings.
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
