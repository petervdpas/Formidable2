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
// the markdown already starts with a `---` block (use MigrateFrontmatter).
var ErrFrontmatterAlreadyPresent = errors.New("pdf: markdown already has a frontmatter block")

// canonicalScaffold is the full picoloom frontmatter the Inject
// utility prepends. Field names must stay in sync with the
// Frontmatter struct's yaml tags in input.go.
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

// InjectFrontmatter prepends the canonical scaffold to a markdown body
// that has no frontmatter, else returns ErrFrontmatterAlreadyPresent.
func InjectFrontmatter(markdown string) (string, error) {
	if fmOpenRe.FindStringIndex(markdown) != nil {
		return "", ErrFrontmatterAlreadyPresent
	}
	if markdown == "" {
		return canonicalScaffold, nil
	}
	return canonicalScaffold + markdown, nil
}

// FrontmatterMigration is the result of MigrateFrontmatter: rewritten
// markdown plus structured metadata about what changed.
type FrontmatterMigration struct {
	Markdown       string               `json:"markdown"`
	Mappings       []FrontmatterMapping `json:"mappings"`
	Preserved      []string             `json:"preserved"`
	Warnings       []string             `json:"warnings"`
	HadFrontmatter bool                 `json:"had_frontmatter"`
}

// FrontmatterMapping records one key rename during migration: From is
// the eisvogel key, To the picoloom destination.
type FrontmatterMapping struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// eisvogelCoverMap maps eisvogel top-level keys to picoloom cover.*
// names; values flow through verbatim.
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

// eisvogelPaperSizeMap rewrites `papersize:` into picoloom `page.size:`;
// unknown values are preserved with a warning.
var eisvogelPaperSizeMap = map[string]string{
	"a4paper":     "a4",
	"letterpaper": "letter",
	"legalpaper":  "legal",
	"a4":          "a4",
	"letter":      "letter",
	"legal":       "legal",
}

// picoloomTopLevelKeys are passed through verbatim; anything else not
// in the eisvogel maps lands in the legacy block.
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

// MigrateFrontmatter rewrites an eisvogel/pandoc frontmatter block into
// picoloom v2 shape. Unrecognized keys are preserved under a `legacy:`
// block so no data is lost. No `---` block returns the input verbatim
// with HadFrontmatter=false; malformed YAML returns ErrFrontmatterMalformed.
// When an eisvogel key and its picoloom destination both exist, the
// picoloom one wins and the eisvogel one is preserved as legacy.
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

	// Two patterns confuse yaml.v3: Handlebars `{{...}}` (`{` is a flow-
	// mapping marker) and `#`-leading values (treated as comments, value
	// silently dropped). Mask then quote; both transforms are reversed
	// before output.
	masked, hbsTokens := maskHandlebars(rawYAML)
	masked = quoteHashLeadingValues(masked)

	src := map[string]any{}
	if strings.TrimSpace(masked) != "" {
		if err := yaml.Unmarshal([]byte(masked), &src); err != nil {
			return FrontmatterMigration{}, fmt.Errorf("%w: %v", ErrFrontmatterMalformed, err)
		}
	}
	// yaml.v3 auto-parses ISO-8601 scalars into time.Time, which would
	// re-emit as "2026-05-15T00:00:00Z" and mangle the user's date.
	src = sanitizeYAMLValues(src).(map[string]any)

	out := FrontmatterMigration{HadFrontmatter: true}
	dst := map[string]any{}
	legacy := map[string]any{}
	coverDst := map[string]any{}
	pageDst := map[string]any{}

	// Pass 1: stage picoloom-shaped blocks so Pass 2's eisvogel keys see
	// the real picoloom state for conflict resolution.
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

	// Pass 2: map eisvogel keys, deferring to Pass 1's picoloom state on
	// conflict.
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

	preservedKeys := sortedKeys(legacy)
	out.Preserved = preservedKeys

	emitted, err := emitMigratedFrontmatter(dst, legacy, preservedKeys)
	if err != nil {
		return FrontmatterMigration{}, fmt.Errorf("pdf: emit migrated frontmatter: %w", err)
	}
	emitted = unmaskHandlebars(emitted, hbsTokens)
	out.Markdown = "---\n" + emitted + "---\n" + body
	return out, nil
}

