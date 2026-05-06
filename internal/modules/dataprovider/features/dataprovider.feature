Feature: Dataprovider read facade
  Dataprovider is the read-only seam between the persisted index +
  the render module and the future wiki HTTP layer. It projects
  index rows into HTTP-friendly summaries and adds collection-mode
  niceties (substring/tag filters, paginated listings, GUID lookup).

  Background:
    Given a fresh dataprovider with these templates:
      | filename       | name      | guid_field | tags_field | item_field | enable_collection |
      | basic.yaml     | Basic     |            |            |            | false             |
      | recepten.yaml  | Recepten  | id         | tags       | title      | true              |
    And these forms under "recepten.yaml":
      | filename         | id    | title                  | tags             |
      | tap.meta.json    | g-tap | Groene Tapenade        | groen,tapenade   |
      | vin.meta.json    | g-vin | Basis Vinaigrette      | saus             |
      | spa.meta.json    | g-spa | Spaanse Groenteschotel | groen,spaans     |

  # ── Templates / forms basics ──────────────────────────────────────

  Scenario: ListTemplates exposes both templates with their collection flag
    When I list templates
    Then the template list has 2 templates
    And template "recepten.yaml" has stem "recepten" and collection enabled

  Scenario: GetTemplate returns the matching template
    When I get template "recepten.yaml"
    Then the template stem is "recepten"
    And the template name is "Recepten"

  Scenario: GetTemplate misses for unknown filename
    When I get template "ghost.yaml"
    Then the template lookup misses

  Scenario: ListForms maps rows into summaries with tags
    When I list forms under "recepten.yaml"
    Then the form list has 3 forms
    And form "tap.meta.json" has tags "groen,tapenade"

  Scenario: ResolveByID finds a form by its GUID
    When I resolve id "g-spa" under "recepten.yaml"
    Then the resolution returns "spa.meta.json"

  Scenario: ResolveByID misses on unknown GUID
    When I resolve id "no-such" under "recepten.yaml"
    Then the resolution misses

  # ── Render ─────────────────────────────────────────────────────────

  Scenario: RenderForm uses the frontmatter title when present
    Given the renderer returns markdown:
      """
      ---
      title: Spaanse Groenteschotel
      ---
      # body
      """
    When I render "spa.meta.json" under "recepten.yaml"
    Then the rendered title is "Spaanse Groenteschotel"

  Scenario: RenderForm falls back to the form summary title when no frontmatter
    Given the renderer returns markdown:
      """
      # plain body, no frontmatter
      """
    When I render "spa.meta.json" under "recepten.yaml"
    Then the rendered title is "Spaanse Groenteschotel"

  # ── Collections ───────────────────────────────────────────────────

  Scenario: ListCollection returns disabled marker for non-collection template
    When I list the collection for "basic.yaml"
    Then the collection is disabled

  Scenario: ListCollection lists all addressable items with hrefs
    When I list the collection for "recepten.yaml"
    Then the collection is enabled
    And the collection total is 3
    And item "g-tap" has self-href "/api/collections/recepten/g-tap"
    And item "g-tap" has html-href "/template/recepten/form/tap.meta.json"

  Scenario: ListCollection q substring matches title and tags case-insensitively
    When I list the collection for "recepten.yaml" with q "GROEN"
    Then the collection contains items "spa.meta.json,tap.meta.json"

  Scenario: ListCollection tags AND-filter narrows the result
    When I list the collection for "recepten.yaml" with tags "groen,spaans"
    Then the collection contains items "spa.meta.json"

  Scenario: ListCollection paginates with limit + offset
    When I list the collection for "recepten.yaml" with limit 1 and offset 1
    Then the collection total is 3
    And the collection page has 1 items

  Scenario: ResolveCollectionByID returns an item with hrefs
    When I resolve collection id "g-vin" under "recepten.yaml"
    Then the collection item filename is "vin.meta.json"
    And the collection item self-href is "/api/collections/recepten/g-vin"
