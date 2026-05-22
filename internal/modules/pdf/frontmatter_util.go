package pdf

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ErrFrontmatterAlreadyPresent is returned by InjectFrontmatter when
// the markdown body already starts with a `---` block. The user
// should run MigrateFrontmatter instead.
var ErrFrontmatterAlreadyPresent = errors.New("pdf: markdown already has a frontmatter block")

// canonicalScaffold is the full-scaffold picoloom frontmatter the
// Inject utility prepends to a template's markdown_template. It
// surfaces every major block (cover + page uncommented; toc / footer
// / signature / watermark / pageBreaks commented out as starting
// points) so the user can fill in what they need without having to
// look up the spec. Stay in sync with internal/modules/pdf/input.go
// Frontmatter struct - the field names below must match its yaml
// tags.
const canonicalScaffold = `---
# PDF export frontmatter (picoloom v2 shape). Inserted by
# Formidable's "Inject PDF Frontmatter" utility. See
# Information → PDF Export → Frontmatter directives for the spec.

# Top-level style: picoloom theme name ("technical", "academic", ...)
# or absolute path to a custom .css file. Empty = picoloom's built-in
# default.
style:

# Document keywords - written into the PDF /Keywords Info-dictionary
# entry by the post-render pdfcpu pass. Searchable by OS file indexers
# and visible in PDF readers' Document Properties.
#keywords:
#  - Audit
#  - Governance

# Page layout.
page:
  size: a4           # a4 | letter | legal
  orientation: portrait
  margin: 1.0        # inches

# Cover page. Pick a layout from <AppRoot>/pdf/covers/, then fill in
# whichever fields your cover HTML uses. Empty fields are skipped.
cover:
  enabled: true
  template: classic  # classic | banner | corporate | (your own)
  title:
  subtitle:
  logo:              # bare filename → looked up in pdf/covers/images/
  author:
  authorTitle:
  organization:
  date:
  version:
  clientName:
  projectName:
  documentType:
  documentID:
  description:
  department:

# Table of contents - uncomment to enable.
#toc:
#  enabled: true
#  title: Contents
#  minDepth: 1
#  maxDepth: 3

# Footer - uncomment to enable.
#footer:
#  enabled: true
#  position: bottom
#  showPageNumber: true
#  text:

# Signature block - uncomment to enable.
#signature:
#  enabled: true
#  name:
#  title:
#  email:
#  organization:
#  imagePath:

# Watermark - uncomment to enable.
#watermark:
#  enabled: true
#  text: DRAFT
#  color: '#888888'
#  opacity: 0.15
#  angle: -30

# Page breaks - uncomment to enable.
#pageBreaks:
#  enabled: true
#  beforeH1: true
#  orphans: 2
#  widows: 2
---
`

// InjectFrontmatter prepends the canonical picoloom scaffold to a
// markdown body that has no existing frontmatter. Refuses with
// ErrFrontmatterAlreadyPresent when a `---` block is already at the
// top - that case is for MigrateFrontmatter.
func InjectFrontmatter(markdown string) (string, error) {
	if fmOpenRe.FindStringIndex(markdown) != nil {
		return "", ErrFrontmatterAlreadyPresent
	}
	if markdown == "" {
		return canonicalScaffold, nil
	}
	return canonicalScaffold + markdown, nil
}

// FrontmatterMigration is the rich result of MigrateFrontmatter: the
// rewritten markdown plus structured metadata about what changed.
// The frontend uses this to render a preview / confirm modal before
// applying the new content to the template editor.
type FrontmatterMigration struct {
	Markdown  string               `json:"markdown"`
	Mappings  []FrontmatterMapping `json:"mappings"`
	Preserved []string             `json:"preserved"`
	Warnings  []string             `json:"warnings"`
	HadFrontmatter bool            `json:"had_frontmatter"`
}

