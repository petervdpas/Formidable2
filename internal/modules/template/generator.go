package template

import (
	"fmt"
	"strings"
	"time"
)

// Shape selects an output style for the markdown-template generator.
type Shape string

const (
	ShapeReport      Shape = "report"
	ShapeMinimal     Shape = "minimal"
	ShapeTable       Shape = "table"
	ShapeFrontmatter Shape = "frontmatter"
)

// ImgMode selects how image fields are emitted.
//
//	url    - `![Label]({{imageURL "key"}})`. Resolved at render time
//	         per the consumer's render.Manager (slideout, wiki, …).
//	inline - `![Label]({{imageBase64 "key"}})`. Bytes inlined as a
//	         `data:<mime>;base64,…` URL. For self-contained exports.
type ImgMode string

const (
	ImgURL    ImgMode = "url"
	ImgInline ImgMode = "inline"
)

// GeneratorOptions carries the per-shape sub-choices the dialog
// surfaces. Bag-of-bools so adding a new option doesn't require
// signature changes throughout the call chain (Service ↔ generator ↔
// Wails binding).
//
// Defaults match the dialog's defaults: linked URL for images, auto-
// wrapped loop iterations, lazy api-card output (one-liner per api
// field).
type GeneratorOptions struct {
	ImgMode   ImgMode `json:"img_mode"`
	WrapLoops bool    `json:"wrap_loops"`
	// ExpandAPI flips api-field output between two visible shapes:
	//   false → `{{apiSection "key"}}`        (lazy one-liner)
	//   true  → per-column `- **<label>**: {{apiBlock "key" "col"}}`
	// Same "visible toggle" rule as ImgMode/WrapLoops - the choice
	// must materialise in the generated source so the user can see
	// what they picked at a glance.
	ExpandAPI bool `json:"expand_api"`
}

