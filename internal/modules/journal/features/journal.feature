Feature: Change journal
  The journal is the canonical record of every mutation under a
  context folder's `templates/` and `storage/` paths, plus per-backend
  sync markers (git, gigot). It writes JSONL to <context>/.changes.log
  and a per-backend cursor to <context>/.changes.cursor.

  Beyond the JS source, the Go journal also:
    - keeps an in-memory dedup of pending changes per backend so
      reads are O(1) instead of full-log scans
    - emits a "journal:changed" Wails event on every mutation so
      frontend pollers can subscribe instead of polling.

  Background:
    Given a system manager rooted at a temp directory
    And a journal manager wrapping that system

  Scenario: Configure seeds an empty cursor file
    When I configure the journal with backend "git"
    Then the file ".changes.cursor" exists
    And the cursor for backend "git" has ts ""

  Scenario: Init creates baseline entries from existing files
    Given the file "templates/basic.yaml" with content "name: Basic"
    And the file "storage/basic/form-1.meta.json" with content "{}"
    When I configure the journal with backend "git"
    And I initialize the journal
    Then the init result reports 2 entries created
    And the journal contains 2 baseline entries
    And the baseline entries cover "templates/basic.yaml"
    And the baseline entries cover "storage/basic/form-1.meta.json"

  Scenario: Init is idempotent when log already exists
    Given the file "templates/basic.yaml" with content "name: Basic"
    When I configure the journal with backend "git"
    And I initialize the journal
    And I initialize the journal
    Then the second init reports created false
    And the journal contains 1 baseline entries

  Scenario: RecordOp tracks templates and storage paths
    When I configure the journal with backend "git"
    And I record op "create" for "templates/basic.yaml"
    And I record op "update" for "storage/basic/form-1.meta.json"
    Then pending for backend "git" contains 2 entries
    And pending for backend "git" includes "templates/basic.yaml" with op "create"
    And pending for backend "git" includes "storage/basic/form-1.meta.json" with op "update"

  Scenario: RecordOp ignores paths outside tracked dirs
    When I configure the journal with backend "git"
    And I record op "create" for "notes/random.md"
    And I record op "create" for "config/user.json"
    Then pending for backend "git" contains 0 entries

  Scenario: Pending dedupes by path (latest op wins)
    When I configure the journal with backend "git"
    And I record op "create" for "templates/basic.yaml"
    And I record op "update" for "templates/basic.yaml"
    And I record op "delete" for "templates/basic.yaml"
    Then pending for backend "git" contains 1 entries
    And pending for backend "git" includes "templates/basic.yaml" with op "delete"

  Scenario: RecordSync advances cursor and clears pending
    When I configure the journal with backend "git"
    And I record op "create" for "templates/basic.yaml"
    And I record op "create" for "templates/people.yaml"
    Then pending for backend "git" contains 2 entries
    When I record sync for backend "git" with version "abc123" and pushed 2
    Then pending for backend "git" contains 0 entries
    And the cursor for backend "git" has version "abc123"

  Scenario: RecordSync for one backend leaves the other backend's pending untouched
    When I configure the journal with backend "git"
    And I record op "create" for "templates/basic.yaml"
    When I record sync for backend "gigot" with version "v1" and pushed 0
    Then pending for backend "git" contains 1 entries
    And pending for backend "gigot" contains 0 entries

  Scenario: RecordRemoteSeen updates only the version (not ts)
    When I configure the journal with backend "git"
    And I record op "create" for "templates/basic.yaml"
    And I record sync for backend "git" with version "v1" and pushed 1
    And I record remote seen for backend "git" with version "v2"
    Then the cursor for backend "git" has version "v2"
    And pending for backend "git" contains 0 entries

  Scenario: RecordOp emits a journal:changed event
    Given an event sink is wired
    When I configure the journal with backend "git"
    And I record op "create" for "templates/basic.yaml"
    Then the event sink received "journal:changed" with op "create" and path "templates/basic.yaml"

  Scenario: RecordSync emits a journal:changed event
    Given an event sink is wired
    When I configure the journal with backend "git"
    And I record sync for backend "git" with version "v1" and pushed 0
    Then the event sink received "journal:changed" with op "sync" and backend "git"

  Scenario: Pending is empty when no backend is configured
    When I configure the journal with backend "none"
    And I record op "create" for "templates/basic.yaml"
    Then pending for backend "git" contains 0 entries

  Scenario: Configure rebuilds pending from an existing log
    Given the file ".changes.log" with content '{"ts":"2026-05-04T10:00:00Z","op":"create","path":"templates/basic.yaml"}\n{"ts":"2026-05-04T11:00:00Z","op":"update","path":"templates/basic.yaml"}\n'
    When I configure the journal with backend "git"
    Then pending for backend "git" contains 1 entries
    And pending for backend "git" includes "templates/basic.yaml" with op "update"

  Scenario: Configure tolerates a corrupted log (skips bad lines, keeps good ones)
    Given the file ".changes.log" with content '{"ts":"2026-05-04T10:00:00Z","op":"create","path":"templates/a.yaml"}\nnot json at all\n{"ts":"","op":"create","path":"templates/b.yaml"}\n{"ts":"2026-05-04T11:00:00Z","op":"create","path":"templates/c.yaml"}\n'
    When I configure the journal with backend "git"
    Then pending for backend "git" contains 2 entries
    And pending for backend "git" includes "templates/a.yaml" with op "create"
    And pending for backend "git" includes "templates/c.yaml" with op "create"

  Scenario: Init reports no-context when journal is not configured
    When I initialize the journal
    Then the init result reason is "no-context"

  Scenario: Init reports empty when configured but no tracked files exist
    When I configure the journal with backend "git"
    And I initialize the journal
    Then the init result reason is "empty"

  Scenario: RecordOp ignores delete on a path outside tracked dirs
    When I configure the journal with backend "git"
    And I record op "delete" for "config/something.json"
    Then pending for backend "git" contains 0 entries

  Scenario: RecordOp rejects an unknown op silently
    When I configure the journal with backend "git"
    And I record op "explode" for "templates/basic.yaml"
    Then pending for backend "git" contains 0 entries

  Scenario: RecordSync with unknown backend is silently ignored
    When I configure the journal with backend "git"
    And I record op "create" for "templates/basic.yaml"
    When I record sync for backend "weird" with version "v1" and pushed 0
    Then pending for backend "git" contains 1 entries
    And the cursor for backend "git" has ts ""
    And the cursor map has no entry for backend "weird"

  Scenario: RecordRemoteSeen emits journal:changed
    Given an event sink is wired
    When I configure the journal with backend "git"
    And I record remote seen for backend "git" with version "v1"
    Then the event sink received "journal:changed" with op "sync" and backend "git"

  # ── ensureGitignorePatterns: keep .changes.log/.changes.cursor out
  # of any synced repo the user points context at. Skipped for
  # local-only mode (backend = "" or "none") and when no .git is in
  # scope. Idempotent.

  Scenario: Configure patches an existing context-level .gitignore
    Given the file ".gitignore" with content "node_modules\n"
    When I configure the journal with backend "git"
    Then the gitignore at ".gitignore" contains pattern ".changes.*"
    And the gitignore at ".gitignore" contains pattern "**/.changes.*"
    And the gitignore at ".gitignore" still contains "node_modules"

  Scenario: Configure walks up to an enclosing .git when context has no gitignore
    Given the temp dir is the repo root with a .git directory
    And the context is a subdirectory "ctx" with no gitignore
    When I configure the journal pointing at the subdirectory with backend "git"
    Then the gitignore at the repo root contains pattern ".changes.*"
    And no gitignore was created in the subdirectory

  Scenario: Local-only backend leaves gitignore untouched
    Given the temp dir is the repo root with a .git directory
    When I configure the journal with backend "none"
    Then no gitignore exists at ".gitignore"

  Scenario: Empty backend leaves gitignore untouched
    Given the temp dir is the repo root with a .git directory
    When I configure the journal with backend ""
    Then no gitignore exists at ".gitignore"

  Scenario: No git in scope means no gitignore is created
    When I configure the journal with backend "git"
    Then no gitignore exists at ".gitignore"

  Scenario: Reconfigure does not duplicate patterns
    Given the temp dir is the repo root with a .git directory
    When I configure the journal with backend "git"
    And I configure the journal with backend "git"
    Then pattern ".changes.*" appears exactly once in ".gitignore"
    And pattern "**/.changes.*" appears exactly once in ".gitignore"

  Scenario: A gitignore without trailing newline gets a separator inserted
    Given the file ".gitignore" with content "node_modules"
    When I configure the journal with backend "git"
    Then the gitignore at ".gitignore" starts with "node_modules\n"
    And the gitignore at ".gitignore" contains pattern ".changes.*"

  # ── Legacy cursor migration ──────────────────────────────────────
  # The JS predecessor wrote cursor entries as bare ts strings:
  # `{"git":"2026-05-04T10:00:00Z"}`. The Go rewrite uses object-form
  # `{"git":{"ts":"...","version":"..."}}`. Configure must accept the
  # legacy form (read path) AND migrate it on disk so subsequent
  # loads don't keep paying the parse-and-translate cost.

  Scenario: Configure migrates a legacy string-form cursor to object-form
    Given the file ".changes.cursor" with content '{"git":"2026-05-04T10:00:00Z"}\n'
    When I configure the journal with backend "git"
    Then the cursor for backend "git" has ts "2026-05-04T10:00:00Z"
    And the on-disk cursor file contains "\"ts\":\"2026-05-04T10:00:00Z\""

  Scenario: Configure leaves a modern-form cursor file untouched in shape
    Given the file ".changes.cursor" with content '{"git":{"ts":"2026-05-04T10:00:00Z","version":"abc"}}\n'
    When I configure the journal with backend "git"
    Then the cursor for backend "git" has ts "2026-05-04T10:00:00Z"
    And the cursor for backend "git" has version "abc"
