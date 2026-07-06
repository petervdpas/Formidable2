package render

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

var absoluteURLRe = regexp.MustCompile(`(?i)^([a-z][a-z0-9+\-.]*:)?//|^(mailto|tel):`)

// isAbsoluteURL is true for `https://`, `//cdn`, `mailto:`, `tel:`, etc.
func isAbsoluteURL(u string) bool {
	return absoluteURLRe.MatchString(u)
}

// stringify normalizes a value to its string representation. nil becomes
// "", integral floats render without a trailing ".0".
func stringify(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 64)
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", x)
	default:
		return fmt.Sprint(x)
	}
}

// truthy mirrors JS `Boolean(v)` for the boolean emitter.
func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != "" && x != "false" && x != "0"
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	default:
		return true
	}
}

// emitList renders a markdown bullet list, one item per line.
func emitList(v any) string { return emitListMode(v, false) }

// emitListMode renders a list value as Markdown, bulleted (ordered=false) or
// numbered (ordered=true). Numbers are emitted as "1." on every item; goldmark
// renumbers each list sequentially. Nested rows indent by the marker width
// (2 spaces for "- ", 3 for "1. ") so CommonMark nests them under their parent;
// indent is clamped to prev+1 so a row can't jump levels or orphan-indent.
func emitListMode(v any, ordered bool) string {
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		return ""
	}
	marker, step := "- ", 2
	if ordered {
		marker, step = "1. ", 3
	}
	out := make([]string, 0, len(arr))
	prev := -1
	for _, item := range arr {
		ind := template.ListItemIndent(item)
		if ind > prev+1 {
			ind = prev + 1
		}
		prev = ind
		prefix := strings.Repeat(" ", step*ind)
		out = append(out, prefix+marker+template.ListItemText(item))
	}
	return strings.Join(out, "\n")
}

// emitTable renders markdown table rows from a []any of []any rows.
func emitTable(v any) string {
	rows, ok := v.([]any)
	if !ok || len(rows) == 0 {
		return ""
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		cells, ok := row.([]any)
		if !ok {
			continue
		}
		strs := make([]string, len(cells))
		for i, c := range cells {
			strs[i] = stringify(c)
		}
		out = append(out, "| "+strings.Join(strs, " | ")+" |")
	}
	return strings.Join(out, "\n")
}

// optionPair extracts (value, label) from one option entry. Label falls
// back to value when absent.
func optionPair(opt any) (string, string) {
	switch x := opt.(type) {
	case string:
		return x, x
	case map[string]any:
		val := stringify(x["value"])
		lab := stringify(x["label"])
		if lab == "" {
			lab = val
		}
		return val, lab
	default:
		s := stringify(opt)
		return s, s
	}
}

// emitBoolean honors `field.options[0]` (true) / `[1]` (false), falling
// back to "True"/"False" when no options are declared.
func emitBoolean(v any, f *template.Field) string {
	b := truthy(v)
	if f != nil && len(f.Options) >= 2 {
		_, lab1 := optionPair(f.Options[0])
		_, lab2 := optionPair(f.Options[1])
		if b {
			return lab1
		}
		return lab2
	}
	if b {
		return "True"
	}
	return "False"
}

// emitOptionLabel returns the option label matching v, falling back to
// v's stringified form.
func emitOptionLabel(v any, f *template.Field) string {
	want := stringify(v)
	if f == nil {
		return want
	}
	for _, opt := range f.Options {
		val, lab := optionPair(opt)
		if val == want {
			return lab
		}
	}
	return want
}

// emitMultioption joins the selected values' labels with commas.
func emitMultioption(v any, f *template.Field) string {
	arr, ok := v.([]any)
	if !ok {
		return ""
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		out = append(out, emitOptionLabel(item, f))
	}
	return strings.Join(out, ", ")
}

// kebab lowercases s and collapses whitespace runs to "-".
func kebab(s string) string {
	s = strings.ToLower(s)
	return regexp.MustCompile(`\s+`).ReplaceAllString(s, "-")
}

// emitYAMLList emits a YAML block sequence, indenting items 2+ by indent
// spaces. Items with YAML special chars get single-quoted (internal
// quotes doubled). Non-array input returns "" so callers can use it
// unconditionally.
func emitYAMLList(v any, indent int) string {
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		return ""
	}
	pad := ""
	if indent > 0 {
		pad = strings.Repeat(" ", indent)
	}
	parts := make([]string, 0, len(arr))
	for i, item := range arr {
		s := template.ListItemText(item)
		if needsYAMLListQuoting(s) {
			s = "'" + strings.ReplaceAll(s, "'", "''") + "'"
		}
		if i == 0 {
			parts = append(parts, "- "+s)
		} else {
			parts = append(parts, pad+"- "+s)
		}
	}
	return strings.Join(parts, "\n")
}

