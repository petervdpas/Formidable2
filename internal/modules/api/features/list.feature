Feature: REST API - paged collection listing
  GET /api/collections/{tpl} returns a paginated, optionally filtered
  view of the collection. Supports limit/offset/q/tags query params and
  participates in HTTP caching via ETag + If-None-Match.

  Background:
    Given an API handler over a stub dataprovider with these templates:
      | filename       | name       | enableCollection | guidField |
      | basic.yaml     | Basic Form | false            |           |
      | recepten.yaml  | Recepten   | true             | guid      |
    And the dataprovider has tagged forms for "recepten.yaml":
      | filename            | id      | title  | tags        |
      | brood.meta.json     | g-1234  | Brood  | bakery,wb   |
      | pasta.meta.json     | g-5678  | Pasta  | italian,wb  |
      | pizza.meta.json     | g-9999  | Pizza  | italian     |

  Scenario: GET returns the full collection by default
    When I GET "/api/collections/recepten"
    Then the response status is 200
    And the response content-type is "application/json; charset=utf-8"
    And the JSON has "collectionEnabled" == true
    And the JSON has "template" == "recepten"
    And the JSON has "total" == 3
    And the JSON has "limit" == 100
    And the JSON has "offset" == 0
    And the JSON "items" has length 3

  Scenario: GET sets ETag and Cache-Control headers
    When I GET "/api/collections/recepten"
    Then the response has header "ETag"
    And the response has header "Cache-Control"

  Scenario: GET with matching If-None-Match returns 304
    Given I GET "/api/collections/recepten" and capture the ETag
    When I GET "/api/collections/recepten" with header "If-None-Match" matching the captured ETag
    Then the response status is 304
    And the response body is empty

  Scenario: GET with non-matching If-None-Match returns 200
    # Use a sentinel without embedded quotes so the gherkin "([^"]*)"
    # capture matches cleanly. The handler's compare is exact-string.
    When I GET "/api/collections/recepten" with header "If-None-Match" "stale-etag"
    Then the response status is 200

  Scenario: GET with limit and offset paginates
    When I GET "/api/collections/recepten?limit=1&offset=1"
    Then the response status is 200
    And the JSON has "total" == 3
    And the JSON has "limit" == 1
    And the JSON has "offset" == 1
    And the JSON "items" has length 1

  Scenario: GET with q filters by case-insensitive substring on title+tags
    When I GET "/api/collections/recepten?q=BROOD"
    Then the JSON has "total" == 1
    And the JSON "items" has length 1

  Scenario: GET with q matches against tags too
    When I GET "/api/collections/recepten?q=italian"
    Then the JSON has "total" == 2

  Scenario: GET with tags ANDs across the filter list
    When I GET "/api/collections/recepten?tags=italian,wb"
    Then the JSON has "total" == 1

  Scenario: GET with single tag returns matches
    When I GET "/api/collections/recepten?tags=bakery"
    Then the JSON has "total" == 1

  Scenario: GET 403 for collection-disabled template
    When I GET "/api/collections/basic"
    Then the response status is 403
    And the JSON has "error" == "collection-disabled"

  Scenario: GET 403 for unknown template
    When I GET "/api/collections/ghost"
    Then the response status is 403
