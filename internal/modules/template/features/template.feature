Feature: Template management
  Templates are YAML files at <context>/templates/<name>.yaml that
  declare a form's fields. The template module owns CRUD plus
  validation (duplicate keys, loop pairing/nesting, single tags field,
  api-field shape, collection-mode requires a GUID field).

  Background:
    Given a system manager rooted at a temp directory
    And a template manager rooted under "templates"

  Scenario: First run on an empty templates folder returns nothing
    When I list templates
    Then the template list is empty

  Scenario: Save then list shows the new file
    When I save a template named "basic.yaml" with the following yaml:
      """
      name: Basic Form
      filename: basic.yaml
      fields:
        - key: title
          type: text
          label: Title
      """
    Then the template list contains "basic.yaml"

  Scenario: Save then load round-trips the template
    When I save a template named "basic.yaml" with the following yaml:
      """
      name: Basic Form
      filename: basic.yaml
      fields:
        - key: title
          type: text
          label: Title
      """
    Then loading "basic.yaml" returns a template named "Basic Form"
    And the template has 1 field
    And field 0 has key "title" and type "text"

  Scenario: Loading a missing template returns an error
    When I load template "ghost.yaml"
    Then the load returns an error

  Scenario: Delete removes the template file
    Given a template "basic.yaml" exists
    When I delete template "basic.yaml"
    Then the template list does not contain "basic.yaml"

  Scenario: Validate flags duplicate field keys
    Given a template with fields:
      | key   | type |
      | name  | text |
      | name  | text |
    Then validation reports a "duplicate-keys" error

  Scenario: Validate ignores duplicate keys in matched loop pairs
    Given a template with fields:
      | key   | type      |
      | items | loopstart |
      | x     | text      |
      | items | loopstop  |
    Then validation reports no errors

  Scenario: Validate flags an unmatched loopstart
    Given a template with fields:
      | key   | type      |
      | items | loopstart |
      | x     | text      |
    Then validation reports an "unmatched-loopstart" error

  Scenario: Validate flags an unmatched loopstop
    Given a template with fields:
      | key   | type     |
      | x     | text     |
      | items | loopstop |
    Then validation reports an "unmatched-loopstop" error

  Scenario: Validate flags loop key mismatch
    Given a template with fields:
      | key   | type      |
      | a     | loopstart |
      | x     | text      |
      | b     | loopstop  |
    Then validation reports a "loop-key-mismatch" error

  Scenario: Validate flags excessive loop nesting (>2 levels)
    Given a template with fields:
      | key | type      |
      | l1  | loopstart |
      | l2  | loopstart |
      | l3  | loopstart |
      | x   | text      |
      | l3  | loopstop  |
      | l2  | loopstop  |
      | l1  | loopstop  |
    Then validation reports an "excessive-loop-nesting" error

  Scenario: Validate flags collections without a guid field
    Given a template with collections enabled and fields:
      | key  | type |
      | name | text |
    Then validation reports a "missing-guid-for-collection" error

  Scenario: Validate accepts collections with a guid field
    Given a template with collections enabled and fields:
      | key  | type |
      | id   | guid |
      | name | text |
    Then validation reports no errors

  Scenario: Validate flags multiple tags fields
    Given a template with fields:
      | key  | type |
      | t1   | tags |
      | t2   | tags |
    Then validation reports a "multiple-tags-fields" error

  Scenario: Validate flags multiple guid fields
    Given a template with fields:
      | key  | type |
      | g1   | guid |
      | g2   | guid |
    Then validation reports a "multiple-guid-fields" error

  Scenario: Save refuses an invalid template
    Given a template with fields:
      | key  | type |
      | g1   | guid |
      | g2   | guid |
    When I save the current template as "bad.yaml"
    Then the save returned a validation error of type "multiple-guid-fields"
    And the template list does not contain "bad.yaml"

  Scenario: Validate flags an api field without a collection
    Given a template with an api field with no collection
    Then validation reports an "api-collection-required" error

  Scenario: Seed basic creates basic.yaml when templates folder is empty
    When I seed the basic template
    Then the template list contains "basic.yaml"
    And loading "basic.yaml" returns a template named "Basic Form"

  Scenario: Seed basic skips when a template already exists
    Given a template "other.yaml" exists
    When I seed the basic template
    Then the template list does not contain "basic.yaml"

  Scenario: Item fields lists top-level text fields only
    Given a template with fields:
      | key       | type      |
      | title     | text      |
      | tag       | tags      |
      | items     | loopstart |
      | inner     | text      |
      | items     | loopstop  |
      | tail      | text      |
    Then the item fields are "title,tail"

  Scenario: GetTemplateDescriptor returns name + parsed yaml + storage path
    Given a template "basic.yaml" exists
    When I request the descriptor for "basic.yaml"
    Then the descriptor name is "basic.yaml"
    And the descriptor has a non-empty storage location

  Scenario: Loading malformed YAML returns an error
    Given the file "templates/broken.yaml" with content "name: [invalid yaml here\nfields"
    When I load template "broken.yaml"
    Then the load returns an error

  Scenario: GetTemplateDescriptor on a missing template returns an error
    When I request the descriptor for "ghost.yaml"
    Then the descriptor request returned an error

  Scenario: Validate flags an api map entry with empty key
    Given a template with an api field with map keys "" "name"
    Then validation reports an "api-map-key-required" error

  Scenario: Validate flags duplicate api map keys (case-insensitive)
    Given a template with an api field with map keys "Name" "name"
    Then validation reports an "api-map-duplicate-keys" error

  Scenario: Listing templates when the folder doesn't exist returns empty
    When I list templates from a nonexistent folder
    Then the template list is empty

  Scenario: Save with empty name is rejected
    When I save the test template with empty name
    Then the save returned an error

  Scenario: Save with a nil template is rejected
    When I save a nil template named "x.yaml"
    Then the save returned an error

  Scenario: GetItemFields on a missing template returns an error
    When I request item fields for "ghost.yaml"
    Then the item fields request returned an error

  # ── facets ────────────────────────────────────────────────────────

  Scenario: Validate accepts a template without facets
    Given a template with fields:
      | key   | type |
      | title | text |
    Then validation reports no errors

  Scenario: Validate accepts a facet within the limits
    Given a template with fields:
      | key   | type |
      | title | text |
    And the template has facet "status" with icon "fa-flag" and options:
      | label     | color  |
      | FLASH     | red    |
      | IMMEDIATE | orange |
      | PRIORITY  | amber  |
      | ROUTINE   | blue   |
    Then validation reports no errors

  Scenario: Validate accepts a label with spaces
    Given a template with fields:
      | key   | type |
      | title | text |
    And the template has facet "status" with icon "fa-flag" and options:
      | label   | color |
      | NO FLAG | gray  |
    Then validation reports no errors

  Scenario: Validate flags more than 5 facets
    Given a template with fields:
      | key   | type |
      | title | text |
    And the template has 6 facets
    Then validation reports a "too-many-facets" error

  Scenario: Validate flags an icon outside the curated palette
    Given a template with fields:
      | key   | type |
      | title | text |
    And the template has facet "status" with icon "fa-rocket" and options:
      | label | color |
      | OPEN  | red   |
    Then validation reports an "unknown-facet-icon" error

  Scenario: Validate flags duplicate facet keys
    Given a template with fields:
      | key   | type |
      | title | text |
    And the template has facet "status" with icon "fa-flag" and options:
      | label | color |
      | OPEN  | red   |
    And the template has facet "status" with icon "fa-check" and options:
      | label | color |
      | DONE  | green |
    Then validation reports a "duplicate-facet-key" error

  Scenario: Validate flags an invalid facet key
    Given a template with fields:
      | key   | type |
      | title | text |
    And the template has facet "Status" with icon "fa-flag" and options:
      | label | color |
      | OPEN  | red   |
    Then validation reports an "invalid-facet-key" error

  Scenario: Validate flags a missing facet icon
    Given a template with fields:
      | key   | type |
      | title | text |
    And the template has facet "status" with icon "" and options:
      | label | color |
      | OPEN  | red   |
    Then validation reports a "missing-facet-icon" error

  Scenario: Validate flags a facet with no options
    Given a template with fields:
      | key   | type |
      | title | text |
    And the template has facet "status" with icon "fa-flag" and no options
    Then validation reports an "empty-facet-options" error

  Scenario: Validate flags duplicate option labels within a facet
    Given a template with fields:
      | key   | type |
      | title | text |
    And the template has facet "status" with icon "fa-flag" and options:
      | label | color |
      | FLASH | red   |
      | FLASH | blue  |
    Then validation reports a "duplicate-facet-label" error

  Scenario: Validate accepts duplicate option labels across facets
    Given a template with fields:
      | key   | type |
      | title | text |
    And the template has facet "status" with icon "fa-flag" and options:
      | label | color |
      | DONE  | red   |
    And the template has facet "review" with icon "fa-user" and options:
      | label | color |
      | DONE  | green |
    Then validation reports no errors

  Scenario: Validate flags an invalid option label format
    Given a template with fields:
      | key   | type |
      | title | text |
    And the template has facet "status" with icon "fa-flag" and options:
      | label | color |
      | flash | red   |
    Then validation reports an "invalid-facet-label" error

  Scenario: Validate flags an unknown option color
    Given a template with fields:
      | key   | type |
      | title | text |
    And the template has facet "status" with icon "fa-flag" and options:
      | label | color    |
      | FLASH | crimson  |
    Then validation reports an "unknown-facet-color" error

  Scenario: facets round-trip through YAML
    Given a template with fields:
      | key   | type |
      | title | text |
    And the template has facet "status" with icon "fa-flag" and options:
      | label     | color  |
      | FLASH     | red    |
      | IMMEDIATE | orange |
    When I marshal the template and reload it
    Then the reloaded template has 1 facet
    And reloaded facet 0 has key "status" and icon "fa-flag"
    And reloaded facet 0 option 0 is "FLASH" colored "red"
    And reloaded facet 0 option 1 is "IMMEDIATE" colored "orange"

  Scenario: legacy flag_definitions migrate to one synthetic facet on read
    When I reload a template authored with legacy flag_definitions:
      | label      | color |
      | NOT IN USE | red   |
      | IN USE     | green |
    Then the reloaded template has 1 facet
    And reloaded facet 0 has key "flag" and icon "fa-flag"
    And reloaded facet 0 option 0 is "NOT IN USE" colored "red"
    And reloaded facet 0 option 1 is "IN USE" colored "green"

  # ── Field-type registry + per-type validation ─────────────────────

  Scenario: Validate flags an unknown field type
    Given a template with fields:
      | key | type    |
      | x   | mystery |
    Then validation reports an "unknown-field-type" error

  Scenario: Validate flags a forbidden attribute on a guid field
    Given a template with one guid field "g" with collapsible true
    Then validation reports a "forbidden-attribute" error for key "g" and attr "collapsible"

  Scenario: Validate flags a forbidden format attribute on a number field
    Given a template with one number field "n" with format "markdown"
    Then validation reports a "forbidden-attribute" error for key "n" and attr "format"

  Scenario: List/table allow collapsible
    Given a template with one list field "li" with collapsible true
    Then validation reports no errors

  Scenario: Loopstart allows summary_field
    Given a template with a loopstart field "items" carrying summary_field "name"
    Then validation reports no errors

  # ── Field-type registry surface ───────────────────────────────────

  Scenario: AllFieldTypes exposes a stable, populated registry
    When I read the field-type registry
    Then the registry contains "text"
    And the registry contains "list"
    And the registry contains "table"
    And the registry contains "loopstart"
    And the registry first id is "text"
    And the registry size is 21

  # ── Collapsible YAML round-trip ───────────────────────────────────

  Scenario: Collapsible true survives YAML round-trip
    Given a template with one list field "li" with collapsible true
    When I marshal the template and reload it
    Then the loaded field "li" has collapsible true

  Scenario: Collapsible absent stays absent
    Given a template with fields:
      | key | type |
      | t   | text |
    When I marshal the template and reload it
    Then the marshaled YAML does not contain "collapsible"
