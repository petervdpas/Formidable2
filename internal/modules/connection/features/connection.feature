Feature: Connection authoring against an interpreted OpenAPI spec
  A connection is an uploaded Swagger/OpenAPI document plus bindings that say
  which operation lists candidates and which one resolves a stored id. Nothing
  is generated: the document is parsed into an operation catalog and every
  binding is checked against it before the definition is allowed onto disk.

  Background:
    Given an empty connection store
    And the spec "crm" is imported:
      """
      openapi: 3.0.3
      info:
        title: CRM
        version: "2.1.0"
      servers:
        - url: https://crm.example.com/v2
      paths:
        /customers:
          get:
            operationId: listCustomers
            parameters:
              - {name: q, in: query, schema: {type: string}}
              - {name: limit, in: query, schema: {type: integer}}
              - {name: offset, in: query, schema: {type: integer}}
          post:
            operationId: createCustomer
        /customers/{customerId}:
          get:
            operationId: getCustomer
            parameters:
              - {name: customerId, in: path, required: true, schema: {type: string}}
      """

  # Reading the document -------------------------------------------------

  Scenario: A YAML upload is catalogued and stored with a yaml extension
    Then the import succeeded
    And the spec is stored as "crm.yaml"
    And the catalog has 3 operations
    And the catalog lists operation "listCustomers"
    And the catalog lists operation "getCustomer"

  Scenario: A document that is not a spec is refused
    When I import the spec "junk":
      """
      just some notes I had lying around
      """
    Then the import failed

  # Binding operations ---------------------------------------------------

  Scenario: A resource binding both verbs is accepted
    When I save the connection:
      """
      id: crm-prod
      name: CRM Production
      spec_file: crm.yaml
      auth: {kind: bearer}
      resources:
        - key: customers
          label: Customers
          list: {operation: listCustomers}
          get: {operation: getCustomer}
          id_path: /id
          label_path: /name
          search_param: q
          pagination: {style: offset, limit_param: limit, offset_param: offset}
      """
    Then the save succeeded
    And connection "crm-prod" is listed as ok

  Scenario: A list-only resource is accepted, since a saved label carries the display
    When I save the connection:
      """
      id: crm-prod
      name: CRM Production
      spec_file: crm.yaml
      resources:
        - key: customers
          list: {operation: listCustomers}
          id_path: /id
          label_path: /name
      """
    Then the save succeeded

  Scenario: A binding to an operation the spec does not have is refused
    When I save the connection:
      """
      id: crm-prod
      name: CRM Production
      spec_file: crm.yaml
      resources:
        - key: customers
          list: {operation: listPeople}
          id_path: /id
      """
    Then the save failed with "unknown-operation"
    And no connection "crm-prod" is stored

  Scenario: A get binding with no free path parameter cannot carry the id
    When I save the connection:
      """
      id: crm-prod
      name: CRM Production
      spec_file: crm.yaml
      resources:
        - key: customers
          list: {operation: listCustomers}
          get: {operation: createCustomer}
          id_path: /id
      """
    Then the save failed with "no-id-param"

  Scenario: A search parameter must be a real query parameter
    When I save the connection:
      """
      id: crm-prod
      name: CRM Production
      spec_file: crm.yaml
      resources:
        - key: customers
          list: {operation: listCustomers}
          id_path: /id
          search_param: search
      """
    Then the save failed with "unknown-param"

  Scenario: Paging must name parameters the operation accepts
    When I save the connection:
      """
      id: crm-prod
      name: CRM Production
      spec_file: crm.yaml
      resources:
        - key: customers
          list: {operation: listCustomers}
          id_path: /id
          pagination: {style: offset, limit_param: perPage, offset_param: start}
      """
    Then the save failed with "unknown-param"

  Scenario: A malformed JSON pointer is refused
    When I save the connection:
      """
      id: crm-prod
      name: CRM Production
      spec_file: crm.yaml
      resources:
        - key: customers
          list: {operation: listCustomers}
          id_path: id
      """
    Then the save failed with "invalid-pointer"

  # Identity and secrets -------------------------------------------------

  Scenario: A connection id that could walk out of the connections folder is refused
    When I save the connection:
      """
      id: ../../escape
      name: Escape
      spec_file: crm.yaml
      resources: []
      """
    Then the save was rejected

  Scenario: API key auth must say where the key goes
    When I save the connection:
      """
      id: crm-prod
      name: CRM Production
      spec_file: crm.yaml
      auth: {kind: apikey}
      resources: []
      """
    Then the save failed with "incomplete-auth"

  Scenario: A stored definition never contains a secret
    When I save the connection:
      """
      id: crm-prod
      name: CRM Production
      spec_file: crm.yaml
      auth: {kind: apikey, in: header, name: X-Api-Key}
      resources: []
      """
    Then the save succeeded
    And the stored definition does not contain a secret

  # Sharing one document across environments -----------------------------

  Scenario: Two connections can point at the same spec
    When I save the connection:
      """
      id: crm-prod
      name: CRM Production
      base_url: https://crm.example.com/v2
      spec_file: crm.yaml
      resources: []
      """
    And I save the connection:
      """
      id: crm-acc
      name: CRM Acceptance
      base_url: https://acc.crm.example.com/v2
      spec_file: crm.yaml
      resources: []
      """
    Then the save succeeded
    When I delete connection "crm-prod"
    Then no connection "crm-prod" is stored
    And the spec "crm.yaml" is still stored
    And connection "crm-acc" is listed as ok

  Scenario: Deleting the last user of a spec removes the spec too
    When I save the connection:
      """
      id: crm-prod
      name: CRM Production
      spec_file: crm.yaml
      resources: []
      """
    And I delete connection "crm-prod"
    Then the spec "crm.yaml" is gone

  # Detecting resources from the document ---------------------------------

  Scenario: A list and its entity operation are proposed as one resource
    Given the spec "shop" is imported:
      """
      openapi: 3.0.3
      info:
        title: Shop
        version: "1.0"
      servers:
        - url: https://shop.example.com/api
      paths:
        /products:
          get:
            operationId: listProducts
            parameters:
              - {name: q, in: query, schema: {type: string}}
              - {name: limit, in: query, schema: {type: integer}}
              - {name: offset, in: query, schema: {type: integer}}
            responses:
              "200":
                description: ok
                content:
                  application/json:
                    schema:
                      type: object
                      properties:
                        items:
                          type: array
                          items:
                            type: object
                            properties:
                              id: {type: string}
                              name: {type: string}
                              price: {type: number}
        /products/{productId}:
          get:
            operationId: getProduct
            parameters:
              - {name: productId, in: path, required: true, schema: {type: string}}
            responses:
              "200":
                description: ok
                content:
                  application/json:
                    schema:
                      type: object
                      properties:
                        id: {type: string}
                        name: {type: string}
      """
    When I detect resources for the spec "shop.yaml"
    Then 1 resource is proposed
    And resource "products" binds "listProducts" to list and "getProduct" to get
    And resource "products" proposes items path "/items", id path "/id" and label path "/name"
    And resource "products" proposes fields "id, name, price"
    And resource "products" flags "label_path" as a guess
    And resource "products" does not flag "id_path" as a guess
    And saving the proposals as connection "shop-prod" on spec "shop.yaml" succeeds

  Scenario: An operation a resource already lists is not proposed again
    Given the spec "shop" is imported:
      """
      openapi: 3.0.3
      info:
        title: Shop
        version: "1.0"
      servers:
        - url: https://shop.example.com/api
      paths:
        /products:
          get:
            operationId: listProducts
        /products/{productId}:
          get:
            operationId: getProduct
            parameters:
              - {name: productId, in: path, required: true, schema: {type: string}}
      """
    And I save the connection:
      """
      id: shop-prod
      name: Shop
      spec_file: shop.yaml
      resources:
        - key: artikelen
          list: {operation: listProducts}
          get: {operation: getProduct}
          id_path: /id
          label_path: /name
      """
    When I detect resources for connection "shop-prod"
    Then 0 resources are proposed

  Scenario: A response keyed by id is proposed as a map-shaped collection
    Given the spec "registry" is imported:
      """
      openapi: 3.0.3
      info:
        title: Registry
        version: "1.0"
      servers:
        - url: https://registry.example.com
      paths:
        /apis.json:
          get:
            operationId: listAPIs
            responses:
              "200":
                description: ok
                content:
                  application/json:
                    schema:
                      type: object
                      additionalProperties:
                        type: object
                        properties:
                          title: {type: string}
                          added: {type: string}
      """
    When I detect resources for the spec "registry.yaml"
    Then 1 resource is proposed
    And resource "apis" reads its items as a "map"
    And resource "apis" proposes items path "", id path "" and label path "/title"
    And resource "apis" does not flag "id_path" as a guess
    And saving the proposals as connection "registry" on spec "registry.yaml" succeeds
