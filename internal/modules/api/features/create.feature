Feature: REST API — POST /api/collections/{tpl} (create + GUID mint)
  Creates a new collection item. Auto-generates a GUID when the body
  doesn't supply one (option B from the design conversation), so
  clients can either round-trip via GET /api/guid or just POST a
  body without a guid and trust the server to mint one. Pairs with
  ?upsert=true for idempotent overwrites.

  Background:
    Given an API handler over a stub dataprovider with these templates:
      | filename       | name       | enableCollection | guidField |
      | basic.yaml     | Basic Form | false            |           |
      | recepten.yaml  | Recepten   | true             | guid      |
    And the templates store design "recepten.yaml" has fields:
      | key   | type | label | optionsRaw |
      | guid  | guid | GUID  |            |
      | naam  | text | Naam  |            |

  Scenario: GET /api/guid returns a fresh UUID
    When I GET "/api/guid"
    Then the response status is 200
    And the response content-type is "application/json; charset=utf-8"
    And the JSON has a non-empty "guid" field

  Scenario: POST creates a new item with client-supplied GUID
    When I POST "/api/collections/recepten" with body:
      """
      {"data":{"guid":"g-abc","naam":"Brood"}}
      """
    Then the response status is 201
    And the response has header "Location"
    And the JSON has "id" == "g-abc"
    And the JSON has "template" == "recepten"
    And the JSON nested "data.naam" == "Brood"
    And the writer recorded 1 save

  Scenario: POST without a GUID auto-generates one server-side
    When I POST "/api/collections/recepten" with body:
      """
      {"data":{"naam":"Pasta"}}
      """
    Then the response status is 201
    And the JSON has a non-empty "id" field
    And the JSON nested "data.naam" == "Pasta"

  Scenario: POST with existing GUID returns 409 by default
    Given the dataprovider has forms for "recepten.yaml":
      | filename            | id      | title  |
      | brood.meta.json     | g-abc   | Brood  |
    When I POST "/api/collections/recepten" with body:
      """
      {"data":{"guid":"g-abc","naam":"Brood 2"}}
      """
    Then the response status is 409
    And the JSON has "error" == "already-exists"

  Scenario: POST with ?upsert=true overwrites and returns 200
    Given the dataprovider has forms for "recepten.yaml":
      | filename            | id      | title  |
      | brood.meta.json     | g-abc   | Brood  |
    When I POST "/api/collections/recepten?upsert=true" with body:
      """
      {"data":{"guid":"g-abc","naam":"Brood 2"}}
      """
    Then the response status is 200
    And the JSON nested "data.naam" == "Brood 2"
    And the JSON has "filename" == "brood.meta.json"

  Scenario: POST 403 for collection-disabled template
    When I POST "/api/collections/basic" with body:
      """
      {"data":{}}
      """
    Then the response status is 403
    And the JSON has "error" == "collection-disabled"

  Scenario: POST 400 on malformed JSON body
    When I POST "/api/collections/recepten" with body:
      """
      {not json
      """
    Then the response status is 400
    And the JSON has "error" == "invalid-json"

  Scenario: POST derives filename from item_field
    Given the templates store design "recepten.yaml" has fields:
      | key   | type | label | optionsRaw |
      | guid  | guid | GUID  |            |
      | naam  | text | Naam  |            |
    And the templates store has design "recepten.yaml":
      | item_field        | naam     |
      | enable_collection | true     |
    And the templates store design "recepten.yaml" has fields:
      | key   | type | label | optionsRaw |
      | guid  | guid | GUID  |            |
      | naam  | text | Naam  |            |
    When I POST "/api/collections/recepten" with body:
      """
      {"data":{"guid":"g-1","naam":"Brood Met Zaden"}}
      """
    Then the response status is 201
    And the JSON has "filename" == "brood-met-zaden.meta.json"
