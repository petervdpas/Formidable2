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
    Given the ODS records:
      | filename     | flag       | apps      | score |
      | r1.meta.json | IN GEBRUIK | FMU       | 10    |
      | r2.meta.json | IN GEBRUIK | Gradework | 20    |
    Then these statistics fail to evaluate:
      | sum(F["score"]) by F["code-repositories"]["application"]                                  |
      | count() where Facet["flag"] gt 5                                                          |
      | count() by F["code-repositories"]["application"] where F["code-repositories"]["application"] eq "FMU" |
      | records() by F["code-repositories"]["application"] scale "no-such-scaling"                |
