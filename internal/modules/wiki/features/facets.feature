Feature: Facets on the wiki list pages
  Templates can declare facets - orthogonal meta-tagging dimensions
  (key + icon + colored options). The wiki surfaces them on two pages:

    * the index page shows each template's facet KEYS so readers can
      see at a glance which dimensions the template carries;
    * the template detail page shows each form's SET facet chips and
      a filter strip (one <select> per facet) above the form list.

  Templates that declare no facets render no facet UI; rows whose
  storage form is missing degrade gracefully (filter strip still
  appears for the contract; chips simply don't).

  Background:
    Given a wiki handler over a stub dataprovider with two templates
    And the dataprovider has forms for "basic.yaml": "x.meta.json", "y.meta.json"

  Scenario: Index page surfaces declared facet keys per template
    Given the template "basic.yaml" declares facets:
      | flag | fa-flag  | DRAFT,gray | DONE,green |
      | size | fa-shirt | S,blue     | L,red      |
    When I GET "/"
    Then the response status is 200
    And the html body contains "data-facet-key=\"flag\""
    And the html body contains "data-facet-key=\"size\""

  Scenario: Index row without facets renders no pill block
    Given the template "basic.yaml" declares facets:
      | flag | fa-flag | DONE,green |
    When I GET "/"
    Then the response status is 200
    # Only the basic row should carry a .facet-pills container - recepten
    # contributes no facets in this scenario.
    And the html body contains "data-facet-key=\"flag\""
    And the html body contains "<li class=\"template-item\">"

  Scenario: Template page renders chips for SET facets only
    Given the template "basic.yaml" declares facets:
      | flag | fa-flag  | DRAFT,gray | DONE,green |
      | size | fa-shirt | S,blue     | L,red      |
    And the form "basic.yaml/x.meta.json" has facets:
      | flag | true  | DONE |
      | size | true  | L    |
    And the form "basic.yaml/y.meta.json" has facets:
      | flag | true  | DRAFT |
      | size | false | S     |
    When I GET "/template/basic"
    Then the response status is 200
    And the html body contains "data-facets=\"flag:DONE,size:L\""
    And the html body contains "data-facets=\"flag:DRAFT\""
    And the html body contains "facet-color--green"
    And the html body contains "facet-color--red"
    And the html body contains "facet-color--gray"
    And the html body does not contain "facet-color--blue"

  Scenario: Template page renders a filter strip with every declared option
    Given the template "basic.yaml" declares facets:
      | flag | fa-flag | DRAFT,gray | DONE,green |
    When I GET "/template/basic"
    Then the response status is 200
    And the html body contains "class=\"facet-filter\""
    And the html body contains "data-facet-filter=\"flag\""
    And the html body contains "<option value=\"DRAFT\""
    And the html body contains "<option value=\"DONE\""

  Scenario: Template without facets renders no chips and no filter strip
    When I GET "/template/basic"
    Then the response status is 200
    And the html body does not contain "class=\"facet-chip"
    And the html body does not contain "class=\"facet-filter\""
    And the html body does not contain "data-facets=\""

  Scenario: Filter strip survives missing per-form facet state
    Given the template "basic.yaml" declares facets:
      | flag | fa-flag | DONE,green |
    # No "the form ... has facets" steps - storage.LoadForm returns nil.
    When I GET "/template/basic"
    Then the response status is 200
    And the html body contains "class=\"facet-filter\""
    And the html body does not contain "class=\"facet-chip"
