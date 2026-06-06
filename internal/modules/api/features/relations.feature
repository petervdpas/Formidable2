Feature: REST API - follow relations

  Background:
    Given an API handler over a stub dataprovider with these templates:
      | filename     | name    | enableCollection | guidField |
      | project.yaml | Project | true             | guid      |
      | person.yaml  | Person  | true             | guid      |
    And the dataprovider has forms for "project.yaml":
      | filename     | id    | title |
      | p1.meta.json | g-p1  | Proj1 |
    And the dataprovider has forms for "person.yaml":
      | filename        | id   | title |
      | alice.meta.json | g-a  | Alice |
      | bob.meta.json   | g-b  | Bob   |
    And the storage holds form "project.yaml":"p1.meta.json" with:
      """
      {"meta":{"id":"g-p1","template":"project"},"data":{"guid":"g-p1","naam":"Proj1"}}
      """
    And the storage holds form "person.yaml":"alice.meta.json" with:
      """
      {"meta":{"id":"g-a","template":"person"},"data":{"guid":"g-a","naam":"Alice"}}
      """
    And the storage holds form "person.yaml":"bob.meta.json" with:
      """
      {"meta":{"id":"g-b","template":"person"},"data":{"guid":"g-b","naam":"Bob"}}
      """
    And the relation store for "project.yaml" relates to "person.yaml" as "one-to-many" with edges:
      | from | to  |
      | g-p1 | g-a |
      | g-p1 | g-b |

  Scenario: list a record's relations
    When I GET "/api/collections/project/g-p1/relations"
    Then the response status is 200
    And the JSON nested "relations[0].to" == "person"
    And the JSON nested "relations[0].cardinality" == "one-to-many"
    And the JSON nested "relations[0].count" == 2
    And the JSON nested "relations[0].ids[0]" == "g-a"
    And the JSON nested "relations[0].href" == "/api/collections/project/g-p1/relations/person"

  Scenario: follow a relation returns the linked records with data
    When I GET "/api/collections/project/g-p1/relations/person"
    Then the response status is 200
    And the JSON nested "to" == "person"
    And the JSON nested "total" == 2
    And the JSON nested "items[0].id" == "g-a"
    And the JSON nested "items[0].data.naam" == "Alice"
    And the JSON nested "items[1].id" == "g-b"

  Scenario: expand=relations embeds the relations on the item
    When I GET "/api/collections/project/g-p1?expand=relations"
    Then the response status is 200
    And the JSON nested "relations[0].to" == "person"
    And the JSON nested "relations[0].count" == 2

  Scenario: the item omits relations without expand
    When I GET "/api/collections/project/g-p1"
    Then the response status is 200
    And the JSON does not have field "relations"

  Scenario: following an undeclared relation is 404
    When I GET "/api/collections/project/g-p1/relations/team"
    Then the response status is 404
    And the JSON has "error" == "no-relation"
