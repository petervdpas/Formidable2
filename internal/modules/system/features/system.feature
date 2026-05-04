Feature: System filesystem operations
  The system module wraps disk operations under a configured app root.
  Every operation resolves paths relative to the root, so callers can
  pass relative paths without worrying about absolute resolution.

  Background:
    Given a system manager rooted at a temp directory

  Scenario: Save then load round-trips a file
    When I save "notes/hello.txt" with content "hello world"
    Then the file "notes/hello.txt" exists
    And loading "notes/hello.txt" returns "hello world"

  Scenario: Delete removes the file and is a no-op when missing
    Given the file "x.txt" with content "x"
    When I delete "x.txt"
    Then the file "x.txt" does not exist
    When I delete "x.txt"
    Then no error occurred

  Scenario: Copy preserves the destination when overwrite is false
    Given the file "src.txt" with content "v1"
    And the file "dst.txt" with content "existing"
    When I copy "src.txt" to "dst.txt" with overwrite false
    Then loading "dst.txt" returns "existing"

  Scenario: Copy with overwrite replaces the destination
    Given the file "src.txt" with content "v1"
    And the file "dst.txt" with content "existing"
    When I copy "src.txt" to "dst.txt" with overwrite true
    Then loading "dst.txt" returns "v1"

  Scenario: EmptyFolder removes contents but keeps the folder
    Given the file "dir/a.txt" with content "a"
    And the file "dir/sub/b.txt" with content "b"
    When I empty the folder "dir"
    Then the folder "dir" exists
    And the folder "dir" is empty

  Scenario: Journal hook receives create, update and delete events
    Given a journal stub is wired
    When I save "j.txt" with content "first"
    And I save "j.txt" with content "second"
    And I delete "j.txt"
    Then the journal recorded operations create, update, delete
