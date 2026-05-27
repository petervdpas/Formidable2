Feature: CSV export schema and row building
  The backend owns the export rules: which field types are exportable,
  which fields are alignable, and how an aligned table expands into one
  column per sub-field. The dialog only renders the schema it is handed.

  Background:
    Given a system manager rooted at a temp directory
    And a csv manager with template and storage deps

  Scenario: No alignment yields one column per mappable field, excluded types dropped
    Given the template "t.yaml" has fields:
      | key  | type  | label | options |
      | id   | guid  | ID    |         |
      | name | text  | Name  |         |
      | note | code  | Note  |         |
    When I request the export schema for "t.yaml" with no alignment
    Then the default columns are "id,name"
    And the echoed align source is ""

  Scenario: Aligning on a table expands it into one column per sub-field
    Given the template "audit.yaml" has fields:
      | key  | type  | label | options                       |
      | code | text  | Code  |                               |
      | gap  | table | GAP   | part:Onderdeel,act:Actie      |
    When I request the export schema for "audit.yaml" aligned on "gap"
    Then the default columns are "code,gap.part,gap.act"
    And the echoed align source is "gap"
    And the alignable fields are "gap"
    And the source options include "gap.part"
    And the source options include "gap.act"

  Scenario: Source subkeys only appear when a table is the align target
    Given the template "audit.yaml" has fields:
      | key  | type  | label | options                  |
      | code | text  | Code  |                          |
      | gap  | table | GAP   | part:Onderdeel,act:Actie |
    When I request the export schema for "audit.yaml" with no alignment
    Then the source options do not include "gap.part"

  Scenario: A list is alignable but never expands into sub-columns
    Given the template "t.yaml" has fields:
      | key  | type | label | options |
      | code | text | Code  |         |
      | tags | list | Tags  |         |
    When I request the export schema for "t.yaml" aligned on "tags"
    Then the default columns are "code,tags"
    And the alignable fields are "tags"

  Scenario: Aligning on a table emits one flat row per table item
    Given the template "audit.yaml" has fields:
      | key  | type  | label | options                  |
      | code | text  | Code  |                          |
      | gap  | table | GAP   | part:Onderdeel,act:Actie |
    And the form "audit.yaml" has data:
      | field | value                                        |
      | code  | CH.02                                         |
      | gap   | [["Governance","DOR"],["CIB","Workflow"]]    |
    When I export "audit.yaml" aligned on "gap" with columns "code,gap.part,gap.act"
    Then the export has 2 data rows
    And the export header is "code|gap.part|gap.act"
    And data row 1 is "CH.02|Governance|DOR"
    And data row 2 is "CH.02|CIB|Workflow"

  Scenario: A form with an empty table still emits one row
    Given the template "audit.yaml" has fields:
      | key  | type  | label | options                  |
      | code | text  | Code  |                          |
      | gap  | table | GAP   | part:Onderdeel,act:Actie |
    And the form "audit.yaml" has data:
      | field | value |
      | code  | CH.09 |
      | gap   | []    |
    When I export "audit.yaml" aligned on "gap" with columns "code,gap.part,gap.act"
    Then the export has 1 data rows
    And data row 1 is "CH.09||"

  Scenario: Aligning on a list emits one row per item
    Given the template "t.yaml" has fields:
      | key  | type | label | options |
      | code | text | Code  |         |
      | tags | list | Tags  |         |
    And the form "t.yaml" has data:
      | field | value           |
      | code  | A1              |
      | tags  | ["red","green"] |
    When I export "t.yaml" aligned on "tags" with columns "code,tags"
    Then the export has 2 data rows
    And data row 1 is "A1|red"
    And data row 2 is "A1|green"

  Scenario: Unknown align source is ignored, no expansion
    Given the template "t.yaml" has fields:
      | key  | type | label | options |
      | code | text | Code  |         |
    When I request the export schema for "t.yaml" aligned on "nope"
    Then the echoed align source is ""
    And the default columns are "code"

  Scenario: Schema reports an error when the template dependency is missing
    Given a csv manager with no template dep
    When I request the export schema for "t.yaml" with no alignment
    Then the schema reports an error
