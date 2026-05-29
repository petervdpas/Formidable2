package index

import "testing"

// FilterOps is the index's published operator list for AggregateRaw
// filters; filterJoins is the validator. This guards the two against
// drift: every published op must be accepted, and an unpublished one
// rejected.
func TestFilterOps_AllAcceptedNoDrift(t *testing.T) {
	for _, op := range FilterOps {
		if _, _, err := filterJoins([]AggFilter{{Kind: "field", Key: "x", Op: op, Value: "1"}}); err != nil {
			t.Errorf("published op %q rejected by filterJoins: %v", op, err)
		}
	}
	if _, _, err := filterJoins([]AggFilter{{Kind: "field", Key: "x", Op: "nope", Value: "1"}}); err == nil {
		t.Error("filterJoins accepted an op not in FilterOps")
	}
}
