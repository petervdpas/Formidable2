# Fields

A **field** is one entry in a template's `fields:` list. Each field
carries a stable `key`, a `type` from the matrix below, and a set of
attributes that the type opts into.

## Common attributes

| Attribute        | Purpose                                                          |
| ---------------- | ---------------------------------------------------------------- |
| `key`            | Stable identifier; used in Handlebars, in `meta.json`, in expressions. |
| `type`           | One of the type ids below.                                       |
| `label`          | Display label shown in the form.                                 |
| `description`    | Help text rendered under the field.                              |
| `default`        | Pre-filled value on new records.                                 |
| `primary_key`    | Marks the field whose value identifies the record.               |
| `summary_field`  | Marks a field surfaced in the storage sidebar's sub-label.       |
| `two_column`     | Renders the field in a two-column row when paired with another. |
| `readonly`       | Disables editing in the form runtime.                            |
| `collapsible`    | For container types — folds the children behind a header.        |
| `expression_item`| Includes the field as a variable in the expression builder.      |
| `format`         | Type-specific format hint (e.g. date, number).                   |
| `options`        | Type-specific list (dropdown choices, file patterns, etc.).      |

Not every attribute applies to every type — the Designer hides rows
that don't apply, and the backend strips disallowed values on save.

## Type matrix

| Type           | Purpose                                                            |
| -------------- | ------------------------------------------------------------------ |
| `text`         | Single-line text.                                                  |
| `textarea`     | Multi-line text with optional Markdown editor.                     |
| `number`       | Numeric input.                                                     |
| `range`        | Slider; honors `min`/`max`/`step` in `options`.                    |
| `date`         | Date picker with localised formatting.                             |
| `boolean`      | Toggle with custom True/False labels in `options`.                 |
| `dropdown`     | Single-select from `options`.                                      |
| `multioption`  | Multi-select checkboxes from `options`.                            |
| `radio`        | Mutually-exclusive radio buttons from `options`.                   |
| `file-path`    | File picker; `options` patterns filter the dialog.                 |
| `folder-path`  | Folder picker.                                                     |
| `list`         | Free-form repeatable string list (chips).                          |
| `table`        | Editable grid; column types include `reference` for record links.  |
| `image`        | Image upload; surfaces `{{imageURL}}` / `{{imageBase64}}`.         |
| `link`         | Free-form URL with optional label.                                 |
| `tags`         | Tag chips bound to `meta.tags`; one per template.                  |
| `api`          | Cross-template lookup against the REST `/api/<tpl>/{id}` route.    |
| `guid`         | Auto-generated record identifier; required by `enable_collection`. |
| `looper`       | Marker — declares a repeatable group below.                        |
| `loopstart`    | Marker — opens a loop body.                                        |
| `loopstop`     | Marker — closes a loop body.                                       |

## Loops

A loop is a repeatable group of fields. Open it with a `loopstart`
entry whose `key` is the loop name; close it with a matching
`loopstop`. Every field between the two becomes part of the loop
body and renders once per iteration.

```yaml
fields:
  - key: chapters
    type: loopstart
    label: "Chapters"
    summary_field: chapter_title
  - key: chapter_title
    type: text
    label: "Chapter title"
  - key: chapter_body
    type: textarea
    label: "Chapter body"
  - key: chapters
    type: loopstop
    label: "Chapters"
```

`summary_field` on `loopstart` picks one of the inner field keys to
summarise each iteration in the form runtime — the value of that
field becomes the row's collapsed label.

### Nested loops

A `loopstart` inside another `loopstart` opens a nested loop. The
maximum depth is **2**: a child loop inside a parent loop is fine; a
grandchild loop fails validation with `excessive-loop-nesting`.

```yaml
fields:
  - key: chapters
    type: loopstart
    label: "Chapters"
    summary_field: chapter_title
  - key: chapter_title
    type: text
  - key: sections                    # inner loop opens here
    type: loopstart
    label: "Sections"
    summary_field: section_title
  - key: section_title
    type: text
  - key: section_body
    type: textarea
  - key: sections                    # inner loop closes
    type: loopstop
  - key: chapters                    # outer loop closes
    type: loopstop
```

The Markdown template renders nested loops with nested
`{{#loop "name"}} ... {{/loop}}` blocks; an auto-generated
`<loopname>_index` is available inside the body. Helpers
`{{loopItemBefore}}` / `{{loopItemAfter}}` expand to the
surrounding separator.

## Plugin field i18n

A plugin form field can declare an `i18n:` base key to enable
translation:

```yaml
- key: schema
  type: text
  label: "Schema"
  description: "DB schema."
  i18n: form.schema
```

The renderer resolves `<plugin-namespace>.form.schema.label` /
`.description` / `.placeholder` against the plugin's locale file.
See the Plugins manual page for the full convention.

## Validation

Save-time validation enforces:

- Unique `key` per template.
- Exactly zero or one `primary_key` field.
- `tags` field count ≤ 1.
- `enable_collection` requires one `guid` field.
- Loop pairing (`loopstart` ↔ `loopstop`) per `looper`.
- `api` field shape (target template + lookup field).

Errors surface in the Designer with the offending field highlighted;
nothing writes to disk while a validation issue is unresolved.
