Feature: Default markdown-template generator
  As a template author with an empty editor,
  I want a one-click button that scaffolds a working template body
  so I can start filling in the gaps instead of staring at a blank page.

  The generator runs purely from the field list — no template needs to
  be saved first. Four shapes cover the common starting points; an
  unknown shape falls back to "report" so a stale frontend never
  produces empty output.

  Background:
    Given a fresh generator world

  # ── Empty inputs ────────────────────────────────────────────────────

  Scenario Outline: Empty fields produce empty output for every shape
    Given no fields
    When I generate with shape "<shape>"
    Then the generated output is empty

    Examples:
      | shape       |
      | report      |
      | minimal     |
      | table       |
      | frontmatter |

  # ── Shape catalog (drives the picker dialog) ────────────────────────

  Scenario: The shape catalog exposes all four shapes with labels
    When I read the shape catalog
    Then the catalog has 4 entries
    And the catalog contains shape "report"
    And the catalog contains shape "minimal"
    And the catalog contains shape "table"
    And the catalog contains shape "frontmatter"
    And every catalog entry has a non-empty label and description

  # ── Report shape (1:1 port of the original generator) ───────────────

  Scenario: Report shape begins with a frontmatter block
    Given the fields:
      | key   | type | label |
      | title | text | Title |
    When I generate with shape "report"
    Then the output starts with "---\ntitle: Auto-generated Report\n"
    And the output contains "toc: true"
    And the output contains "_{{fieldDescription \"title\"}}_"

  Scenario: Report shape includes a debug log block
    Given the fields:
      | key      | type     | label    |
      | title    | text     | Title    |
      | priority | dropdown | Priority |
    When I generate with shape "report"
    Then the output contains "_Debug: Remove this section when your template is complete._"
    And the output contains "**title**: `{{json (fieldRaw \"title\")}}`"
    And the output contains "**priority** _(options)_: `{{json (fieldMeta \"priority\" \"options\")}}`"

  Scenario: Report shape renders boolean as if/else
    Given the fields:
      | key  | type    | label |
      | done | boolean | Done  |
    When I generate with shape "report"
    Then the output contains "{{#if (fieldRaw \"done\")}}"
    And the output contains "✅ Done is checked"
    And the output contains "❌ Done is not checked"

  Scenario: Report shape wraps loop fields and adds a synthetic index
    Given the fields:
      | key   | type      | label |
      | items | loopstart |       |
      | name  | text      | Name  |
      | items | loopstop  |       |
    When I generate with shape "report"
    Then the output contains "{{#loop \"items\"}}"
    And the output contains "{{/loop}}"
    And the output contains "**items_index**:"

  # ── Minimal shape ───────────────────────────────────────────────────

  Scenario: Minimal shape skips frontmatter and debug section
    Given the fields:
      | key   | type | label |
      | title | text | Title |
    When I generate with shape "minimal"
    Then the output does not start with "---"
    And the output does not contain "Debug: Remove this section"
    And the output contains "## Title"
    And the output contains "{{field \"title\"}}"

  # ── Table shape ─────────────────────────────────────────────────────

  Scenario: Table shape emits a single key/value table
    Given the fields:
      | key   | type    | label |
      | title | text    | Title |
      | done  | boolean | Done  |
      | tags  | tags    | Tags  |
    When I generate with shape "table"
    Then the output contains "| Field | Value |"
    And the output contains "|-------|-------|"
    And the output contains "| Title | {{field \"title\"}} |"
    And the output contains "| Tags | {{tags (fieldRaw \"tags\")}} |"

  Scenario: Table shape collapses inner-loop fields into the loop key row
    Given the fields:
      | key   | type      | label |
      | title | text      | Title |
      | items | loopstart |       |
      | name  | text      |       |
      | items | loopstop  |       |
    When I generate with shape "table"
    Then the output contains "| items |"
    And the output does not contain "| name |"

  # ── Frontmatter-only shape ──────────────────────────────────────────

  Scenario: Frontmatter shape emits only a YAML data block
    Given the fields:
      | key   | type    | label |
      | title | text    |       |
      | done  | boolean |       |
      | tags  | tags    |       |
    When I generate with shape "frontmatter"
    Then the output starts with "---\n"
    And the output contains "title: {{json (fieldRaw \"title\")}}"
    And the output contains "done: {{json (fieldRaw \"done\")}}"
    And the output contains "tags: {{json (fieldRaw \"tags\")}}"
    And the output does not contain "##"

  Scenario: Frontmatter shape does not surface inner-loop fields as keys
    Given the fields:
      | key   | type      | label |
      | items | loopstart |       |
      | name  | text      |       |
      | items | loopstop  |       |
    When I generate with shape "frontmatter"
    Then the output contains "items: {{json (fieldRaw \"items\")}}"
    And the output does not contain "name:"

  # ── Robustness ──────────────────────────────────────────────────────

  Scenario: Unknown shape falls back to report
    Given the fields:
      | key | type | label |
      | x   | text |       |
    When I generate with shape "this-shape-does-not-exist"
    Then the output contains "title: Auto-generated Report"
