Feature: User configuration management
  Formidable2 keeps user preferences in a JSON profile file under
  config/, with .boot.json pointing at the active profile. The config
  manager seeds defaults on first run and supports multiple profiles.

  Background:
    Given a config manager rooted at a fresh temp directory

  Scenario: First run seeds defaults
    Then the file "config/.boot.json" exists
    And the file "config/user.json" exists
    And the active profile filename is "user.json"
    And the loaded config has theme "light"
    And the loaded config has language "en"
    And the loaded config has internal_server_port 8383
    And the loaded config has font_size 14

  Scenario: Update merges into the cached config and persists
    When I update the config with theme "dark" and font_size 16
    Then the loaded config has theme "dark"
    And the loaded config has font_size 16
    And the loaded config has language "en"
    And the disk file "config/user.json" reflects theme "dark"

  Scenario: Switching profiles loads the new one
    Given a profile "work.json" exists with theme "purplish"
    When I switch the active profile to "work.json"
    Then the active profile filename is "work.json"
    And the loaded config has theme "purplish"
    And .boot.json's active_profile is "work.json"

  Scenario: Active profile cannot be deleted
    When I delete the profile "user.json"
    Then the delete result is failure with code "active_profile"

  Scenario: .boot.json cannot be deleted
    When I delete the profile ".boot.json"
    Then the delete result is failure with code "boot_forbidden"

  Scenario: Listing profiles excludes the boot pointer
    Given a profile "work.json" exists with theme "dark"
    When I list available profiles
    Then the profile list contains "user.json"
    And the profile list contains "work.json"
    And the profile list does not contain ".boot.json"

  Scenario: Virtual structure auto-creates context layout
    When I request the virtual structure
    Then the folder "templates" exists
    And the folder "storage" exists

  Scenario: Virtual structure picks up new templates after dirty marker
    Given the file "templates/basic.yaml" with content "name: Basic"
    When I dirty the virtual structure
    And I request the virtual structure
    Then the virtual structure contains template "basic"
    And the folder "storage/basic" exists
    And the folder "storage/basic/images" exists

  Scenario: Deleting a non-existent profile returns not_found
    When I delete the profile "ghost.json"
    Then the delete result is failure with code "not_found"

  Scenario: Exporting a non-existent profile returns not_found
    When I export the profile "ghost.json" to "exports/out.json"
    Then the export result is failure with code "not_found"

  Scenario: Importing the same profile twice without overwrite is rejected
    Given an external profile file "import.json" exists with theme "dark"
    When I import the profile from "import.json" as "alt.json"
    Then the import result is success with filename "alt.json"
    When I import the profile from "import.json" as "alt.json"
    Then the import result is failure with code "exists"

  Scenario: Importing .boot.json as a profile is rejected
    Given an external profile file "import.json" exists with theme "dark"
    When I import the profile from "import.json" as ".boot.json"
    Then the import result is failure with code "boot_forbidden"

  Scenario: Legacy boot.json is migrated to .boot.json on first read
    Given the file "config/boot.json" with content '{"active_profile":"user.json"}'
    When I reinitialize the config manager
    Then the file "config/.boot.json" exists
    And the file "config/boot.json" does not exist
    And the active profile filename is "user.json"

  Scenario: Importing an invalid file is rejected
    Given an external file "bad.json" exists with content "not json {[}"
    When I import the profile from "bad.json" as "alt.json"
    Then the import result is failure with code "invalid_config"

  Scenario: Loading a config with missing fields fills defaults and rewrites the file
    Given the file "config/user.json" with content '{"theme":"dark"}'
    And I invalidate the config cache
    When I load the config
    Then the loaded config has theme "dark"
    And the loaded config has language "en"
    And the loaded config has internal_server_port 8383
    And the disk file "config/user.json" reflects theme "dark"

  Scenario: First run seeds default status buttons
    Then the status button "reloader" is on
    And the status button "charpicker" is on
    And the status button "language" is on
    And the status button "gitquick" is off
    And the status button "gigotload" is off

  Scenario: Toggling a status button persists and leaves siblings untouched
    When I set status button "language" to off
    Then the status button "language" is off
    And the status button "reloader" is on
    And the status button "charpicker" is on
    And the disk file "config/user.json" reflects status button "language" off
    And the disk file "config/user.json" reflects status button "reloader" on

  Scenario: Re-enabling a status button persists
    When I set status button "gitquick" to on
    Then the status button "gitquick" is on
    And the disk file "config/user.json" reflects status button "gitquick" on

  Scenario: Loading a config without status_buttons fills the defaults
    Given the file "config/user.json" with content '{"theme":"dark"}'
    And I invalidate the config cache
    When I load the config
    Then the status button "reloader" is on
    And the status button "charpicker" is on
    And the status button "language" is on
    And the status button "gitquick" is off
    And the status button "gigotload" is off

  # ──────────────────────────────────────────────────────────────────────
  # EnabledTemplates - per-profile template curation. The list is the
  # literal set of visible templates: empty means none are visible.
  # Deleted templates are silently pruned on read.
  # ──────────────────────────────────────────────────────────────────────

  Scenario: Empty enabled list reports no template as enabled
    Then template "basic.yaml" is not enabled
    And template "anything-at-all.yaml" is not enabled

  Scenario: Populated enabled list only allows listed templates
    When I set the enabled templates to "basic.yaml,report.yaml"
    Then template "basic.yaml" is enabled
    And template "report.yaml" is enabled
    And template "hidden.yaml" is not enabled

  Scenario: Enabled list persists across save and load
    When I set the enabled templates to "basic.yaml"
    And I invalidate the config cache
    And I load the config
    Then the enabled templates list is "basic.yaml"
    And the disk file "config/user.json" contains "enabled_templates"

  Scenario: Reconcile prunes templates that no longer exist on disk
    Given the live templates folder contains "basic.yaml,report.yaml"
    When I set the enabled templates to "basic.yaml,deleted.yaml,report.yaml"
    And I reconcile enabled templates
    Then the enabled templates list is "basic.yaml,report.yaml"
    And the disk file "config/user.json" does not contain "deleted.yaml"

  Scenario: Reconcile is a no-op when the enabled list is empty
    Given the live templates folder contains "basic.yaml"
    When I reconcile enabled templates
    Then the enabled templates list is empty

  Scenario: List enabled templates returns the intersection with the live folder
    Given the live templates folder contains "alpha.yaml,beta.yaml,gamma.yaml"
    When I set the enabled templates to "gamma.yaml,alpha.yaml"
    And I list enabled templates
    Then the listed enabled templates are "alpha.yaml,gamma.yaml"

  Scenario: List enabled templates returns none when the list is empty
    Given the live templates folder contains "alpha.yaml,beta.yaml"
    When I list enabled templates
    Then the listed enabled templates are empty

  Scenario: List enabled templates self-heals stale entries
    Given the live templates folder contains "basic.yaml"
    When I set the enabled templates to "basic.yaml,deleted.yaml"
    And I list enabled templates
    Then the listed enabled templates are "basic.yaml"
    And the enabled templates list is "basic.yaml"

  Scenario: List enabled templates returns none when prune empties the list
    Given the live templates folder contains "basic.yaml,report.yaml"
    When I set the enabled templates to "removed.yaml"
    And I list enabled templates
    Then the listed enabled templates are empty

  Scenario: Without a template lister wired, list enabled returns empty
    When I clear the template lister
    And I list enabled templates
    Then the listed enabled templates are empty

  Scenario: Empty filename is never enabled
    When I set the enabled templates to "basic.yaml"
    Then template "" is not enabled
