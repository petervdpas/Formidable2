package render

// HelperCategory groups helpers into sections in the reference panel.
// The values are stable: the frontend uses them as i18n key suffixes
// (e.g. `render.helpers.category.field`).
type HelperCategory string

const (
	HelperCategoryComparison HelperCategory = "comparison"
	HelperCategoryMath       HelperCategory = "math"
	HelperCategoryCollection HelperCategory = "collection"
	HelperCategoryString     HelperCategory = "string"
	HelperCategoryFormat     HelperCategory = "format"
	HelperCategoryScratch    HelperCategory = "scratch"
	HelperCategoryLookup     HelperCategory = "lookup"
	HelperCategoryField      HelperCategory = "field"
	HelperCategoryImage      HelperCategory = "image"
	HelperCategoryLoop       HelperCategory = "loop"
	HelperCategoryStats      HelperCategory = "stats"
	HelperCategoryTags       HelperCategory = "tags"
	HelperCategoryAPI        HelperCategory = "api"
	HelperCategoryDate       HelperCategory = "date"
	HelperCategoryMeta       HelperCategory = "meta"
)

// HelperDescriptor describes one registered Handlebars helper, JSON-
// serialised for the Wails service. Fields are terse: these are
// reference cards, not tutorials.
type HelperDescriptor struct {
	// Name is the helper identifier as used in `{{<name> …}}`.
	Name string `json:"name"`
	// Signature is a usage hint; square brackets mark optional args.
	Signature string `json:"signature"`
	// Description is a one-sentence summary; the vue-i18n fallback.
	Description string `json:"description"`
	// Example is a single-line, copy-pasteable invocation.
	Example string `json:"example"`
	// Category groups the helper in the reference panel.
	Category HelperCategory `json:"category"`
}

