Feature: Fonts resource
  User fonts live under <AppRoot>/fonts/. They can be uploaded, listed, loaded
  back and deleted. Factory (seed) fonts scaffold on boot and are restorable.

  Background:
    Given an empty fonts library

  Scenario: An empty library lists no fonts
    Then the font list is empty

  Scenario: Saving a font makes it appear with a family name
    When I save font "Inter.woff2" with bytes "inter-bytes"
    Then the font list contains "Inter.woff2"
    And the font "Inter.woff2" has family "Inter"

  Scenario: Save round-trips through Load
    When I save font "Round.ttf" with bytes "round-bytes"
    And I load font "Round.ttf"
    Then the loaded font bytes equal "round-bytes"

  Scenario: Deleting a font removes it
    Given a saved font "Gone.otf" with bytes "x"
    When I delete font "Gone.otf"
    Then the font list does not contain "Gone.otf"

  Scenario: Deleting a missing font is a no-op
    When I delete font "Absent.woff2"
    Then no font error occurred

  Scenario: An unsupported extension is rejected
    When I save font "evil.exe" with bytes "x"
    Then the font save is rejected as invalid

  Scenario: A traversal name is rejected
    When I save font "../escape.woff2" with bytes "x"
    Then the font save is rejected as invalid

  Scenario: A factory font scaffolds, is flagged, and is restored after delete
    Given a factory font "Brand.woff2" with bytes "brand-bytes"
    When I scaffold the fonts library
    Then the font list contains "Brand.woff2"
    And the font "Brand.woff2" is a seed
    When I delete font "Brand.woff2"
    And I restore default fonts
    Then the font list contains "Brand.woff2"
