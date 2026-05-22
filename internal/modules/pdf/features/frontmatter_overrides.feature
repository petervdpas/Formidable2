Feature: PDF Frontmatter - parse, merge, project to picoloom.Input
  The pdf module parses YAML frontmatter from a markdown document
  (post-raymond expansion in Stage 4) and merges four layers in
  priority order: document frontmatter > form meta > template
  manifest > global config. The merged Frontmatter is projected to
  a picoloom.Input via a pure BuildInput function. Malformed
  frontmatter is tolerated - the body always survives and the
  caller renders with defaults.

  Background:
    Given a fresh frontmatter test world

  # ---------- Parse ----------

  Scenario: Empty input yields empty Frontmatter and empty body
    When I parse the markdown ""
    Then the parse returned no error
    And the parsed body is empty
    And the parsed style is empty

  Scenario: Markdown without a leading delimiter has no frontmatter
    When I parse the markdown "# Just a title\n\ntext"
    Then the parse returned no error
    And the parsed body equals the original input
    And the parsed style is empty

  Scenario: Valid frontmatter sets style and strips block from body
    When I parse the markdown:
      """
      ---
      style: technical
      ---
      # Body
      """
    Then the parse returned no error
    And the parsed style is "technical"
    And the parsed body equals "# Body"

  Scenario: Missing closing delimiter is reported as malformed
    When I parse the markdown:
      """
      ---
      style: technical
      no close here
      """
    Then the parse returned a malformed-frontmatter error
    And the parsed body equals the original input
    And the parsed style is empty

  Scenario: Malformed YAML returns the body verbatim
    When I parse the markdown:
      """
      ---
      style: [unterminated
      ---
      # body
      """
    Then the parse returned a malformed-frontmatter error
    And the parsed body equals the original input

  Scenario: Unknown keys are silently ignored
    When I parse the markdown:
      """
      ---
      style: technical
      garbage_field: 42
      ---
      # body
      """
    Then the parse returned no error
    And the parsed style is "technical"

  # ---------- Merge priority ----------

  Scenario: Highest-priority layer wins on a scalar
    Given a frontmatter layer "doc" with style "technical"
    And a frontmatter layer "global" with style "default"
    When I merge layers in order doc, global
    Then the merged style is "technical"

  Scenario: Lower layer fills in where higher is empty
    Given a frontmatter layer "doc" with cover title "From Doc"
    And a frontmatter layer "global" with cover author "From Global"
    When I merge layers in order doc, global
    Then the merged cover title is "From Doc"
    And the merged cover author is "From Global"

  Scenario: Middle layer wins where neither edge layer asserts a value
    Given a frontmatter layer "doc" with no opinions
    And a frontmatter layer "meta" with style "academic"
    And a frontmatter layer "global" with style "default"
    When I merge layers in order doc, meta, global
    Then the merged style is "academic"

  Scenario: Explicit false bool overrides lower true
    Given a frontmatter layer "doc" with cover enabled false
    And a frontmatter layer "global" with cover enabled true
    When I merge layers in order doc, global
    Then the merged cover enabled is false

  # ---------- BuildInput ----------

  Scenario: Empty Frontmatter projects to a minimal picoloom.Input
    Given an empty merged frontmatter
    When I build the picoloom.Input with body "# Hello"
    Then the Input markdown equals "# Hello"
    And the Input has no Cover block
    And the Input has no TOC block
    And the Input has no Watermark block

  Scenario: Cover with Enabled=false is omitted from picoloom.Input
    Given a merged frontmatter with cover enabled false and title "ignored"
    When I build the picoloom.Input with body "body"
    Then the Input has no Cover block

  Scenario: Cover block present without explicit enabled defaults to on
    Given a merged frontmatter with cover title "T" and no explicit enabled
    When I build the picoloom.Input with body "body"
    Then the Input has a Cover block with title "T"