// FrontmatterMapping records one key rename or value transform that
// happened during migration. From is the original eisvogel key; To
// is the picoloom destination ("cover.title", "page.size", etc.).
type FrontmatterMapping struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// eisvogelCoverMap maps eisvogel-style top-level keys to picoloom
// cover.* field names. Values flow through verbatim - only the key
// path changes.
var eisvogelCoverMap = map[string]string{
	"title":        "title",
	"subtitle":     "subtitle",
	"author":       "author",
	"date":         "date",
	"version":      "version",
	"logo":         "logo",
	"description":  "description",
	"organization": "organization",
	"department":   "department",
	"clientname":   "clientName",
	"projectname":  "projectName",
	"documenttype": "documentType",
	"documentid":   "documentID",
	"authortitle":  "authorTitle",
}

// eisvogelPaperSizeMap rewrites pandoc/eisvogel `papersize:` values
// into picoloom-acceptable `page.size:` values. Unknown values pass
// through verbatim with a warning attached.
var eisvogelPaperSizeMap = map[string]string{
	"a4paper":     "a4",
	"letterpaper": "letter",
	"legalpaper":  "legal",
	"a4":          "a4",
	"letter":      "letter",
	"legal":       "legal",
}

// picoloomTopLevelKeys is the set of top-level keys MigrateFrontmatter
// passes through verbatim (already picoloom-shaped). Anything not in
// this set AND not in the eisvogel mapping tables ends up in the
// legacy block.
var picoloomTopLevelKeys = map[string]bool{
	"style":      true,
	"page":       true,
	"cover":      true,
	"toc":        true,
	"footer":     true,
	"signature":  true,
	"watermark":  true,
	"pageBreaks": true,
}

