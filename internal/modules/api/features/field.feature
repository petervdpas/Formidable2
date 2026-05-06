Feature: REST API — PATCH /api/collections/{tpl}/{id}/field/{key}
  Updates a single named field. Body is either `{"value": …}` (envelope
  form) or a raw scalar/array/object — both shapes are accepted so
  curl-style invocations stay readable. Refuses guid-key updates
  (immutable) and unknown fields.

  Background:
    Given an API handler over a stub dataprovider with these templates:
      | filename       | name       | enableCollection | guidField |
      | basic.yaml     | Basic Form | false            |           |
      | recepten.yaml  | Recepten   | true             | guid      |
    And the templates store design "recepten.yaml" has fields:
      | key   | type | label | optionsRaw |
      | guid  | guid | GUID  |            |
      | naam  | text | Naam  |            |
      | aantal| number | Aantal |          |
    And the dataprovider has forms for "recepten.yaml":
      | filename            | id      | title  |
      | brood.meta.json     | g-abc   | Brood  |
    And the storage holds form "recepten.yaml":"brood.meta.json" with:
      """
      {"meta":{"id":"g-abc","template":"recepten"},"data":{"guid":"g-abc","naam":"Brood","aantal":4}}
      """

  Scenario: PATCH /field/{key} with {value: …} envelope
    When I PATCH "/api/collections/recepten/g-abc/field/naam" with body:
      """
      {"value":"Brood (updated)"}
      """
    Then the response status is 200
    And the JSON has "template" == "recepten"
    And the JSON has "id" == "g-abc"
    And the JSON has "filename" == "brood.meta.json"
    And the JSON nested "changed.naam" == "Brood (updated)"
    And the writer recorded 1 save

  Scenario: PATCH /field/{key} with raw scalar body
    When I PATCH "/api/collections/recepten/g-abc/field/aantal" with body:
      """
      8
      """
    Then the response status is 200
    And the JSON nested "changed.aantal" == "8"

  Scenario: PATCH guid field returns 409 guid-immutable
    When I PATCH "/api/collections/recepten/g-abc/field/guid" with body:
      """
      {"value":"g-other"}
      """
    Then the response status is 409
    And the JSON has "error" == "guid-immutable"
    And the writer recorded 0 saves

  Scenario: PATCH unknown field returns 400
    When I PATCH "/api/collections/recepten/g-abc/field/no-such" with body:
      """
      {"value":"x"}
      """
    Then the response status is 400
    And the JSON has "error" == "unknown-field"

  Scenario: PATCH unknown id returns 404
    When I PATCH "/api/collections/recepten/no-such/field/naam" with body:
      """
      {"value":"x"}
      """
    Then the response status is 404
    And the JSON has "error" == "not-found"

  Scenario: PATCH 403 for disabled template
    When I PATCH "/api/collections/basic/anything/field/naam" with body:
      """
      {"value":"x"}
      """
    Then the response status is 403

  Scenario: PATCH 400 on malformed JSON
    When I PATCH "/api/collections/recepten/g-abc/field/naam" with body:
      """
      {bad
      """
    Then the response status is 400
    And the JSON has "error" == "invalid-json"
