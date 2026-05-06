Feature: REST API — collections directory + count
  The wiki API mirrors the original Formidable internalServer's
  /api/collections/* surface. Slice A1 covers the two simplest reads:
  the directory of collection-enabled templates and the per-template
  count. Both pull from the dataprovider only — no filesystem reads.

  Background:
    Given an API handler over a stub dataprovider with these templates:
      | filename       | name       | enableCollection | guidField |
      | basic.yaml     | Basic Form | false            |           |
      | recepten.yaml  | Recepten   | true             | guid      |
      | leeg.yaml      | Leeg       | true             | guid      |
    And the dataprovider has forms for "recepten.yaml":
      | filename            | id      | title  |
      | brood.meta.json     | g-1234  | Brood  |
      | pasta.meta.json     | g-5678  | Pasta  |

  Scenario: GET /api/collections lists collection-enabled templates only
    When I GET "/api/collections"
    Then the response status is 200
    And the response content-type is "application/json; charset=utf-8"
    And the JSON array contains a row with id "recepten" and href "/api/collections/recepten"
    And the JSON array contains a row with id "leeg"
    And the JSON array does NOT contain a row with id "basic"

  Scenario: GET /api/collections returns name from yaml (or stem when missing)
    When I GET "/api/collections"
    Then the JSON row with id "recepten" has name "Recepten"

  Scenario: GET /api/collections/{tpl}/count returns total
    When I GET "/api/collections/recepten/count"
    Then the response status is 200
    And the response content-type is "application/json; charset=utf-8"
    And the JSON has "template" == "recepten"
    And the JSON has "total" == 2

  Scenario: GET /api/collections/{tpl}/count returns 403 for non-collection template
    When I GET "/api/collections/basic/count"
    Then the response status is 403
    And the JSON has "error" == "collection-disabled"

  Scenario: GET /api/collections/{tpl}/count returns 403 for unknown template
    # Treats unknown == disabled to avoid leaking storage layout via
    # 404 vs 403. Same posture as the original internalServer.
    When I GET "/api/collections/ghost/count"
    Then the response status is 403

  Scenario: Path traversal in template segment is refused
    When I GET "/api/collections/../etc/count"
    Then the response status is 301
