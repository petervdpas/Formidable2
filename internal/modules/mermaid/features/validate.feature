Feature: Mermaid source validation
  Validate parses Mermaid source, reports a canonical diagram type, and surfaces
  positioned issues. Empty source is OK with no type.

  # ── Happy paths ───────────────────────────────────────────────────

  Scenario: A flowchart validates and reports its type
    Given the Mermaid source:
      """
      flowchart TD
        A-->B
      """
    When I validate it
    Then validation succeeds
    And the diagram type is "flowchart"
    And there are no issues

  Scenario: graph syntax is canonicalised to flowchart
    Given the Mermaid source:
      """
      graph LR
        A-->B
      """
    When I validate it
    Then validation succeeds
    And the diagram type is "flowchart"

  Scenario: stateDiagram-v2 is canonicalised to state
    Given the Mermaid source:
      """
      stateDiagram-v2
        [*] --> S
      """
    When I validate it
    Then validation succeeds
    And the diagram type is "state"

  Scenario: A gantt chart validates
    Given the Mermaid source:
      """
      gantt
        title A
        dateFormat YYYY-MM-DD
        section S
        Task: a1, 2014-01-01, 30d
      """
    When I validate it
    Then validation succeeds
    And the diagram type is "gantt"

  Scenario: A sequence diagram validates
    Given the Mermaid source:
      """
      sequenceDiagram
        Alice->>Bob: hi
      """
    When I validate it
    Then validation succeeds
    And the diagram type is "sequence"

  Scenario: Leading frontmatter is stripped, not rejected
    Given the Mermaid source:
      """
      ---
      title: X
      ---
      gantt
        title A
        dateFormat YYYY-MM-DD
        section S
        Task: a1, 2014-01-01, 30d
      """
    When I validate it
    Then validation succeeds
    And the diagram type is "gantt"

  Scenario: Leading comment lines are skipped
    Given the Mermaid source:
      """
      %% a note
      sequenceDiagram
        Alice->>Bob: hi
      """
    When I validate it
    Then validation succeeds
    And the diagram type is "sequence"

  # ── Boundary ──────────────────────────────────────────────────────

  Scenario: Empty source is OK with no type
    Given empty Mermaid source
    When I validate it
    Then validation succeeds
    And the diagram type is empty
    And there are no issues

  Scenario: Whitespace-only source is OK with no type
    Given whitespace-only Mermaid source
    When I validate it
    Then validation succeeds
    And the diagram type is empty
    And there are no issues

  Scenario: Frontmatter with no diagram after it is OK with no type
    Given the Mermaid source:
      """
      ---
      title: X
      ---
      """
    When I validate it
    Then validation succeeds
    And the diagram type is empty

  # ── Unhappy paths ─────────────────────────────────────────────────

  Scenario: An unknown diagram type is a parse error
    Given the Mermaid source:
      """
      bananachart
        x
      """
    When I validate it
    Then validation fails
    And the diagram type is empty
    And there is 1 issue
    And issue 1 has code "parse_error"
    And issue 1 has severity "error"

  Scenario: A bad value reports the offending line
    Given the Mermaid source:
      """
      journey
        title T
        section S
        Task: 9: Me
      """
    When I validate it
    Then validation fails
    And there is 1 issue
    And issue 1 is at line 4
    And issue 1 message contains "score"

  Scenario: A malformed pie entry reports its line
    Given the Mermaid source:
      """
      pie
        "A" : notanumber
      """
    When I validate it
    Then validation fails
    And the diagram type is empty
    And there is 1 issue
    And issue 1 has code "parse_error"
    And issue 1 is at line 2
    And issue 1 message contains "invalid pie entry"

  Scenario: A pie chart with no data entries fails
    Given the Mermaid source:
      """
      pie
      """
    When I validate it
    Then validation fails
    And issue 1 message contains "at least one data entry"

  Scenario: Unclosed frontmatter is not stripped and fails to parse
    Given the Mermaid source:
      """
      ---
      title: X
      flowchart TD
        A-->B
      """
    When I validate it
    Then validation fails
    And issue 1 has code "parse_error"
