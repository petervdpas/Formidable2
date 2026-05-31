Feature: Statistical DSL round-trip
  Compile is the inverse of Parse: a canonical DSL string parses to a config
  that compiles back to the same string. String equality is the contract, so
  every clause (measures, dimensions, bins, top-N, filters, scale, percentage
  base) must survive the trip unchanged. See design/statistics-dsl.md.

  Scenario: Every canonical clause round-trips
    Then these DSL strings round-trip:
      | count()                                                                |
      | records()                                                              |
      | count(), records()                                                     |
      | sum(F["score"])                                                        |
      | avg(F["score"])                                                        |
      | min(F["score"])                                                        |
      | max(F["score"])                                                        |
      | median(F["score"])                                                     |
      | stddev(F["score"])                                                     |
      | percentile(F["score"], 90)                                             |
      | percentile(F["score"], 99.9)                                           |
      | count() by Facet["flag"]                                               |
      | count() by F["status"]                                                 |
      | count() by F["components"]["item"]                       |
      | count() by F["created"]@year                                           |
      | count() by F["created"]@month                                          |
      | count() by F["created"]@day                                            |
      | count() by F["status"] top 5                                           |
      | count() by Facet["flag"], F["status"]                                  |
      | count() where Facet["flag"] eq "IN OMLOOP"                            |
      | count() where Facet["flag"] ne "IN OMLOOP"                            |
      | count() where F["score"] gt 5                                          |
      | count() where F["score"] ge 5                                          |
      | count() where F["score"] lt 5                                          |
      | count() where F["score"] le 5                                          |
      | count() where Facet["flag"] eq "IN OMLOOP" and F["score"] gt 5        |
      | count() scale "qzm-urgency"                                           |
      | count() pct forms                                                      |
      | count() pct none                                                       |
      | records() by F["components"]["item"] top 10 where Facet["flag"] eq "IN OMLOOP" scale "qzm-urgency" pct forms |

  Scenario: The default percentage base is omitted from the canonical form
    Then these DSL strings round-trip:
      | count() by Facet["flag"] |
