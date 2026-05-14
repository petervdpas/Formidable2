Feature: PDF Service — Stage 1 skeleton
  The pdf module is the Formidable surface for picoloom-based PDF
  export. Stage 1 ships only the type surface + Wails bindings;
  every action returns ErrPDFNotActivated until Stage 2 lands the
  Chrome activation flow.

  Background:
    Given a pdf service with a fresh manager

  Scenario: Status while inactive
    When I ask Status from the service
    Then the status reports not active
    And the status source is "unset"
    And the status browser bin is empty
    And the status version is empty

  Scenario: Activate is blocked until Stage 2
    When I Activate through the service with no overrides
    Then the service action returned ErrPDFNotActivated
    And the status reports not active

  Scenario: Deactivate is a no-op while inactive
    When I Deactivate through the service
    Then the service action returned ErrPDFNotActivated
    And the status reports not active

  Scenario: ExportPDF refuses to render without activation
    When I ExportPDF through the service for form "abc-123" with no options
    Then the service action returned ErrPDFNotActivated
    And the export result is empty

  Scenario: ExportPDF with empty form GUID still surfaces ErrPDFNotActivated
    When I ExportPDF through the service for form "" with no options
    Then the service action returned ErrPDFNotActivated
