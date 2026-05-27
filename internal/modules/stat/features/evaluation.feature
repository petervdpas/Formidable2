Feature: Statistical evaluation over the index
  The engine aggregates real records into a grid: counts and distinct-form
  records, numeric reduces, where-filters, top-N capping, and percentages.

  Background:
    Given the ODS records:
      | filename     | flag            | fcdm     | apps    | score |
      | r1.meta.json | IN GEBRUIK      | AANWEZIG | FMU,FMU | 10    |
      | r2.meta.json | IN GEBRUIK      | AANWEZIG | FMU     | 20    |
      | r3.meta.json | NIET IN GEBRUIK | AANWEZIG | Gradework | 30  |
      | r4.meta.json | NIET IN GEBRUIK | AANWEZIG | Gradework | 40  |

  Scenario: count() by a facet tallies records per category
    Given a statistic "by-flag":
      """
      count() by Facet["flag"]
      """
    When I evaluate the statistic "by-flag"
    Then evaluation succeeds
    And category "IN GEBRUIK", measure 0 is "2"
    And category "NIET IN GEBRUIK", measure 0 is "2"

  Scenario: count() and records() diverge on a fanned-out table column
    Given a statistic "apps":
      """
      count(), records() by F["code-repositories"]["application"] where Facet["flag"] eq "IN GEBRUIK"
      """
    When I evaluate the statistic "apps"
    Then evaluation succeeds
    And category "FMU", measure 0 is "3"
    And category "FMU", measure 1 is "2"

  Scenario: A reduce sums and averages a numeric field per category
    Given a statistic "score-sum":
      """
      sum(F["score"]), avg(F["score"]) by Facet["flag"]
      """
    When I evaluate the statistic "score-sum"
    Then evaluation succeeds
    And category "IN GEBRUIK", measure 0 is "30"
    And category "IN GEBRUIK", measure 1 is "15"
    And category "NIET IN GEBRUIK", measure 0 is "70"
    And category "NIET IN GEBRUIK", measure 1 is "35"

  Scenario: A numeric comparison filter scopes rows before grouping
    Given a statistic "heavy":
      """
      count() by Facet["flag"] where F["score"] gt 15
      """
    When I evaluate the statistic "heavy"
    Then evaluation succeeds
    And category "IN GEBRUIK", measure 0 is "1"
    And category "NIET IN GEBRUIK", measure 0 is "2"

  Scenario: top-N keeps the biggest categories and drops the tail
    Given a statistic "top-app":
      """
      count() by F["code-repositories"]["application"] top 1
      """
    When I evaluate the statistic "top-app"
    Then evaluation succeeds
    And the grid has 1 categories
    And category "FMU" is present
    And category "Gradework" is absent

  Scenario: Percentage base distribution divides by the grid total
    Given a statistic "app-share":
      """
      count() by F["code-repositories"]["application"]
      """
    When I evaluate the statistic "app-share"
    Then evaluation succeeds
    And category "FMU", measure 0 is "60" percent
    And category "Gradework", measure 0 is "40" percent

  Scenario: Percentage base forms divides by the record count
    Given a statistic "app-of-forms":
      """
      count() by F["code-repositories"]["application"] pct forms
      """
    When I evaluate the statistic "app-of-forms"
    Then evaluation succeeds
    And category "FMU", measure 0 is "75" percent
    And category "Gradework", measure 0 is "50" percent