// emitYAMLString encodes v as a quoted YAML scalar, safe to drop into a
// frontmatter value position (`title: {{yamlString …}}`). Frontmatter is YAML,
// not HTML, so the value must be YAML-encoded, not entity-escaped: a bare
// `{{field}}` returns a plain string that raymond HTML-escapes (`&` -> `&amp;`),
// which then reaches the PDF literally. This always quotes (single-quoted by
// default, apostrophes doubled), so a numeric- or colon-bearing value stays a
// string; values carrying a newline, carriage return or tab switch to
// double-quoted style with the matching escapes to stay on one line.
func emitYAMLString(v any) string {
	s := stringify(v)
	if strings.ContainsAny(s, "\n\r\t") {
		var b strings.Builder
		b.WriteByte('"')
		for _, r := range s {
			switch r {
			case '\\':
				b.WriteString(`\\`)
			case '"':
				b.WriteString(`\"`)
			case '\n':
				b.WriteString(`\n`)
			case '\r':
				b.WriteString(`\r`)
			case '\t':
				b.WriteString(`\t`)
			default:
				b.WriteRune(r)
			}
		}
		b.WriteByte('"')
		return b.String()
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func needsYAMLListQuoting(s string) bool {
	if s == "" {
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

// emitTags renders "#kebab-tag, #other" (withHash) or "kebab, other".
func emitTags(v any, withHash bool) string {
	arr, ok := v.([]any)
	if !ok {
		return ""
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		k := kebab(stringify(item))
		if withHash {
			out = append(out, "#"+k)
		} else {
			out = append(out, k)
		}
	}
	return strings.Join(out, ", ")
}

// resolveLinkHref decides the final href for a link field's value.
// `formidable://<tpl>:<df>` URLs go through the transport rewriter when
// one is installed; a nil hook passes them through untouched (the
// slideout's Vue click interceptor handles clicks in-app). Absolute and
// `file:` URLs pass through. Relative paths resolve via LinkURL.
func resolveLinkHref(href string, opts *Options) string {
	if href == "" {
		return ""
	}
	if strings.HasPrefix(href, "formidable://") {
		if opts != nil && opts.FormidableLinkURL != nil {
			if tpl, df, ok := parseFormidableURL(href); ok {
				if rewritten := opts.FormidableLinkURL(tpl, df); rewritten != "" {
					return rewritten
				}
			}
		}
		return href
	}
	if isAbsoluteURL(href) || strings.HasPrefix(href, "file:") {
		return href
	}
	if opts != nil && opts.LinkURL != nil {
		return opts.LinkURL(href)
	}
	return href
}

// parseFormidableURL splits `formidable://<tpl>:<df>` into its parts.
// Returns ok=false on malformed input so the caller can fall back to the
// original href instead of producing a half-broken URL.
func parseFormidableURL(href string) (template, datafile string, ok bool) {
	const prefix = "formidable://"
	if !strings.HasPrefix(href, prefix) {
		return "", "", false
	}
	rest := href[len(prefix):]
	idx := strings.Index(rest, ":")
	if idx <= 0 || idx == len(rest)-1 {
		return "", "", false
	}
	return rest[:idx], rest[idx+1:], true
}

// emitLink accepts `string` or `{href, text}` and returns Markdown
// `[label](href)`. Empty href and text yields "".
func emitLink(v any, opts *Options) string {
	var href, text string
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		href = x
	case map[string]any:
		href = stringify(x["href"])
		text = stringify(x["text"])
	default:
		return ""
	}

	href = resolveLinkHref(href, opts)
	label := text
	if label == "" {
		label = href
	}
	if label == "" {
		return ""
	}
	return "[" + label + "](" + href + ")"
}

// emitImage resolves the image filename to a URL via Options.ImageURL.
// Absolute and `file:` URLs pass through.
func emitImage(v any, opts *Options) string {
	name, ok := v.(string)
	if !ok || name == "" {
		return ""
	}
	if isAbsoluteURL(name) || strings.HasPrefix(name, "file:") {
		return name
	}
	if opts != nil && opts.ImageURL != nil {
		return opts.ImageURL(name)
	}
	return "images/" + name
}

// emitFieldValue dispatches to the per-type emitter, falling back to
// stringify for unknown types.
func emitFieldValue(v any, f *template.Field, opts *Options) string {
	if f == nil {
		return stringify(v)
	}
	switch f.Type {
	case "list":
		return emitList(v)
	case "table":
		return emitOptionLabel(v, f)
	case "boolean":
		return emitBoolean(v, f)
	case "dropdown", "radio":
		return emitOptionLabel(v, f)
	case "multioption":
		return emitMultioption(v, f)
	case "tags":
		return emitTags(v, true)
	case "link":
		return emitLink(v, opts)
	case "image":
		return emitImage(v, opts)
	case "mermaid":
		return emitMermaid(v)
	case "slide":
		return renderSlide(v, f, opts)
	case "textarea":
		return stringify(v)
	default:
		return stringify(v)
	}
}

// emitMermaid wraps the diagram source in a ```mermaid fenced block: the
// portable, canonical representation. Markdown export keeps it verbatim;
// the HTML/PDF pipeline turns the fence into a rendered diagram. Empty
// source emits nothing (no stray empty fence).
func emitMermaid(v any) string {
	src := strings.TrimRight(stringify(v), "\n")
	if strings.TrimSpace(src) == "" {
		return ""
	}
	return "```mermaid\n" + src + "\n```"
}
