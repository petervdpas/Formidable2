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

// ImgMode selects how image fields are emitted: url -> {{imageURL}}, inline -> {{imageBase64}} (self-contained).
type ImgMode string

const (
	ImgURL    ImgMode = "url"
	ImgInline ImgMode = "inline"
)

// GeneratorOptions carries the per-shape sub-choices the dialog surfaces.
type GeneratorOptions struct {
	ImgMode   ImgMode `json:"img_mode"`
	WrapLoops bool    `json:"wrap_loops"`
	// ExpandAPI false -> {{apiSection}} one-liner; true -> per-column blocks. Must materialise in the source.
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

// GenerateMarkdownTemplate produces default Handlebars markdown for fields in the requested shape.
// Unknown shape falls back to report; unknown image mode to ImgURL.
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

// imageHelperCall returns the Handlebars expression for an image field's URL per ImgMode.
func imageHelperCall(key string, mode ImgMode) string {
	switch mode {
	case ImgInline:
		return fmt.Sprintf(`{{imageBase64 "%s"}}`, key)
	default:
		return fmt.Sprintf(`{{imageURL "%s"}}`, key)
	}
}

// loopBodyWrap wraps an inner loop body in {{loopItemBefore/After}} when WrapLoops is on; wrap state
// lives in the body (not the opener) so a reader sees at a glance whether wrapping is in effect.
func loopBodyWrap(body string, wrap bool) string {
	if !wrap {
		return body
	}
	return "{{loopItemBefore}}\n" + body + "\n{{loopItemAfter}}"
}

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
			i--

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

// renderFieldValueBlock returns the Handlebars body for one field, shared between report and minimal shapes.
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
		// Value lives in meta.facets, not form.data, so {{field}} would be empty; {{virtual-field}} projects it.
		return fmt.Sprintf(`{{virtual-field "%s"}}`, key)
	case "formula":
		// The formula's output is written into the target data field, so render that slot.
		if f.TargetKey == "" {
			return ""
		}
		return fmt.Sprintf(`{{field "%s"}}`, f.TargetKey)
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
		// {{field}} renders the list via emitList, which nests indented rows into
		// a proper sub-list. The old {{#each (fieldRaw)}}- {{this}} pattern was flat
		// by construction (the "- " sits at column 0), so it could never nest.
		return fmt.Sprintf(`{{field "%s"}}`, key)
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
	case "mermaid":
		// Dedicated {{mermaid}} accessor (parallels image's helper). Guard the
		// empty state so a blank field doesn't emit a stray empty fence.
		return fmt.Sprintf(
			"{{#if (fieldRaw \"%s\")}}\n{{mermaid \"%s\"}}\n{{else}}\n_No diagram for %s_\n{{/if}}",
			key, key, label,
		)
	case "api":
		// ExpandAPI on emits per-column "header paragraph + blank line + {{apiBlock}}" blocks: the blank
		// line is what goldmark needs to lift a multi-line table/list out of the surrounding paragraph;
		// without it the pipes render as literal text. Empty Map falls back to apiSection.
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
		// Virtual fields have no data slot; surface the {{virtual-field}} projection instead of fieldRaw.
		*logs = append(*logs, fmt.Sprintf("> **%s** _(facet)_: `{{virtual-field \"%s\"}}`", key, key))
		return
	}
	if typ == "formula" {
		// Virtual: the value is written into the target field, which carries its own debug line.
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
	case "formula":
		if f.TargetKey == "" {
			return fmt.Sprintf(`| %s |  |`, label)
		}
		return fmt.Sprintf(`| %s | {{field "%s"}} |`, label, f.TargetKey)
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
		// Image and api fields don't fit a YAML metadata block, so frontmatter skips them.
		if t == "image" || t == "api" {
			continue
		}
		seen[key] = true
		// Facet has no data slot; yamlString encodes the label as a YAML string so a colon or & survives.
		if t == "facet" {
			lines = append(lines, fmt.Sprintf(`%s: {{yamlString (virtual-field "%s")}}`, key, key))
			continue
		}
		// Formula has no data slot of its own; project the target field's value, YAML-encoded.
		if t == "formula" {
			if f.TargetKey != "" {
				lines = append(lines, fmt.Sprintf(`%s: {{yamlString (field "%s")}}`, key, f.TargetKey))
			}
			continue
		}
		lines = append(lines, fmt.Sprintf(`%s: {{json (fieldRaw "%s")}}`, key, key))
	}
	lines = append(lines, "---", "")
	return strings.Join(lines, "\n")
}
