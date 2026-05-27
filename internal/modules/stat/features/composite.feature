Feature: Composite statistical objects (hop routes)
  A composite drills one branch of a rank-1 parent into a child statistic,
  leaving sibling branches as solid leaves. A child may only drill a branch it
  filters the parent base to; a drilled child honors its own scale clause,
  exactly as it does standalone. See design/statistics-composite.md.

  Background:
    Given the ODS records:
      | filename     | flag            | fcdm          | apps    |
      | r1.meta.json | IN GEBRUIK      | NIET AANWEZIG | FMU     |
      | r2.meta.json | IN GEBRUIK      | AANWEZIG      | FMU     |
      | r3.meta.json | NIET IN GEBRUIK | AANWEZIG      | FMU     |
    And a scaling "fcdm-urgency" over facet "fcdm" with default factor "1":
      | option        | factor |
      | AANWEZIG      | 0.5    |
      | NIET AANWEZIG | 2      |
    And a statistic "in-use":
      """
      count() by Facet["flag"]
      """

  Scenario: A parent branch drills into a child, the sibling stays a leaf
    Given a statistic "apps":
      """
      records() by F["code-repositories"]["application"] where Facet["flag"] eq "IN GEBRUIK"
      """
    And a composite "in-use-by-app" drills "in-use" branch "IN GEBRUIK" into "apps"
    When I evaluate the composite "in-use-by-app"
    Then evaluation succeeds
    And in branch "IN GEBRUIK" application "FMU" weighs "2"
    And branch "NIET IN GEBRUIK" is a solid leaf

  Scenario: A drilled child honors its own scale clause
    Given a statistic "apps":
      """
      records() by F["code-repositories"]["application"] where Facet["flag"] eq "IN GEBRUIK" scale "fcdm-urgency"
      """
    And a composite "in-use-by-app" drills "in-use" branch "IN GEBRUIK" into "apps"
    When I evaluate the composite "in-use-by-app"
    Then evaluation succeeds
    And in branch "IN GEBRUIK" application "FMU" weighs "2.5"

  Scenario: A child that does not filter the base to the branch is rejected
    Given a statistic "apps-unscoped":
      """
      records() by F["code-repositories"]["application"]
      """
    And a composite "bad" drills "in-use" branch "IN GEBRUIK" into "apps-unscoped"
    When I evaluate the composite "bad"
    Then evaluation fails

  Scenario: A branch that is not a parent category is rejected
    Given a statistic "apps":
      """
      records() by F["code-repositories"]["application"] where Facet["flag"] eq "GHOST"
      """
    And a composite "ghosted" drills "in-use" branch "GHOST" into "apps"
    When I evaluate the composite "ghosted"
    Then evaluation fails

  Scenario: A multi-dimension parent is rejected
    Given a statistic "two-dim":
      """
      count() by Facet["flag"], Facet["fcdm"]
      """
    And a statistic "apps":
      """
      records() by F["code-repositories"]["application"] where Facet["flag"] eq "IN GEBRUIK"
      """
    And a composite "wide" drills "two-dim" branch "IN GEBRUIK" into "apps"
    When I evaluate the composite "wide"
    Then evaluation fails