// builtinHelpers is the static catalog of every helper this module
// registers, drift-guarded by TestCatalog_MatchesRegisteredHelpers.
// Keep entries grouped by Category and alphabetical within each group;
// add one whenever a `tpl.RegisterHelper` call is added.
var builtinHelpers = []HelperDescriptor{
	// ── comparison ───────────────────────────────────────────────
	{Name: "compare", Signature: `{{compare a op b}}`, Description: "True when the comparison `a <op> b` holds (`===`, `!==`, `<`, `<=`, `>`, `>=`).", Example: `{{#if (compare score ">=" 80)}}pass{{/if}}`, Category: HelperCategoryComparison},
	{Name: "eq", Signature: `{{eq a b}}`, Description: "Strict equality.", Example: `{{#if (eq status "open")}}open{{/if}}`, Category: HelperCategoryComparison},
	{Name: "gt", Signature: `{{gt a b}}`, Description: "Greater-than.", Example: `{{#if (gt count 0)}}has items{{/if}}`, Category: HelperCategoryComparison},
	{Name: "gte", Signature: `{{gte a b}}`, Description: "Greater-than-or-equal.", Example: `{{#if (gte age 18)}}adult{{/if}}`, Category: HelperCategoryComparison},
	{Name: "lt", Signature: `{{lt a b}}`, Description: "Less-than.", Example: `{{#if (lt n 10)}}small{{/if}}`, Category: HelperCategoryComparison},
	{Name: "lte", Signature: `{{lte a b}}`, Description: "Less-than-or-equal.", Example: `{{#if (lte temp 0)}}freezing{{/if}}`, Category: HelperCategoryComparison},
	{Name: "ne", Signature: `{{ne a b}}`, Description: "Strict inequality.", Example: `{{#if (ne status "draft")}}live{{/if}}`, Category: HelperCategoryComparison},

	// ── math ─────────────────────────────────────────────────────
	{Name: "abs", Signature: `{{abs n}}`, Description: "Absolute value.", Example: `{{abs delta}}`, Category: HelperCategoryMath},
	{Name: "add", Signature: `{{add a b}}`, Description: "Arithmetic sum.", Example: `{{add 1 2}}`, Category: HelperCategoryMath},
	{Name: "ceil", Signature: `{{ceil n}}`, Description: "Round up to the next integer.", Example: `{{ceil 1.3}}`, Category: HelperCategoryMath},
	{Name: "divide", Signature: `{{divide a b}}`, Description: "Arithmetic division.", Example: `{{divide total count}}`, Category: HelperCategoryMath},
	{Name: "floor", Signature: `{{floor n}}`, Description: "Round down to the next integer.", Example: `{{floor 1.7}}`, Category: HelperCategoryMath},
	{Name: "math", Signature: `{{math a op b}}`, Description: "Generic math dispatch (`add`/`subtract`/`multiply`/`divide`/`mod`/`pad`/`abs`/`round`/`ceil`/`floor`).", Example: `{{math 3 "multiply" 4}}`, Category: HelperCategoryMath},
	{Name: "mod", Signature: `{{mod a b}}`, Description: "Modulo (remainder).", Example: `{{mod n 2}}`, Category: HelperCategoryMath},
	{Name: "multiply", Signature: `{{multiply a b}}`, Description: "Arithmetic product.", Example: `{{multiply qty unitPrice}}`, Category: HelperCategoryMath},
	{Name: "pad", Signature: `{{pad n width}}`, Description: "Left-pad an integer to the given width with zeros.", Example: `{{pad index 3}}`, Category: HelperCategoryMath},
	{Name: "round", Signature: `{{round n}}`, Description: "Round to the nearest integer.", Example: `{{round avg}}`, Category: HelperCategoryMath},
	{Name: "subtract", Signature: `{{subtract a b}}`, Description: "Arithmetic difference.", Example: `{{subtract end start}}`, Category: HelperCategoryMath},

	// ── collection ───────────────────────────────────────────────
	{Name: "includes", Signature: `{{includes arr value}}`, Description: "True when the array contains the given value (string match).", Example: `{{#if (includes tags "draft")}}DRAFT{{/if}}`, Category: HelperCategoryCollection},
	{Name: "isSelected", Signature: `{{#isSelected arr value}}…{{else}}…{{/isSelected}}`, Description: "Block helper: renders the inner block when `arr` includes `value`.", Example: `{{#isSelected items "x"}}yes{{else}}no{{/isSelected}}`, Category: HelperCategoryCollection},
	{Name: "length", Signature: `{{length arr}}`, Description: "Length of an array or slice. Non-array → 0.", Example: `{{length items}} items`, Category: HelperCategoryCollection},

	// ── string ───────────────────────────────────────────────────
	{Name: "camel", Signature: `{{camel s}}`, Description: "Lowercase the first letter.", Example: `{{camel "FooBar"}}`, Category: HelperCategoryString},
	{Name: "pascal", Signature: `{{pascal s}}`, Description: "Uppercase the first letter.", Example: `{{pascal "fooBar"}}`, Category: HelperCategoryString},

	// ── date / time ──────────────────────────────────────────────
	{Name: "today", Signature: `{{today}}`, Description: "Current date as YYYY-MM-DD. Stamps the document with the moment it was rendered. Common in PDF frontmatter `date:` fields.", Example: `date: '{{today}}'`, Category: HelperCategoryDate},
	{Name: "now", Signature: `{{now [layout] [locale]}}`, Description: "Current time formatted with a Go time layout, optionally translated into a locale (en, nl, de, fr). Default layout is \"2006-01-02 15:04:05\".", Example: `{{now "Mon, 02 Jan 2006" "nl"}}`, Category: HelperCategoryDate},
	{Name: "dateFormat", Signature: `{{dateFormat value layout [locale]}}`, Description: "Reformat a stored date / datetime string with a Go time layout, optionally translated into a locale (en, nl, de, fr). Recognises YYYY-MM-DD, RFC 3339, and `2006-01-02 15:04:05`. Unparseable input passes through.", Example: `{{dateFormat (field "due") "Monday 2 January 2006" "nl"}}`, Category: HelperCategoryDate},

	// ── format ───────────────────────────────────────────────────
	{Name: "json", Signature: `{{json value}}`, Description: "Render a value as pretty-printed JSON (2-space indent). Safe - output is not HTML-escaped.", Example: `{{json record}}`, Category: HelperCategoryFormat},
	{Name: "log", Signature: `{{log value}}`, Description: "Emit a `[LOG]`-prefixed JSON dump into the rendered output. Debugging aid.", Example: `{{log ctx}}`, Category: HelperCategoryFormat},

	// ── scratch (per-call vars) ──────────────────────────────────
	{Name: "getVar", Signature: `{{getVar name}}`, Description: "Read a per-render scratch variable previously set by `setVar`.", Example: `{{getVar "total"}}`, Category: HelperCategoryScratch},
	{Name: "setVar", Signature: `{{setVar name value}}`, Description: "Store a per-render scratch variable. Returns empty so it can be inlined.", Example: `{{setVar "total" (add a b)}}`, Category: HelperCategoryScratch},

	// ── lookup ───────────────────────────────────────────────────
	{Name: "cell", Signature: `{{cell row colName tableKey}}`, Description: "Read one cell from a table row by column name. Resolves column index via the table field's options.", Example: `{{cell row "amount" "lineItems"}}`, Category: HelperCategoryLookup},
	{Name: "lookupOption", Signature: `{{lookupOption options value}}`, Description: "Find the option object (`{value, label, …}`) matching the given value in an options array.", Example: `{{lookupOption status.options status.value}}`, Category: HelperCategoryLookup},

	// ── field accessors ──────────────────────────────────────────
	{Name: "board", Signature: `{{board [scope=false]}}`, Description: "Render the plan-board record (Project Mode) as a mermaid Gantt chart followed by a GFM table of its events. Sections are the project's resources; each event is a task. A leading \"Project\" bar spans the board's full from/to window (so the axis shows the whole period, not just the events); pass `scope=false` to leave it out and let the chart auto-fit to the events. The table carries the four event axes (resource, kind, start, end) plus one column per author-added events-loop field. Empty when there are no events.", Example: `{{board scope=false}}`, Category: HelperCategoryField},
	{Name: "boardSlices", Signature: `{{#boardSlices [count=N] [size=N] [trim=true]}}…{{/boardSlices}}`, Description: "Block helper: iterate the plan board as calendar slices for printing. `count` = number of pages (the divisor: count=2 → \"1 of 2\", \"2 of 2\"); `size` = axis columns per page instead (size wins if both given); no divisor = one page (the whole board). By default it tiles the WHOLE project window, so the full project prints across the pages with empty stretches shown as framed empty pages; `trim=true` tiles only the span the events cover, drops empty pages, and omits the project scope bar (each page fits to its events), so the index runs as far as there are events. An event straddling a boundary appears in each slice it touches (clipped in that slice's Gantt, full dates in its table). Each iteration exposes `gantt` and `table` (drop in with triple-stash), plus `index`, `total`, `isFirst`, `isLast`, `from`, `to`.", Example: "{{#boardSlices count=2}}\n{{{gantt}}}\n\n{{{table}}}\n{{#unless isLast}}<div class=\"page-break\"></div>{{/unless}}\n{{/boardSlices}}", Category: HelperCategoryField},
	{Name: "boardMeta", Signature: `{{boardMeta "prop" [unit]}}`, Description: "Read one scalar off the plan-board record: `name`, `from`, `to`, `timeblock`, `duration` (calendar days, or `weeks`/`months` via the unit arg), `ticks` (axis column count), `events` (event count), `resources` (resource count). Empty for an unknown prop or an unset axis.", Example: `Duration: {{boardMeta "duration" "weeks"}} weeks`, Category: HelperCategoryField},
	{Name: "field", Signature: `{{field "key" [mode]}}`, Description: "Polymorphic field renderer. Mode defaults to `label`; supports `value`, `href`, `text`, `default` (link fields render as Markdown links by default).", Example: `{{field "status"}}`, Category: HelperCategoryField},
	{Name: "fieldDescription", Signature: `{{fieldDescription "key"}}`, Description: "Description string from the field's template definition.", Example: `{{fieldDescription "status"}}`, Category: HelperCategoryField},
	{Name: "fieldMeta", Signature: `{{fieldMeta "key" "prop"}}`, Description: "Read a property off the field's template definition (`key`, `type`, `label`, `description`, `options`, or empty for the whole field). For an api field, `options` returns its Map columns in `{value,label}` shape - the same header idiom as a table field.", Example: `{{fieldMeta "status" "type"}}`, Category: HelperCategoryField},
	{Name: "fieldRaw", Signature: `{{fieldRaw "key"}}`, Description: "Raw stored value for a field - bypasses the formatting in `{{field}}`.", Example: `{{fieldRaw "tags"}}`, Category: HelperCategoryField},
	{Name: "list", Signature: `{{list "key" [ordered=true]}}`, Description: "Render a list field as Markdown: bulleted by default, or numbered with `ordered=true` (or `mode=\"ordered\"` - the string must be quoted). Indented rows nest into a sub-list in either mode.", Example: `{{list "steps" ordered=true}}`, Category: HelperCategoryField},
	{Name: "mermaid", Signature: `{{mermaid "key"}}`, Description: "Emit a mermaid field's diagram source as a fenced ```mermaid block. The dedicated accessor for the mermaid field type; markdown export keeps the fence verbatim, the HTML/PDF pipeline renders it as a diagram. Empty source emits nothing.", Example: `{{mermaid "flow"}}`, Category: HelperCategoryField},
	{Name: "virtual-field", Signature: `{{virtual-field "key"}}`, Description: "Render a virtual (data-less) field's projection by template field key. Today: facet fields → the selected option label, empty when unset. Use `{{field}}` for regular fields; this helper fails safe to empty for any non-virtual key.", Example: `{{virtual-field "status_inline"}}`, Category: HelperCategoryField},

	// ── image ────────────────────────────────────────────────────
	{Name: "imageBase64", Signature: `{{imageBase64 "key"}}`, Description: "`data:<mime>;base64,…` URL for an image field's stored filename. Used for self-contained markdown exports.", Example: `![logo]({{imageBase64 "logo"}})`, Category: HelperCategoryImage},
	{Name: "imageURL", Signature: `{{imageURL "key"}}`, Description: "Resolve an image field's filename to a transport-specific URL (slideout: `/api/images/…`; wiki: `/storage/…/images/…`).", Example: `![logo]({{imageURL "logo"}})`, Category: HelperCategoryImage},

	// ── loop ─────────────────────────────────────────────────────
	{Name: "loop", Signature: `{{#loop "key"}}…{{/loop}}`, Description: "Iterate over a loop field's entries. Each iteration's context exposes the entry's fields plus `_loopKey` / `_loopIndex`.", Example: `{{#loop "members"}}- {{field "name"}}{{/loop}}`, Category: HelperCategoryLoop},
	{Name: "loopIndex", Signature: `{{loopIndex}}`, Description: "Current iteration's 1-based index. Empty outside a loop body.", Example: `Item #{{loopIndex}}`, Category: HelperCategoryLoop},
	{Name: "loopItemAfter", Signature: `{{loopItemAfter}}`, Description: "Closes the iteration `<section>` opened by `loopItemBefore`. Empty outside a loop.", Example: `{{loopItemBefore}}…{{loopItemAfter}}`, Category: HelperCategoryLoop},
	{Name: "loopItemBefore", Signature: `{{loopItemBefore [extra-classes…]}}`, Description: "Open a `<section class=\"loop-item\">` wrapper around the iteration body. Variadic extras append additional CSS classes.", Example: `{{loopItemBefore "highlight"}}`, Category: HelperCategoryLoop},
	{Name: "loopItemClass", Signature: `{{loopItemClass [extra…]}}`, Description: "Compose a class attribute string with `loop-item` as the base. For custom wrappers when you don't want `loopItemBefore`/`After`.", Example: `<article class="{{loopItemClass "odd"}}">`, Category: HelperCategoryLoop},
	{Name: "loopKey", Signature: `{{loopKey}}`, Description: "Key of the loop currently being iterated. Empty outside a loop body.", Example: `Section: {{loopKey}}`, Category: HelperCategoryLoop},

	// ── stats ────────────────────────────────────────────────────
	{Name: "stats", Signature: `{{stats table [colIndex]}}`, Description: "Summary statistics for a numeric column of a table field. `colIndex` defaults to 1 (zero-indexed).", Example: `{{stats lineItems 2}}`, Category: HelperCategoryStats},

	// ── tags / lists ─────────────────────────────────────────────
	{Name: "tags", Signature: `{{tags arr [withHash=true]}}`, Description: "Render an array as comma-joined kebab-cased labels (`#audit, #governance`). Pass `withHash=false` to drop the leading hash.", Example: `Topics: {{tags (fieldRaw "topics") withHash=false}}`, Category: HelperCategoryTags},
	{Name: "yamlList", Signature: `{{yamlList arr [indent=N]}}`, Description: "Emit a YAML block-sequence chunk (`- a\\n- b\\n…`) from an array. Use at column 0 inside a `keys:` block - see PDF frontmatter `keywords:` for the canonical case.", Example: `{{yamlList (fieldRaw "adapter-tags")}}`, Category: HelperCategoryTags},
	{Name: "yamlString", Signature: `{{yamlString value}}`, Description: "Encode a scalar as a quoted YAML string for a frontmatter value, so `&`, `:`, `#` and quotes survive instead of being HTML-escaped (`&amp;`) by a bare `{{field}}`. Always quotes, so numeric- or boolean-looking values stay strings. The single-value counterpart to `yamlList`.", Example: `title: {{yamlString (fieldRaw "name")}}`, Category: HelperCategoryTags},

	// ── api fields ───────────────────────────────────────────────
	{Name: "apiBlock", Signature: `{{apiBlock "fieldKey" "columnKey"}}`, Description: "Type-aware block render for one column of an api-field's first referenced record (scalar passthrough; tags joined; lists as markdown bullets; tables as pipe-tables).", Example: `{{apiBlock "ref" "lineItems"}}`, Category: HelperCategoryAPI},
	{Name: "apiCol", Signature: `{{apiCol "fieldKey" "columnKey"}}`, Description: "Read one projected column from an api-field's first referenced record. Scalars pass through; non-scalars render as compact JSON.", Example: `{{apiCol "ref" "title"}}`, Category: HelperCategoryAPI},
	{Name: "apiGuid", Signature: `{{apiGuid "fieldKey"}}`, Description: "GUID(s) of an api-field's referenced record(s) - a single id, or ids joined by `, ` for a to-many reference. Empty when nothing has been picked.", Example: `{{apiGuid "ref"}}`, Category: HelperCategoryAPI},
	{Name: "apiRows", Signature: `{{apiRows "fieldKey"}}`, Description: "The api-field's referenced records for `{{#each}}`. Each row is a map: column key -> value (`{{this.term}}`), plus `link` (`{{this.link}}`) and `labels` (`{{this.labels.term}}`). Pair with `{{fieldMeta \"fieldKey\" \"options\"}}` for the header row.", Example: `{{#each (apiRows "ref")}}- [{{this.term}}]({{this.link}}){{/each}}`, Category: HelperCategoryAPI},
	{Name: "apiSection", Signature: `{{apiSection "fieldKey"}}`, Description: "Full embedded-card markdown, one card per referenced record (so a to-many reference shows them all) - header + per-column lines wrapped in `<section class=\"api-card\">`. The first column's value links to the record (the rendered \"go to record\") when a link resolver is wired.", Example: `{{apiSection "ref"}}`, Category: HelperCategoryAPI},
	{Name: "apiTable", Signature: `{{apiTable "fieldKey" ["col" ...]}}`, Description: "All referenced records of an api field as one markdown pipe-table: one row per referenced record, cells single-line (collections joined, newlines collapsed). Extra string args name the columns to show, in that order (`{{apiTable \"ref\" \"term\" \"status\"}}`); with none, every Map column is shown. Header is the Map alias when declared, else the column key. Empty when nothing is picked.", Example: `{{apiTable "ref" "term" "status"}}`, Category: HelperCategoryAPI},

	// ── meta (current-render identity) ───────────────────────────
	{Name: "datafile", Signature: `{{datafile}}`, Description: "Filename of the data file being rendered (e.g. `chapter-01.meta.json`).", Example: `path: {{datafile}}`, Category: HelperCategoryMeta},
	{Name: "datafileStem", Signature: `{{datafileStem}}`, Description: "Filename of the data file with `.meta.json` stripped - useful as a stable slug for wiki paths or anchors.", Example: `path: {{templateStem}}/{{datafileStem}}`, Category: HelperCategoryMeta},
	{Name: "templateName", Signature: `{{templateName}}`, Description: "Filename of the template being rendered (e.g. `recipes.yaml`).", Example: `template: {{templateName}}`, Category: HelperCategoryMeta},
	{Name: "templateStem", Signature: `{{templateStem}}`, Description: "Filename of the template with the `.yaml` extension stripped - the slug form used in URLs and wiki paths.", Example: `slug: {{templateStem}}`, Category: HelperCategoryMeta},
}

// Catalog returns a copy of the helper descriptor list; callers may
// mutate it freely.
func Catalog() []HelperDescriptor {
	out := make([]HelperDescriptor, len(builtinHelpers))
	copy(out, builtinHelpers)
	return out
}
