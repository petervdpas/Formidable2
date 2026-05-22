Feature: REST API - collections directory + count
  The wiki API mirrors the original Formidable internalServer's
  /api/collections/* surface. Slice A1 covers the two simplest reads:
  the directory of collection-enabled templates and the per-template
  count. Both pull from the dataprovider only - no filesystem reads.

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

  # ── Facet query filtering ──────────────────────────────────────────
  # Conventions: ?facet.<key>=LABEL - multiple keys AND together; the
  # record must have set=true AND selected==value for every key.

  Scenario: List filters by a single facet
    Given the templates store design "recepten.yaml" has facet "status" with icon "fa-flag" and options:
      | label | color |
      | DONE  | green |
      | OPEN  | red   |
    And the storage form "recepten.yaml":"brood.meta.json" has facet "status" set true selected "DONE"
    And the storage form "recepten.yaml":"pasta.meta.json" has facet "status" set true selected "OPEN"
    When I GET "/api/collections/recepten?facet.status=DONE"
    Then the response status is 200
    And the JSON has "total" == 1
    And the JSON nested "items[0].id" == "g-1234"

  Scenario: List ANDs multiple facet filters
    Given the templates store design "recepten.yaml" has facet "status" with icon "fa-flag" and options:
      | label | color |
      | DONE  | green |
      | OPEN  | red   |
    And the templates store design "recepten.yaml" has facet "size" with icon "fa-shirt" and options:
      | label | color |
      | BIG   | blue  |
      | SMALL | gray  |
    And the storage form "recepten.yaml":"brood.meta.json" has facet "status" set true selected "DONE"
    And the storage form "recepten.yaml":"brood.meta.json" has facet "size" set true selected "BIG"
    And the storage form "recepten.yaml":"pasta.meta.json" has facet "status" set true selected "DONE"
    And the storage form "recepten.yaml":"pasta.meta.json" has facet "size" set true selected "SMALL"
    When I GET "/api/collections/recepten?facet.status=DONE&facet.size=BIG"
    Then the response status is 200
    And the JSON has "total" == 1
    And the JSON nested "items[0].id" == "g-1234"

  Scenario: Records that don't satisfy a facet are filtered out
    Given the templates store design "recepten.yaml" has facet "status" with icon "fa-flag" and options:
      | label | color |
      | DONE  | green |
    And the storage form "recepten.yaml":"brood.meta.json" has facet "status" set true selected "DONE"
    When I GET "/api/collections/recepten?facet.status=DONE"
    Then the response status is 200
    And the JSON has "total" == 1

  Scenario: set=false on the record fails the filter (mere presence isn't enough)
    Given the templates store design "recepten.yaml" has facet "status" with icon "fa-flag" and options:
      | label | color |
      | DONE  | green |
    And the storage form "recepten.yaml":"brood.meta.json" has facet "status" set false selected "DONE"
    When I GET "/api/collections/recepten?facet.status=DONE"
    Then the response status is 200
    And the JSON has "total" == 0

  Scenario: Unknown facet key returns 400 unknown_facet
    Given the templates store design "recepten.yaml" has facet "status" with icon "fa-flag" and options:
      | label | color |
      | DONE  | green |
    When I GET "/api/collections/recepten?facet.ghost=ANY"
    Then the response status is 400
    And the JSON has "error" == "unknown_facet"
    And the JSON has "key" == "ghost"

  Scenario: Unknown facet option label returns 400 unknown_facet_option
    Given the templates store design "recepten.yaml" has facet "status" with icon "fa-flag" and options:
      | label | color |
      | DONE  | green |
    When I GET "/api/collections/recepten?facet.status=GHOST"
    Then the response status is 400
    And the JSON has "error" == "unknown_facet_option"
    And the JSON has "key" == "status"
    And the JSON has "label" == "GHOST"

  Scenario: Empty facet value is silently ignored (no filter applied)
    Given the templates store design "recepten.yaml" has facet "status" with icon "fa-flag" and options:
      | label | color |
      | DONE  | green |
    And the storage form "recepten.yaml":"brood.meta.json" has facet "status" set true selected "DONE"
    When I GET "/api/collections/recepten?facet.status="
    Then the response status is 200
    And the JSON has "total" == 2

  Scenario: Facet filter on a template with no declared facets returns 400 for any facet.* param
    When I GET "/api/collections/recepten?facet.anything=X"
    Then the response status is 400
    And the JSON has "error" == "unknown_facet"
    And the JSON has "key" == "anything"

  Scenario: Count endpoint applies the same facet filter
    Given the templates store design "recepten.yaml" has facet "status" with icon "fa-flag" and options:
      | label | color |
      | DONE  | green |
      | OPEN  | red   |
    And the storage form "recepten.yaml":"brood.meta.json" has facet "status" set true selected "DONE"
    And the storage form "recepten.yaml":"pasta.meta.json" has facet "status" set true selected "OPEN"
    When I GET "/api/collections/recepten/count?facet.status=DONE"
    Then the response status is 200
    And the JSON has "total" == 1

  Scenario: Count endpoint rejects unknown facet key
    Given the templates store design "recepten.yaml" has facet "status" with icon "fa-flag" and options:
      | label | color |
      | DONE  | green |
    When I GET "/api/collections/recepten/count?facet.ghost=X"
    Then the response status is 400
    And the JSON has "error" == "unknown_facet"
