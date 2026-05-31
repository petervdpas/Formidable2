Feature: Scaling (weighted measures)
  A scaling is a reusable named weighting. A statistic references it through a
  `scale "<name>"` clause; the engine then weights each count()/records()
  contribution by a per-form factor drawn from a facet, instead of adding 1.
  See design/statistics-scaling.md.

  Background:
    Given the SAMPLE records:
      | filename     | flag       | qzm          | apps      |
      | r1.meta.json | IN OMLOOP | NIET ZONNIG | QMU,QMU   |
      | r2.meta.json | IN OMLOOP | ZONNIG      | QMU       |
      | r3.meta.json | IN OMLOOP |               | Bladework |
    And a scaling "qzm-urgency" over facet "qzm" with default factor "1":
      | option        | factor |
      | ZONNIG      | 0.5    |
      | NIET ZONNIG | 2      |

  Scenario: records() + scale sums one factor per distinct form
    Given a statistic "apps":
      """
      records() by F["components"]["item"] scale "qzm-urgency"
      """
    When I evaluate the statistic "apps"
    Then evaluation succeeds
    And application "QMU" weighs "2.5"

  Scenario: count() + scale sums one factor per row
    Given a statistic "apps":
      """
      count() by F["components"]["item"] scale "qzm-urgency"
      """
    When I evaluate the statistic "apps"
    Then evaluation succeeds
    And application "QMU" weighs "4.5"

  Scenario: A form with the facet unset falls to the default factor
    Given a statistic "apps":
      """
      records() by F["components"]["item"] scale "qzm-urgency"
      """
    When I evaluate the statistic "apps"
    Then evaluation succeeds
    And application "Bladework" weighs "1"

  Scenario: Without a scale clause records() counts distinct forms
    Given a statistic "apps-plain":
      """
      records() by F["components"]["item"]
      """
    When I evaluate the statistic "apps-plain"
    Then evaluation succeeds
    And application "QMU" weighs "2"
