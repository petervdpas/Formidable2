Feature: Per-profile index
  The index is a SQLite cache of disk state - one file per profile,
  built on first run by scanning the context's templates/ and storage/
  trees. After that, in-app writes flow in via per-event hooks
  (template/form save/delete) and external writes (gigot pull, an
  outside editor) are reconciled by RescanAll. Profile switches close
  the current handle and open a different file; data does not bleed
  across profiles.

  Background:
    Given a fresh index for a temp profile

  # ── First run ─────────────────────────────────────────────────────

  Scenario: First-run RescanAll populates the index from disk
    Given a template "basic.yaml" on disk with fields:
      | key    | type |
      | id     | guid |
      | labels | tags |
    And a form "one.meta.json" under "basic.yaml" with values:
      | key    | value         |
      | id     | g1            |
      | labels | alpha,common  |
    And a form "two.meta.json" under "basic.yaml" with values:
      | key    | value         |
      | id     | g2            |
      | labels | beta,common   |
    When I run RescanAll
    Then the index lists templates "basic.yaml"
    And the index has 2 forms for template "basic.yaml"
    And form "one.meta.json" under "basic.yaml" has tags "alpha,common"
    And the index rev is 1

  Scenario: First-run on an empty context
    When I run RescanAll
    Then the index has 0 templates
    And the index rev is 0

  # ── External-edit reconciliation (gigot pull) ─────────────────────

  Scenario: Sync added a template - RescanAll picks it up
    Given a template "basic.yaml" on disk with fields:
      | key | type |
      | id  | guid |
    And I run RescanAll
    And a template "looper.yaml" on disk with fields:
      | key | type |
      | x   | text |
    When I run RescanAll
    Then the index lists templates "basic.yaml,looper.yaml"

  Scenario: Sync edited a form - RescanAll re-reads its content
    Given a template "basic.yaml" on disk with fields:
      | key    | type |
      | labels | tags |
    And a form "one.meta.json" under "basic.yaml" with values:
      | key    | value     |
      | labels | old,older |
    And I run RescanAll
    And the form "one.meta.json" under "basic.yaml" is rewritten with values:
      | key    | value         |
      | labels | fresh,brand   |
    When I run RescanAll
    Then form "one.meta.json" under "basic.yaml" has tags "brand,fresh"

  Scenario: Sync deleted a form - RescanAll removes the row
    Given a template "basic.yaml" on disk with fields:
      | key | type |
      | x   | text |
    And a form "doomed.meta.json" under "basic.yaml" with values:
      | key | value |
      | x   | y     |
    And I run RescanAll
    And the form "doomed.meta.json" under "basic.yaml" is removed from disk
    When I run RescanAll
    Then the index has 0 forms for template "basic.yaml"

  Scenario: Sync deleted the template - cascade wipes its forms
    Given a template "basic.yaml" on disk with fields:
      | key | type |
      | x   | text |
    And a form "one.meta.json" under "basic.yaml" with values:
      | key | value |
      | x   | y     |
    And I run RescanAll
    And the template "basic.yaml" is removed from disk
    When I run RescanAll
    Then the index has 0 templates
    And the index has 0 forms for template "basic.yaml"

  # ── Resilience to bad files (gigot pull / external editor) ───────

  Scenario: One malformed form does not abort the entire rescan
    Given a template "basic.yaml" on disk with fields:
      | key    | type |
      | labels | tags |
    And a template "looper.yaml" on disk with fields:
      | key | type |
      | x   | text |
    And a form "good.meta.json" under "basic.yaml" with values:
      | key    | value |
      | labels | a     |
    And a malformed form "BAD.meta.json" exists under "basic.yaml"
    And a form "also-good.meta.json" under "looper.yaml" with values:
      | key | value |
      | x   | y     |
    When I run RescanAll tolerating load errors
    Then the last RescanAll error mentions "BAD.meta.json"
    And the index has 1 forms for template "basic.yaml"
    And the index has 1 forms for template "looper.yaml"

  Scenario: RescanAll on an unchanged index does not bump rev
    Given a template "basic.yaml" on disk with fields:
      | key | type |
      | x   | text |
    And I run RescanAll
    When I run RescanAll
    Then the index rev is 1

  # ── Profile switch ───────────────────────────────────────────────

  Scenario: Switching profiles isolates each profile's data
    Given a template "personal.yaml" on disk with fields:
      | key | type |
      | x   | text |
    And I run RescanAll
    And I switch to a fresh profile
    And a template "billing.yaml" on disk with fields:
      | key | type |
      | y   | text |
    When I run RescanAll
    Then the index lists templates "billing.yaml"

  Scenario: Switching back to a previous profile sees its data intact
    Given a template "a.yaml" on disk with fields:
      | key | type |
      | x   | text |
    And I run RescanAll
    And I remember the current profile as "A"
    And I switch to a fresh profile
    And a template "b.yaml" on disk with fields:
      | key | type |
      | y   | text |
    And I run RescanAll
    And I switch back to profile "A"
    Then the index lists templates "a.yaml"

  # ── Full-text search (FTS5 index built through the reconcile path) ─

  Scenario: Full-text search finds records by their field text
    Given a template "notes.yaml" on disk with fields:
      | key  | type     |
      | body | textarea |
    And a form "fiber.meta.json" under "notes.yaml" with values:
      | key  | value                                |
      | body | fiber optic cable through the duct   |
    And a form "copper.meta.json" under "notes.yaml" with values:
      | key  | value                                |
      | body | copper cabling at the patch panel    |
    When I run RescanAll
    Then searching "notes.yaml" for "fiber" yields forms "fiber.meta.json"
    And searching "notes.yaml" for "duct" yields forms "fiber.meta.json"
    And searching "notes.yaml" for "panel" yields forms "copper.meta.json"

  Scenario: Multi-term search is an AND over the record text
    Given a template "notes.yaml" on disk with fields:
      | key  | type     |
      | body | textarea |
    And a form "a.meta.json" under "notes.yaml" with values:
      | key  | value                              |
      | body | fiber optic cable through the duct |
    And a form "b.meta.json" under "notes.yaml" with values:
      | key  | value                       |
      | body | fiber spliced near the riser |
    When I run RescanAll
    Then searching "notes.yaml" for "fiber duct" yields forms "a.meta.json"
    And searching "notes.yaml" for "fiber" yields 2 forms

  Scenario: A blank query matches nothing
    Given a template "notes.yaml" on disk with fields:
      | key  | type     |
      | body | textarea |
    And a form "a.meta.json" under "notes.yaml" with values:
      | key  | value |
      | body | hello |
    When I run RescanAll
    Then searching "notes.yaml" for "   " yields 0 forms

  Scenario: Raw FTS operators in the query do not error
    Given a template "notes.yaml" on disk with fields:
      | key  | type     |
      | body | textarea |
    And a form "a.meta.json" under "notes.yaml" with values:
      | key  | value |
      | body | hello |
    When I run RescanAll
    Then searching "notes.yaml" for "cable AND (" yields 0 forms

  # ── On-demand reindex (RescanTemplate) ───────────────────────────

  Scenario: Reindex a collection drops a form deleted from disk
    Given a template "notes.yaml" on disk with fields:
      | key  | type     |
      | body | textarea |
    And a form "keep.meta.json" under "notes.yaml" with values:
      | key  | value |
      | body | keep me |
    And a form "drop.meta.json" under "notes.yaml" with values:
      | key  | value |
      | body | drop me |
    And I run RescanAll
    And the form "drop.meta.json" under "notes.yaml" is removed from disk
    When I run RescanTemplate for "notes.yaml"
    Then the index has 1 forms for template "notes.yaml"
    And searching "notes.yaml" for "drop" yields 0 forms

  Scenario: Reindex tolerates a malformed form and rebuilds the rest
    Given a template "notes.yaml" on disk with fields:
      | key  | type     |
      | body | textarea |
    And a form "good.meta.json" under "notes.yaml" with values:
      | key  | value |
      | body | indexed text |
    And a malformed form "BAD.meta.json" exists under "notes.yaml"
    When I run RescanTemplate for "notes.yaml" tolerating load errors
    Then the last reindex error mentions "BAD.meta.json"
    And the index has 1 forms for template "notes.yaml"
    And searching "notes.yaml" for "indexed" yields forms "good.meta.json"

  Scenario: Reindexing a template removed from disk wipes its collection
    Given a template "notes.yaml" on disk with fields:
      | key  | type     |
      | body | textarea |
    And a form "a.meta.json" under "notes.yaml" with values:
      | key  | value |
      | body | hello |
    And I run RescanAll
    And the template "notes.yaml" is removed from disk
    When I run RescanTemplate for "notes.yaml"
    Then the index has 0 forms for template "notes.yaml"
