package pdf

import (
	"errors"
	"fmt"
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
// Frontmatter struct — the field names below must match its yaml
// tags.
const canonicalScaffold = `---
# PDF export frontmatter (picoloom v2 shape). Inserted by
# Formidable's "Inject PDF Frontmatter" utility. See
# Information → PDF Export → Frontmatter directives for the spec.

# Top-level style: picoloom theme name ("technical", "academic", ...)
# or absolute path to a custom .css file. Empty = picoloom's built-in
# default.
style:

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

# Table of contents — uncomment to enable.
#toc:
#  enabled: true
#  title: Contents
#  minDepth: 1
#  maxDepth: 3

# Footer — uncomment to enable.
#footer:
#  enabled: true
#  position: bottom
#  showPageNumber: true
#  text:

# Signature block — uncomment to enable.
#signature:
#  enabled: true
#  name:
#  title:
#  email:
#  organization:
#  imagePath:

# Watermark — uncomment to enable.
#watermark:
#  enabled: true
#  text: DRAFT
#  color: '#888888'
#  opacity: 0.15
#  angle: -30

# Page breaks — uncomment to enable.
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
// top — that case is for MigrateFrontmatter.
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
// cover.* field names. Values flow through verbatim — only the key
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
// data is lost — the user can review and prune them manually.
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

	src := map[string]any{}
	if strings.TrimSpace(rawYAML) != "" {
		if err := yaml.Unmarshal([]byte(rawYAML), &src); err != nil {
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

	// Pass 1 — pass-through picoloom-shaped blocks. Merge into the
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

	// Pass 2 — map eisvogel keys, deferring to the picoloom state set
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
	out.Markdown = "---\n" + emitted + "---\n" + body
	return out, nil
}

// emitMigratedFrontmatter renders the picoloom-shaped destination map
// plus the legacy block as a YAML body (without the surrounding `---`
// fences — caller wraps them). Order: picoloom blocks in canonical
// order first, then the legacy block (so legacy stays visibly at the
// bottom as "stuff to review").
func emitMigratedFrontmatter(dst map[string]any, legacy map[string]any, preservedKeys []string) (string, error) {
	var b strings.Builder
	canonicalOrder := []string{"style", "page", "cover", "toc", "footer", "signature", "watermark", "pageBreaks"}
	for _, k := range canonicalOrder {
		v, ok := dst[k]
		if !ok {
			continue
		}
		chunk, err := marshalBlock(k, v)
		if err != nil {
			return "", err
		}
		b.WriteString(chunk)
	}
	// Any picoloom-shaped keys we don't recognize (forward-compat —
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
		b.WriteString("# no picoloom v2 equivalent — review and remove if not needed.\n")
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
