Feature: Cleanup corrects facet meta on disk
  Migrate Meta seeds a facet field's default into forms that predate the field
  and writes that correction to disk, while never overwriting a real choice or
  resurrecting an explicitly cleared facet. The Then steps inspect the raw file
  on disk, not a sanitized load, so a seeded-only-in-memory default cannot pass.

  Background:
    Given a system manager rooted at a temp directory
    And a storage manager wrapping that system
    And a template "gaps" with facet "status" options "OPEN,CLOSED" and a facet field defaulting to "OPEN"

  Scenario: a form missing the facet gets the default written to disk
    Given a raw form file "gaps.yaml" / "f1" on disk with no facets
    When I run meta migration for "gaps.yaml"
    Then the migration migrated 1 and skipped 0
    And the form file "gaps.yaml" / "f1" on disk has facet "status" selected "OPEN"

  Scenario: an explicit selection is never overwritten by the default
    Given a raw form file "gaps.yaml" / "f2" on disk with facet "status" selected "CLOSED"
    When I run meta migration for "gaps.yaml"
    Then the migration migrated 0 and skipped 1
    And the form file "gaps.yaml" / "f2" on disk has facet "status" selected "CLOSED"

  Scenario: an explicitly cleared facet is not re-seeded
    Given a raw form file "gaps.yaml" / "f3" on disk with facet "status" cleared
    When I run meta migration for "gaps.yaml"
    Then the migration migrated 0 and skipped 1
    And the form file "gaps.yaml" / "f3" on disk has facet "status" cleared
