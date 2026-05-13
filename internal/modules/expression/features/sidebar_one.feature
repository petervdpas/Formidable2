Feature: Per-record sidebar evaluation (EvaluateSidebarOne)
  The Storage workspace's list items refresh themselves after a save
  by asking the backend to re-evaluate just their own sidebar
  expression, instead of triggering a full-list pass that would
  thrash the sidebar scroll. Each scenario locks in one rule of that
  contract.

  Scenario: Evaluates one record by datafile
    Given a template with sidebar expression "name" and expression field "name"
    And records:
      | filename | title | name  |
      | a.json   | A     | alpha |
      | b.json   | B     | bravo |
    When I evaluate the sidebar for "b.json"
    Then the result filename is "b.json"
    And the result text is "bravo"
    And there is no result error

  Scenario: Missing record returns an empty SidebarItem (not an error)
    Given a template with sidebar expression "name" and expression field "name"
    And records:
      | filename | title | name  |
      | a.json   | A     | alpha |
    When I evaluate the sidebar for "nope.json"
    Then the result filename is ""
    And the result text is ""
    And there is no result error

  Scenario: Template without sidebar expression returns ErrNoExpression
    Given a template with no sidebar expression
    When I evaluate the sidebar for "a.json"
    Then the call returned ErrNoExpression

  Scenario: Empty expression result falls back to record title
    Given a template with sidebar expression "name == 'alpha' ? '' : name" and expression field "name"
    And records:
      | filename | title    | name  |
      | a.json   | Title-A  | alpha |
    When I evaluate the sidebar for "a.json"
    Then the result text is "Title-A"

  Scenario: Runtime expression error is captured in SidebarItem.Error
    Given a template with sidebar expression "missing.field" and expression field "name"
    And records:
      | filename | title    | name  |
      | a.json   | Title-A  | alpha |
    When I evaluate the sidebar for "a.json"
    Then the result error is non-empty
    And the result classes contain "expr-error"
    And the result text is "Title-A"
