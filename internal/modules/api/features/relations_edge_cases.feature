Feature: REST API - relation follow edge cases

  Background:
    Given an API handler over a stub dataprovider with these templates:
      | filename     | name    | enableCollection | guidField |
      | project.yaml | Project | true             | guid      |
      | person.yaml  | Person  | true             | guid      |
      | notes.yaml   | Notes   | false            |           |
    And the dataprovider has forms for "project.yaml":
      | filename     | id   | title |
      | p1.meta.json | g-p1 | Proj1 |
      | p2.meta.json | g-p2 | Proj2 |
    And the dataprovider has forms for "person.yaml":
      | filename        | id  | title |
      | alice.meta.json | g-a | Alice |
      | bob.meta.json   | g-b | Bob   |
    And the storage holds form "person.yaml":"alice.meta.json" with:
      """
      {"meta":{"id":"g-a","template":"person"},"data":{"naam":"Alice"}}
      """
    And the storage holds form "person.yaml":"bob.meta.json" with:
      """
      {"meta":{"id":"g-b","template":"person"},"data":{"naam":"Bob"}}
      """
    And the storage holds form "project.yaml":"p2.meta.json" with:
      """
      {"meta":{"id":"g-p2","template":"project"},"data":{"naam":"Proj2"}}
      """
    And the relation store for "project.yaml" relates to "person.yaml" as "one-to-many" with edges:
      | from | to    |
      | g-p1 | g-a   |
      | g-p1 | g-b   |
      | g-p1 | ghost |
    And the relation store for "project.yaml" relates to "notes.yaml" as "one-to-many"
    And the relation store for "project.yaml" relates to "project.yaml" as "many-to-one" with edges:
      | from | to   |
      | g-p1 | g-p2 |

  Scenario: follow tolerates a dangling edge (target deleted)
    When I GET "/api/collections/project/g-p1/relations/person"
    Then the response status is 200
    And the JSON nested "total" == 3
    And the JSON "items" has length 2
    And the JSON nested "items[0].id" == "g-a"
    And the JSON nested "items[1].id" == "g-b"

  Scenario: follow paginates with limit/offset
    When I GET "/api/collections/project/g-p1/relations/person?limit=1&offset=1"
    Then the response status is 200
    And the JSON nested "total" == 3
    And the JSON "items" has length 1
    And the JSON nested "items[0].id" == "g-b"

  Scenario: a record with no outgoing edges follows to an empty set
    When I GET "/api/collections/person/g-a/relations/project"
    Then the response status is 404
    And the JSON has "error" == "no-relation"

  Scenario: follow to a declared but collection-disabled target is 403
    When I GET "/api/collections/project/g-p1/relations/notes"
    Then the response status is 403
    And the JSON has "error" == "collection-disabled"

  Scenario: POST to a relation route is rejected with 405 + Allow
    When I POST "/api/collections/project/g-p1/relations" with body:
      """
      {}
      """
    Then the response status is 405
    And the response header "Allow" contains "GET"

  Scenario: follow a self relation over the API
    When I GET "/api/collections/project/g-p1/relations/project"
    Then the response status is 200
    And the JSON nested "total" == 1
    And the JSON nested "items[0].id" == "g-p2"
