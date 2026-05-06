Feature: /storage/* static handler
  The wiki context serves form-referenced images via
  /storage/{tpl}/images/{name} so the rendered HTML can use efficient
  external <img src="/storage/..."> URLs (the in-app slideout uses
  base64 data URLs because the Wails webview blocks file://; both
  paths flow through the same render pipeline). The handler is
  read-only; uploads go through the storage module's domain API.

  Background:
    Given a wiki handler with a stub storage holding "basic" → "logo.png" of "PNGBYTES"

  Scenario: Existing image is served with correct MIME
    When I GET "/storage/basic/images/logo.png"
    Then the response status is 200
    And the response content-type is "image/png"
    And the response body is "PNGBYTES"

  Scenario: Missing image returns 404
    When I GET "/storage/basic/images/ghost.png"
    Then the response status is 404

  Scenario: Path traversal in image name is refused
    When I GET "/storage/basic/images/..secret"
    Then the response status is 404

  Scenario: Wrong method returns 405
    When I POST "/storage/basic/images/logo.png"
    Then the response status is 405

  Scenario: Empty image name returns 404
    When I GET "/storage/basic/images/"
    Then the response status is 404
