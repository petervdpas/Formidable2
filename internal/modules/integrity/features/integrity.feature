Feature: Storage integrity analysis
  The integrity module audits a template's stored forms against the
  current field declarations. Phase 1 (this feature) is analyze-only:
  AnalyzeTemplate produces a Report listing every drift it finds -
  missing/extra fields, type mismatches, bad date formats, meta-block
  problems, and unreadable files - without modifying anything on disk.

  Background:
    Given a template "basic.yaml" with fields:
      | key    | type    |
      | title  | text    |
      | due    | date    |
      | active | boolean |
      | count  | number  |

  Scenario: A clean form yields zero issues
    Given a form "a.meta.json" with data:
      | key    | value      |
      | title  | hello      |
      | due    | 2026-06-01 |
      | active | true       |
      | count  | 7          |
    When I analyze "basic.yaml"
    Then the report has 1 form scanned
    And the report has 0 issues

  Scenario: Missing top-level field is reported
    Given a form "a.meta.json" with data:
      | key    | value      |
      | title  | hello      |
      | due    | 2026-06-01 |
      | active | true       |
    When I analyze "basic.yaml"
    Then the report has a "missing_field" issue at "count" on "a.meta.json"

  Scenario: Extra/orphan field is reported
    Given a form "a.meta.json" with data:
      | key    | value      |
      | title  | hello      |
      | due    | 2026-06-01 |
      | active | true       |
      | count  | 7          |
      | zombie | x          |
    When I analyze "basic.yaml"
    Then the report has an "extra_field" issue at "zombie" on "a.meta.json"

  Scenario: Wrong type is a type mismatch
    Given a form "a.meta.json" with data:
      | key    | value |
      | title  | hello |
      | active | yes   |
    When I analyze "basic.yaml"
    Then the report has a "type_mismatch" issue at "active" on "a.meta.json"

  Scenario: Bad date format is its own issue kind
    Given a form "a.meta.json" with data:
      | key   | value      |
      | title | hello      |
      | due   | 21/07/2025 |
    When I analyze "basic.yaml"
    Then the report has a "bad_date_format" issue at "due" on "a.meta.json"

  Scenario: Unreadable form contributes a single issue
    Given an unreadable form "broken.meta.json"
    When I analyze "basic.yaml"
    Then the report has an "unreadable" issue on "broken.meta.json"
    And the form "broken.meta.json" has exactly 1 issue

  Scenario: Unknown template returns an error
    When I analyze "ghost.yaml"
    Then an integrity error occurred

  # A guid field is the form's identity source: meta.id must mirror it,
  # and an empty field is backfilled from meta.id. The cleanup tool flags
  # the drift (guid_unsynced) and repairs it via sync_guid.

  Scenario: A guid field matching meta.id is clean
    Given a template "ids.yaml" with fields:
      | key | type |
      | id  | guid |
    And a form "a.meta.json" with meta id "G-1" and data:
      | key | value |
      | id  | G-1   |
    When I analyze "ids.yaml"
    Then the report has 0 issues

  Scenario: An empty guid field while meta.id is set is drift
    Given a template "ids.yaml" with fields:
      | key | type |
      | id  | guid |
    And a form "a.meta.json" with meta id "G-9" and data:
      | key | value |
      | id  |       |
    When I analyze "ids.yaml"
    Then the report has a "guid_unsynced" issue at "id" on "a.meta.json"

  Scenario: Repairing an empty guid field backfills it from meta.id
    Given a template "ids.yaml" with fields:
      | key | type |
      | id  | guid |
    And a form "a.meta.json" with meta id "G-9" and data:
      | key | value |
      | id  |       |
    When I repair "guid_unsynced" with strategy "sync_guid" on "ids.yaml"
    Then the repair applied 1 fix
    And the form "a.meta.json" data "id" equals the meta id
    And the form "a.meta.json" meta id equals "G-9"
    And the repair leaves 0 issues

  Scenario: The guid field is canonical - meta.id is synced from a populated field
    Given a template "ids.yaml" with fields:
      | key | type |
      | id  | guid |
    And a form "a.meta.json" with meta id "stale" and data:
      | key | value     |
      | id  | real-guid |
    When I repair "guid_unsynced" with strategy "sync_guid" on "ids.yaml"
    Then the repair applied 1 fix
    And the form "a.meta.json" meta id equals "real-guid"
    And the repair leaves 0 issues
