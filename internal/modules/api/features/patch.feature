Feature: REST API — PATCH /api/collections/{tpl}/{id} (merge update)
  Shallow-merges the incoming meta/data into the existing form. The
  body's data[guidKey] must match the path id (or be absent — in
  which case it's force-set to the path id post-merge so the form
  stays addressable). Optional `If-Match` header gates against
  concurrent modification — mismatched returns 412.

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

  Scenario: PATCH shallow-merges incoming data into existing
    When I PATCH "/api/collections/recepten/g-abc" with body:
      """
      {"data":{"naam":"Brood (updated)"}}
      """
    Then the response status is 200
    And the JSON has "id" == "g-abc"
    And the JSON nested "data.naam" == "Brood (updated)"
    # Existing keys not in the patch survive.
    And the JSON nested "data.portie" == "4"
    And the writer recorded 1 save

  Scenario: PATCH meta-only leaves data untouched
    When I PATCH "/api/collections/recepten/g-abc" with body:
      """
      {"meta":{"flagged":true}}
      """
    Then the response status is 200
    And the JSON nested "data.naam" == "Brood"
    And the JSON nested "data.portie" == "4"

  Scenario: PATCH 409 when body guid differs from path id
    When I PATCH "/api/collections/recepten/g-abc" with body:
      """
      {"data":{"guid":"g-other"}}
      """
    Then the response status is 409
    And the JSON has "error" == "guid-mismatch"

  Scenario: PATCH 404 for unknown id
    When I PATCH "/api/collections/recepten/no-such" with body:
      """
      {"data":{}}
      """
    Then the response status is 404
    And the JSON has "error" == "not-found"

  Scenario: PATCH 403 for disabled template
    When I PATCH "/api/collections/basic/anything" with body:
      """
      {"data":{}}
      """
    Then the response status is 403

  Scenario: PATCH with matching If-Match goes through
    Given I GET "/api/collections/recepten/g-abc" and capture the ETag
    When I PATCH "/api/collections/recepten/g-abc" with header "If-Match" matching the captured ETag and body:
      """
      {"data":{"naam":"Patched"}}
      """
    Then the response status is 200
    And the JSON nested "data.naam" == "Patched"

  Scenario: PATCH with mismatched If-Match returns 412
    When I PATCH "/api/collections/recepten/g-abc" with header "If-Match" "stale-etag" and body:
      """
      {"data":{"naam":"X"}}
      """
    Then the response status is 412
    And the JSON has "error" == "precondition-failed"

  Scenario: PATCH 400 on malformed JSON
    When I PATCH "/api/collections/recepten/g-abc" with body:
      """
      {bad
      """
    Then the response status is 400
    And the JSON has "error" == "invalid-json"