// quoteHashLeadingRe matches an unquoted key:value line whose value
// starts with `#`. Anchored multiline so standalone `# comment` lines
// are unaffected.
var quoteHashLeadingRe = regexp.MustCompile(`(?m)^(\s*[^\s:#][^:\n]*:\s)(#[^\n]*)$`)

// quoteHashLeadingValues single-quotes `#`-leading values so yaml.v3
// doesn't drop them as comments.
func quoteHashLeadingValues(src string) string {
	return quoteHashLeadingRe.ReplaceAllStringFunc(src, func(line string) string {
		m := quoteHashLeadingRe.FindStringSubmatch(line)
		if len(m) != 3 {
			return line
		}
		val := strings.TrimRight(m[2], " \t")
		escaped := strings.ReplaceAll(val, "'", "''")
		return m[1] + "'" + escaped + "'"
	})
}

// hbsRe matches one Handlebars expression. Non-greedy so `{{a}}{{b}}`
// produces two matches; Handlebars doesn't nest delimiters.
var hbsRe = regexp.MustCompile(`\{\{[\s\S]*?\}\}`)

// maskHandlebars replaces every `{{...}}` with a unique ASCII sentinel
// that survives yaml.Marshal/Unmarshal unquoted, so the round trip is
// lossless. Returns the masked string and a token map.
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

// unmaskHandlebars restores `{{...}}` from sentinels. Replaces longest
// keys first so a sentinel never clobbers a prefix substring (e.g.
// __HBS_1__ inside __HBS_10__).
func unmaskHandlebars(src string, tokens map[string]string) string {
	if len(tokens) == 0 {
		return src
	}
	out := src
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

// emitMigratedFrontmatter renders dst then the legacy block as an
// unfenced YAML body, canonical block order first so legacy stays at
// the bottom as "stuff to review".
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
	// Unrecognized picoloom-shaped keys (forward-compat).
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

// yamlRawLinePrefix flags a keyword item that bypasses `- ITEM`
// block-sequence wrapping so a `{{yamlList ...}}` lands at column 0 and
// its multi-line expansion plugs into the block sequence.
const yamlRawLinePrefix = "__YAML_RAW_LINE__"

// tagsHelperRe matches a standalone `{{tags <ARG> [withHash=<bool>]}}`
// filling a whole keyword element; ARG is captured verbatim.
var tagsHelperRe = regexp.MustCompile(`^\s*\{\{\s*tags\s+(.+?)(?:\s+withHash\s*=\s*\w+)?\s*\}\}\s*$`)

// rewriteTagsHelperToYamlList rewrites a wholly-{{tags}} keyword element
// into a raw-line `{{yamlList <ARG>}}`: tags emits a comma-blob at a
// keyword position whereas yamlList emits real list items. Other items
// pass through untouched.
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

// marshalKeywordsBlock emits the `keywords:` sequence by hand because
// yaml.Marshal would leave sentinel elements unquoted, and the later
// unmask pass would then drop bare `{{...}}` into unquoted scalar
// position (invalid YAML). Quoting sentinel elements keeps post-unmask
// text valid.
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

func marshalBlock(key string, value any) (string, error) {
	raw, err := yaml.Marshal(map[string]any{key: value})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// sanitizeYAMLValues converts time.Time scalars back to date strings so
// the round-trip emit doesn't turn `date: 2026-05-15` into a full
// RFC3339 timestamp.
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

// parseEisvogelKeywords normalises `keywords:` into a []any of strings.
// Recognises a YAML sequence and the eisvogel bracket-string DSL
// (`'[a, b, {{tags ...}}]'`); the comma-split is safe because
// Handlebars are already masked to comma-free sentinels by this point.
// Any other shape returns (nil, false) so the caller preserves it under
// legacy.
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
