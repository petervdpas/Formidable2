Feature: CSV preview and write
  The csv module reads and writes CSV files inside the app context.
  Mirrors `controls/csvManager.js` semantics: header row first, RFC-4180
  quoting, configurable delimiter, LF line endings (not CRLF).

  Background:
    Given a system manager rooted at a temp directory
    And a csv manager wrapping that system

  Scenario: Preview a small CSV with default delimiter
    Given the file "addresses.csv" with content "name,city\nAlice,Amsterdam\nBob,Berlin\n"
    When I preview "addresses.csv" with delimiter ","
    Then the preview headers are "name,city"
    And the preview row count is 2
    And the preview row 0 contains "Alice,Amsterdam"
    And the preview row 1 contains "Bob,Berlin"

  Scenario: Preview supports a custom delimiter
    Given the file "addresses.csv" with content "name;city\nAlice;Amsterdam\nBob;Berlin\n"
    When I preview "addresses.csv" with delimiter ";"
    Then the preview headers are "name,city"
    And the preview row count is 2

  Scenario: Preview handles quoted fields with embedded comma
    Given the file "data.csv" with the following content:
      """
      name,address
      Alice,"Main St, 1"
      Bob,Side Rd 2
      """
    When I preview "data.csv" with delimiter ","
    Then the preview row 0 contains "Alice,Main St, 1"
    And the preview row 1 contains "Bob,Side Rd 2"

  Scenario: Preview handles escaped quotes
    Given the file "data.csv" with the following content:
      """
      name,quote
      Alice,"She said ""hi"""
      """
    When I preview "data.csv" with delimiter ","
    Then the preview row 0 contains 'Alice,She said "hi"'

  Scenario: Preview returns an empty result for an empty file
    Given the file "empty.csv" with content ""
    When I preview "empty.csv" with delimiter ","
    Then the preview headers count is 0
    And the preview row count is 0

  Scenario: Preview returns an error for a missing file
    When I preview "ghost.csv" with delimiter ","
    Then the preview returned an error

  Scenario: Preview returns an error for malformed CSV
    Given the file "bad.csv" with the following content:
      """
      name,city
      "unterminated,Amsterdam
      """
    When I preview "bad.csv" with delimiter ","
    Then the preview returned an error

  Scenario: Write a CSV and read it back
    When I write "out.csv" with rows "name,city|Alice,Amsterdam|Bob,Berlin" and delimiter ","
    Then the write result is success
    And the file "out.csv" exists
    When I preview "out.csv" with delimiter ","
    Then the preview headers are "name,city"
    And the preview row count is 2

  Scenario: Write quotes cells containing the delimiter
    When I write "tricky.csv" with the following rows and delimiter ",":
      """
      name,city
      Alice|Amsterdam, NL
      """
    Then the file "tricky.csv" exists
    When I preview "tricky.csv" with delimiter ","
    Then the preview row 0 contains "Alice,Amsterdam, NL"

  Scenario: Write uses LF line endings (not CRLF)
    When I write "lf.csv" with rows "a,b|1,2" and delimiter ","
    Then the file "lf.csv" has no carriage returns

  Scenario: Write to a nested directory creates parents
    When I write "deep/dir/out.csv" with rows "a,b|1,2" and delimiter ","
    Then the file "deep/dir/out.csv" exists

  Scenario: Write with semicolon delimiter
    When I write "eu.csv" with rows "a;b|1;2" and delimiter ";"
    Then the file "eu.csv" exists
    When I preview "eu.csv" with delimiter ";"
    Then the preview headers are "a,b"
    And the preview row 0 contains "1,2"

  Scenario: Write with empty rows produces an empty file
    When I write "blank.csv" with rows "" and delimiter ","
    Then the file "blank.csv" exists
    And the file "blank.csv" is empty
