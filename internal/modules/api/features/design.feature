Feature: REST API — template design endpoint
  GET /api/collections/{tpl}/design returns the structured template
  design (fields, options, markdown_template, etc.) so API consumers
  can build forms client-side without hitting the YAML directly.

  Background:
    Given an API handler over a stub dataprovider with these templates:
      | filename       | name       | enableCollection | guidField |
      | basic.yaml     | Basic Form | false            |           |
      | recepten.yaml  | Recepten   | true             | guid      |
    And the templates store has design "recepten.yaml":
      | name              | Recepten              |
      | item_field        | naam                  |
      | markdown_template | # {{naam}}\n          |
      | enable_collection | true                  |

  Scenario: GET design returns header fields and metadata
    When I GET "/api/collections/recepten/design"
    Then the response status is 200
    And the response content-type is "application/json; charset=utf-8"
    And the JSON has "name" == "Recepten"
    And the JSON has "filename" == "recepten.yaml"
    And the JSON has "item_field" == "naam"
    And the JSON has "enable_collection" == true

  Scenario: GET design includes fields with normalized options
    Given the templates store design "recepten.yaml" has fields:
      | key      | type     | label   | optionsRaw            |
      | guid     | guid     | GUID    |                       |
      | naam     | text     | Naam    |                       |
      | type     | dropdown | Type    | bread:Brood,pasta:Pasta |
    When I GET "/api/collections/recepten/design"
    Then the JSON "fields" has length 3
    And the JSON nested fields[2] "type" == "dropdown"
    And the JSON nested fields[2].options[0] "value" == "bread"
    And the JSON nested fields[2].options[0] "label" == "Brood"

  Scenario: GET design fills missing label from key
    Given the templates store design "recepten.yaml" has fields:
      | key   | type | label | optionsRaw |
      | naam  | text |       |            |
    When I GET "/api/collections/recepten/design"
    Then the JSON nested fields[0] "label" == "naam"

  Scenario: GET design 404 for unknown template
    When I GET "/api/collections/ghost/design"
    Then the response status is 404
    And the JSON has "error" == "template-not-found"

  Scenario: GET design 403 for collection-disabled template
    When I GET "/api/collections/basic/design"
    Then the response status is 403
    And the JSON has "error" == "collection-disabled"

  Scenario: GET design carries ETag header
    When I GET "/api/collections/recepten/design"
    Then the response has header "ETag"
    And the response has header "Cache-Control"

  Scenario: GET design includes facets[] when the template declares them
    Given the templates store design "recepten.yaml" has facet "status" with icon "fa-flag" and options:
      | label | color |
      | DONE  | green |
      | OPEN  | red   |
    And the templates store design "recepten.yaml" has facet "size" with icon "fa-shirt" and options:
      | label | color |
      | BIG   | blue  |
    When I GET "/api/collections/recepten/design"
    Then the response status is 200
    And the JSON "facets" has length 2
    And the JSON nested "facets[0].key" == "status"
    And the JSON nested "facets[0].icon" == "fa-flag"
    And the JSON nested "facets[0].options[0].label" == "DONE"
    And the JSON nested "facets[0].options[0].color" == "green"
    And the JSON nested "facets[1].key" == "size"

  Scenario: GET design omits facets when none are declared
    When I GET "/api/collections/recepten/design"
    Then the response status is 200
    And the JSON does not have field "facets"
