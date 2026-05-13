Feature: Auth middleware closes the API trust boundary
  The wiki/api HTTP surface inherits the active desktop profile's
  identity. Without these middlewares it is vulnerable to CSRF from
  any browser tab, to remote callers via port-forwards/tunnels, and
  to unverified attribution. Each scenario locks in one layer of the
  defense and the consequence on the audit-block stamping.

  Background:
    Given an auth handler stack mounted on a downstream echo handler
    And the allowed origin is "http://127.0.0.1:8080"

  Scenario: Loopback request with desktop resolver succeeds
    Given a desktop resolver returning profile "peter" / "Peter" / "peter@example.com"
    When a request arrives from "127.0.0.1:54321" with method "GET" and origin ""
    Then the response status is 200
    And the downstream handler observed identity "peter"

  Scenario: Non-loopback request is rejected before reaching the handler
    Given a desktop resolver returning profile "peter" / "Peter" / "peter@example.com"
    When a request arrives from "192.168.1.5:54321" with method "GET" and origin ""
    Then the response status is 403
    And the downstream handler was not invoked
    And the response body contains "non-loopback"

  Scenario: Tunnel-style external IP is rejected
    Given a desktop resolver returning profile "peter" / "Peter" / "peter@example.com"
    When a request arrives from "203.0.113.42:443" with method "POST" and origin "http://127.0.0.1:8080"
    Then the response status is 403
    And the downstream handler was not invoked

  Scenario: Cross-origin write is rejected (CSRF defense)
    Given a desktop resolver returning profile "peter" / "Peter" / "peter@example.com"
    When a request arrives from "127.0.0.1:54321" with method "POST" and origin "https://evil.example.com"
    Then the response status is 403
    And the downstream handler was not invoked
    And the response body contains "cross-origin"

  Scenario: Header-less write is rejected (no Origin, no Referer)
    Given a desktop resolver returning profile "peter" / "Peter" / "peter@example.com"
    When a request arrives from "127.0.0.1:54321" with method "POST" and origin ""
    Then the response status is 403
    And the response body contains "missing-origin"

  Scenario: Allowlisted-origin write is attributed to the desktop profile
    Given a desktop resolver returning profile "peter" / "Peter" / "peter@example.com"
    When a request arrives from "127.0.0.1:54321" with method "POST" and origin "http://127.0.0.1:8080"
    Then the response status is 200
    And the downstream handler observed identity "peter"

  Scenario: Subscription resolver surfaces 501 not-implemented
    Given the subscription resolver is mounted
    When a request arrives from "127.0.0.1:54321" with method "GET" and origin ""
    Then the response status is 501
    And the downstream handler was not invoked

  Scenario: Resolver returning an invalid identity is rejected
    Given a resolver returning an invalid identity
    When a request arrives from "127.0.0.1:54321" with method "GET" and origin ""
    Then the response status is 403
    And the response body contains "invalid-identity"
