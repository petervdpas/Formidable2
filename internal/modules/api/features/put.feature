Feature: REST API - PUT /api/collections/{tpl}/{id} (replace + upsert)
  Replaces an existing item. Without ?upsert=true, an unknown id
  returns 404; with upsert it creates the item at the requested GUID.
  Body's data[guidKey] must match the path id (or be absent - in
  which case the path id is injected).

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

  Scenario: PUT replaces an existing item by GUID
    When I PUT "/api/collections/recepten/g-abc" with body:
      """
      {"data":{"guid":"g-abc","naam":"Brood (updated)"}}
      """
    Then the response status is 200
    And the JSON has "id" == "g-abc"
    And the JSON has "filename" == "brood.meta.json"
    And the JSON nested "data.naam" == "Brood (updated)"
    And the writer recorded 1 save

  Scenario: PUT injects the path id when body has no guid
    When I PUT "/api/collections/recepten/g-abc" with body:
      """
      {"data":{"naam":"Brood"}}
      """
    Then the response status is 200
    And the JSON has "id" == "g-abc"

  Scenario: PUT 409 when body guid mismatches path id
    When I PUT "/api/collections/recepten/g-abc" with body:
      """
      {"data":{"guid":"g-other","naam":"X"}}
      """
    Then the response status is 409
    And the JSON has "error" == "guid-mismatch"

  Scenario: PUT 404 for unknown id without upsert
    When I PUT "/api/collections/recepten/no-such" with body:
      """
      {"data":{"guid":"no-such","naam":"X"}}
      """
    Then the response status is 404
    And the JSON has "error" == "not-found"

  Scenario: PUT with ?upsert=true creates the item at the requested GUID
    When I PUT "/api/collections/recepten/g-new?upsert=true" with body:
      """
      {"data":{"guid":"g-new","naam":"Pasta"}}
      """
    Then the response status is 201
    And the JSON has "id" == "g-new"
    And the JSON nested "data.naam" == "Pasta"

  Scenario: PUT 403 for collection-disabled template
    When I PUT "/api/collections/basic/anything" with body:
      """
      {"data":{}}
      """
    Then the response status is 403
    And the JSON has "error" == "collection-disabled"

  Scenario: PUT 400 on malformed JSON
    When I PUT "/api/collections/recepten/g-abc" with body:
      """
      {bad json
      """
    Then the response status is 400
    And the JSON has "error" == "invalid-json"
