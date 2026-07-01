Feature: Wiki presentations surface
  Presentation templates are surfaced separately from the normal form pages:
  the index lists them under Presentations with per-deck play links, and each
  deck plays as a full-screen reveal.js slideshow.

  Scenario: A presentation lists its decks and plays them
    Given a wiki handler with a presentation template "talk.yaml" named "Talk 2026" with decks:
      | value | label     |
      | intro | Intro     |
      | deep  | Deep dive |
    When I GET "/"
    Then the response status is 200
    And the html body contains "Presentations"
    And the html links to "/template/talk/slides/intro"
    And the html links to "/template/talk/slides/deep"

    When I GET "/template/talk/slides/deep"
    Then the response status is 200
    And the html body contains "data-width"
    And the html body contains "/_/js/reveal.js"
    And the html body contains "/_/js/deck-init.js"

  Scenario: A non-presentation template has no deck route
    Given a wiki handler over a stub dataprovider with two templates
    When I GET "/template/basic/slides"
    Then the response status is 404
