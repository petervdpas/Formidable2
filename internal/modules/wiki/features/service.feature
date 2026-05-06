Feature: Wiki Service
  The Service is the Wails-bound surface the About workspace toggle
  drives. It wraps the lifecycle Manager + the read-path Handler and
  delegates external "open URL" actions (system browser, in-app
  webview window) to hooks the composition root installs.

  Background:
    Given a wiki service over a stub dataprovider and a configured port

  Scenario: Service starts the listener on the configured port
    Given the configured port is 0
    When I StartServer through the service
    Then the service reports running
    And the service-reported port is non-zero

  Scenario: Service stops the listener cleanly
    Given the configured port is 0
    And the service has started the server
    When I StopServer through the service
    Then the service reports not running

  Scenario: Status while idle reports zero port and not running
    Given the configured port is 0
    Then the service reports not running
    And the service-reported port is zero

  Scenario: Auto-restart on a different port picks up the new config
    Given the configured port is 0
    And the service has started the server
    And I remember the service port
    When I StopServer through the service
    And the configured port changes to 0
    And I StartServer through the service
    Then the service reports running
    And the new service-reported port differs from the remembered one

  Scenario: OpenInBrowser fails when the server is not running
    When I OpenInBrowser through the service
    Then the service action returned an error containing "not running"

  Scenario: OpenInBrowser delegates to the registered opener
    Given the configured port is 0
    And the service has started the server
    When I OpenInBrowser through the service
    Then the registered browser opener was invoked with the loopback URL

  Scenario: OpenInternalWiki errors when no window opener is installed
    Given the configured port is 0
    And the service has started the server
    When I OpenInternalWiki through the service
    Then the service action returned an error containing "not wired"

  Scenario: OpenInternalWiki delegates to the installed window opener
    Given the configured port is 0
    And the service has started the server
    And a window opener is installed
    When I OpenInternalWiki through the service
    Then the registered window opener was invoked with the loopback URL

  Scenario: Read-path is reachable end-to-end via the service
    Given the configured port is 0
    And the service has started the server
    When I HTTP GET the service root URL
    Then the live response status is 200
