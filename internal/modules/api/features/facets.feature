Feature: REST API — facets discovery endpoint
  GET /api/collections/{tpl}/facets returns the template's facet
  contract — the filter primitives an API consumer can pass as
  ?facet.<key>=LABEL on the list / count endpoints. Separate from
  /design which carries data-structure metadata; facets are filter
  metadata.

  Background:
    Given an API handler over a stub dataprovider with these templates:
      | filename       | name       | enableCollection | guidField |
      | basic.yaml     | Basic Form | false            |           |
      | recepten.yaml  | Recepten   | true             | guid      |

  Scenario: GET facets returns the declared facet contract
    Given the templates store design "recepten.yaml" has facet "status" with icon "fa-flag" and options:
      | label | color |
      | DONE  | green |
      | OPEN  | red   |
    And the templates store design "recepten.yaml" has facet "size" with icon "fa-shirt" and options:
      | label | color |
      | BIG   | blue  |
      | SMALL | gray  |
    When I GET "/api/collections/recepten/facets"
    Then the response status is 200
    And the response content-type is "application/json; charset=utf-8"
    And the JSON has "template" == "recepten"
    And the JSON "facets" has length 2
    And the JSON nested "facets[0].key" == "status"
    And the JSON nested "facets[0].icon" == "fa-flag"
    And the JSON nested "facets[0].options[0].label" == "DONE"
    And the JSON nested "facets[0].options[0].color" == "green"
    And the JSON nested "facets[1].key" == "size"
    And the JSON nested "facets[1].options[1].label" == "SMALL"

  Scenario: GET facets returns empty array for a template without facets
    When I GET "/api/collections/recepten/facets"
    Then the response status is 200
    And the JSON has "template" == "recepten"
    And the JSON "facets" has length 0

  Scenario: GET facets returns 403 for a non-collection template
    When I GET "/api/collections/basic/facets"
    Then the response status is 403
    And the JSON has "error" == "collection-disabled"

  Scenario: GET facets returns 403 for an unknown template
    When I GET "/api/collections/ghost/facets"
    Then the response status is 403
    And the JSON has "error" == "collection-disabled"

  Scenario: GET facets carries ETag header
    Given the templates store design "recepten.yaml" has facet "status" with icon "fa-flag" and options:
      | label | color |
      | DONE  | green |
    When I GET "/api/collections/recepten/facets"
    Then the response has header "ETag"
    And the response has header "Cache-Control"

  Scenario: Path traversal in template segment is refused
    When I GET "/api/collections/../etc/facets"
    Then the response status is 301
