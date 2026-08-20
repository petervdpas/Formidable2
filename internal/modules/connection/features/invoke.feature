Feature: Invoking a bound resource
  The invoker assembles each call from the catalog and the resource binding at
  runtime. A consuming field stores the id, shows the label, and picks which of
  the declared remote fields it wants extracted alongside them.

  Background:
    Given a remote that answers with:
      """
      {
        "data": [
          {"id": 7, "name": "Ada", "email": "ada@example.com",
           "address": {"city": "Delft"}, "vip": true, "salary": 90000},
          {"id": 8, "name": "Grace", "email": "grace@example.com",
           "address": {"city": "Utrecht"}, "vip": false, "salary": 95000}
        ]
      }
      """
    And a connection bound to it declaring the fields "email, city, vip"

  # What a reference always carries --------------------------------------

  Scenario: Every item yields the id to store and the label to show
    When I list "customers"
    Then I get 2 items
    And item 1 has id "7" and label "Ada"
    And item 2 has id "8" and label "Grace"

  Scenario: A record with no id cannot be referenced, so it is dropped
    Given a remote that answers with:
      """
      {"data": [{"name": "Anonymous"}, {"id": 8, "name": "Grace"}]}
      """
    When I list "customers"
    Then I get 1 item
    And item 1 has id "8" and label "Grace"

  # What the consuming field chooses -------------------------------------

  Scenario: A field extracts only the remote fields it asked for
    When I list "customers" selecting "email"
    Then item 1 has 1 field
    And item 1 field "email" is "ada@example.com"

  Scenario: Several fields, including one nested inside the record
    When I list "customers" selecting "email, city"
    Then item 1 has 2 fields
    And item 1 field "email" is "ada@example.com"
    And item 1 field "city" is "Delft"

  Scenario: Asking for nothing gives every field the connection declares
    When I list "customers"
    Then item 1 has 3 fields
    And item 1 field "vip" is "true"

  Scenario: A field the connection does not declare is refused, not silently dropped
    When I list "customers" selecting "salary"
    Then the call failed with "unknown_field"

  Scenario: A declared field missing from a record is absent rather than blank
    Given a remote that answers with:
      """
      {"data": [{"id": 1, "name": "Sparse", "address": {"city": "Delft"}}]}
      """
    When I list "customers"
    Then item 1 has no field "email"
    And item 1 field "city" is "Delft"

  # Resolving a stored id back to a record --------------------------------

  Scenario: A stored id is fetched through the get binding
    Given a remote that answers with:
      """
      {"id": 7, "name": "Ada", "email": "ada@example.com", "address": {"city": "Delft"}}
      """
    When I fetch "7" from "customers" selecting "city"
    Then the fetched item has id "7" and label "Ada"
    And the fetched item field "city" is "Delft"
    And the request path was "/customers/7"

  # What goes on the wire -------------------------------------------------

  Scenario: Search, paging and design-time fixed parameters are all sent
    When I list "customers" searching "ada" with limit 25 and offset 50
    Then the request query "q" was "ada"
    And the request query "limit" was "25"
    And the request query "offset" was "50"
    And the request header "tenant" was "acme"

  Scenario: The remote refusing the credential is reported as such
    Given the remote answers with status 401
    When I list "customers"
    Then the call failed with "unauthorized"

  Scenario: An HTML login page answering 200 is not read as an empty result
    Given the remote answers with an HTML page
    When I list "customers"
    Then the call failed with "bad_response"

  # The test console -----------------------------------------------------

  Scenario: An operation is run straight from the document
    When I try the operation "listCustomers" with:
      | tenant | acme |
    Then the try returned status 200
    And the try body contains "Ada"

  Scenario: A failing status is shown rather than swallowed
    Given the remote answers with status 404
    When I try the operation "listCustomers" with:
      | tenant | acme |
    Then the try returned status 404
    And the try is marked failed

  Scenario: An HTML login page is shown verbatim instead of refused
    Given the remote answers with an HTML page
    When I try the operation "listCustomers" with:
      | tenant | acme |
    Then the try body contains "please log in"
    And the try body is not JSON

  Scenario: A parameter the document does not declare is refused
    When I try the operation "listCustomers" with:
      | tenant   | acme |
      | nonsense | 1    |
    Then the try was refused with "unknown_field"

  Scenario: A required parameter left empty never reaches the remote
    When I try the operation "getCustomer" with:
      | tenant | acme |
    Then the try was refused with "missing_param"
