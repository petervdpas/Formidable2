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

  Scenario: Loading a missing file returns an error
    When I load "missing.txt"
    Then the load returns an error

  Scenario: AppendFile creates a missing file then appends to it
    When I append to "log/x.log" the content "line1\n"
    Then the file "log/x.log" exists
    When I append to "log/x.log" the content "line2\n"
    Then loading "log/x.log" returns "line1\nline2\n"

  Scenario: EmptyFolder errors when the directory does not exist
    When I empty the folder "nope"
    Then an error occurred

  Scenario: DeleteFolder removes a tree recursively
    Given the file "tree/a.txt" with content "a"
    And the file "tree/sub/b.txt" with content "b"
    When I delete the folder "tree"
    Then the folder "tree" does not exist

  Scenario: ExecuteCommand rejects an empty command
    When I execute the command "   "
    Then an error occurred

  Scenario: Save leaves no temp residue (atomic write discipline)
    When I save "atomic/one.txt" with content "ok"
    Then the file "atomic/one.txt" exists
    And the directory "atomic" contains exactly 1 entry

  Scenario: Save over an existing file replaces it without temp residue
    Given the file "atomic/two.txt" with content "v1"
    When I save "atomic/two.txt" with content "v2"
    Then loading "atomic/two.txt" returns "v2"
    And the directory "atomic" contains exactly 1 entry

  Scenario: Saving over a path that is a directory returns an error
    Given the directory "occupied"
    When I save "occupied" with content "oops"
    Then an error occurred
