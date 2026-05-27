Feature: Percentage base under scaling
  When a measure is weighted, a percentage must keep its numerator and
  denominator in the same currency. The `forms` base divides the weighted cell
  value by the weighted form total (the sum of every form's factor), never the
  raw form count. Mixing the two produced percentages over 100% (e.g. 153%),
  which is the bug this guards.

  Background:
    Given the ODS records:
      | filename     | fcdm          | apps      |
      | r1.meta.json | NIET AANWEZIG | FMU       |
      | r2.meta.json | NIET AANWEZIG | FMU       |
      | r3.meta.json | AANWEZIG      | Gradework |
      | r4.meta.json | AANWEZIG      | Gradework |
    And a scaling "fcdm-urgency" over facet "fcdm" with default factor "1":
      | option        | factor |
      | AANWEZIG      | 1      |
      | NIET AANWEZIG | 4      |

  Scenario: pct forms under scaling divides by the weighted form total
    Given a statistic "apps":
      """
      records() by F["code-repositories"]["application"] scale "fcdm-urgency" pct forms
      """
    When I evaluate the statistic "apps"
    Then evaluation succeeds
    And application "FMU" weighs "8"
    And application "Gradework" weighs "2"
    And category "FMU", measure 0 is "80" percent
    And category "Gradework", measure 0 is "20" percent

  Scenario: pct distribution under scaling sums weighted cells to 100 percent
    Given a statistic "apps-dist":
      """
      records() by F["code-repositories"]["application"] scale "fcdm-urgency"
      """
    When I evaluate the statistic "apps-dist"
    Then evaluation succeeds
    And category "FMU", measure 0 is "80" percent
    And category "Gradework", measure 0 is "20" percent

  Scenario: Without scaling pct forms divides by the raw form count
    Given a statistic "apps-plain":
      """
      records() by F["code-repositories"]["application"] pct forms
      """
    When I evaluate the statistic "apps-plain"
    Then evaluation succeeds
    And application "FMU" weighs "2"
    And category "FMU", measure 0 is "50" percent
