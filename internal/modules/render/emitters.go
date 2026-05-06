package render

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

var absoluteURLRe = regexp.MustCompile(`(?i)^([a-z][a-z0-9+\-.]*:)?//|^(mailto|tel):`)

// isAbsoluteURL mirrors `isAbsoluteUrl` in the original renderer.
// True for `https://`, `//cdn`, `mailto:`, `tel:`, etc.
func isAbsoluteURL(u string) bool {
	return absoluteURLRe.MatchString(u)
}

// stringify normalizes a value to its string representation. nil → "",
// floats render without trailing ".0" when integral.
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

// truthy mirrors JS `Boolean(v)` for the boolean emitter — non-empty
// strings, non-zero numbers, and non-nil objects are true.
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

// emitList → markdown bullet list, one item per line.
func emitList(v any) string {
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		return ""
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		out = append(out, "- "+stringify(item))
	}
	return strings.Join(out, "\n")
}

// emitTable → markdown table rows. Each row is a []any; cells are
// stringified and wrapped in `| … |` separators.
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

// optionPair extracts (value, label) from one option entry.
// Strings become {value:s, label:s}; maps look at "value" + "label" with
// label falling back to value. Anything else stringifies.
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

// emitBoolean honors `field.options[0]` (true) / `[1]` (false). Falls
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

// emitOptionLabel — dropdown / radio / table-column lookup. Returns the
// option label that matches v; falls back to v's stringified form.
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

// emitMultioption — array of selected values → comma-joined labels.
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

// kebab → lowercase + spaces collapsed to "-". Matches the original
// `tag.toLowerCase().replace(/\s+/g, "-")`.
func kebab(s string) string {
	s = strings.ToLower(s)
	return regexp.MustCompile(`\s+`).ReplaceAllString(s, "-")
}

// emitTags → "#kebab-tag, #other" (with hash) or "kebab, other" (without).
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
// Three branches:
//
//   - `formidable://<template>:<datafile>` URLs go through the
//     transport-specific rewriter when one is installed (wiki HTTP →
//     /template/<stem>/form/<datafile>, future Azure/GitHub exports →
//     their own slug schemes). nil hook = pass through untouched
//     (slideout case — Vue click interceptor handles clicks in-app).
//   - Absolute URLs (http/https/etc.) and `file:` URIs are passthrough.
//   - Otherwise it's a relative path: LinkURL resolves it against
//     template storage if provided.
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
// Returns ok=false on malformed input so the caller can fall back to
// the original href instead of producing a half-broken URL.
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

// emitLink accepts `string` or `{href, text}`. Returns a Markdown
// `[label](href)`. Empty href + empty text → "".
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

// emitImage resolves the image filename to a URL via the configured
// strategy. Absolute paths and `file:` URIs are passed through.
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

// emitFieldValue dispatches to the per-type emitter. Unknown types
// fall back to plain stringify. Used by the {{field}} helper.
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
	case "textarea":
		return stringify(v)
	default:
		return stringify(v)
	}
}
