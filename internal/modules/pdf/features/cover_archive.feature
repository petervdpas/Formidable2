Feature: PDF Service - Cover Archive Export / Import
  The pdf module's Wails surface exposes ExportCoverArchive and
  ImportCoverArchive so teams can ship cover designs as portable .zip
  bundles. The .zip contains the cover .html at the root and any
  referenced images under images/. Imports validate before commit;
  exports record refs the .html mentions but that aren't on disk.

  Background:
    Given a pdf service with a fresh manager
    And the scaffolded cover library is materialised on disk

  # ─── Export ───────────────────────────────────────────────────────

  Scenario: Export a cover with bundled images
    Given a user cover "team" exists with image refs "formidable.svg, banner.png"
    And an image "banner.png" exists on disk under the covers images dir
    When I ExportCoverArchive cover "team" to "/tmp/team.zip"
    Then the service action returned no error
    And the export archive zip contains "team.html"
    And the export archive zip contains "images/formidable.svg"
    And the export archive zip contains "images/banner.png"
    And the export archive reports no missing images

  Scenario: Export a cover with no image references
    Given a user cover "plain" exists with no image refs
    When I ExportCoverArchive cover "plain" to "/tmp/plain.zip"
    Then the service action returned no error
    And the export archive zip contains "plain.html"
    And the export archive reports 0 bundled images

  Scenario: Export records image refs that don't exist on disk
    Given a user cover "half" exists with image refs "ghost.png, formidable.svg"
    When I ExportCoverArchive cover "half" to "/tmp/half.zip"
    Then the service action returned no error
    And the export archive zip contains "images/formidable.svg"
    And the export archive reports missing image "ghost.png"

  Scenario: Export refuses an unknown cover
    When I ExportCoverArchive cover "nope" to "/tmp/nope.zip"
    Then the service action returned ErrCoverArchiveNotFound

  Scenario Outline: Export refuses invalid cover names
    When I ExportCoverArchive cover "<name>" to "/tmp/x.zip"
    Then the service action returned ErrCoverArchiveInvalid

    Examples:
      | name      |
      |           |
      | sub/team  |
      | sub\team  |
      | .hidden   |
      | signature |

  Scenario: Export refuses an empty zip path
    Given a user cover "team" exists with no image refs
    When I ExportCoverArchive cover "team" to ""
    Then the service action returned ErrCoverArchiveInvalid

  # ─── Import ───────────────────────────────────────────────────────

  Scenario: Import a valid archive materialises the cover
    Given a cover archive at "/tmp/team.zip" with cover "team" and image "logo.svg"
    When I ImportCoverArchive from "/tmp/team.zip" with overwrite=false
    Then the service action returned no error
    And the imported cover name is "team"
    And the cover "team" exists on disk
    And the cover image "logo.svg" exists on disk
    And the import was not flagged as overwriting

  Scenario: Import refuses to overwrite an existing cover by default
    Given a user cover "team" exists with no image refs
    And a cover archive at "/tmp/team.zip" with cover "team" and image "logo.svg"
    When I ImportCoverArchive from "/tmp/team.zip" with overwrite=false
    Then the service action returned ErrCoverArchiveExists
    And the cover "team" on disk still matches its original body

  Scenario: Import overwrites when the overwrite flag is set
    Given a user cover "team" exists with no image refs
    And a cover archive at "/tmp/team.zip" with cover "team" and image "logo.svg"
    When I ImportCoverArchive from "/tmp/team.zip" with overwrite=true
    Then the service action returned no error
    And the import was flagged as overwriting
    And the cover "team" on disk no longer matches its original body

  Scenario: Import refuses a missing zip
    When I ImportCoverArchive from "/tmp/missing.zip" with overwrite=false
    Then the service action returned ErrCoverArchiveNotFound

  Scenario: Import refuses an empty zip path
    When I ImportCoverArchive from "" with overwrite=false
    Then the service action returned ErrCoverArchiveInvalid

  Scenario: Import refuses malformed zip bytes
    Given a malformed zip is on disk at "/tmp/bad.zip"
    When I ImportCoverArchive from "/tmp/bad.zip" with overwrite=false
    Then the service action returned ErrCoverArchiveInvalid

  Scenario: Import refuses a zip with no cover html at root
    Given a zip at "/tmp/no-html.zip" containing only entry "images/logo.png"
    When I ImportCoverArchive from "/tmp/no-html.zip" with overwrite=false
    Then the service action returned ErrCoverArchiveInvalid

  Scenario: Import refuses a zip with multiple html at root
    Given a zip at "/tmp/multi.zip" containing two cover html files
    When I ImportCoverArchive from "/tmp/multi.zip" with overwrite=false
    Then the service action returned ErrCoverArchiveInvalid

  Scenario: Import refuses a zip whose html stem is reserved
    Given a zip at "/tmp/reserved.zip" with cover named "signature"
    When I ImportCoverArchive from "/tmp/reserved.zip" with overwrite=false
    Then the service action returned ErrCoverArchiveInvalid

  Scenario: Import refuses a zip with path traversal
    Given a zip at "/tmp/trav.zip" with a traversal entry
    When I ImportCoverArchive from "/tmp/trav.zip" with overwrite=false
    Then the service action returned ErrCoverArchiveTraversal
    And no cover was materialised from the traversal zip

  Scenario: Import refuses a zip with an entry outside the expected layout
    Given a zip at "/tmp/extra.zip" with an unexpected entry "unexpected.js"
    When I ImportCoverArchive from "/tmp/extra.zip" with overwrite=false
    Then the service action returned ErrCoverArchiveInvalid

  Scenario: Import refuses a zip whose cover html fails validation
    Given a zip at "/tmp/broken.zip" with cover html that fails validation
    When I ImportCoverArchive from "/tmp/broken.zip" with overwrite=false
    Then the service action returned ErrCoverInvalid

  # ─── Round-trip ───────────────────────────────────────────────────

  Scenario: Export then Import round-trips on a clean fs
    Given a user cover "team" exists with image refs "formidable.svg"
    When I ExportCoverArchive cover "team" to "/tmp/team.zip"
    And I move the exported zip to a fresh fs at "/tmp/team.zip"
    And I ImportCoverArchive from "/tmp/team.zip" with overwrite=false on the fresh fs
    Then the service action returned no error
    And the cover "team" exists on the fresh fs
    And the cover image "formidable.svg" exists on the fresh fs
