Feature: Offline export template dependencies
  An offline bundle must open self-contained: every template a selection links
  to (through declared relations or api fields) is pulled in automatically, so a
  reader never clicks a link to a page that is not in the zip. The backend
  resolves the closure and the exporter applies it, independent of the frontend.

  Background:
    Given a wiki handler with a template dependency graph

  Scenario: A direct link pulls in the target template
    Given template "aanpak.yaml" links to "controls.yaml"
    When I resolve export dependencies for "aanpak.yaml"
    Then the required templates are "aanpak.yaml, controls.yaml"
    And the added templates are "controls.yaml"
    And "controls.yaml" is included because of "aanpak.yaml"

  Scenario: Dependencies are followed transitively
    Given template "aanpak.yaml" links to "controls.yaml"
    And template "controls.yaml" links to "richtlijnen.yaml"
    When I resolve export dependencies for "aanpak.yaml"
    Then the required templates are "aanpak.yaml, controls.yaml, richtlijnen.yaml"
    And the added templates are "controls.yaml, richtlijnen.yaml"

  Scenario: A cycle terminates instead of looping
    Given template "a.yaml" links to "b.yaml"
    And template "b.yaml" links to "a.yaml"
    When I resolve export dependencies for "a.yaml"
    Then the required templates are "a.yaml, b.yaml"
    And the added templates are "b.yaml"

  Scenario: An explicit pick is never reported as auto-added
    Given template "aanpak.yaml" links to "controls.yaml"
    When I resolve export dependencies for "aanpak.yaml, controls.yaml"
    Then the required templates are "aanpak.yaml, controls.yaml"
    And no templates are added

  Scenario: A dangling target is reported, not fabricated
    Given template "aanpak.yaml" links to unknown "ghost.yaml"
    When I resolve export dependencies for "aanpak.yaml"
    Then the required templates are "aanpak.yaml"
    And no templates are added
    And the missing templates are "ghost.yaml"

  Scenario: A shared dependency records every pick that needs it
    Given template "a.yaml" links to "shared.yaml"
    And template "b.yaml" links to "shared.yaml"
    When I resolve export dependencies for "a.yaml, b.yaml"
    Then the added templates are "shared.yaml"
    And "shared.yaml" is included because of "a.yaml, b.yaml"

  Scenario: The produced zip contains the auto-included template
    Given a document template "aanpak.yaml" with one record
    And a document template "controls.yaml" with one record
    And template "aanpak.yaml" links to "controls.yaml"
    When I export the bundle selecting "aanpak.yaml"
    Then the bundle contains "template-controls.html"
    And the bundle contains "form-controls-r-meta-json.html"
