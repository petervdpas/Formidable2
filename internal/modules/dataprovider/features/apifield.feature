Feature: API-field row fetching
  As a host form's frontend (api-field picker),
  I want to resolve a (source-template, guid, columnKeys) tuple into a flat
  row of projected values
  so that the host form can stamp denormalised data alongside the guid.

  Backend lives on dataprovider.Manager (it already mediates between the
  index and storage; per-template ACL hooks ride the same context.Context).

  The picker always references the source by GUID. Source templates that
  don't have collection enabled are not pickable — the same rule the
  wiki's /api/collections/* layer enforces. Non-scalar source values are
  JSON-flattened to a string so the host form's storage stays scalar.

  Background:
    Given a fresh dataprovider world

  # ── Happy path ──────────────────────────────────────────────────────

  Scenario: Fetches scalar columns verbatim
    Given a collection-enabled template "people.yaml" with form "alice.meta.json" guid "g-1" data:
      | key   | value           |
      | name  | Alice           |
      | email | alice@a.com     |
    When I fetch api-field row from "people.yaml" guid "g-1" columns "name,email"
    Then the row has column "name" string-valued "Alice"
    And the row has column "email" string-valued "alice@a.com"

  Scenario: Missing column key surfaces as nil
    Given a collection-enabled template "people.yaml" with form "alice.meta.json" guid "g-1" data:
      | key  | value |
      | name | Alice |
    When I fetch api-field row from "people.yaml" guid "g-1" columns "name,email"
    Then the row has column "name" string-valued "Alice"
    And the row has column "email" with no value

  # ── JSON flatten ─────────────────────────────────────────────────────

  Scenario: Tags column flattens to JSON string
    Given a collection-enabled template "people.yaml" with form "alice.meta.json" guid "g-1" tags column "tags":
      | a |
      | b |
      | c |
    When I fetch api-field row from "people.yaml" guid "g-1" columns "tags"
    Then the row has column "tags" json-valued `["a","b","c"]`

  Scenario: Map column flattens to JSON string
    Given a collection-enabled template "people.yaml" with form "alice.meta.json" guid "g-1" map column "address" with key "street" value "1 Main"
    When I fetch api-field row from "people.yaml" guid "g-1" columns "address"
    Then the row has column "address" json-valued `{"street":"1 Main"}`

  # ── Unhappy paths ────────────────────────────────────────────────────

  Scenario: Unknown source template surfaces a structured error
    When I fetch api-field row from "ghost.yaml" guid "g-1" columns "name"
    Then the fetch errors with kind "template-not-found"

  Scenario: Source template without collection enabled is rejected
    Given a non-collection template "notes.yaml"
    When I fetch api-field row from "notes.yaml" guid "g-1" columns "name"
    Then the fetch errors with kind "collection-disabled"

  Scenario: Missing guid surfaces a structured error
    Given a collection-enabled template "people.yaml"
    When I fetch api-field row from "people.yaml" guid "g-missing" columns "name"
    Then the fetch errors with kind "guid-not-found"

  Scenario: Empty columns slice returns an empty row
    Given a collection-enabled template "people.yaml" with form "alice.meta.json" guid "g-1" data:
      | key  | value |
      | name | Alice |
    When I fetch api-field row from "people.yaml" guid "g-1" columns ""
    Then the row has 0 columns
