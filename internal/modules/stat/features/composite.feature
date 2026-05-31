Feature: Composite statistical objects (hop routes)
  A composite drills one branch of a rank-1 parent into a child statistic,
  leaving sibling branches as solid leaves. A child may only drill a branch it
  filters the parent base to; a drilled child honors its own scale clause,
  exactly as it does standalone. See design/statistics-composite.md.

  Background:
    Given the SAMPLE records:
      | filename     | flag            | qzm          | apps    |
      | r1.meta.json | IN OMLOOP      | NIET ZONNIG | QMU     |
      | r2.meta.json | IN OMLOOP      | ZONNIG      | QMU     |
      | r3.meta.json | NIET IN OMLOOP | ZONNIG      | QMU     |
    And a scaling "qzm-urgency" over facet "qzm" with default factor "1":
      | option        | factor |
      | ZONNIG      | 0.5    |
      | NIET ZONNIG | 2      |
    And a statistic "in-use":
      """
      count() by Facet["flag"]
      """

  Scenario: A parent branch drills into a child, the sibling stays a leaf
    Given a statistic "apps":
      """
      records() by F["components"]["item"] where Facet["flag"] eq "IN OMLOOP"
      """
    And a composite "in-use-by-app" drills "in-use" branch "IN OMLOOP" into "apps"
    When I evaluate the composite "in-use-by-app"
    Then evaluation succeeds
    And in branch "IN OMLOOP" application "QMU" weighs "2"
    And branch "NIET IN OMLOOP" is a solid leaf

  Scenario: A drilled child honors its own scale clause
    Given a statistic "apps":
      """
      records() by F["components"]["item"] where Facet["flag"] eq "IN OMLOOP" scale "qzm-urgency"
      """
    And a composite "in-use-by-app" drills "in-use" branch "IN OMLOOP" into "apps"
    When I evaluate the composite "in-use-by-app"
    Then evaluation succeeds
    And in branch "IN OMLOOP" application "QMU" weighs "2.5"

  Scenario: A scaled parent weights both slices of the parent ring
    Given a statistic "in-use-weighted":
      """
      count() by Facet["flag"] scale "qzm-urgency"
      """
    And a statistic "apps":
      """
      records() by F["components"]["item"] where Facet["flag"] eq "IN OMLOOP" scale "qzm-urgency"
      """
    And a composite "weighted-by-app" drills "in-use-weighted" branch "IN OMLOOP" into "apps"
    When I evaluate the composite "weighted-by-app"
    Then evaluation succeeds
    And parent branch "IN OMLOOP" weighs "2.5"
    And parent branch "NIET IN OMLOOP" weighs "0.5"
    And in branch "IN OMLOOP" application "QMU" weighs "2.5"

  Scenario: A child that does not filter the base to the branch is rejected
    Given a statistic "apps-unscoped":
      """
      records() by F["components"]["item"]
      """
    And a composite "bad" drills "in-use" branch "IN OMLOOP" into "apps-unscoped"
    When I evaluate the composite "bad"
    Then evaluation fails

  Scenario: A branch that is not a parent category is rejected
    Given a statistic "apps":
      """
      records() by F["components"]["item"] where Facet["flag"] eq "GHOST"
      """
    And a composite "ghosted" drills "in-use" branch "GHOST" into "apps"
    When I evaluate the composite "ghosted"
    Then evaluation fails

  Scenario: A multi-dimension parent is rejected
    Given a statistic "two-dim":
      """
      count() by Facet["flag"], Facet["qzm"]
      """
    And a statistic "apps":
      """
      records() by F["components"]["item"] where Facet["flag"] eq "IN OMLOOP"
      """
    And a composite "wide" drills "two-dim" branch "IN OMLOOP" into "apps"
    When I evaluate the composite "wide"
    Then evaluation fails
