Feature: PDF Service - Cover Image Library
  Cover HTML files reference logo / banner assets that live under
  <AppRoot>/pdf/covers/images/. The Wails service exposes a
  List / Save / Delete surface so the user can curate that library
  from the UI without dropping into the filesystem. Saved bodies
  flow as base64 over the JSON Wails boundary; ListCoverImages flags
  seed images (currently formidable.svg) so the frontend can phrase
  the destructive action as Reset rather than Delete.

  Background:
    Given a pdf cover image library scaffolded on disk

  Scenario: List finds the bundled seed image after scaffold
    When I ListCoverImages
    Then the cover image list contains "formidable.svg"
    And cover image "formidable.svg" is flagged as a seed

  Scenario: Save persists a user-uploaded image
    When I SaveCoverImage "team-logo.png" with bytes "PNG-DATA-HERE"
    Then the cover image action returned no error
    And the cover image list contains "team-logo.png"
    And cover image "team-logo.png" is not flagged as a seed

  Scenario: Save round-trips through LoadCoverImage
    When I SaveCoverImage "round.png" with bytes "round-trip-bytes"
    And I LoadCoverImage "round.png"
    Then the loaded cover image bytes equal "round-trip-bytes"

  Scenario: Save rejects path traversal
    When I SaveCoverImage "../escape.png" with bytes "data"
    Then the cover image action returned an error

  Scenario: Save rejects unsupported extension
    When I SaveCoverImage "logo.exe" with bytes "data"
    Then the cover image action returned an error

  Scenario: Save rejects empty body
    When I SaveCoverImage "empty.png" with bytes ""
    Then the cover image action returned an error

  Scenario: Delete a user image clears it from the library
    Given a cover image "stale.png" exists with bytes "to-be-removed"
    When I DeleteCoverImage "stale.png"
    Then the cover image action returned no error
    And the cover image list does not contain "stale.png"

  Scenario: Delete is a no-op for a missing image
    When I DeleteCoverImage "never-was.png"
    Then the cover image action returned no error

  Scenario: Delete a seed image, then a fresh scaffold restores it
    When I DeleteCoverImage "formidable.svg"
    Then the cover image list does not contain "formidable.svg"
    When the cover image library is scaffolded again
    Then the cover image list contains "formidable.svg"
