Feature: SingleFileRepository (SFR)
  The sfr module saves, loads, lists and deletes single-file blobs in
  a caller-provided directory using a base filename. It normalizes the
  filename by stripping `.md` and the configured extension, then
  re-attaching the extension. Default extension is `.meta.json` and
  default format is JSON. Base filenames must not contain path
  separators — escaping the directory via `..` or `/` is rejected.

  Background:
    Given a system manager rooted at a temp directory
    And an sfr manager wrapping that system

  Scenario: Save then load round-trips a JSON object
    When I save under "storage/basic" with base "form-1" and data {"title":"hi","n":2}
    Then the file "storage/basic/form-1.meta.json" exists
    When I load under "storage/basic" with base "form-1"
    Then the loaded JSON has field "title" equal to "hi"
    And the loaded JSON has field "n" equal to 2

  Scenario: Base filename normalisation strips .md
    When I save under "storage/basic" with base "form-1.md" and data {"x":1}
    Then the file "storage/basic/form-1.meta.json" exists

  Scenario: Base filename normalisation strips configured extension
    When I save under "storage/basic" with base "form-1.meta.json" and data {"x":1}
    Then the file "storage/basic/form-1.meta.json" exists
    And the file "storage/basic/form-1.meta.json.meta.json" does not exist

  Scenario: Listing files filters by default extension
    Given a saved entry under "storage/basic" with base "form-1" and data {"x":1}
    And a saved entry under "storage/basic" with base "form-2" and data {"x":2}
    And the file "storage/basic/notes.txt" with content "stray"
    When I list files under "storage/basic"
    Then the list contains "form-1.meta.json"
    And the list contains "form-2.meta.json"
    And the list does not contain "notes.txt"

  Scenario: Listing files supports a custom extension
    Given the file "storage/basic/a.txt" with content "a"
    And the file "storage/basic/b.txt" with content "b"
    And the file "storage/basic/c.json" with content "{}"
    When I list files under "storage/basic" with extension ".txt"
    Then the list contains "a.txt"
    And the list contains "b.txt"
    And the list does not contain "c.json"

  Scenario: Delete removes the file
    Given a saved entry under "storage/basic" with base "form-1" and data {"x":1}
    When I delete under "storage/basic" with base "form-1"
    Then the file "storage/basic/form-1.meta.json" does not exist

  Scenario: Loading a missing file returns an error
    When I load under "storage/basic" with base "missing"
    Then the load returns an error

  Scenario: Path traversal via parent dir is rejected
    When I save under "storage/basic" with base "../escape" and data {"x":1}
    Then the save result is a failure
    And the file "storage/escape.meta.json" does not exist

  Scenario: Path traversal via slash is rejected
    When I save under "storage/basic" with base "subdir/file" and data {"x":1}
    Then the save result is a failure

  Scenario: Empty base filename is rejected
    When I save under "storage/basic" with base "" and data {"x":1}
    Then the save result is a failure

  Scenario: Backslash in base filename is rejected
    When I save under "storage/basic" with base "win\\path" and data {"x":1}
    Then the save result is a failure

  Scenario: Lone dot as base filename is rejected
    When I save under "storage/basic" with base "." and data {"x":1}
    Then the save result is a failure

  Scenario: Delete on a missing entry is a no-op (no error)
    When I delete under "storage/basic" with base "ghost"
    Then the file "storage/basic/ghost.meta.json" does not exist

  Scenario: Listing a missing directory returns an error
    When I list files under "never/created"
    Then the list returns an error
