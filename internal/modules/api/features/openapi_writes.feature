Feature: REST API — OpenAPI spec covers the write surface
  Once the write endpoints land, the OpenAPI document advertises
  POST/PUT/PATCH/DELETE/batch alongside the read paths, plus the
  per-template Upsert_<stem> request schemas and the shared
  FieldPatchBody / BatchRequest / BatchResponse shapes.

  Background:
    Given an API handler over a stub dataprovider with these templates:
      | filename       | name       | enableCollection | guidField |
      | recepten.yaml  | Recepten   | true             | guid      |
    And the templates store design "recepten.yaml" has fields:
      | key   | type | label | optionsRaw |
      | guid  | guid | GUID  |            |
      | naam  | text | Naam  |            |

  Scenario: Spec advertises GET /guid utility
    When I GET "/api/openapi.json"
    Then the response status is 200
    And the JSON has path "/guid"

  Scenario: Spec includes write operations on /{template}
    When I GET "/api/openapi.json"
    Then the spec path "/collections/{template}" has method "post" with summary "Create item (or upsert with ?upsert=true)"

  Scenario: Spec includes PUT/PATCH/DELETE on /{template}/{id}
    When I GET "/api/openapi.json"
    Then the spec path "/collections/{template}/{id}" has method "put" with summary "Replace item by GUID (or upsert)"
    And the spec path "/collections/{template}/{id}" has method "patch" with summary "Merge update (partial) by GUID"
    And the spec path "/collections/{template}/{id}" has method "delete" with summary "Delete item by GUID"

  Scenario: Spec advertises single-field PATCH
    When I GET "/api/openapi.json"
    Then the JSON has path "/collections/{template}/{id}/field/{key}"

  Scenario: Spec advertises batch endpoint
    When I GET "/api/openapi.json"
    Then the JSON has path "/collections/{template}/batch"

  Scenario: Per-template Upsert_<stem> schema is generated
    When I GET "/api/openapi.json"
    Then the JSON nested "components.schemas.Upsert_recepten.type" == "object"
    And the JSON nested "components.schemas.UpsertPartial_recepten.type" == "object"

  Scenario: Shared write schemas are present
    When I GET "/api/openapi.json"
    Then the JSON nested "components.schemas.FieldPatchBody.description" == "Either { value: ... } or a raw JSON value."
    And the JSON nested "components.schemas.BatchRequest.required[0]" == "items"
    And the JSON nested "components.schemas.BatchResponse.required[0]" == "template"

  Scenario: KeyParam is registered for the field-patch route
    When I GET "/api/openapi.json"
    Then the JSON nested "components.parameters.KeyParam.name" == "key"
