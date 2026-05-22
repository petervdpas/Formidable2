Feature: REST API - single collection item
  GET /api/collections/{tpl}/{id} returns one item with full data and
  meta. HEAD returns just the validators (ETag + status code) - useful
  for clients that want to check freshness before pulling the body.

  Background:
    Given an API handler over a stub dataprovider with these templates:
      | filename       | name       | enableCollection | guidField |
      | basic.yaml     | Basic Form | false            |           |
      | recepten.yaml  | Recepten   | true             | guid      |
    And the dataprovider has tagged forms for "recepten.yaml":
      | filename            | id      | title  | tags       |
      | brood.meta.json     | g-1234  | Brood  | bakery,wb  |
    And the storage holds form "recepten.yaml":"brood.meta.json" with:
      """
      {"meta":{"id":"g-1234","template":"recepten","tags":["bakery","wb"]},"data":{"guid":"g-1234","naam":"Brood","portie":4}}
      """

  Scenario: GET single item returns full data + meta + links
    When I GET "/api/collections/recepten/g-1234"
    Then the response status is 200
    And the response content-type is "application/json; charset=utf-8"
    And the JSON has "template" == "recepten"
    And the JSON has "id" == "g-1234"
    And the JSON has "filename" == "brood.meta.json"
    And the JSON has "title" == "Brood"
    And the JSON nested "data.guid" == "g-1234"
    And the JSON nested "data.naam" == "Brood"
    And the JSON nested "links.self" == "/api/collections/recepten/g-1234"
    And the JSON nested "links.html" == "/template/recepten/form/brood.meta.json"

  Scenario: GET single item carries ETag header
    When I GET "/api/collections/recepten/g-1234"
    Then the response has header "ETag"

  Scenario: GET with matching If-None-Match returns 304 with ETag
    Given I GET "/api/collections/recepten/g-1234" and capture the ETag
    When I GET "/api/collections/recepten/g-1234" with header "If-None-Match" matching the captured ETag
    Then the response status is 304
    And the response body is empty
    And the response has header "ETag"

  Scenario: GET unknown id returns 404 with error body
    When I GET "/api/collections/recepten/no-such-id"
    Then the response status is 404
    And the JSON has "error" == "not-found"

  Scenario: GET on disabled template returns 403
    When I GET "/api/collections/basic/anything"
    Then the response status is 403
    And the JSON has "error" == "collection-disabled"

  Scenario: GET on unknown template returns 403 (no existence leak)
    When I GET "/api/collections/ghost/anything"
    Then the response status is 403

  Scenario: HEAD on existing item returns 200 with ETag and empty body
    When I HEAD "/api/collections/recepten/g-1234"
    Then the response status is 200
    And the response has header "ETag"
    And the response body is empty

  Scenario: HEAD on unknown id returns 404 with empty body
    When I HEAD "/api/collections/recepten/no-such-id"
    Then the response status is 404
    And the response body is empty

  Scenario: HEAD on disabled template returns 403
    When I HEAD "/api/collections/basic/anything"
    Then the response status is 403
