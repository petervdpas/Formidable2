Feature: Wiki chrome (CSS / JS / topbar)
  The wiki pages share a layout with a fixed topbar (logo + back/forward
  + breadcrumbs + search) and link to embedded CSS/JS from /_/. The
  formidable-prose stylesheet is streamed from the render module so
  the wiki view stays byte-identical to the in-app slideout's preview.

  Background:
    Given a wiki handler over a stub dataprovider with two templates

  Scenario: Index page links the chrome assets
    When I GET "/"
    Then the response status is 200
    And the html body contains "/_/css/base.css"
    And the html body contains "/_/css/header.css"
    And the html body contains "/_/css/content.css"
    And the html body contains "/_/css/formidable-prose.css"
    And the html body contains "/_/js/crumbs.js"
    And the html body contains "/_/js/filter.js"
    And the html body has element id "topbar"
    And the html body has element id "crumbs"
    And the html body has element id "q"

  Scenario: Static CSS asset is served from /_/
    When I GET "/_/css/base.css"
    Then the response status is 200
    And the response content-type is "text/css; charset=utf-8"

  Scenario: Static JS asset is served from /_/
    When I GET "/_/js/crumbs.js"
    Then the response status is 200
    And the response content-type is "text/javascript; charset=utf-8"

  Scenario: Wiki logo is served from /_/img
    When I GET "/_/img/logo.png"
    Then the response status is 200
    And the response content-type is "image/png"

  Scenario: formidable-prose.css comes from the render module
    When I GET "/_/css/formidable-prose.css"
    Then the response status is 200
    And the html body contains ".formidable-prose"

  Scenario: Missing static asset returns 404
    When I GET "/_/css/ghost.css"
    Then the response status is 404

  Scenario: Path traversal under /_/ is refused
    When I GET "/_/css/..%2Fsecrets.txt"
    Then the response status is 404