// MigrateFrontmatter scans an eisvogel/pandoc-style frontmatter block
// and rewrites it into picoloom v2 shape. Unrecognized top-level keys
// are preserved verbatim under a `legacy:` block at the bottom so no
// data is lost - the user can review and prune them manually.
//
// Behavior:
//   - No leading `---` block → returns markdown verbatim, HadFrontmatter=false,
//     no error. (The caller can show "nothing to migrate" instead of
//     creating noise.)
//   - Malformed YAML → returns ErrFrontmatterMalformed.
//   - Already picoloom-shaped keys (cover/page/toc/...) pass through
//     untouched; mapping only applies to the eisvogel-flat keys.
//   - When BOTH an eisvogel key and its picoloom destination exist,
//     the picoloom one wins (the eisvogel one is preserved as legacy
//     with a warning).
func MigrateFrontmatter(markdown string) (FrontmatterMigration, error) {
	openLoc := fmOpenRe.FindStringIndex(markdown)
	if openLoc == nil {
		return FrontmatterMigration{Markdown: markdown}, nil
	}
	rest := markdown[openLoc[1]:]
	closeLoc := fmCloseRe.FindStringIndex(rest)
	if closeLoc == nil {
		return FrontmatterMigration{}, fmt.Errorf("%w: missing closing `---`", ErrFrontmatterMalformed)
	}
	rawYAML := rest[:closeLoc[0]]
	body := rest[closeLoc[1]:]
	body = trimOneLeadingNewline(body)

	// Template source can carry two patterns that confuse yaml.v3:
	//
	//  1. Handlebars expressions (`{{field "x"}}`) - `{` is a flow-
	//     mapping marker, breaks parse.
	//  2. Values starting with `#` after `: ` - yaml treats them as
	//     comments and silently drops the value (the user's
	//     `titlepage-color: #F8F8F8` would migrate to nothing).
	//
	// Mask Handlebars first, then quote `#`-leading values so yaml.v3
	// keeps the value intact. Both transforms are reversed before the
	// final output is returned.
	masked, hbsTokens := maskHandlebars(rawYAML)
	masked = quoteHashLeadingValues(masked)

	src := map[string]any{}
	if strings.TrimSpace(masked) != "" {
		if err := yaml.Unmarshal([]byte(masked), &src); err != nil {
			return FrontmatterMigration{}, fmt.Errorf("%w: %v", ErrFrontmatterMalformed, err)
		}
	}
	// yaml.v3 auto-parses ISO-8601 scalars into time.Time, which would
	// emit as "2026-05-15T00:00:00Z" on the way back out and mangle the
	// user's date string. Walk the tree and convert time.Time values
	// back to "YYYY-MM-DD" (or RFC3339 when the time-of-day matters).
	src = sanitizeYAMLValues(src).(map[string]any)

	out := FrontmatterMigration{HadFrontmatter: true}
	dst := map[string]any{}
	legacy := map[string]any{}
	coverDst := map[string]any{}
	pageDst := map[string]any{}

	// Pass 1 - pass-through picoloom-shaped blocks. Merge into the
	// staging maps so later eisvogel keys see the real picoloom state.
	keys := sortedKeys(src)
	for _, k := range keys {
		if !picoloomTopLevelKeys[k] {
			continue
		}
		switch k {
		case "cover":
			if m, ok := src[k].(map[string]any); ok {
				for kk, vv := range m {
					coverDst[kk] = vv
				}
				continue
			}
		case "page":
			if m, ok := src[k].(map[string]any); ok {
				for kk, vv := range m {
					pageDst[kk] = vv
				}
				continue
			}
		}
		dst[k] = src[k]
	}

	// Pass 2 - map eisvogel keys, deferring to the picoloom state set
	// up in Pass 1 when a conflict arises.
	for _, k := range keys {
		if picoloomTopLevelKeys[k] {
			continue
		}
		v := src[k]
		switch {
		case eisvogelCoverMap[strings.ToLower(k)] != "":
			target := eisvogelCoverMap[strings.ToLower(k)]
			if existing, ok := coverDst[target]; ok && !isEmpty(existing) {
				legacy[k] = v
				out.Warnings = append(out.Warnings,
					fmt.Sprintf("preserved %q under legacy; cover.%s was already set", k, target))
				continue
			}
			coverDst[target] = v
			out.Mappings = append(out.Mappings,
				FrontmatterMapping{From: k, To: "cover." + target})

		case strings.EqualFold(k, "keywords"):
			parsed, ok := parseEisvogelKeywords(v)
			if !ok {
				legacy[k] = v
				out.Warnings = append(out.Warnings,
					fmt.Sprintf("keywords %q has an unrecognised shape; preserved under legacy", v))
				continue
			}
			if existing, ok := dst["keywords"]; ok && !isEmpty(existing) {
				legacy[k] = v
				out.Warnings = append(out.Warnings,
					"preserved \"keywords\" under legacy; top-level keywords was already set")
				continue
			}
			dst["keywords"] = rewriteTagsHelperToYamlList(parsed, hbsTokens)
			out.Mappings = append(out.Mappings,
				FrontmatterMapping{From: "keywords", To: "keywords"})

		case strings.EqualFold(k, "papersize"):
			s, _ := v.(string)
			mapped, known := eisvogelPaperSizeMap[strings.ToLower(strings.TrimSpace(s))]
			if !known {
				legacy[k] = v
				out.Warnings = append(out.Warnings,
					fmt.Sprintf("papersize %q has no picoloom equivalent; preserved under legacy", s))
				continue
			}
			if existing, ok := pageDst["size"]; ok && !isEmpty(existing) {
				legacy[k] = v
				out.Warnings = append(out.Warnings,
					"preserved \"papersize\" under legacy; page.size was already set")
				continue
			}
			pageDst["size"] = mapped
			out.Mappings = append(out.Mappings,
				FrontmatterMapping{From: k, To: "page.size"})

		default:
			legacy[k] = v
		}
	}

	if len(coverDst) > 0 {
		dst["cover"] = coverDst
	}
	if len(pageDst) > 0 {
		dst["page"] = pageDst
	}

	// Sorted legacy keys for stable output.
	preservedKeys := sortedKeys(legacy)
	out.Preserved = preservedKeys

	emitted, err := emitMigratedFrontmatter(dst, legacy, preservedKeys)
	if err != nil {
		return FrontmatterMigration{}, fmt.Errorf("pdf: emit migrated frontmatter: %w", err)
	}
	// Restore Handlebars tokens in the emitted YAML and in the body
	// (yaml.Marshal escaped nothing because the sentinels are plain
	// alphanumeric; replaceAll is safe).
	emitted = unmaskHandlebars(emitted, hbsTokens)
	out.Markdown = "---\n" + emitted + "---\n" + body
	return out, nil
}

