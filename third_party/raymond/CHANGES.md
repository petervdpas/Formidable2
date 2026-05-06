# Local fork notes

This is `aymerick/raymond` v2.0.2 with the following patch applied for
Formidable2's render pipeline. The upstream repo has been dormant since
2018, so the fix lives here and is wired in via a `replace` directive in
the parent `go.mod`.

## Patch: standalone-tag whitespace control

`parser/whitespace.go` — `isPrevWhitespaceProgram` and
`isNextWhitespaceProgram` now check `node.Original` instead of
`node.Value`. The reference implementation
(handlebars.js's `whitespace-control.js`) tests `prev.original`, but the
original raymond port read the live (post-strip) value.

### Symptom that motivated the patch

Two adjacent standalone block tags on consecutive lines, e.g.

```handlebars
{{/with}}
{{#each rows}}
```

The `{{/with}}` close-standalone strips the leading `\n` off the
inter-block content `\n  ` → leaves `  `. When the visitor later asks
"is `{{#each}}` preceded by whitespace?", the post-strip `node.Value`
no longer matches the regex (which wants `\r?\n\s*$`). So `{{#each}}`
loses its standalone classification and emits stray indent + a blank
line between the two blocks, plus a blank line per `{{#each}}` iteration
(its program body keeps its leading `\n`).

Reading `node.Original` instead — which still holds `\n  ` — restores
the spec-correct behaviour and makes block-after-block templates
(like the recipe template's `{{#with}}`-then-`{{#each}}` for table
header + data rows) render contiguously.

### Regression coverage

`handlebars/whitespace_test.go` adds an "adjacent standalone block
tags" case to `whitespaceControlTests`. Output expected:

```
|hdr|
|a|
|b|
```

(no blank lines between the with-body and the each-body, no blank
lines between each iterations).

The full upstream `handlebars/whitespace_test.go` suite still passes,
i.e. the fix doesn't regress existing whitespace behaviour.

## Patch: options-only variadic helpers

`eval.go` — `callFunc` now special-cases helpers declared as
`func(opts *Options) any`: if the helper takes only `*Options`, raymond
calls it with options regardless of how many positional args the
template passed. The helper reads positional args from `opts.Params()`
itself.

### Symptom that motivated the patch

The original JS Formidable's `field` helper supports both
`{{field "key"}}` (1-arg, mode defaults to "label") and
`{{field "key" "mode"}}` (2-arg, explicit mode). Handlebars.js handles
this via JS's loose arity — the helper signature `function(key, mode,
options)` accepts a variable number of positional args.

Raymond's arity check is strict: a helper with declared signature
`(key string, options *Options)` rejects 2-positional calls with
`Helper field called with argument 1 with type string but it should be
*raymond.Options`. There's no way to declare a polymorphic-arity
helper in upstream raymond.

### Workaround / fix

Helpers that need handlebars.js's variable-arity behaviour register as
`func(opts *Options) any` and read positional args via `opts.Params()`.
The patch is a 4-line guard in `callFunc` that returns early when the
helper takes only `*Options`, skipping the arity check.

Affected helper: `internal/modules/render/helpers.go`'s `field`. Other
helpers (`fieldRaw`, `fieldMeta`, `cell`, `loop`, …) keep their typed
signatures.

### Regression coverage

Existing raymond tests assume helpers with declared positional
parameters reject extra args — none of them register an options-only
helper, so this guard doesn't affect their paths. The Formidable2
render module's `helpers_test.go` covers both the 1-arg and 2-arg
`{{field …}}` invocations.
