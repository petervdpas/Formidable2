Feature: Calling an operation of the document from a program
  A plugin script names an operation by its id and may send any method the
  document declares, with a body. The console in the API clients panel stays
  read-only; this is the path a program takes.

  Background:
    Given a remote that records each request
    And a client for the document with write operations

  Scenario: A write goes out once, with its body as JSON
    When the program calls "addCustomer" with the JSON body:
      """
      {"name": "Ada"}
      """
    Then the call returned status 201
    And the remote received 1 request
    And the remote received "POST" on "/customers" as "application/json"
    And the remote received the body:
      """
      {"name":"Ada"}
      """

  Scenario: A file is sent as it is, byte for byte
    When the program calls "putDocument" for "inv-7" with a PDF file as body
    Then the remote received "PUT" on "/documents/inv-7" as "application/pdf"
    And the remote received exactly the bytes of the PDF file

  Scenario: A delete carries no body
    When the program calls "removeCustomer" for "7" without a body
    Then the call returned status 204
    And the remote received "DELETE" on "/customers/7" as ""

  Scenario: A required body left out never reaches the remote
    When the program calls "addCustomer" without a body
    Then the call was refused with "missing_param"
    And the remote received 0 requests

  Scenario: A multipart upload is refused
    When the program calls "upload" with a PDF file as body
    Then the call was refused with "binding_invalid"
    And the remote received 0 requests

  Scenario: A write the remote refuses carries its code
    Given the recording remote answers with status 403
    When the program calls "wipeCustomers" without a body
    Then the call was refused with "forbidden"
    And the remote received 1 request

  Scenario: The console still refuses a write
    When I try the write operation "wipeCustomers" in the console
    Then the call was refused with "method_not_allowed"
    And the remote received 0 requests