// quoteHashLeadingRe matches an unquoted key:value line where the
// value starts with `#`. The capture groups split the line so the
// rewrite can wrap the value in single quotes while preserving any
// leading indentation. Anchored to ^...$ in multiline mode so
// `# comment` lines on their own (no preceding key) are unaffected.
var quoteHashLeadingRe = regexp.MustCompile(`(?m)^(\s*[^\s:#][^:\n]*:\s)(#[^\n]*)$`)

// quoteHashLeadingValues protects values like `titlepage-color: #F8F8F8`
// from being silently dropped by yaml.v3 (which treats `#` after
// whitespace as a comment marker). Single-quotes the value, escaping
// any internal `'` by doubling. Values that already start with a
// quote pass through untouched.
func quoteHashLeadingValues(src string) string {
	return quoteHashLeadingRe.ReplaceAllStringFunc(src, func(line string) string {
		m := quoteHashLeadingRe.FindStringSubmatch(line)
		if len(m) != 3 {
			return line
		}
		val := strings.TrimRight(m[2], " \t")
		// Escape single quotes via YAML's '' convention.
		escaped := strings.ReplaceAll(val, "'", "''")
		return m[1] + "'" + escaped + "'"
	})
}

// hbsRe matches a single Handlebars expression. Non-greedy so
// adjacent expressions like `{{a}}{{b}}` produce two matches instead
// of one wide one. Raymond/Handlebars don't nest delimiters, so this
// is sufficient.
var hbsRe = regexp.MustCompile(`\{\{[\s\S]*?\}\}`)

// maskHandlebars replaces every `{{...}}` in src with a unique
// sentinel and returns the masked string + a token map. The sentinels
// are plain ASCII identifiers that survive yaml.Marshal/Unmarshal
// without quoting, so the round trip is lossless.
func maskHandlebars(src string) (string, map[string]string) {
	tokens := map[string]string{}
	i := 0
	out := hbsRe.ReplaceAllStringFunc(src, func(match string) string {
		key := fmt.Sprintf("__HBS_%d__", i)
		i++
		tokens[key] = match
		return key
	})
	return out, tokens
}

