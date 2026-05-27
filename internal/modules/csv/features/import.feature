Feature: CSV import mapping and coercion
  The import flow maps CSV headers onto template fields and coerces each
  cell into the field's typed value. The backend owns which fields are
  mappable (excluded types stripped) so the import dialog never re-derives
  that rule on the client.

  Background:
    Given a system manager rooted at a temp directory
    And a csv manager with template and storage deps

  Scenario: Mappable fields strip excluded types, sourced from the template
    Given the template "people.yaml" has fields:
      | key   | type      | label |
      | id    | guid      | ID    |
      | name  | text      | Name  |
      | bio   | code      | Bio   |
      | start | loopstart | Loop  |
      | role  | text      | Role  |
      | stop  | loopstop  | Loop  |
    When I request mappable fields for "people.yaml"
    Then the mappable field keys are "id,name,role"

  Scenario: Header names auto-map to matching field keys
    Given the template "people.yaml" has fields:
      | key  | type | label |
      | name | text | Name  |
      | age  | text | Age   |
    When I request mappable fields for "people.yaml"
    And I suggest mappings for headers "name,age"
    Then header "name" maps to field "name"
    And header "age" maps to field "age"

  Scenario: An unmatched header maps to nothing
    Given the template "people.yaml" has fields:
      | key  | type | label |
      | name | text | Name  |
    When I request mappable fields for "people.yaml"
    And I suggest mappings for headers "name,unknown"
    Then header "name" maps to field "name"
    And header "unknown" maps to nothing

  Scenario: Transform rules are applied to a cell
    When I apply transform "uppercase" param "" to "hello"
    Then the transformed value is "HELLO"

  Scenario: Trim transform strips surrounding whitespace
    When I apply transform "trim" param "" to "  spaced  "
    Then the transformed value is "spaced"

  Scenario: Coerce a number cell
    When I coerce "7" as "number"
    Then the coerced value is "7"

  Scenario: Coerce a boolean cell
    When I coerce "yes" as "boolean"
    Then the coerced value is "true"

  Scenario: Coerce a tags cell into a list
    When I coerce "red; green; blue" as "tags"
    Then the coerced list is "red,green,blue"
