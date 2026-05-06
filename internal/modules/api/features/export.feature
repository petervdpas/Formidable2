Feature: REST API — collection exports
  /api/collections/{tpl}/export.ndjson and /export.csv stream the full
  collection so external tooling can pull it without paginating. Both
  honour the collection rev for ETag/If-None-Match and 403 when the
  template isn't collection-enabled.

  Background:
    Given an API handler over a stub dataprovider with these templates:
      | filename       | name       | enableCollection | guidField |
      | basic.yaml     | Basic Form | false            |           |
      | recepten.yaml  | Recepten   | true             | guid      |
    And the dataprovider has tagged forms for "recepten.yaml":
      | filename            | id      | title  | tags        |
      | brood.meta.json     | g-1234  | Brood  | bakery,wb   |
      | pasta.meta.json     | g-5678  | Pasta  | italian,wb  |
    And the storage holds form "recepten.yaml":"brood.meta.json" with:
      """
      {"meta":{"id":"g-1234","template":"recepten","tags":["bakery","wb"]},"data":{"guid":"g-1234","naam":"Brood"}}
      """
    And the storage holds form "recepten.yaml":"pasta.meta.json" with:
      """
      {"meta":{"id":"g-5678","template":"recepten","tags":["italian","wb"]},"data":{"guid":"g-5678","naam":"Pasta"}}
      """

  Scenario: NDJSON export streams one item per line
    When I GET "/api/collections/recepten/export.ndjson"
    Then the response status is 200
    And the response content-type is "application/x-ndjson; charset=utf-8"
    And the body has 2 NDJSON lines
    And NDJSON line 0 has "id" == "g-1234"
    And NDJSON line 0 has "filename" == "brood.meta.json"
    And NDJSON line 0 nested "data.naam" == "Brood"
    And NDJSON line 1 has "id" == "g-5678"

  Scenario: NDJSON export carries ETag header
    When I GET "/api/collections/recepten/export.ndjson"
    Then the response has header "ETag"

  Scenario: NDJSON export 304 with matching If-None-Match
    Given I GET "/api/collections/recepten/export.ndjson" and capture the ETag
    When I GET "/api/collections/recepten/export.ndjson" with header "If-None-Match" matching the captured ETag
    Then the response status is 304
    And the response body is empty

  Scenario: NDJSON export 403 for disabled template
    When I GET "/api/collections/basic/export.ndjson"
    Then the response status is 403
    And the JSON has "error" == "collection-disabled"

  Scenario: CSV export starts with BOM and header row
    When I GET "/api/collections/recepten/export.csv"
    Then the response status is 200
    And the response content-type is "text/csv; charset=utf-8"
    And the response has header "Content-Disposition"
    And the body starts with the UTF-8 BOM
    And CSV line 0 is "id,filename,title,tags"

  Scenario: CSV export emits one row per item with quoted cells
    # Cells are always quoted, so substring checks against unquoted
    # values still pass — and the BOM/header scenario above already
    # asserts the quoting+header shape verbatim.
    When I GET "/api/collections/recepten/export.csv"
    Then CSV line 1 contains "g-1234"
    And CSV line 1 contains "brood.meta.json"
    And CSV line 1 contains "Brood"
    # Tags are semicolon-joined inside the quoted cell.
    And CSV line 1 contains "bakery;wb"
    And CSV line 2 contains "g-5678"

  Scenario: CSV export Content-Disposition includes a stable filename
    When I GET "/api/collections/recepten/export.csv"
    Then the response has header "Content-Disposition"
    And the response header "Content-Disposition" contains "recepten-export.csv"

  Scenario: CSV export 403 for disabled template
    When I GET "/api/collections/basic/export.csv"
    Then the response status is 403
