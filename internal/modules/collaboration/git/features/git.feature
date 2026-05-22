Feature: Git collaboration backend
  The git module wraps github.com/go-git/go-git for read-only inspection
  (status, branches, log, remotes) and HTTPS clone with optional PAT.
  No system git binary required - everything runs in-process.

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

  # ── Clone (local file:// - no network) ───────────────────────────────

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

  # ── Commit ───────────────────────────────────────────────────────────

  Scenario: Commit creates a new commit with modified files
    Given the temp dir has a commit on "a.txt" with content "v1"
    And "a.txt" is rewritten to "v2"
    When I commit with message "second"
    And I read the log with limit 0
    Then the commit succeeded
    And the log has 2 commits
    And log entry 0 has subject "second"

  Scenario: Commit picks up untracked files
    Given the temp dir has a commit on "a.txt" with content "v1"
    And the file "new.txt" exists with content "x"
    When I commit with message "add new"
    Then the commit succeeded
    And after commit status reports clean

  Scenario: Commit picks up deleted files
    Given the temp dir has a commit on "a.txt" with content "v1"
    And "a.txt" is removed from the worktree
    When I commit with message "drop a"
    Then the commit succeeded
    And after commit status reports clean

  Scenario: Commit refuses an empty message
    Given the temp dir has a commit on "a.txt" with content "v1"
    And "a.txt" is rewritten to "v2"
    When I commit with message ""
    Then the operation returned an error

  Scenario: Commit refuses a clean worktree
    Given the temp dir has a commit on "a.txt" with content "v1"
    When I commit with message "no-op"
    Then the operation returned an error

  Scenario: Commit refuses an empty author
    Given the temp dir has a commit on "a.txt" with content "v1"
    And "a.txt" is rewritten to "v2"
    When I commit with message "x" and empty author
    Then the operation returned an error

  # ── Discard ──────────────────────────────────────────────────────────

  Scenario: Discard restores a modified file from HEAD
    Given the temp dir has a commit on "a.txt" with content "v1"
    And "a.txt" is rewritten to "v2"
    When I discard "a.txt"
    Then file "a.txt" exists with content "v1"

  Scenario: Discard removes an untracked file
    Given the temp dir is a git repo
    And the file "junk.txt" exists with content "x"
    When I discard "junk.txt"
    Then file "junk.txt" does not exist

  Scenario: Discard restores a worktree-deleted file
    Given the temp dir has a commit on "a.txt" with content "v1"
    And "a.txt" is removed from the worktree
    When I discard "a.txt"
    Then file "a.txt" exists with content "v1"

  Scenario: Discard refuses an empty file path
    Given the temp dir has a commit on "a.txt" with content "v1"
    When I discard ""
    Then the operation returned an error

  Scenario: Discard refuses a traversal path
    Given the temp dir has a commit on "a.txt" with content "v1"
    When I discard "../escape"
    Then the operation returned an error

  # ── Fetch / Push (file:// - no network) ──────────────────────────────

  Scenario: Push advances the remote
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    And a new commit "x.txt" with content "x" in "client"
    When I push from "client"
    Then the push succeeded
    And push is not already-up-to-date

  Scenario: Push reports already-up-to-date when nothing to send
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    When I push from "client"
    Then the push succeeded
    And push is already-up-to-date

  Scenario: Push refuses an empty path
    When I push with an empty path
    Then the operation returned an error

  Scenario: Fetch updates the tracking ref after the source advances
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    And the bare repo gains another commit
    When I fetch from "client"
    Then the fetch succeeded
    And fetch is not already-up-to-date

  Scenario: Fetch on an unchanged remote reports already-up-to-date
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    When I fetch from "client"
    Then the fetch succeeded
    And fetch is already-up-to-date

  Scenario: Fetch refuses an empty path
    When I fetch with an empty path
    Then the operation returned an error

  Scenario: Pull advances the local branch when remote has new commits
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    And the bare repo gains another commit
    When I pull from "client"
    Then the pull succeeded
    And pull is not already-up-to-date

  Scenario: Pull reports already-up-to-date when remote is unchanged
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    When I pull from "client"
    Then the pull succeeded
    And pull is already-up-to-date

  Scenario: Pull refuses an empty path
    When I pull with an empty path
    Then the operation returned an error

  Scenario: Pull refuses a dirty worktree
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    And the bare repo gains another commit
    And "seed.txt" is rewritten to "dirty" inside "client"
    When I pull from "client"
    Then the operation returned an error

  Scenario: Pull refuses divergent history
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    And the bare repo gains another commit
    And a new commit "local.txt" with content "local" in "client"
    When I pull from "client"
    Then the operation returned an error

  # ── Service-level wiring: Push/Pull inform the journal ─────────────
  # Service is the layer that auto-fills the PAT from keychain AND
  # reports outbound (Push) / remote-seen (Pull) events to the
  # journal. These scenarios use a fakeJournal recorder so we can
  # assert what the Service told the journal - independent of the
  # journal module's internal state machine.

  Scenario: Service Push that advances the remote records a sync entry
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    And a journal-recording git service
    And a new commit "x.txt" with content "x" in "client"
    When I push from "client" via the service
    Then the push succeeded
    And the journal recorded 1 sync for backend "git"
    And the journal recorded 0 remote-seens
    And the recorded sync version equals the push NewHead

  Scenario: Service Push that is already-up-to-date records remote-seen only
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    And a journal-recording git service
    When I push from "client" via the service
    Then the push succeeded
    And push is already-up-to-date
    And the journal recorded 0 syncs
    And the journal recorded 1 remote-seen for backend "git"

  Scenario: Service Push that errors records nothing
    Given a journal-recording git service
    When I push with an empty path via the service
    Then the operation returned an error
    And the journal recorded 0 syncs
    And the journal recorded 0 remote-seens

  Scenario: Service Push on a non-repo path records nothing
    Given a journal-recording git service
    When I push from "not-a-repo" via the service
    Then the operation returned an error
    And the journal recorded 0 syncs
    And the journal recorded 0 remote-seens

  Scenario: Service Pull that advances local records remote-seen
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    And a journal-recording git service
    And the bare repo gains another commit
    When I pull from "client" via the service
    Then the pull succeeded
    And pull is not already-up-to-date
    And the journal recorded 0 syncs
    And the journal recorded 1 remote-seen for backend "git"
    And the recorded remote-seen version equals the pull NewHead

  Scenario: Service Pull that is already-up-to-date still records remote-seen
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    And a journal-recording git service
    When I pull from "client" via the service
    Then the pull succeeded
    And pull is already-up-to-date
    And the journal recorded 0 syncs
    And the journal recorded 1 remote-seen for backend "git"

  Scenario: Service Pull on a dirty worktree records nothing
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    And a journal-recording git service
    And the bare repo gains another commit
    And "seed.txt" is rewritten to "dirty" inside "client"
    When I pull from "client" via the service
    Then the operation returned an error
    And the journal recorded 0 syncs
    And the journal recorded 0 remote-seens

  Scenario: Service Pull with empty path records nothing
    Given a journal-recording git service
    When I pull with an empty path via the service
    Then the operation returned an error
    And the journal recorded 0 syncs
    And the journal recorded 0 remote-seens

  Scenario: Nil journal does not panic on Push
    Given a git service with no journal recorder
    And a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    When I push from "client" via the service
    Then the push succeeded

  Scenario: Nil journal does not panic on Pull
    Given a git service with no journal recorder
    And a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    When I pull from "client" via the service
    Then the pull succeeded

  # ── PullWithStash: journal-aware auto-stash + pull + restore ───────
  # The journal's Pending(backend) drives which paths to snapshot.
  # Conflicts (pull moved a path under the stash) are returned in a
  # dedicated bucket so the UI can offer manual recovery instead of
  # silently overwriting.

  Scenario: Stash-pull round-trips a user edit when remote touches an unrelated file
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    And a journal-recording git service
    And the journal pending for "git" includes "seed.txt" with op "update"
    And "seed.txt" is rewritten to "user-edit" inside "client"
    And the bare repo gains another commit
    When I pull-with-stash from "client" via the service
    Then the pull succeeded
    And pull is not already-up-to-date
    And the stash result has 0 overrides
    And the stash result restored "seed.txt"
    And file "seed.txt" inside "client" has content "user-edit"
    And no stash directory exists under "client"

  Scenario: Stash-pull on a non-record file overrides silently with author info
    # seed.txt is plain text (NOT storage/<tpl>/<n>.meta.json), so
    # recmerge can't reconcile. Pull wins on disk; the override is
    # reported with the post-pull commit author.
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    And a journal-recording git service
    And the journal pending for "git" includes "seed.txt" with op "update"
    And "seed.txt" is rewritten to "user-edit" inside "client"
    And the bare repo rewrites "seed.txt" to "remote-edit"
    When I pull-with-stash from "client" via the service
    Then the pull succeeded
    And pull is not already-up-to-date
    And the stash result has 1 override
    And the stash result has "seed.txt" in overrides
    And the override for "seed.txt" names an author
    And file "seed.txt" inside "client" has content "remote-edit"
    And no stash directory exists under "client"

  Scenario: Stash-pull with no journal pending degrades to a normal pull
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    And a journal-recording git service
    And the bare repo gains another commit
    When I pull-with-stash from "client" via the service
    Then the pull succeeded
    And pull is not already-up-to-date
    And the stash result has 0 overrides
    And no stash directory exists under "client"

  Scenario: Stash-pull leaves unrelated dirt untouched
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    And a journal-recording git service
    And the journal pending for "git" includes "seed.txt" with op "update"
    And "seed.txt" is rewritten to "user-edit" inside "client"
    And the file "scratch.txt" exists with content "untouched" inside "client"
    And the bare repo gains another commit
    When I pull-with-stash from "client" via the service
    Then the pull succeeded
    And the stash result has 0 overrides
    And file "scratch.txt" inside "client" has content "untouched"

  # ── Sysgit dispatch (self-cloned mode) ────────────────────────────────
  # When the user flips the "cloned outside Formidable" toggle, the
  # Service routes Fetch/Push/Pull through a system-git surface so the
  # OS credential helper resolves auth. These scenarios use a fake
  # Sysgit recorder so we can prove the dispatch decision without
  # spawning the real binary - and prove the fallback path stays
  # untouched whenever the toggle is off or the binary is missing.

  Scenario: Toggle off - Fetch stays on the go-git path
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    And a journal-recording git service
    And a fake sysgit recorder marked available
    And the self-cloned toggle is off
    When I fetch from "client" via the service
    Then the fake sysgit recorded 0 calls

  Scenario: Toggle on but binary missing - Fetch falls back to go-git
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    And a journal-recording git service
    And a fake sysgit recorder marked unavailable
    And the self-cloned toggle is on
    When I fetch from "client" via the service
    Then the fake sysgit recorded 0 calls

  Scenario: Toggle on with available binary - Fetch shells out
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    And a journal-recording git service
    And a fake sysgit recorder marked available
    And the self-cloned toggle is on
    When I fetch from "client" via the service
    Then the fake sysgit recorded 1 call
    And the fake sysgit was asked for remote "origin"

  Scenario: Toggle on with available binary - Push records a sync entry
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    And a journal-recording git service
    And a fake sysgit recorder marked available
    And the self-cloned toggle is on
    When I push from "client" via the service
    Then the push succeeded
    And the fake sysgit recorded 1 call
    And the journal recorded 1 sync for backend "git"
    And the journal recorded 0 remote-seens
    And the recorded sync version equals the push NewHead

  Scenario: Toggle on with available binary - up-to-date Push records remote-seen
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    And a journal-recording git service
    And a fake sysgit recorder marked available and reporting up-to-date
    And the self-cloned toggle is on
    When I push from "client" via the service
    Then the push succeeded
    And push is already-up-to-date
    And the journal recorded 0 syncs
    And the journal recorded 1 remote-seen for backend "git"

  Scenario: Toggle on with available binary - Pull records remote-seen
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    And a journal-recording git service
    And a fake sysgit recorder marked available
    And the self-cloned toggle is on
    When I pull from "client" via the service
    Then the pull succeeded
    And the fake sysgit recorded 1 call
    And the journal recorded 0 syncs
    And the journal recorded 1 remote-seen for backend "git"

  Scenario: Sysgit Push that errors records nothing
    Given a bare repo seeded with one commit
    And a clone of the bare repo at "client" inside temp
    And a journal-recording git service
    And a fake sysgit recorder marked available with error "auth required"
    And the self-cloned toggle is on
    When I push from "client" via the service
    Then the operation returned an error
    And the journal recorded 0 syncs
    And the journal recorded 0 remote-seens

  Scenario: Sysgit Push on a non-repo path leaves the journal alone
    # sysgit "succeeds" (fake returns nil) but the path isn't a repo,
    # so headHash returns "" - recording an empty version would
    # corrupt the cursor.
    Given a journal-recording git service
    And a fake sysgit recorder marked available
    And the self-cloned toggle is on
    When I push from "not-a-repo" via the service
    Then the push succeeded
    And the push NewHead is empty
    And the journal recorded 0 syncs
    And the journal recorded 0 remote-seens
