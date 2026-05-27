Feature: Scaling (weighted measures)
  A scaling is a reusable named weighting. A statistic references it through a
  `scale "<name>"` clause; the engine then weights each count()/records()
  contribution by a per-form factor drawn from a facet, instead of adding 1.
  See design/statistics-scaling.md.

  Background:
    Given the ODS records:
      | filename     | flag       | fcdm          | apps      |
      | r1.meta.json | IN GEBRUIK | NIET AANWEZIG | FMU,FMU   |
      | r2.meta.json | IN GEBRUIK | AANWEZIG      | FMU       |
      | r3.meta.json | IN GEBRUIK |               | Gradework |
    And a scaling "fcdm-urgency" over facet "fcdm" with default factor "1":
      | option        | factor |
      | AANWEZIG      | 0.5    |
      | NIET AANWEZIG | 2      |

  Scenario: records() + scale sums one factor per distinct form
    Given a statistic "apps":
      """
      records() by F["code-repositories"]["application"] scale "fcdm-urgency"
      """
    When I evaluate the statistic "apps"
    Then evaluation succeeds
    And application "FMU" weighs "2.5"

  Scenario: count() + scale sums one factor per row
    Given a statistic "apps":
      """
      count() by F["code-repositories"]["application"] scale "fcdm-urgency"
      """
    When I evaluate the statistic "apps"
    Then evaluation succeeds
    And application "FMU" weighs "4.5"

  Scenario: A form with the facet unset falls to the default factor
    Given a statistic "apps":
      """
      records() by F["code-repositories"]["application"] scale "fcdm-urgency"
      """
    When I evaluate the statistic "apps"
    Then evaluation succeeds
    And application "Gradework" weighs "1"

  Scenario: Without a scale clause records() counts distinct forms
    Given a statistic "apps-plain":
      """
      records() by F["code-repositories"]["application"]
      """
    When I evaluate the statistic "apps-plain"
    Then evaluation succeeds
    And application "FMU" weighs "2"
