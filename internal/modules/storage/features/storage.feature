Feature: Form storage
  Forms are JSON `.meta.json` files stored at
  `<context>/storage/<template-name>/`. Each is `{meta, data}` — meta
  carries id/author/template/created/updated/flagged/tags, data carries
  the form-field values. Sanitization (driven by the template's fields)
  fills defaults for missing values.

  Background:
    Given a system manager rooted at a temp directory
    And a storage manager wrapping that system

  Scenario: Empty storage folder lists no forms
    Given the template "basic" has no forms yet
    When I list forms for "basic.yaml"
    Then the form list is empty

  Scenario: Save then list shows the new form
    Given the template "basic" has no forms yet
    When I save a form "basic.yaml" / "form-1" with data:
      | key   | value     |
      | title | Hello     |
    Then the form list for "basic.yaml" contains "form-1.meta.json"

  Scenario: Save then load round-trips the form data
    Given the template "basic" has no forms yet
    When I save a form "basic.yaml" / "form-1" with data:
      | key   | value     |
      | title | Hello     |
    Then loading "basic.yaml" / "form-1" returns data field "title" equal to "Hello"
    And the loaded form's meta has a non-empty "created" timestamp

  Scenario: Sanitize fills defaults for fields missing in the input
    Given a basic template with a "count" number field defaulting to 7
    When I save a form "basic.yaml" / "form-2" with data:
      | key   | value |
      | title | X     |
    Then loading "basic.yaml" / "form-2" returns data field "count" equal to 7

  Scenario: Loading a missing form returns nil
    When I load form "basic.yaml" / "ghost"
    Then the loaded form is nil

  Scenario: Delete removes the form file
    Given the template "basic" has no forms yet
    And a saved form "basic.yaml" / "form-1"
    When I delete form "basic.yaml" / "form-1"
    Then the form list for "basic.yaml" does not contain "form-1.meta.json"

  Scenario: Extended list includes title from the item_field
    Given a basic template with item_field set to "title"
    And a saved form "basic.yaml" / "form-1" with title "First"
    And a saved form "basic.yaml" / "form-2" with title "Second"
    When I request the extended list for "basic.yaml"
    Then the extended list has 2 entries
    And the extended entry for "form-1.meta.json" has title "First"
    And the extended entry for "form-2.meta.json" has title "Second"

  Scenario: Extended list falls back to filename when item_field value is empty
    Given a basic template with item_field set to "title"
    And a saved form "basic.yaml" / "form-1" with empty title
    When I request the extended list for "basic.yaml"
    Then the extended entry for "form-1.meta.json" has title "form-1.meta.json"

  Scenario: SaveImageFile lands the bytes under storage/<template>/images/
    When I save image bytes "deadbeef" to "basic.yaml" as "pic.png"
    Then the file "storage/basic/images/pic.png" exists
    And the saved image result is success

  Scenario: SaveForm preserves a previously-set id across edits
    Given the template "basic" has no forms yet
    When I save a form "basic.yaml" / "form-1" with data:
      | key   | value     |
      | title | First     |
    And I capture the form's id
    When I save a form "basic.yaml" / "form-1" with data:
      | key   | value     |
      | title | Updated   |
    Then the form's id matches the captured id

  Scenario: Path traversal in datafile name is rejected
    When I save a form "basic.yaml" / "../escape" with data:
      | key   | value |
      | title | X     |
    Then the save returned an error

  # ── facets ────────────────────────────────────────────────────────

  Scenario: Sanitize preserves an explicit facet entry from raw meta
    Given the template "basic" has no forms yet
    When I save a form "basic.yaml" / "form-faceted" with raw meta facet "flag" set true selected "FLASH"
    Then the loaded form's meta has facet "flag" set true selected "FLASH"

  Scenario: Sanitize migrates legacy flagged+flag_state into facets.flag
    Given the template "basic" has no forms yet
    When I save a form "basic.yaml" / "form-legacy" with raw meta flagged true and flag_state ""
    Then the loaded form's meta has facet "flag" set true selected ""

  Scenario: Sanitize emits no facets when nothing is provided
    Given the template "basic" has no forms yet
    When I save a form "basic.yaml" / "form-clean" with data:
      | key   | value |
      | title | Hello |
    Then the loaded form's meta has no facets
