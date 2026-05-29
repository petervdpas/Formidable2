Feature: Query matrix engine
  The execute step runs a Spec over an in-memory string matrix: project,
  filter, distinct, sort, group and aggregate. Type is reapplied per
  operation; a typed column that won't round-trip is surfaced, not dropped.

  Background:
    Given a data matrix:
      | #form | region | kind    | amount |
      | f1    | east    | apple   | 10     |
      | f1    | east    | pear    | 30     |
      | f2    | west    | apple   | 20     |
      | f2    | west    | apple   | 40     |
      | f3    | east    | cherry  | 50     |
    And the numeric columns "amount"

  Scenario: Project and filter
    When I select "region, kind, amount"
    And I filter "region" "eq" "east"
    And I run the query
    Then the query succeeds
    And the result has 3 rows
    And the result columns are "region, kind, amount"

  Scenario: Numeric filter coerces, not lexical
    When I select "amount"
    And I filter "amount" "ge" "30"
    And I run the query
    Then the result has 3 rows

  Scenario: Distinct collapses the projected tuple
    When I select "region, kind"
    And I want distinct rows
    And I run the query
    Then the result has 4 rows

  Scenario: Numeric sort orders by value, not text
    When I select "amount"
    And I order by "amount" ascending numeric
    And I run the query
    Then row 0 is "10"
    And row 4 is "50"

  Scenario: Group with count and distinct-record count
    When I select "region, amount"
    And I group by "region"
    And I measure count as "rows"
    And I measure count_distinct as "forms"
    And I run the query
    Then the result has 2 rows
    And the result columns are "region, rows, forms"
    And row 0 is "east, 3, 2"
    And row 1 is "west, 2, 1"

  Scenario: Group with numeric aggregates
    When I select "region, amount"
    And I group by "region"
    And I measure sum of "amount" as "sum"
    And I measure avg of "amount" as "avg"
    And I measure min of "amount" as "min"
    And I measure max of "amount" as "max"
    And I run the query
    Then row 0 is "east, 90, 30, 10, 50"
    And row 1 is "west, 60, 30, 20, 40"

  Scenario: A typed column that won't round-trip is surfaced as an anomaly
    Given a data matrix:
      | #form | region | amount |
      | f1    | east    | 10     |
      | f2    | west    | NEE    |
    And the numeric columns "amount"
    When I select "region, amount"
    And I group by "region"
    And I measure sum of "amount" as "sum"
    And I run the query
    Then the query succeeds
    And the query reports 1 anomaly

  Scenario: Projecting a source the matrix does not carry fails
    When I select "ghost"
    And I run the query
    Then the query fails
