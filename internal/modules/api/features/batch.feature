Feature: REST API - POST /api/collections/{tpl}/batch
  Bulk apply many items in a single request. `?mode=create` refuses
  existing GUIDs (per-item error), `replace` is full-upsert, `merge`
  is partial-upsert. Per-item failures are collected in the response
  rather than aborting the batch - clients see what landed and what
  didn't in one round-trip.

  Background:
    Given an API handler over a stub dataprovider with these templates:
      | filename       | name       | enableCollection | guidField |
      | basic.yaml     | Basic Form | false            |           |
      | recepten.yaml  | Recepten   | true             | guid      |
    And the templates store design "recepten.yaml" has fields:
      | key   | type | label | optionsRaw |
      | guid  | guid | GUID  |            |
      | naam  | text | Naam  |            |
    And the dataprovider has forms for "recepten.yaml":
      | filename            | id      | title  |
      | brood.meta.json     | g-abc   | Brood  |
    And the storage holds form "recepten.yaml":"brood.meta.json" with:
      """
      {"meta":{"id":"g-abc","template":"recepten","tags":["bakery"]},"data":{"guid":"g-abc","naam":"Brood","portie":4}}
      """

  Scenario: Default mode is create - refuses existing guid
    When I POST "/api/collections/recepten/batch" with body:
      """
      {"items":[
        {"data":{"guid":"g-abc","naam":"Brood 2"}},
        {"data":{"guid":"g-new","naam":"Pasta"}}
      ]}
      """
    Then the response status is 200
    And the JSON has "template" == "recepten"
    And the JSON has "mode" == "create"
    And the JSON has "totalItems" == 2
    And the JSON "created" has length 1
    And the JSON "errors" has length 1
    And the JSON nested "created[0].id" == "g-new"
    And the JSON nested "errors[0].error" == "already-exists"

  Scenario: mode=replace upserts, overwriting existing
    When I POST "/api/collections/recepten/batch?mode=replace" with body:
      """
      {"items":[
        {"data":{"guid":"g-abc","naam":"Brood (replaced)"}},
        {"data":{"guid":"g-new","naam":"Pizza"}}
      ]}
      """
    Then the response status is 200
    And the JSON has "mode" == "replace"
    And the JSON "updated" has length 1
    And the JSON "created" has length 1
    And the JSON nested "updated[0].id" == "g-abc"
    And the JSON nested "created[0].id" == "g-new"

  Scenario: mode=merge shallow-merges into existing
    When I POST "/api/collections/recepten/batch?mode=merge" with body:
      """
      {"items":[
        {"data":{"guid":"g-abc","naam":"Brood (merged)"}}
      ]}
      """
    Then the response status is 200
    And the JSON has "mode" == "merge"
    And the JSON "updated" has length 1
    And the writer recorded 1 save

  Scenario: Items without GUID are reported as errors, not aborts
    When I POST "/api/collections/recepten/batch" with body:
      """
      {"items":[
        {"data":{"naam":"No GUID"}},
        {"data":{"guid":"g-x","naam":"Pasta"}}
      ]}
      """
    Then the response status is 200
    And the JSON "errors" has length 1
    And the JSON "created" has length 1
    And the JSON nested "errors[0].error" == "guid-missing"
    And the JSON nested "errors[0].index" == 0

  Scenario: Empty items array returns an empty summary
    When I POST "/api/collections/recepten/batch" with body:
      """
      {"items":[]}
      """
    Then the response status is 200
    And the JSON has "totalItems" == 0
    And the JSON "created" has length 0
    And the JSON "updated" has length 0
    And the JSON "errors" has length 0

  Scenario: Missing items array returns 400
    When I POST "/api/collections/recepten/batch" with body:
      """
      {}
      """
    Then the response status is 400
    And the JSON has "error" == "items-missing"

  Scenario: Invalid mode returns 400
    When I POST "/api/collections/recepten/batch?mode=zzz" with body:
      """
      {"items":[]}
      """
    Then the response status is 400
    And the JSON has "error" == "invalid-mode"

  Scenario: 403 for disabled template
    When I POST "/api/collections/basic/batch" with body:
      """
      {"items":[]}
      """
    Then the response status is 403
    And the JSON has "error" == "collection-disabled"

  Scenario: 400 on malformed JSON
    When I POST "/api/collections/recepten/batch" with body:
      """
      {bad
      """
    Then the response status is 400
    And the JSON has "error" == "invalid-json"
