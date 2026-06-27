Feature: Form lifecycle
  The form module orchestrates a (template, datafile) pair: build a
  view (with type defaults), save user-edited values back through
  storage, list per-template summaries, and delete. End-to-end
  scenarios run against a real system + sfr + template + storage
  stack so JSON disk I/O and storage.Sanitize are exercised.

  Background:
    Given a real form stack on a temp directory

  Scenario: BuildView with no datafile returns an unsaved view with defaults
    Given a template "basic.yaml" with a text field "title"
    When I build the view for template "basic.yaml" with no datafile
    Then the view is unsaved
    And the view value "title" is ""

  Scenario: SaveValues round-trips a text field
    Given a template "basic.yaml" with a text field "title"
    When I save form "self.meta.json" under "basic.yaml" with values:
      | key   | value |
      | title | Hello |
    Then the saved view value "title" is "Hello"
    And reopening "self.meta.json" yields title "Hello"

  Scenario: SaveValues round-trips a link object
    Given a template "links.yaml" with a link field "ref"
    When I save form "self.meta.json" under "links.yaml" with link "ref" of href "formidable://other.yaml:o.meta.json" and text "Go there"
    Then the saved link "ref" has href "formidable://other.yaml:o.meta.json"
    And the saved link "ref" has text "Go there"

  Scenario: SaveValues round-trips a date string
    Given a template "diary.yaml" with a date field "when"
    When I save form "self.meta.json" under "diary.yaml" with values:
      | key  | value      |
      | when | 2026-05-06 |
    Then the saved view value "when" is "2026-05-06"

  Scenario: BuildView preserves loop entries
    Given a template "loops.yaml" with a loop "items" containing field "name" of type "text"
    When I save form "self.meta.json" under "loops.yaml" with loop "items" entries:
      | name |
      | a    |
      | b    |
    Then reopening "self.meta.json" yields loop "items" with 2 entries
    And loop "items" entry 0 has name "a"
    And loop "items" entry 1 has name "b"

  Scenario: SaveValues without a datafile is rejected
    Given a template "basic.yaml" with a text field "title"
    When I save form "" under "basic.yaml" with values:
      | key   | value |
      | title | x     |
    Then the save returns an error

  Scenario: DeleteForm removes the form
    Given a template "basic.yaml" with a text field "title"
    When I save form "to-delete.meta.json" under "basic.yaml" with values:
      | key   | value |
      | title | bye   |
    And I delete form "to-delete.meta.json" under "basic.yaml"
    Then reopening "to-delete.meta.json" returns an unsaved view

  Scenario: Copy creates a new record with a fresh id but identical contents
    Given a template "copyable.yaml" with a guid field "id" and a text field "title"
    When I save form "orig.meta.json" under "copyable.yaml" with values:
      | key   | value    |
      | title | Original |
    And I copy form "orig.meta.json" to "dup.meta.json" under "copyable.yaml"
    Then the copy has a fresh id
    And the copy value "title" is "Original"
    And the original "orig.meta.json" under "copyable.yaml" keeps its id

  Scenario: ListForms returns an entry per saved form
    Given a template "basic.yaml" with a text field "title"
    When I save form "one.meta.json" under "basic.yaml" with values:
      | key   | value |
      | title | One   |
    And I save form "two.meta.json" under "basic.yaml" with values:
      | key   | value |
      | title | Two   |
    Then listing forms under "basic.yaml" yields 2 entries
