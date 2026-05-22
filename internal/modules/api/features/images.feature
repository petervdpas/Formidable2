Feature: REST API - image bytes
  GET /api/images/{tpl}/{filename} serves a stored image from a
  template's storage/<tpl>/images/ folder.

  Two formats:
    * ?format=raw (default) - raw image bytes with the image's MIME
      Content-Type. Used by the slideout's <img src> in the webview
      (via Wails AssetMiddleware) and by external HTTP clients fetching
      the file directly.
    * ?format=url - returns the data URL string
      ("data:image/...;base64,...") as text. For self-contained exports
      (single-file HTML/PDF) where the consumer wants to embed the
      image inline.

  Background:
    Given an API handler over a stub dataprovider with these templates:
      | filename       | name       | enableCollection | guidField |
      | recepten.yaml  | Recepten   | true             | guid      |
    And the storage has image "recepten.yaml":"cake.png" with bytes "PNGFAKE"

  Scenario: GET image bytes returns raw bytes by default
    When I GET "/api/images/recepten/cake.png"
    Then the response status is 200
    And the response content-type is "image/png"
    And the response body equals "PNGFAKE"

  Scenario: format=raw is the explicit default
    When I GET "/api/images/recepten/cake.png?format=raw"
    Then the response status is 200
    And the response content-type is "image/png"
    And the response body equals "PNGFAKE"

  Scenario: format=url returns the data URL string
    When I GET "/api/images/recepten/cake.png?format=url"
    Then the response status is 200
    And the response content-type is "text/plain; charset=utf-8"
    And the response body starts with "data:image/png;base64,"

  Scenario: Unknown format value is rejected
    When I GET "/api/images/recepten/cake.png?format=bogus"
    Then the response status is 400

  Scenario: Missing image returns 404
    When I GET "/api/images/recepten/ghost.png"
    Then the response status is 404

  Scenario: Path traversal in filename is rejected
    When I GET "/api/images/recepten/..%2Fescape.png"
    Then the response status is 400

  Scenario: Non-GET methods return 405
    When I POST "/api/images/recepten/cake.png" with body:
      """
      whatever
      """
    Then the response status is 405

  Scenario: HEAD returns headers without body
    When I HEAD "/api/images/recepten/cake.png"
    Then the response status is 200
    And the response content-type is "image/png"
    And the response body is empty
