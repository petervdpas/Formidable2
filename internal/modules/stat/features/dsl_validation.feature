Feature: Statistical DSL validation
  Malformed DSL is rejected at parse time; structurally valid DSL that would
  over-count or reference something unresolvable is rejected at evaluate time.
  Either way the engine never charts a wrong or silent result.

  Scenario: Syntactically malformed DSL fails to parse
    Then these DSL strings fail to parse:
      |                                            |
      | count(F["score"])                          |
      | sum()                                      |
      | bogus()                                    |
      | count() by                                 |
      | count() where Facet["flag"]                |
      | count() where Facet["flag"] eq             |
      | count() pct sideways                       |

  Scenario: Well-formed DSL that would over-count or dangle fails to evaluate
    Given the SAMPLE records:
      | filename     | flag       | apps      | score |
      | r1.meta.json | IN OMLOOP | QMU       | 10    |
      | r2.meta.json | IN OMLOOP | Bladework | 20    |
    Then these statistics fail to evaluate:
      | sum(F["score"]) by F["components"]["item"]                                  |
      | count() where Facet["flag"] gt 5                                                          |
      | count() by F["components"]["item"] where F["components"]["item"] eq "QMU" |
      | records() by F["components"]["item"] scale "no-such-scaling"                |
