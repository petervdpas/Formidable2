Feature: Statistical evaluation over the index
  The engine aggregates real records into a grid: counts and distinct-form
  records, numeric reduces, where-filters, top-N capping, and percentages.

  Background:
    Given the SAMPLE records:
      | filename     | flag            | qzm     | apps    | score |
      | r1.meta.json | IN OMLOOP      | ZONNIG | QMU,QMU | 10    |
      | r2.meta.json | IN OMLOOP      | ZONNIG | QMU     | 20    |
      | r3.meta.json | NIET IN OMLOOP | ZONNIG | Bladework | 30  |
      | r4.meta.json | NIET IN OMLOOP | ZONNIG | Bladework | 40  |

  Scenario: count() by a facet tallies records per category
    Given a statistic "by-flag":
      """
      count() by Facet["flag"]
      """
    When I evaluate the statistic "by-flag"
    Then evaluation succeeds
    And category "IN OMLOOP", measure 0 is "2"
    And category "NIET IN OMLOOP", measure 0 is "2"

  Scenario: count() and records() diverge on a fanned-out table column
    Given a statistic "apps":
      """
      count(), records() by F["components"]["item"] where Facet["flag"] eq "IN OMLOOP"
      """
    When I evaluate the statistic "apps"
    Then evaluation succeeds
    And category "QMU", measure 0 is "3"
    And category "QMU", measure 1 is "2"

  Scenario: A reduce sums and averages a numeric field per category
    Given a statistic "score-sum":
      """
      sum(F["score"]), avg(F["score"]) by Facet["flag"]
      """
    When I evaluate the statistic "score-sum"
    Then evaluation succeeds
    And category "IN OMLOOP", measure 0 is "30"
    And category "IN OMLOOP", measure 1 is "15"
    And category "NIET IN OMLOOP", measure 0 is "70"
    And category "NIET IN OMLOOP", measure 1 is "35"

  Scenario: A numeric comparison filter scopes rows before grouping
    Given a statistic "heavy":
      """
      count() by Facet["flag"] where F["score"] gt 15
      """
    When I evaluate the statistic "heavy"
    Then evaluation succeeds
    And category "IN OMLOOP", measure 0 is "1"
    And category "NIET IN OMLOOP", measure 0 is "2"

  Scenario: top-N keeps the biggest categories and drops the tail
    Given a statistic "top-app":
      """
      count() by F["components"]["item"] top 1
      """
    When I evaluate the statistic "top-app"
    Then evaluation succeeds
    And the grid has 1 categories
    And category "QMU" is present
    And category "Bladework" is absent

  Scenario: Percentage base distribution divides by the grid total
    Given a statistic "app-share":
      """
      count() by F["components"]["item"]
      """
    When I evaluate the statistic "app-share"
    Then evaluation succeeds
    And category "QMU", measure 0 is "60" percent
    And category "Bladework", measure 0 is "40" percent

  Scenario: Percentage base forms divides by the record count
    Given a statistic "app-of-forms":
      """
      count() by F["components"]["item"] pct forms
      """
    When I evaluate the statistic "app-of-forms"
    Then evaluation succeeds
    And category "QMU", measure 0 is "75" percent
    And category "Bladework", measure 0 is "50" percent