// unmaskHandlebars restores the original `{{...}}` expressions by
// replacing each sentinel with its captured source. Idempotent -
// applying twice has no effect (sentinels are gone after the first
// pass).
func unmaskHandlebars(src string, tokens map[string]string) string {
	if len(tokens) == 0 {
		return src
	}
	out := src
	// Sort by length descending so we never replace a prefix substring
	// (e.g. __HBS_1__ before __HBS_10__).
	keys := make([]string, 0, len(tokens))
	for k := range tokens {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, k := range keys {
		out = strings.ReplaceAll(out, k, tokens[k])
	}
	return out
}

// emitMigratedFrontmatter renders the picoloom-shaped destination map
// plus the legacy block as a YAML body (without the surrounding `---`
// fences - caller wraps them). Order: picoloom blocks in canonical
// order first, then the legacy block (so legacy stays visibly at the
// bottom as "stuff to review").
func emitMigratedFrontmatter(dst map[string]any, legacy map[string]any, preservedKeys []string) (string, error) {
	var b strings.Builder
	canonicalOrder := []string{"style", "keywords", "page", "cover", "toc", "footer", "signature", "watermark", "pageBreaks"}
	for _, k := range canonicalOrder {
		v, ok := dst[k]
		if !ok {
			continue
		}
		var chunk string
		var err error
		if k == "keywords" {
			items, _ := v.([]any)
			chunk = marshalKeywordsBlock(items)
		} else {
			chunk, err = marshalBlock(k, v)
			if err != nil {
				return "", err
			}
		}
		b.WriteString(chunk)
	}
	// Any picoloom-shaped keys we don't recognize (forward-compat -
	// shouldn't happen today but cheap to handle).
	for k := range dst {
		if !contains(canonicalOrder, k) {
			chunk, err := marshalBlock(k, dst[k])
			if err != nil {
				return "", err
			}
			b.WriteString(chunk)
		}
	}
	if len(preservedKeys) > 0 {
		b.WriteString("\n")
		b.WriteString("# Preserved verbatim from the original frontmatter. These keys had\n")
		b.WriteString("# no picoloom v2 equivalent - review and remove if not needed.\n")
		b.WriteString("legacy:\n")
		for _, k := range preservedKeys {
			sub := map[string]any{k: legacy[k]}
			raw, err := yaml.Marshal(sub)
			if err != nil {
				return "", err
			}
			for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
				b.WriteString("  ")
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
	}
	return b.String(), nil
}

// yamlRawLinePrefix flags a keyword item that should bypass the
// `- ITEM` block-sequence wrapping and be emitted as a raw line in
// the output (used by the tags→yamlList rewrite path so the helper
// invocation lands at column 0 and its multi-line expansion plugs
// directly into the block sequence).
const yamlRawLinePrefix = "__YAML_RAW_LINE__"

// tagsHelperRe matches a standalone `{{tags <ARG> [withHash=<bool>]}}`
// invocation that wholly fills a keyword element. The argument is
// captured verbatim - could be a bare identifier, a string literal, a
// subexpression like `(fieldRaw "x")`, whatever raymond accepts.
// Anchored so partial matches inside other text don't trigger.
var tagsHelperRe = regexp.MustCompile(`^\s*\{\{\s*tags\s+(.+?)(?:\s+withHash\s*=\s*\w+)?\s*\}\}\s*$`)

// rewriteTagsHelperToYamlList walks keyword items and rewrites any
// element that is a sentinel mapping to a wholly-tags-helper source
// into a raw-line `{{yamlList <ARG>}}` directive. The handlebars
// expression for a keyword position emits a comma-blob at render
// time; yamlList emits real list items.
//
// Non-sentinel items, sentinels backed by other helpers (e.g.
// `{{field "x"}}`), and partial matches all pass through untouched.
func rewriteTagsHelperToYamlList(items []any, hbsTokens map[string]string) []any {
	if len(items) == 0 || len(hbsTokens) == 0 {
		return items
	}
	out := make([]any, len(items))
	copy(out, items)
	for i, it := range out {
		s, ok := it.(string)
		if !ok {
			continue
		}
		src, ok := hbsTokens[strings.TrimSpace(s)]
		if !ok {
			continue
		}
		m := tagsHelperRe.FindStringSubmatch(src)
		if len(m) != 2 {
			continue
		}
		arg := strings.TrimSpace(m[1])
		out[i] = yamlRawLinePrefix + "{{yamlList " + arg + "}}"
	}
	return out
}

// marshalKeywordsBlock emits the top-level `keywords:` sequence by
// hand. yaml.Marshal would emit each element unquoted (the values are
// either plain words or `__HBS_N__` sentinels - all alphanumeric to
// the YAML lexer), which means the global unmask pass at the end of
// MigrateFrontmatter would drop bare `{{…}}` Handlebars expressions
// into unquoted scalar position - invalid YAML. Quoting sentinel-
// containing elements ourselves keeps the post-unmask text valid:
// `'{{…}}'` is a single-quoted scalar.
func marshalKeywordsBlock(items []any) string {
	var b strings.Builder
	b.WriteString("keywords:\n")
	for _, it := range items {
		s, ok := it.(string)
		if !ok {
			s = fmt.Sprintf("%v", it)
		}
		if strings.HasPrefix(s, yamlRawLinePrefix) {
			b.WriteString(strings.TrimPrefix(s, yamlRawLinePrefix))
			b.WriteString("\n")
			continue
		}
		if needsYAMLQuoting(s) {
			esc := strings.ReplaceAll(s, "'", "''")
			b.WriteString("- '")
			b.WriteString(esc)
			b.WriteString("'\n")
		} else {
			b.WriteString("- ")
			b.WriteString(s)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func needsYAMLQuoting(s string) bool {
	if s == "" {
		return true
	}
	if strings.Contains(s, "__HBS_") {
		return true
	}
	if strings.ContainsAny(s, "{}[]:#&*!|>%@`,") {
		return true
	}
	switch s[0] {
	case '-', '?', '\'', '"':
		return true
	}
	return false
}

// marshalBlock emits one top-level YAML block with the form
// `key: <value>\n` (scalars) or `key:\n  …\n` (maps), without
// double-indentation.
func marshalBlock(key string, value any) (string, error) {
	raw, err := yaml.Marshal(map[string]any{key: value})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// sanitizeYAMLValues walks an unmarshalled YAML tree and converts
// time.Time scalars back to strings so the round-trip emit doesn't
// mangle the user's date strings. yaml.v3 auto-parses ISO-8601-ish
// scalars (e.g. "2026-05-15") into time.Time; without this pass,
// `date: 2026-05-15` would re-emit as "2026-05-15T00:00:00Z".
func sanitizeYAMLValues(v any) any {
	switch x := v.(type) {
	case time.Time:
		if x.Hour() == 0 && x.Minute() == 0 && x.Second() == 0 && x.Nanosecond() == 0 {
			return x.Format("2006-01-02")
		}
		return x.Format(time.RFC3339)
	case map[string]any:
		for k, vv := range x {
			x[k] = sanitizeYAMLValues(vv)
		}
		return x
	case []any:
		for i, vv := range x {
			x[i] = sanitizeYAMLValues(vv)
		}
		return x
	}
	return v
}

// parseEisvogelKeywords normalises whatever shape `keywords:` came in
// as into a YAML sequence (`[]any` of strings) for the migrated
// frontmatter. Two real-world shapes are recognised:
//
//   - YAML sequence: `keywords: [a, b, c]` or block form. yaml.v3
//     parses these into `[]any`; pass through verbatim.
//   - Eisvogel/PandocPrint bracket-string DSL:
//     `keywords: '[Aanpak, Management, {{tags …}}]'`. Strip the outer
//     brackets, split on commas, trim each element. Handlebars
//     expressions have already been masked to sentinels by the time we
//     see the value, so commas inside `{{…}}` are not present and the
//     split is safe.
//
// Returns (nil, false) for any other shape (numeric, plain string
// without brackets, map, ...) so the caller can preserve the original
// under legacy with a warning. Empty result → also (nil, false): an
// empty keywords list is meaningless.
func parseEisvogelKeywords(v any) ([]any, bool) {
	switch x := v.(type) {
	case []any:
		out := make([]any, 0, len(x))
		for _, item := range x {
			s, ok := item.(string)
			if !ok {
				s = fmt.Sprintf("%v", item)
			}
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			out = append(out, s)
		}
		if len(out) == 0 {
			return nil, false
		}
		return out, true
	case string:
		s := strings.TrimSpace(x)
		if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
			return nil, false
		}
		inner := strings.TrimSpace(s[1 : len(s)-1])
		if inner == "" {
			return nil, false
		}
		parts := strings.Split(inner, ",")
		out := make([]any, 0, len(parts))
		for _, p := range parts {
			t := strings.TrimSpace(p)
			if t == "" {
				continue
			}
			out = append(out, t)
		}
		if len(out) == 0 {
			return nil, false
		}
		return out, true
	}
	return nil, false
}

// isEmpty reports whether v represents a "no opinion" YAML value
// (nil, empty string, empty map / slice).
func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	switch x := v.(type) {
	case string:
		return x == ""
	case map[string]any:
		return len(x) == 0
	case []any:
		return len(x) == 0
	}
	return false
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func contains(xs []string, x string) bool {
	for _, e := range xs {
		if e == x {
			return true
		}
	}
	return false
}
