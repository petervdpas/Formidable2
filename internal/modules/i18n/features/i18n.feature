Feature: Internationalization bundles
  The i18n module loads locale bundles embedded in the binary and
  serves them through the Wails service. UI labels are sourced
  centrally - the frontend never hardcodes user-facing strings.

  Background:
    Given a fresh i18n manager

  Scenario: Default locale is English
    Then the default locale is "en"
    And the locale "en" is available
    And the locale "nl" is available

  Scenario: Loading a known bundle returns its translations
    When I load the bundle for "en"
    Then the bundle contains key "status.ready"

  Scenario: Loading an unknown bundle returns an error
    When I load the bundle for "klingon"
    Then an i18n error occurred

  Scenario: AvailableLocales is sorted
    When I list available locales
    Then the locale list is sorted alphabetically
    And the locale list contains "en"
    And the locale list contains "nl"
