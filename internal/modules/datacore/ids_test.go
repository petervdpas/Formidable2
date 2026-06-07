package datacore

import "testing"

func TestNewID_BareWhenNoTemplate(t *testing.T) {
	if got := NewID("", "x.meta.json"); got != "x.meta.json" {
		t.Fatalf("NewID with empty template = %q, want bare id", got)
	}
}

// The whole point of the composite key: the same filename in two different
// templates must be two distinct tensor identities, not one collided record.
func TestNewID_DisambiguatesSameFilenameAcrossTemplates(t *testing.T) {
	dt := New()
	dt.Ingest(Record{ID: NewID("a.yaml", "x.meta.json"), Fields: map[string]string{"name": "A"}})
	dt.Ingest(Record{ID: NewID("b.yaml", "x.meta.json"), Fields: map[string]string{"name": "B"}})

	if got := dt.View().Count(); got != 2 {
		t.Fatalf("same filename across templates collided: count = %d, want 2", got)
	}
	counts := map[string]int{}
	for _, b := range dt.View().Distribution("name") {
		counts[b.Value] = b.Count
	}
	if counts["A"] != 1 || counts["B"] != 1 {
		t.Fatalf("distribution = %v, want one each of A and B", counts)
	}
}
