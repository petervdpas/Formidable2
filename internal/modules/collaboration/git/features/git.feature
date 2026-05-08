Feature: Git collaboration backend
  The git module wraps github.com/go-git/go-git for read-only inspection
  (status, branches, log, remotes) and HTTPS clone with optional PAT.
  No system git binary required — everything runs in-process.

  Background:
    Given a fresh temp directory
    And a git manager

  # ── Repo discovery ───────────────────────────────────────────────────

  Scenario: A non-repo directory is not detected as a repo
    When I check IsGitRepo on the temp dir
    Then the result is false

  Scenario: An initialized directory is detected as a repo
    Given the temp dir is a git repo
    When I check IsGitRepo on the temp dir
    Then the result is true

  Scenario: IsGitRepo walks up from a subdirectory
    Given the temp dir is a git repo
    And a subdirectory "deep/nested" exists
    When I check IsGitRepo on "deep/nested"
    Then the result is true

  Scenario: RepoRoot errors on a plain directory
    When I get the repo root
    Then the operation returned an error

  # ── Status ───────────────────────────────────────────────────────────

  Scenario: Status reports a clean worktree after first commit
    Given the temp dir has a commit on "a.txt" with content "hello"
    When I check the status
    Then status reports clean
    And the status branch is one of "master,main"

  Scenario: Status flags a modified file
    Given the temp dir has a commit on "a.txt" with content "hello"
    And "a.txt" is rewritten to "changed"
    When I check the status
    Then status reports modified "a.txt"
    And status is not clean

  Scenario: Status flags an untracked file
    Given the temp dir has a commit on "a.txt" with content "hello"
    And the file "new.txt" exists with content "x"
    When I check the status
    Then status reports untracked "new.txt"

  Scenario: Status errors on a non-repo directory
    When I check the status
    Then the operation returned an error

  # ── Branches ─────────────────────────────────────────────────────────

  Scenario: Branches lists locals with the current branch
    Given the temp dir has a commit on "a.txt" with content "hello"
    And a local branch "feature" pointing at HEAD
    When I list branches
    Then the branches list contains "feature"

  # ── Log ──────────────────────────────────────────────────────────────

  Scenario: Log returns commits newest first
    Given the temp dir has a commit on "a.txt" with content "1" and message "first"
    And the temp dir has a commit on "a.txt" with content "2" and message "second"
    And the temp dir has a commit on "a.txt" with content "3" and message "third"
    When I read the log with limit 0
    Then the log has 3 commits
    And log entry 0 has subject "third"
    And log entry 2 has subject "first"

  Scenario: Log respects the limit argument
    Given the temp dir has a commit on "a.txt" with content "1" and message "c1"
    And the temp dir has a commit on "a.txt" with content "2" and message "c2"
    And the temp dir has a commit on "a.txt" with content "3" and message "c3"
    When I read the log with limit 2
    Then the log has 2 commits

  Scenario: Empty repo log returns no commits
    Given the temp dir is a git repo
    When I read the log with limit 0
    Then the log has 0 commits

  # ── Clone (local file:// — no network) ───────────────────────────────

  Scenario: Clone copies a local repo via file:// URL
    Given a source repo with a commit
    When I clone the source into "cloned" inside temp
    Then the destination is a git repo
    And the clone result head has 40 characters
    And the clone result branch is one of "master,main"

  Scenario: Clone refuses a non-empty destination
    Given a source repo with a commit
    And the destination "cloned" inside temp contains a leftover file
    When I clone the source into "cloned" inside temp
    Then the operation returned an error

  Scenario: Clone validates the URL field
    When I clone with an empty URL
    Then the operation returned an error

  # ── Clone (auth on the wire) ─────────────────────────────────────────

  Scenario: Clone with PAT sends HTTP Basic auth
    Given an HTTP test server that returns 401
    When I attempt to clone the test server with PAT "azure-pat-xyz"
    Then the captured Authorization header is BasicAuth for username "x-access-token" and password "azure-pat-xyz"
    And the operation returned an error

  Scenario: Anonymous clone sends no Authorization header
    Given an HTTP test server that returns 401
    When I attempt to clone the test server with no PAT
    Then no Authorization header was captured
    And the operation returned an error

  Scenario: Azure DevOps URL shape uses the same auth path
    Given an HTTP test server that returns 401
    When I attempt to clone path "/myorg/myproject/_git/myrepo" with PAT "ado-pat-xyz"
    Then the captured Authorization header is BasicAuth for username "x-access-token" and password "ado-pat-xyz"
