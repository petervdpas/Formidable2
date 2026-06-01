Feature: Formula fields (the formula editor backend)
  Formulas are named per-record expressions evaluated by the engine: computed in
  declared order (a later formula may reference an earlier one), a failing
  formula is skipped rather than aborting the batch, text concatenation coerces
  non-string values, and the editor's palettes are backed by a function catalog.

  Background:
    Given a formula context:
      | key   | value      |
      | n     | 5          |
      | neg   | -3         |
      | flag  | true       |
      | code  | CH.02      |
      | story | 5          |
      | arch  | HOOG       |
      | day   | 2026-01-01 |

  # ── Control structures ────────────────────────────────────────────────
  Scenario: Control structures
    When I evaluate these expressions:
      | expression                          | result |
      | F["n"] > 3 ? "big" : "small"        | big    |
      | F["n"] < 3 ? "big" : "small"        | small  |
      | F["n"] == 5                         | true   |
      | F["n"] != 5                         | false  |
      | F["n"] >= 5                         | true   |
      | F["n"] <= 4                         | false  |
      | F["n"] > 3 && F["flag"]             | true   |
      | F["n"] > 9 \|\| F["flag"]           | true   |
      | !F["flag"]                          | false  |
      | F["arch"] == "HOOG" ? 10 : 1        | 10     |

  # ── Math functions ────────────────────────────────────────────────────
  Scenario: Math functions
    When I evaluate these expressions:
      | expression         | result |
      | max(F["n"], 9)     | 9      |
      | min(F["n"], 2)     | 2      |
      | abs(F["neg"])      | 3      |
      | round(2.6)         | 3      |
      | sum([1, 2, 3])     | 6      |
      | mean([2, 4])       | 3      |
      | F["n"] * 0.5       | 2.5    |

  # ── Text functions ────────────────────────────────────────────────────
  Scenario: Text functions
    When I evaluate these expressions:
      | expression                              | result  |
      | str(F["story"])                         | 5       |
      | str(F["code"]) + "-" + str(F["story"])  | CH.02-5 |
      | defaultText("", "fallback")             | fallback |
      | defaultText(F["code"], "x")             | CH.02   |
      | notEmpty(F["code"])                     | true    |
      | notEmpty("")                            | false   |

  # ── Date functions (deterministic) ────────────────────────────────────
  Scenario: Date functions
    When I evaluate these expressions:
      | expression                              | result |
      | today() == today()                      | true   |
      | ageInDays(today())                      | 0      |
      | daysBetween("2026-01-01", "2026-01-08") | 7      |
      | isOverdue("2000-01-01")                 | true   |
      | isOverdue(today())                      | false  |

  # ── Declared-order chaining ───────────────────────────────────────────
  Scenario: Formulas compute in declared order with chaining
    When I evaluate the formulas:
      | key      | type   | expression        |
      | marge    | number | F["n"] * 10       |
      | weighted | number | F["marge"] + 1    |
    Then formula "marge" is "50"
    And formula "weighted" is "51"

  Scenario: A categorical field drives a text formula
    When I evaluate the formulas:
      | key  | type | expression                           |
      | band | text | F["arch"] == "HOOG" ? "high" : "low" |
    Then formula "band" is "high"

  # ── Robustness ────────────────────────────────────────────────────────
  Scenario: A malformed formula is skipped, not fatal
    When I evaluate the formulas:
      | key  | type   | expression  |
      | good | number | F["n"] * 2  |
      | bad  | number | F["n" *     |
    Then formula "good" is "10"
    And formula "bad" is absent

  Scenario: Text concatenation coerces a number field
    When I evaluate the value:
      """
      str(F["code"]) + "-" + str(F["story"])
      """
    Then the value is "CH.02-5"

  Scenario: A bare concat of text and number is an error
    When I evaluate the value:
      """
      F["code"] + F["story"]
      """
    Then evaluation fails

  # ── Function catalog (backs the editor palettes) ──────────────────────
  Scenario: The catalog offers control, math, text and date entries
    When I list the formula functions
    Then the catalog includes "if / then / else"
    And the catalog includes "max"
    And the catalog includes "str"
    And the catalog includes "today"
    And the catalog has an entry in category "control"
    And the catalog has an entry in category "math"
    And the catalog has an entry in category "text"
    And the catalog has an entry in category "date"
    And every function has a category and a snippet
