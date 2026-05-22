Feature: GiGot collaboration backend
  The gigot module speaks JSON-over-HTTP to a GiGot server and walks
  the active context folder against a local ledger (.formidable/sync.json)
  to compute pending changes. Push commits diffs to the server; Pull
  fetches the server's tree and writes new/changed blobs; Reclone wipes
  managed paths first; Sync = Push + Pull. The Service layer threads
  these through OS-keychain token resolution and journal hops.

  Background:
    Given a fresh context folder
    And a fake gigot server
    And a gigot manager bound to that server

  # ── LedgerSummary ────────────────────────────────────────────────────

  Scenario: Empty context reports zero scanned, no pending
    When I call LedgerSummary on the context folder
    Then the summary scanned count is 0
    And the summary changed list is empty
    And the summary deleted list is empty

  Scenario: First sync (no ledger) lists every managed file as Changed
    Given the file "templates/a.yaml" exists with content "x"
    And the file "templates/b.yaml" exists with content "y"
    When I call LedgerSummary on the context folder
    Then the summary scanned count is 2
    And the summary has 2 changed entries
    And the summary deleted list is empty

  Scenario: Ledger in sync reports no pending
    Given the file "templates/a.yaml" exists with content "stable"
    And the ledger records "templates/a.yaml" with the current blob sha at version "v1"
    When I call LedgerSummary on the context folder
    Then the summary scanned count is 1
    And the summary changed list is empty
    And the summary deleted list is empty
    And the summary version is "v1"

  Scenario: Modified file appears in Changed
    Given the file "templates/a.yaml" exists with content "edited"
    And the ledger records "templates/a.yaml" with sha "stale" at version "v1"
    When I call LedgerSummary on the context folder
    Then the summary has 1 changed entry containing "templates/a.yaml"

  Scenario: Deleted file appears in Deleted
    Given the file "templates/kept.yaml" exists with content "k"
    And the ledger records "templates/kept.yaml" with the current blob sha at version "v1"
    And the ledger records "templates/gone.yaml" with sha "anything" at version "v1"
    When I call LedgerSummary on the context folder
    Then the summary changed list is empty
    And the summary has 1 deleted entry containing "templates/gone.yaml"

  Scenario: LedgerSummary is read-only - repeated calls don't mutate the ledger
    Given the file "templates/a.yaml" exists with content "edited"
    And the ledger records "templates/a.yaml" with sha "stale" at version "v1"
    When I call LedgerSummary on the context folder 3 times
    Then the ledger version is still "v1"

  # ── PushLocal ────────────────────────────────────────────────────────

  Scenario: PushLocal on empty context returns ErrEmptyContext
    When I push local with message ""
    Then the operation returned ErrEmptyContext

  Scenario: First-sync seeding skips paths the server already has
    Given the file "templates/a.yaml" exists with content "stable"
    And the server's tree at version "v1" lists "templates/a.yaml" with the current local blob sha
    When I push local with message ""
    Then the push result is a noop
    And the ledger version is "v1"
    And the server did not receive any commits

  Scenario: PushLocal commits a changed file
    Given the file "templates/a.yaml" exists with content "edited"
    And the ledger records "templates/a.yaml" with sha "oldsha" at version "parentV"
    And the server head is at version "parentV"
    And the server accepts the next commit as version "newV"
    When I push local with message ""
    Then the push result has pushed=1 deleted=0
    And the ledger version is "newV"

  Scenario: PushLocal forwards the user-supplied commit message
    Given the file "templates/a.yaml" exists with content "edited"
    And the ledger records "templates/a.yaml" with sha "oldsha" at version "parentV"
    And the server head is at version "parentV"
    And the server accepts the next commit as version "newV"
    When I push local with message "Refactor field labels"
    Then the captured commit message equals "Refactor field labels"

  Scenario: Blank user message falls back to the auto-generated audit string
    Given the file "templates/a.yaml" exists with content "edited"
    And the ledger records "templates/a.yaml" with sha "oldsha" at version "parentV"
    And the server head is at version "parentV"
    And the server accepts the next commit as version "newV"
    When I push local with message ""
    Then the captured commit message contains "sync"
    And the captured commit message contains "templates/a.yaml"

  Scenario: Whitespace-only message falls back to the auto-generated audit string
    Given the file "templates/a.yaml" exists with content "edited"
    And the ledger records "templates/a.yaml" with sha "oldsha" at version "parentV"
    And the server head is at version "parentV"
    And the server accepts the next commit as version "newV"
    When I push local with message "   \n\t  "
    Then the captured commit message contains "sync"

  # ── PullLocal ────────────────────────────────────────────────────────

  Scenario: PullLocal writes a new file from the server
    Given the server's tree at version "v1" lists "templates/a.yaml" with content "hello"
    When I pull local
    Then the pull result has files=1 deleted=0
    And the ledger version is "v1"
    And the local file "templates/a.yaml" contains "hello"

  Scenario: PullLocal removes a locally-managed file that vanished from the server
    Given the file "templates/gone.yaml" exists with content "x"
    And the ledger records "templates/gone.yaml" with sha "stale" at version "old"
    And the server's tree at version "new" is empty
    When I pull local
    Then the pull result has files=0 deleted=1
    And the local file "templates/gone.yaml" does not exist

  Scenario: PullLocal short-circuits when the local SHA matches the server
    Given the file "templates/a.yaml" exists with content "stable"
    And the server's tree at version "v1" lists "templates/a.yaml" with the current local blob sha
    When I pull local
    Then the pull result has files=0 deleted=0
    And the server did not receive a file fetch for "templates/a.yaml"

  Scenario: PullLocal emits the Start → Tree → Fetch → Done progress phases
    Given the server's tree at version "v1" lists "templates/a.yaml" with content "hello"
    When I pull local with progress recording
    Then the first emitted phase is Start
    And the last emitted phase is Done
    And one Tree phase was emitted with total 1
    And one Fetch phase was emitted

  Scenario: PullLocal emits a Delete phase for each vanished managed path
    Given the file "templates/gone.yaml" exists with content "x"
    And the ledger records "templates/gone.yaml" with sha "stale" at version "old"
    And the server's tree at version "new" is empty
    When I pull local with progress recording
    Then one Delete phase was emitted for "templates/gone.yaml"

  # ── Reclone ──────────────────────────────────────────────────────────

  Scenario: Reclone wipes managed paths before pulling
    Given the file "templates/old.yaml" exists with content "stale"
    And the server's tree at version "v1" is empty
    When I reclone
    Then the local file "templates/old.yaml" does not exist
    And the ledger version is "v1"

  Scenario: Reclone emits a Wipe phase before any pull-side phase
    Given the file "templates/old.yaml" exists with content "x"
    And the server's tree at version "v1" is empty
    When I reclone with progress recording
    Then a Wipe phase was emitted before any Tree, Fetch, or Delete phase

  # ── Sync ─────────────────────────────────────────────────────────────

  Scenario: Sync runs Push then Pull at the manager layer
    Given the file "templates/a.yaml" exists with content "stable"
    And the server's tree at version "v0" lists "templates/a.yaml" with the current local blob sha
    When I sync with message ""
    Then the sync result is a noop
    And the ledger version is "v0"

  # ── Service-level: token + journal hops ──────────────────────────────

  Scenario: Service resolves the bearer from the keychain
    Given a gigot service wired with a keychain entry "tok-abc"
    When the service issues Ping
    Then the captured Authorization header equals "Bearer tok-abc"

  Scenario: Service errors with ErrMissingToken when the keychain entry is missing
    Given a gigot service wired with no keychain entry
    When the service issues Ping
    Then the operation returned ErrMissingToken

  Scenario: Service PushLocal records a journal sync entry on success
    Given a gigot service with a journal recorder
    And the file "templates/a.yaml" exists with content "fresh"
    And the server head is at version "v0"
    And the server's tree at version "v0" is empty
    And the server accepts the next commit as version "afterPush"
    When the service pushes local with message ""
    Then the journal recorded one gigot sync at version "afterPush"
    And the journal did not record a remote-seen entry

  Scenario: Service PullLocal records a remote-seen entry on success
    Given a gigot service with a journal recorder
    And the server's tree at version "afterPull" is empty
    When the service pulls local
    Then the journal recorded one gigot remote-seen at version "afterPull"
    And the journal did not record a sync entry

  Scenario: Service Sync routes through wrappers so journal hops fire exactly once
    Given a gigot service with a journal recorder
    And the file "templates/a.yaml" exists with content "stable"
    And the server's tree at version "v0" lists "templates/a.yaml" with the current local blob sha
    When the service syncs with message ""
    Then the journal did not record a sync entry
    And the journal recorded exactly one gigot remote-seen entry
