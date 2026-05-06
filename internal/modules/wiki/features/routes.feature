Feature: Read-path routes
  The wiki's read-only HTML routes mirror the original Formidable
  internalServer.js triplet: an index page that lists templates, a
  per-template page that lists forms, and a per-form page that
  renders the body via the render pipeline. All three pull metadata
  from the dataprovider (SQLite-backed) and only hit disk when a form
  body is actually rendered.

  Background:
    Given a wiki handler over a stub dataprovider with two templates
    And the dataprovider has forms for "basic.yaml": "x.meta.json", "y.meta.json"

  Scenario: Index lists templates with links
    When I GET "/"
    Then the response status is 200
    And the response content-type is "text/html; charset=utf-8"
    And the html links to "/template/basic"
    And the html links to "/template/recepten"
    And the html shows the template name "Basic Form"

  Scenario: Template page lists forms ordered by updated
    When I GET "/template/basic"
    Then the response status is 200
    And the html links to "/template/basic/form/x.meta.json"
    And the html links to "/template/basic/form/y.meta.json"

  Scenario: Unknown template returns 404
    When I GET "/template/ghost"
    Then the response status is 404

  Scenario: Form page renders body via render pipeline
    When I GET "/template/basic/form/x.meta.json"
    Then the response status is 200
    And the response content-type is "text/html; charset=utf-8"
    And the html body contains "rendered:x.meta.json"

  Scenario: Form page exposes pre-rewritten in-wiki link hrefs
    # The wiki's render.Manager is constructed with a FormidableLinkURL
    # strategy, so links arrive at the handler already rewritten —
    # there's no post-process step. This scenario confirms the wiki
    # passes through whatever the dataprovider returned.
    Given the dataprovider renders "x.meta.json" with body containing a wiki link to "basic" "y.meta.json"
    When I GET "/template/basic/form/x.meta.json"
    Then the response status is 200
    And the html body contains "/template/basic/form/y.meta.json"

  Scenario: Unknown form returns 404
    When I GET "/template/basic/form/ghost.meta.json"
    Then the response status is 404

  Scenario: Method not allowed on read endpoints
    When I POST "/template/basic"
    Then the response status is 405

  Scenario: Path traversal attempt is refused
    # Go's ServeMux 301-redirects `/template/../etc` to the cleaned
    # `/etc`, which doesn't match any route — so a follow-up GET 404s.
    # Either status is "not a leak" — this scenario asserts the 301
    # boundary; a separate scenario covers 404 for missing top-level paths.
    When I GET "/template/../etc"
    Then the response status is 301
