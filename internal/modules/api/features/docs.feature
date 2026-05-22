Feature: REST API - Swagger UI + asset bundle
  /api/docs/ serves the embedded swagger-ui-dist shell + Formidable's
  back-link pill. The shell points at /api/openapi.json so the spec
  the UI renders is always live.

  Background:
    Given an API handler over a stub dataprovider with these templates:
      | filename       | name       | enableCollection | guidField |
      | recepten.yaml  | Recepten   | true             | guid      |

  Scenario: GET /api/docs redirects to the trailing-slash form
    When I GET "/api/docs"
    Then the response status is 301
    And the response header "Location" contains "/api/docs/"

  Scenario: GET /api/docs/ serves the swagger UI shell
    When I GET "/api/docs/"
    Then the response status is 200
    And the response content-type is "text/html; charset=utf-8"
    And the body contains "SwaggerUIBundle"
    And the body contains "/api/openapi.json"

  Scenario: Bundled CSS is served with the right MIME type
    When I GET "/api/docs/swagger-ui.css"
    Then the response status is 200
    And the response content-type is "text/css; charset=utf-8"

  Scenario: Bundled JS is served with the right MIME type
    When I GET "/api/docs/swagger-ui-bundle.js"
    Then the response status is 200
    And the response content-type is "text/javascript; charset=utf-8"

  Scenario: Standalone preset is served
    When I GET "/api/docs/swagger-ui-standalone-preset.js"
    Then the response status is 200
    And the response content-type is "text/javascript; charset=utf-8"

  Scenario: Back-link script is served and contains the marker class
    When I GET "/api/docs/swagger-back.js"
    Then the response status is 200
    And the response content-type is "text/javascript; charset=utf-8"
    And the body contains "fm-docs-back"

  Scenario: Unknown asset under /api/docs/ returns 404
    When I GET "/api/docs/nonsense"
    Then the response status is 404
