Feature: REST API - OpenAPI spec generator
  GET /api/openapi.json returns a fresh OpenAPI 3.0.3 document built
  per request from the current set of collection-enabled templates.
  Per-template Data_<tpl> schemas are derived from the template's
  fields so /api/docs (Swagger UI) can offer a typed Try-It-Out
  experience even after templates change.

  Background:
    Given an API handler over a stub dataprovider with these templates:
      | filename       | name       | enableCollection | guidField |
      | basic.yaml     | Basic Form | false            |           |
      | recepten.yaml  | Recepten   | true             | guid      |
    And the templates store design "recepten.yaml" has fields:
      | key      | type     | label   | optionsRaw            |
      | guid     | guid     | GUID    |                       |
      | naam     | text     | Naam    |                       |
      | type     | dropdown | Type    | bread:Brood,pasta:Pasta |
      | tags     | tags     | Tags    |                       |
      | aantal   | number   | Aantal  |                       |
      | actief   | boolean  | Actief  |                       |

  Scenario: GET /api/openapi.json returns 3.0.3 envelope
    When I GET "/api/openapi.json"
    Then the response status is 200
    And the response content-type is "application/json; charset=utf-8"
    And the JSON has "openapi" == "3.0.3"
    And the JSON nested "info.title" == "Formidable Collections API"

  Scenario: Spec advertises the collection-enabled templates only
    When I GET "/api/openapi.json"
    Then the JSON nested "components.schemas.Data_recepten.type" == "object"
    And the JSON does NOT have schema "Data_basic"

  Scenario: Spec includes the read paths we ship
    When I GET "/api/openapi.json"
    Then the JSON has path "/collections"
    And the JSON has path "/collections/{template}"
    And the JSON has path "/collections/{template}/count"
    And the JSON has path "/collections/{template}/{id}"
    And the JSON has path "/collections/{template}/design"
    And the JSON has path "/collections/{template}/export.ndjson"
    And the JSON has path "/collections/{template}/export.csv"

  Scenario: Per-template data schema reflects field types
    When I GET "/api/openapi.json"
    Then the JSON nested "components.schemas.Data_recepten.properties.naam.type" == "string"
    And the JSON nested "components.schemas.Data_recepten.properties.aantal.type" == "number"
    And the JSON nested "components.schemas.Data_recepten.properties.actief.type" == "boolean"
    And the JSON nested "components.schemas.Data_recepten.properties.tags.type" == "array"

  Scenario: Dropdown field projects to enum
    When I GET "/api/openapi.json"
    Then the JSON nested "components.schemas.Data_recepten.properties.type.enum[0]" == "bread"
    And the JSON nested "components.schemas.Data_recepten.properties.type.enum[1]" == "pasta"

  Scenario: GUID field is required at the data schema
    When I GET "/api/openapi.json"
    Then the JSON nested "components.schemas.Data_recepten.required[0]" == "guid"

  Scenario: Spec includes the relation-follow paths
    When I GET "/api/openapi.json"
    Then the JSON has path "/collections/{template}/{id}/relations"
    And the spec path "/collections/{template}/{id}/relations" has method "get" with summary "List a record's relations"
    And the JSON has path "/collections/{template}/{id}/relations/{to}"
    And the spec path "/collections/{template}/{id}/relations/{to}" has method "get" with summary "Follow one relation to its linked records"
