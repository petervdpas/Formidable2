Feature: Percentage base under scaling
  When a measure is weighted, a percentage must keep its numerator and
  denominator in the same currency. The `forms` base divides the weighted cell
  value by the weighted form total (the sum of every form's factor), never the
  raw form count. Mixing the two produced percentages over 100% (e.g. 153%),
  which is the bug this guards.

  Background:
    Given the SAMPLE records:
      | filename     | qzm          | apps      |
      | r1.meta.json | NIET ZONNIG | QMU       |
      | r2.meta.json | NIET ZONNIG | QMU       |
      | r3.meta.json | ZONNIG      | Bladework |
      | r4.meta.json | ZONNIG      | Bladework |
    And a scaling "qzm-urgency" over facet "qzm" with default factor "1":
      | option        | factor |
      | ZONNIG      | 1      |
      | NIET ZONNIG | 4      |

  Scenario: pct forms under scaling divides by the weighted form total
    Given a statistic "apps":
      """
      records() by F["components"]["item"] scale "qzm-urgency" pct forms
      """
    When I evaluate the statistic "apps"
    Then evaluation succeeds
    And application "QMU" weighs "8"
    And application "Bladework" weighs "2"
    And category "QMU", measure 0 is "80" percent
    And category "Bladework", measure 0 is "20" percent

  Scenario: pct distribution under scaling sums weighted cells to 100 percent
    Given a statistic "apps-dist":
      """
      records() by F["components"]["item"] scale "qzm-urgency"
      """
    When I evaluate the statistic "apps-dist"
    Then evaluation succeeds
    And category "QMU", measure 0 is "80" percent
    And category "Bladework", measure 0 is "20" percent

  Scenario: Without scaling pct forms divides by the raw form count
    Given a statistic "apps-plain":
      """
      records() by F["components"]["item"] pct forms
      """
    When I evaluate the statistic "apps-plain"
    Then evaluation succeeds
    And application "QMU" weighs "2"
    And category "QMU", measure 0 is "50" percent
