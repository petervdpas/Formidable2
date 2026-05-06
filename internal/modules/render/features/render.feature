Feature: Markdown render pipeline
  The render module turns a (template, form values) pair into
  Markdown via Handlebars and then into HTML via goldmark + chroma.
  It mirrors the original Formidable's controls/markdownRenderer.js
  and controls/htmlRenderer.js, with an Image URL strategy that lets
  the desktop, the future internal HTTP server, and export tooling
  plug different URL schemes.

  Background:
    Given a fresh render Manager with no image URL strategy

  Scenario: Empty template returns the placeholder sentinel
    When I render markdown for a template with no markdown_template
    Then the markdown is "# No template defined."

  Scenario: Bare value substitution
    Given a template with markdown "# {{title}}" and field "title" of type "text"
    And the form has values:
      | key   | value |
      | title | Hello |
    When I render markdown
    Then the markdown is "# Hello"

  Scenario: Field helper renders dropdown labels by default
    Given a template with markdown "Color: {{field \"color\"}}"
    And the dropdown field "color" has options "r:Red,b:Blue"
    And the form has values:
      | key   | value |
      | color | r     |
    When I render markdown
    Then the markdown is "Color: Red"

  Scenario: Field helper with mode=value emits the raw stored value
    Given a template with markdown "{{field \"color\" mode=\"value\"}}"
    And the dropdown field "color" has options "r:Red,b:Blue"
    And the form has values:
      | key   | value |
      | color | r     |
    When I render markdown
    Then the markdown is "r"

  Scenario: Loop helper iterates with synthetic index
    Given a template with markdown:
      """
      {{#loop "items"}}- {{name}} ({{items_index}}){{/loop}}
      """
    And a loop "items" with field "name" of type "text"
    And the form loop "items" has entries:
      | name |
      | a    |
      | b    |
    When I render markdown
    Then the markdown contains "- a (1)"
    And the markdown contains "- b (2)"

  Scenario: Image URL falls back to images/<name> with no strategy
    Given a template with markdown "![logo]({{field \"logo\"}})" and field "logo" of type "image"
    And the form has values:
      | key  | value    |
      | logo | icon.png |
    When I render markdown
    Then the markdown is "![logo](images/icon.png)"

  Scenario: Image URL strategy plugs a desktop file:// scheme
    Given an image URL strategy that returns "file:///abs/storage/{template}/images/{name}"
    And a template with markdown "![logo]({{field \"logo\"}})" and field "logo" of type "image"
    And the form has values:
      | key  | value    |
      | logo | icon.png |
    When I render the form for template "recepten.yaml" and datafile "df"
    Then the markdown is "![logo](file:///abs/storage/recepten.yaml/images/icon.png)"
    And the html contains "src=\"file:///abs/storage/recepten.yaml/images/icon.png\""

  Scenario: HTML stage strips frontmatter
    When I render html from "---\ntitle: x\n---\n# Body\n"
    Then the html does not contain "title:"
    And the html contains "<h1"

  Scenario: HTML stage decorates hashtags outside code blocks
    When I render html from "Look at #foo and #bar."
    Then the html contains "inline-tag\">#foo"
    And the html contains "inline-tag\">#bar"

  Scenario: HTML stage leaves hashtags inside fenced code untouched
    When I render html from a fenced code block containing "#nope"
    Then the html does not contain "inline-tag"

  Scenario: HTML stage applies chroma syntax highlighting
    When I render html from a fenced go code block "func main() {}"
    Then the html contains "<pre"
    And the html contains "class=\"hljs-"

  Scenario: GFM tables are enabled
    When I render html from a 2-row markdown table
    Then the html contains "<table"

  Scenario: Table helper emits a contiguous markdown table
    Given a template with markdown "{{table \"ingredients\"}}"
    And the table field "ingredients" has columns "name:Ingredient,qty:Quantity"
    And the form table "ingredients" has rows:
      | Olive oil | 4 tbsp       |
      | Lemon     | juice of one |
    When I render markdown
    Then the markdown contains "| Ingredient | Quantity |"
    And the markdown contains "| --- | --- |"
    And the markdown contains "| Olive oil | 4 tbsp |"
    And the markdown contains "| Lemon | juice of one |"
    And the markdown does not contain "\n\n|"

  Scenario: Frontmatter parses and round-trips
    When I parse frontmatter from "---\ntitle: Hello\ncount: 3\n---\n# body\n"
    Then the frontmatter title is "Hello"
    And the frontmatter count is 3
    And the frontmatter body is "# body\n"
