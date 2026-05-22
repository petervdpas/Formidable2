Feature: Formidable URL navigation
  The nav module owns formidable:// URL routing - parsing,
  validation against the live template + storage trees, and
  translating "follow this link" into a config-state change plus a
  nav:changed event the frontend listens for.

  Background:
    Given a real system + template + storage stack
    And a template "basic.yaml" exists with a field "title" of type "text"
    And a form "test.meta.json" saved under "basic.yaml" with title "Hello"

  Scenario: Parse a well-formed URL
    When I parse "formidable://basic.yaml:test.meta.json"
    Then the parsed target template is "basic.yaml"
    And the parsed target datafile is "test.meta.json"
    And the parsed target fragment is empty

  Scenario: Parse with a fragment
    When I parse "formidable://basic.yaml:test.meta.json#section"
    Then the parsed target fragment is "section"

  Scenario: Parse rejects path traversal
    When I parse "formidable://../escape:test.meta.json"
    Then the parse returns nil

  Scenario: Parse rejects path separators
    When I parse "formidable://sub/basic.yaml:test.meta.json"
    Then the parse returns nil

  Scenario: Parse rejects an empty datafile
    When I parse "formidable://basic.yaml:"
    Then the parse returns nil

  Scenario: NavigateToFormidable updates config and emits an event on a valid URL
    When I navigate to "formidable://basic.yaml:test.meta.json"
    Then the navigation succeeds
    And the config reflects template "basic.yaml" and datafile "test.meta.json"
    And the config ribbon is "storage"
    And a "nav:changed" event was emitted with template "basic.yaml" and datafile "test.meta.json"

  Scenario: NavigateToFormidable rejects an unknown template
    When I navigate to "formidable://ghost.yaml:test.meta.json"
    Then the navigation fails with a non-empty error
    And no config update was made
    And no "nav:changed" event was emitted

  Scenario: NavigateToFormidable rejects a missing datafile
    When I navigate to "formidable://basic.yaml:does-not-exist.meta.json"
    Then the navigation fails with a non-empty error
    And no config update was made

  Scenario: NavigateToFormidable rejects a malformed URL
    When I navigate to "https://example.com/not-formidable"
    Then the navigation fails with a non-empty error
    And no config update was made
    And no "nav:changed" event was emitted

  Scenario: ResolveFormidable validates without mutating state
    When I resolve "formidable://basic.yaml:test.meta.json"
    Then the resolution succeeds
    And no config update was made
    And no "nav:changed" event was emitted