// ShapeInfo is the catalog record for the dialog's shape picker.
type ShapeInfo struct {
	ID          Shape  `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// Shapes returns the catalog used by the dialog picker.
func Shapes() []ShapeInfo {
	return []ShapeInfo{
		{
			ID:          ShapeReport,
			Label:       "Report (full + debug)",
			Description: "Frontmatter, per-field heading and value, plus a debug block listing each field's raw value. Best when wiring up a new template.",
		},
		{
			ID:          ShapeMinimal,
			Label:       "Minimal",
			Description: "Just heading + value per field. No frontmatter, no debug block.",
		},
		{
			ID:          ShapeTable,
			Label:       "Key/Value Table",
			Description: "Single Markdown table with one row per top-level field. Compact summary view.",
		},
		{
			ID:          ShapeFrontmatter,
			Label:       "Frontmatter only",
			Description: "Emit fields as a YAML data block. Image fields are skipped - they don't fit a metadata block.",
		},
	}
}

// GenerateMarkdownTemplate produces default Handlebars-flavored markdown
// for the given fields, in the requested shape and options.
//
// Empty/nil fields → empty string. Unknown shape → falls back to report
// so a stale frontend doesn't produce nothing. Unknown image mode →
// falls back to ImgURL.
func GenerateMarkdownTemplate(shape Shape, opts GeneratorOptions, fields []Field) string {
	if len(fields) == 0 {
		return ""
	}
	if opts.ImgMode != ImgURL && opts.ImgMode != ImgInline {
		opts.ImgMode = ImgURL
	}
	switch shape {
	case ShapeMinimal:
		return generateMinimal(fields, opts)
	case ShapeTable:
		return generateTable(fields, opts.ImgMode)
	case ShapeFrontmatter:
		return generateFrontmatter(fields)
	case ShapeReport:
		return generateReport(fields, opts)
	default:
		return generateReport(fields, opts)
	}
}

// imageHelperCall returns the Handlebars expression used to resolve an
// image field's URL according to the current ImgMode.
func imageHelperCall(key string, mode ImgMode) string {
	switch mode {
	case ImgInline:
		return fmt.Sprintf(`{{imageBase64 "%s"}}`, key)
	default:
		return fmt.Sprintf(`{{imageURL "%s"}}`, key)
	}
}

// loopBodyWrap inserts {{loopItemBefore}} / {{loopItemAfter}} around
// an inner loop body when WrapLoops is on. The loop opener stays bare
// (`{{#loop "key"}}`) regardless - wrap state lives in the body so a
// reader of the generated source can see at a glance whether iteration
// wrapping is in effect.
func loopBodyWrap(body string, wrap bool) string {
	if !wrap {
		return body
	}
	return "{{loopItemBefore}}\n" + body + "\n{{loopItemAfter}}"
}

// ─── Report shape (port of templateGenerator.js) ──────────────────────

func generateReport(fields []Field, opts GeneratorOptions) string {
	frontmatter := strings.Join([]string{
		"---",
		"title: Auto-generated Report",
		"author: Formidable Generator",
		"date: " + time.Now().UTC().Format("2006-01-02"),
		"toc: true",
		"toc-title: Contents",
		"toc-own-page: true",
		"---",
		"",
	}, "\n")

	var topLevelLogs []string
	body := renderFieldBlocks(fields, 2, &topLevelLogs, opts)
	content := strings.Join(body, "\n---\n\n")
	logSection := buildLogSection(topLevelLogs)

	return frontmatter + "\n" + content + logSection
}

func renderFieldBlocks(fields []Field, headingLevel int, logs *[]string, opts GeneratorOptions) []string {
	result := make([]string, 0, len(fields))
	seen := map[string]bool{}

	for i := 0; i < len(fields); i++ {
		f := fields[i]
		key := f.Key
		if key == "" {
			key = "unknown"
		}
		t := strings.ToLower(f.Type)

		if t == "loopstart" {
			loopKey := key
			inner := []Field{}
			depth := 1
			i++
			for i < len(fields) && depth > 0 {
				ff := fields[i]
				switch strings.ToLower(ff.Type) {
				case "loopstart":
					depth++
				case "loopstop":
					depth--
				}
				if depth > 0 {
					inner = append(inner, ff)
				}
				i++
			}
			i-- // correct overshoot

			indexField := Field{
				Key:         loopKey + "_index",
				Label:       loopKey + " index",
				Type:        "number",
				Description: "Auto-generated index for loop \"" + loopKey + "\"",
			}
			inner = append([]Field{indexField}, inner...)

			var loopLogs []string
			loopBody := strings.Join(renderFieldBlocks(inner, headingLevel+1, &loopLogs, opts), "\n---\n\n")
			loopLogBlock := buildLogSection(loopLogs)

			result = append(result, fmt.Sprintf(
				"\n%s Loop: %s\n\n{{#loop \"%s\"}}\n%s%s\n{{/loop}}\n",
				strings.Repeat("#", headingLevel), loopKey, loopKey,
				loopBodyWrap(loopBody, opts.WrapLoops), loopLogBlock,
			))
			seen[loopKey+"_index"] = true
			continue
		}

		if t == "loopstop" || seen[key] {
			continue
		}
		result = append(result, generateSingleFieldBlock(f, headingLevel, logs, opts))
		seen[key] = true
	}
	return result
}

func generateSingleFieldBlock(f Field, headingLevel int, logs *[]string, opts GeneratorOptions) string {
	key := f.Key
	if key == "" {
		key = "unknown"
	}
	label := f.Label
	if label == "" {
		label = key
	}
	t := strings.ToLower(f.Type)

	collectLogs(logs, key, t)

	heading := strings.Repeat("#", headingLevel)
	header := fmt.Sprintf("%s %s\n\n_{{fieldDescription \"%s\"}}_\n", heading, label, key)
	return header + "\n" + renderFieldValueBlock(f, opts)
}

// renderFieldValueBlock returns the Handlebars body for one field -
// shared between report and minimal shapes (minimal just drops the
// surrounding header and debug logs). Takes the full Field so type-
// specific branches can read shape-bearing properties (the api branch
// reads f.Map[] when ExpandAPI is on; the image branch is type/key-
// only). opts carries the per-shape sub-choices.
func renderFieldValueBlock(f Field, opts GeneratorOptions) string {
	key := f.Key
	if key == "" {
		key = "unknown"
	}
	label := f.Label
	if label == "" {
		label = key
	}
	typ := strings.ToLower(f.Type)
	imgMode := opts.ImgMode
	switch typ {
	case "facet":
		// Virtual field - its value lives in meta.facets[FacetKey], not
		// in form.data, so {{field}} would always return empty here.
		// {{virtual-field}} dispatches on the template's field type and
		// projects the right value (the selected facet option label).
		return fmt.Sprintf(`{{virtual-field "%s"}}`, key)
	case "boolean":
		return fmt.Sprintf(
			"{{#if (fieldRaw \"%s\")}}\n✅ %s is checked\n{{else}}\n❌ %s is not checked\n{{/if}}",
			key, label, label,
		)
	case "radio", "dropdown":
		return fmt.Sprintf(
			"Selected: {{field \"%s\"}}\n(Value: {{field \"%s\" \"value\"}})",
			key, key,
		)
	case "multioption":
		return fmt.Sprintf(`- Labels:
{{#each (fieldRaw "%s") as |val idx|}}
  {{#with (lookupOption (fieldMeta "%s" "options") val) as |opt|}}
    {{opt.label}}{{#unless (eq idx (subtract (length (fieldRaw "%s")) 1))}}, {{/unless}}
  {{/with}}
{{/each}}

- Values: {{fieldRaw "%s"}}

- All Options:
{{#with (fieldRaw "%s") as |selected|}}
  {{#each (fieldMeta "%s" "options") as |opt|}}
  - [{{#if (includes selected opt.value)}}x{{else}} {{/if}}] {{opt.label}}
  {{/each}}
{{/with}}`, key, key, key, key, key, key)
	case "tags":
		return fmt.Sprintf(`{{#if (fieldRaw "%s")}}
Tags(regular): {{field "%s"}}
Tags(with #): {{tags (fieldRaw "%s") withHash=true}}
Tags(without #): {{tags (fieldRaw "%s") withHash=false}}
{{else}}
_No tags specified_
{{/if}}`, key, key, key, key)
	case "list":
		return fmt.Sprintf(`{{#each (fieldRaw "%s")}}
- {{this}}
{{/each}}`, key)
	case "table":
		return fmt.Sprintf(`{{#if (fieldRaw "%s")}}

<!-- Column Values -->
  {{#with (fieldMeta "%s" "options") as |headers|}}
|{{#each headers}}{{value}} |{{/each}}
|{{#each headers}}--|{{/each}}
  {{/with}}
  {{#each (fieldRaw "%s")}}
|{{#each this}}{{this}} |{{/each}}
  {{/each}}

<!-- Column Labels -->
  {{#with (fieldMeta "%s" "options") as |headers|}}
|{{#each headers}}{{#if label}}{{label}}{{else}}{{value}}{{/if}} |{{/each}}
|{{#each headers}}--|{{/each}}
  {{/with}}
  {{#each (fieldRaw "%s")}}
|{{#each this}}{{this}} |{{/each}}
  {{/each}}

{{/if}}`, key, key, key, key, key)
	case "image":
		return fmt.Sprintf(
			"{{#if (fieldRaw \"%s\")}}\n![%s](%s)\n{{else}}\n_No image uploaded for %s_\n{{/if}}",
			key, label, imageHelperCall(key, imgMode), label,
		)
	case "api":
		// Two visible shapes per the ExpandAPI toggle:
		//   • OFF → {{apiSection "key"}} - runtime helper renders the
		//     full embedded card with header + per-column rows. Lazy.
		//   • ON  → per-column block, wrapped in <section class="api-card">
		//     so the user can hand-tune individual columns or delete
		//     some. Each column uses a "header paragraph + value
		//     paragraph" layout (header on its own line, blank line,
		//     {{apiBlock}} on the next paragraph). That layout is
		//     uniform across scalar and block-typed source columns -
		//     scalars render inline-ish, table/list columns get the
		//     blank line goldmark needs to lift a markdown table or
		//     bullet list out into its own block. Without this two-
		//     paragraph form a {{apiBlock}} that returns a multi-line
		//     table gets gobbled into the surrounding paragraph and
		//     the pipes render as literal text.
		// Empty Map[] → fall back to apiSection (no columns to expand).
		if opts.ExpandAPI && len(f.Map) > 0 {
			var b strings.Builder
			b.WriteString(fmt.Sprintf(
				"<section class=\"api-card\" data-source=\"%s\">\n\n",
				f.Collection,
			))
			for _, m := range f.Map {
				colLabel := strings.TrimSpace(m.Label)
				if colLabel == "" {
					colLabel = m.Key
				}
				b.WriteString(fmt.Sprintf("**%s**:\n\n", colLabel))
				b.WriteString(fmt.Sprintf("{{apiBlock \"%s\" \"%s\"}}\n\n", key, m.Key))
			}
			b.WriteString("</section>")
			return b.String()
		}
		return fmt.Sprintf(`{{apiSection "%s"}}`, key)
	default:
		return fmt.Sprintf(`{{field "%s"}}`, key)
	}
}

func collectLogs(logs *[]string, key, typ string) {
	if typ == "facet" {
		// Virtual fields carry no data slot - fieldRaw is meaningless.
		// Surface the projection that {{virtual-field}} resolves so
		// the debug block shows the selected option label instead.
		*logs = append(*logs, fmt.Sprintf("> **%s** _(facet)_: `{{virtual-field \"%s\"}}`", key, key))
		return
	}
	*logs = append(*logs, fmt.Sprintf("> **%s**: `{{json (fieldRaw \"%s\")}}`", key, key))
	switch typ {
	case "dropdown", "radio", "multioption", "table":
		*logs = append(*logs, fmt.Sprintf("> **%s** _(options)_: `{{json (fieldMeta \"%s\" \"options\")}}`", key, key))
	}
}

func buildLogSection(logs []string) string {
	if len(logs) == 0 {
		return ""
	}
	parts := []string{
		"",
		"",
		"---",
		"",
		"> _Debug: Remove this section when your template is complete._",
		">",
	}
	parts = append(parts, logs...)
	parts = append(parts, "")
	return strings.Join(parts, "\n")
}

// ─── Minimal shape ────────────────────────────────────────────────────

func generateMinimal(fields []Field, opts GeneratorOptions) string {
	parts := renderMinimalBlocks(fields, 2, opts)
	return strings.Join(parts, "\n\n") + "\n"
}

func renderMinimalBlocks(fields []Field, headingLevel int, opts GeneratorOptions) []string {
	out := make([]string, 0, len(fields))
	seen := map[string]bool{}
	heading := strings.Repeat("#", headingLevel)

	for i := 0; i < len(fields); i++ {
		f := fields[i]
		key := f.Key
		if key == "" {
			key = "unknown"
		}
		t := strings.ToLower(f.Type)

		if t == "loopstart" {
			loopKey := key
			inner := []Field{}
			depth := 1
			i++
			for i < len(fields) && depth > 0 {
				ff := fields[i]
				switch strings.ToLower(ff.Type) {
				case "loopstart":
					depth++
				case "loopstop":
					depth--
				}
				if depth > 0 {
					inner = append(inner, ff)
				}
				i++
			}
			i--
			body := strings.Join(renderMinimalBlocks(inner, headingLevel+1, opts), "\n\n")
			out = append(out, fmt.Sprintf("%s Loop: %s\n\n{{#loop \"%s\"}}\n%s\n{{/loop}}",
				heading, loopKey, loopKey, loopBodyWrap(body, opts.WrapLoops)))
			continue
		}
		if t == "loopstop" || seen[key] {
			continue
		}
		seen[key] = true
		label := f.Label
		if label == "" {
			label = key
		}
		out = append(out, fmt.Sprintf("%s %s\n\n%s", heading, label, renderFieldValueBlock(f, opts)))
	}
	return out
}

// ─── Table shape ──────────────────────────────────────────────────────

func generateTable(fields []Field, imgMode ImgMode) string {
	rows := make([]string, 0, len(fields))
	rows = append(rows, "| Field | Value |")
	rows = append(rows, "|-------|-------|")

	seen := map[string]bool{}
	depth := 0
	for _, f := range fields {
		t := strings.ToLower(f.Type)
		key := f.Key
		if key == "" {
			key = "unknown"
		}
		if t == "loopstart" {
			if depth == 0 && !seen[key] {
				rows = append(rows, fmt.Sprintf(`| %s | {{json (fieldRaw "%s")}} |`, key, key))
				seen[key] = true
			}
			depth++
			continue
		}
		if t == "loopstop" {
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth > 0 || seen[key] {
			continue
		}
		seen[key] = true
		rows = append(rows, tableRowForField(f, key, imgMode))
	}
	return strings.Join(rows, "\n") + "\n"
}

func tableRowForField(f Field, key string, imgMode ImgMode) string {
	label := f.Label
	if label == "" {
		label = key
	}
	switch strings.ToLower(f.Type) {
	case "facet":
		return fmt.Sprintf(`| %s | {{virtual-field "%s"}} |`, label, key)
	case "tags":
		return fmt.Sprintf(`| %s | {{tags (fieldRaw "%s")}} |`, label, key)
	case "image":
		return fmt.Sprintf(`| %s | ![%s](%s) |`, label, label, imageHelperCall(key, imgMode))
	case "list", "multioption", "table", "api":
		return fmt.Sprintf(`| %s | {{json (fieldRaw "%s")}} |`, label, key)
	default:
		return fmt.Sprintf(`| %s | {{field "%s"}} |`, label, key)
	}
}

// ─── Frontmatter-only shape ───────────────────────────────────────────

func generateFrontmatter(fields []Field) string {
	lines := []string{"---"}
	seen := map[string]bool{}
	depth := 0
	for _, f := range fields {
		t := strings.ToLower(f.Type)
		key := f.Key
		if key == "" {
			key = "unknown"
		}
		if t == "loopstart" {
			if depth == 0 && !seen[key] {
				lines = append(lines, fmt.Sprintf(`%s: {{json (fieldRaw "%s")}}`, key, key))
				seen[key] = true
			}
			depth++
			continue
		}
		if t == "loopstop" {
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth > 0 || seen[key] {
			continue
		}
		// Image fields don't fit a YAML metadata block (they're binary-
		// shaped), so frontmatter skips them. Api fields are also
		// skipped - their value is a `{guid, ...denormalised_columns}`
		// object, which clutters frontmatter; the host can read the
		// guid via {{apiGuid}} elsewhere if it really needs it.
		if t == "image" || t == "api" {
			continue
		}
		seen[key] = true
		// Virtual fields (facet) have no data slot - read the
		// projection via the virtual-field helper instead of fieldRaw.
		// Quote-wrap the helper call so YAML parses the value as a
		// string even when the resolved label contains a colon, etc.
		if t == "facet" {
			lines = append(lines, fmt.Sprintf(`%s: '{{virtual-field "%s"}}'`, key, key))
			continue
		}
		lines = append(lines, fmt.Sprintf(`%s: {{json (fieldRaw "%s")}}`, key, key))
	}
	lines = append(lines, "---", "")
	return strings.Join(lines, "\n")
}
