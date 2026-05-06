Feature: REST API — DELETE /api/collections/{tpl}/{id}
  Removes a collection item by GUID. 204 on success, 404 when the id
  is unknown, 403 when the template isn't collection-enabled. Empty
  body in all cases (DELETE responses don't carry a JSON envelope).

  Background:
    Given an API handler over a stub dataprovider with these templates:
      | filename       | name       | enableCollection | guidField |
      | basic.yaml     | Basic Form | false            |           |
      | recepten.yaml  | Recepten   | true             | guid      |
    And the templates store design "recepten.yaml" has fields:
      | key   | type | label | optionsRaw |
      | guid  | guid | GUID  |            |
    And the dataprovider has forms for "recepten.yaml":
      | filename            | id      | title  |
      | brood.meta.json     | g-abc   | Brood  |

  Scenario: DELETE removes an existing item
    When I DELETE "/api/collections/recepten/g-abc"
    Then the response status is 204
    And the response body is empty
    And the writer recorded 1 delete

  Scenario: DELETE 404 for unknown id
    When I DELETE "/api/collections/recepten/no-such"
    Then the response status is 404
    And the JSON has "error" == "not-found"
    And the writer recorded 0 deletes

  Scenario: DELETE 403 for collection-disabled template
    When I DELETE "/api/collections/basic/anything"
    Then the response status is 403
    And the JSON has "error" == "collection-disabled"

  Scenario: DELETE 500 when the storage layer fails
    Given the writer fails the next delete with "disk-full"
    When I DELETE "/api/collections/recepten/g-abc"
    Then the response status is 500
    And the JSON has "error" == "delete-failed"
