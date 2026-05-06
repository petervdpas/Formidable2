Feature: Internal HTTP server lifecycle
  The wiki module owns a runtime-controllable HTTP listener that the
  About workspace toggles on and off. Starting binds a TCP port;
  stopping releases it gracefully. Status reports the live state and
  is safe to call at any time.

  Background:
    Given a wiki manager

  Scenario: A fresh manager is not running
    Then the server is not running
    And the reported port is zero

  Scenario: Start brings the server up
    When I start the server on a random port
    Then the server is running
    And the reported port is non-zero
    And the started-at timestamp is set

  Scenario: Started server accepts requests
    When I start the server on a random port
    Then HTTP GET on "/" returns a response

  Scenario: Stop brings the server down
    Given the server has started on a random port
    When I stop the server
    Then the server is not running
    And HTTP GET on "/" fails

  Scenario: Stop when idle is a no-op
    When I stop the server
    Then no error is returned

  Scenario: Double-start errors
    Given the server has started on a random port
    When I start the server on a random port
    Then a start error is returned

  Scenario: Restart on the same port works
    Given the server has started on a random port
    And I remember the bound port
    When I stop the server
    And I start the server on the remembered port
    Then the server is running
    And the bound port matches the remembered port

  Scenario: Port-in-use returns a start error
    Given the server has started on a random port
    And I remember the bound port
    When a second manager tries to start on the remembered port
    Then a start error is returned

  Scenario: Custom handler responds
    Given a custom handler returning "hello"
    When I start the server on a random port
    Then HTTP GET on "/" returns body "hello"

  Scenario: Status while idle is empty
    Then the status is not running with port zero
