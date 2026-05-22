Feature: PDF Service - Stage 2 activation
  The pdf module's Wails surface exposes Status / Activate / Deactivate
  / ExportPDF. Activation adopts a system or managed-cache Chrome /
  Chromium binary and persists per-machine to <AppRoot>/config/
  pdf-state.json. Export is gated on activation; until Stage 4 lands,
  active Export returns a "Stage 4 stub" error and inactive Export
  returns ErrPDFNotActivated.

  Background:
    Given a pdf service with a fresh manager

  Scenario: Status while inactive
    When I ask Status from the service
    Then the status reports not active
    And the status source is "unset"
    And the status browser bin is empty
    And the status version is empty

  Scenario: ExportPDF refuses to render without activation
    When I ExportPDF through the service for form "abc-123" with no options
    Then the service action returned ErrPDFNotActivated
    And the export result is empty

  Scenario: ExportPDF with empty form GUID still surfaces ErrPDFNotActivated
    When I ExportPDF through the service for form "" with no options
    Then the service action returned ErrPDFNotActivated

  Scenario: Auto-pick Activate with no Chrome anywhere
    When I Activate through the service with no overrides
    Then the service action returned ErrNoBrowserFound
    And the status reports not active

  Scenario: Activate adopts an explicit valid path
    Given a fake browser exists at "/usr/bin/chromium" reporting version "Chromium 148.0"
    When I Activate through the service with BrowserBin "/usr/bin/chromium"
    Then the service action returned no error
    And the status reports active
    And the status browser bin is "/usr/bin/chromium"
    And the status source is "system"

  Scenario: Activate rejects a missing explicit path
    When I Activate through the service with BrowserBin "/no/such/chrome"
    Then the service action returned ErrInvalidBrowserBin
    And the status reports not active

  Scenario: Deactivate after Activate clears the status
    Given a fake browser exists at "/usr/bin/chromium" reporting version "Chromium 148.0"
    And the service has been activated with BrowserBin "/usr/bin/chromium"
    When I Deactivate through the service
    Then the service action returned no error
    And the status reports not active
    And the status source is "unset"
